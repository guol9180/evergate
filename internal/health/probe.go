package health

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"gorm.io/gorm"

	"evergate/internal/pool"
	"evergate/internal/retry"
	"evergate/internal/store"
)

const (
	ProbeResultSuccess      = "success"
	ProbeResultFailure      = "failure"
	ProbeResultInconclusive = "inconclusive"
)

var (
	ErrChannelDisabled = errors.New("channel disabled")
	ErrNoAvailableKey  = errors.New("no available key")
)

// HTTPClient 保持与 net/http.Client 兼容，测试可注入可控服务。
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// ProberOptions 构造单个渠道轻量探测器。
type ProberOptions struct {
	DB        *gorm.DB
	MasterKey string
	Pool      *pool.Manager
	Client    HTTPClient
	Now       func() time.Time
}

// Prober 执行 GET {base_url}/models，并同步驱动 key 状态机与渠道健康字段。
type Prober struct {
	opt ProberOptions
}

func NewProber(opt ProberOptions) *Prober {
	if opt.Client == nil {
		opt.Client = http.DefaultClient
	}
	if opt.Now == nil {
		opt.Now = func() time.Time { return time.Now().UTC() }
	}
	return &Prober{opt: opt}
}

// ProbeResult 是一次探测的可见摘要；只包含 key ID，不包含任何明文/密文。
type ProbeResult struct {
	ChannelID   int64     `json:"channel_id"`
	KeyID       int64     `json:"key_id"`
	Result      string    `json:"result"`
	Status      string    `json:"status"`
	StatusCode  int       `json:"status_code"`
	LatencyMS   int64     `json:"latency_ms"`
	ErrorType   string    `json:"error_type,omitempty"`
	Error       string    `json:"error,omitempty"`
	ProbedAt    time.Time `json:"probed_at"`
	NextProbeAt time.Time `json:"next_probe_at"`
}

// ProbeChannel 使用 pool 选出的 key 发起轻量探测，成功/失败驱动状态机；
// 404/405 表示上游不支持该探测路径，只释放探测名额并记录 inconclusive。
func (p *Prober) ProbeChannel(ctx context.Context, ch *store.Channel, keys []*store.ChannelKey, timeout, nextInterval time.Duration) (ProbeResult, error) {
	if ch == nil {
		return ProbeResult{}, errors.New("channel is nil")
	}
	if !ch.Enabled {
		return ProbeResult{}, ErrChannelDisabled
	}
	if p.opt.Pool == nil {
		return ProbeResult{}, errors.New("pool is nil")
	}
	key := p.opt.Pool.Acquire(ch.ID, keys)
	if key == nil {
		return ProbeResult{}, ErrNoAvailableKey
	}
	result := ProbeResult{ChannelID: ch.ID, KeyID: key.ID}

	var response probeResponse
	plain, decryptErr := store.Decrypt(key.KeyCipher, p.opt.MasterKey)
	if decryptErr == nil {
		response = p.doModelsProbe(ctx, ch, plain, timeout)
	} else {
		response.Err = decryptErr
		response.Message = decryptErr.Error()
	}
	result.StatusCode = response.StatusCode
	result.LatencyMS = response.LatencyMS
	result.Error = response.Message
	if ctx.Err() != nil {
		p.opt.Pool.CancelProbe(key.ID)
		return ProbeResult{}, ctx.Err()
	}

	now := p.opt.Now().UTC()
	result.ProbedAt = now
	result.NextProbeAt = now.Add(nextInterval)
	switch {
	case response.Err == nil && result.StatusCode >= 200 && result.StatusCode < 300:
		result.Result = ProbeResultSuccess
		p.opt.Pool.ReportSuccess(key.ID)
	case result.StatusCode == http.StatusNotFound || result.StatusCode == http.StatusMethodNotAllowed:
		result.Result = ProbeResultInconclusive
		result.ErrorType = "inconclusive"
		if result.Error == "" {
			result.Error = fmt.Sprintf("上游不支持 /models 探测（HTTP %d）", result.StatusCode)
		}
		p.opt.Pool.CancelProbe(key.ID)
	default:
		result.Result = ProbeResultFailure
		classification := retry.Classify(retry.Result{
			Err: response.Err, StatusCode: response.StatusCode,
			RetryAfter: response.RetryAfter, BodySnippet: response.Body,
		})
		result.ErrorType = string(classification.Kind)
		if result.ErrorType == "" {
			result.ErrorType = "transient"
		}
		p.opt.Pool.ReportFailure(key.ID, classification)
	}

	status := DeriveStatus(&store.Channel{Enabled: ch.Enabled, LastProbeResult: result.Result}, keys, p.opt.Pool.StateMachine().State)
	result.Status = status
	if err := p.persist(ch, key.ID, result, status); err != nil {
		return result, fmt.Errorf("persist probe result: %w", err)
	}
	return result, nil
}

type probeResponse struct {
	StatusCode int
	LatencyMS  int64
	RetryAfter time.Duration
	Body       string
	Message    string
	Err        error
}

func (p *Prober) doModelsProbe(ctx context.Context, ch *store.Channel, plainKey string, timeout time.Duration) probeResponse {
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	url := strings.TrimSuffix(ch.BaseURL, "/") + "/models"
	req, reqErr := http.NewRequestWithContext(probeCtx, http.MethodGet, url, nil)
	if reqErr != nil {
		return probeResponse{Message: reqErr.Error(), Err: reqErr}
	}
	req.Header.Set("Authorization", "Bearer "+plainKey)
	req.Header.Set("Accept", "application/json")

	start := p.opt.Now()
	resp, doErr := p.opt.Client.Do(req)
	latency := p.opt.Now().Sub(start).Milliseconds()
	if doErr != nil {
		return probeResponse{LatencyMS: latency, Message: doErr.Error(), Err: doErr}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	snippet := summarizeBody(body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := fmt.Sprintf("上游 /models 探测失败（HTTP %d）", resp.StatusCode)
		if snippet != "" {
			message += ": " + snippet
		}
		return probeResponse{
			StatusCode: resp.StatusCode, LatencyMS: latency,
			RetryAfter: retry.ParseRetryAfter(resp.Header.Get("Retry-After"), p.opt.Now()),
			Body:       snippet, Message: message,
		}
	}
	return probeResponse{StatusCode: resp.StatusCode, LatencyMS: latency}
}

func (p *Prober) persist(ch *store.Channel, keyID int64, result ProbeResult, status string) error {
	if p.opt.DB == nil {
		return nil
	}
	if err := p.opt.DB.Transaction(func(tx *gorm.DB) error {
		keyUpdates := map[string]any{}
		switch result.Result {
		case ProbeResultSuccess:
			keyUpdates["success_count"] = gorm.Expr("success_count + 1")
			keyUpdates["last_error"] = ""
		case ProbeResultFailure:
			keyUpdates["fail_count"] = gorm.Expr("fail_count + 1")
			keyUpdates["last_error"] = result.Error
		}
		keyUpdates["last_used_at"] = result.ProbedAt
		if len(keyUpdates) > 0 {
			if err := tx.Model(&store.ChannelKey{}).Where("id = ?", keyID).Updates(keyUpdates).Error; err != nil {
				return err
			}
		}
		return tx.Model(&store.Channel{}).Where("id = ?", ch.ID).Updates(map[string]any{
			"status":                 status,
			"last_probe_at":          result.ProbedAt,
			"last_probe_result":      result.Result,
			"last_probe_status_code": result.StatusCode,
			"last_probe_latency_ms":  result.LatencyMS,
			"last_probe_error_type":  result.ErrorType,
			"last_probe_error":       result.Error,
			"next_probe_at":          result.NextProbeAt,
		}).Error
	}); err != nil {
		return err
	}
	ch.Status = status
	ch.LastProbeAt = &result.ProbedAt
	ch.LastProbeResult = result.Result
	ch.LastProbeStatusCode = result.StatusCode
	ch.LastProbeLatencyMS = result.LatencyMS
	ch.LastProbeErrorType = result.ErrorType
	ch.LastProbeError = result.Error
	ch.NextProbeAt = &result.NextProbeAt
	return nil
}

func summarizeBody(body []byte) string {
	snippet := strings.TrimSpace(string(bytes.ToValidUTF8(body, []byte(" "))))
	snippet = strings.Join(strings.Fields(snippet), " ")
	if len(snippet) > 300 {
		snippet = snippet[:300] + "…"
	}
	return snippet
}
