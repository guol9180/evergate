package relay

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"evergate/internal/store"
)

// newCatalogGateway 在测试环境上挂带令牌鉴权的下游模型目录路由（T93）。
func newCatalogGateway(t *testing.T, env *testEnv, plain string) *httptest.Server {
	t.Helper()
	sum := sha256.Sum256([]byte(plain))
	if err := env.db.Create(&store.Token{Name: "catalog", KeyHash: hex.EncodeToString(sum[:]), Enabled: true}).Error; err != nil {
		t.Fatalf("create token: %v", err)
	}
	gin.SetMode(gin.TestMode)
	e := gin.New()
	e.GET("/v1/models", TokenAuth(env.db), env.HandleModels)
	e.GET("/v1beta/models", RejectQueryKey(), TokenAuth(env.db), env.HandleGeminiModels)
	gw := httptest.NewServer(e)
	t.Cleanup(gw.Close)
	return gw
}

func getWithHeaders(t *testing.T, url string, headers map[string]string) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get %s: %v", url, err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, string(body)
}

func TestModelsEndpointFiltersByTokenModels(t *testing.T) {
	env := newEnv(t)
	env.addChannel(t, "a", "http://127.0.0.1:1", 0, []string{"m3", "m1"}, nil, "sk-a")
	env.addChannel(t, "b", "http://127.0.0.1:1", 0, []string{"m2", "m1"}, nil, "sk-b")
	plain := "eg-catalog-token"
	gw := newCatalogGateway(t, env, plain)

	var token store.Token
	if err := env.db.Where("name = ?", "catalog").First(&token).Error; err != nil {
		t.Fatalf("load token: %v", err)
	}
	token.AllowedModels = []string{"m2"}
	if err := env.db.Save(&token).Error; err != nil {
		t.Fatalf("set allowed models: %v", err)
	}

	resp, body := getWithHeaders(t, gw.URL+"/v1/models", map[string]string{"Authorization": "Bearer " + plain})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
	var out struct {
		Object string `json:"object"`
		Data   []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Object != "list" || len(out.Data) != 1 || out.Data[0].ID != "m2" {
		t.Fatalf("令牌限制 m2 时应只返回 m2: %s", body)
	}

	// 解除限制后返回全部启用渠道模型，且排序去重
	token.AllowedModels = nil
	if err := env.db.Save(&token).Error; err != nil {
		t.Fatalf("clear allowed models: %v", err)
	}
	_, body = getWithHeaders(t, gw.URL+"/v1/models", map[string]string{"Authorization": "Bearer " + plain})
	out.Data = nil
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Data) != 3 || out.Data[0].ID != "m1" || out.Data[1].ID != "m2" || out.Data[2].ID != "m3" {
		t.Fatalf("未限制时应返回排序去重的 m1/m2/m3: %s", body)
	}
}

func TestModelsEndpointAnthropicShape(t *testing.T) {
	env := newEnv(t)
	env.addChannel(t, "a", "http://127.0.0.1:1", 0, []string{"claude-a", "claude-b"}, nil, "sk-a")
	gw := newCatalogGateway(t, env, "eg-anthropic-token")

	resp, body := getWithHeaders(t, gw.URL+"/v1/models", map[string]string{
		"x-api-key": "eg-anthropic-token", "anthropic-version": "2023-06-01",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
	var out struct {
		Data []struct {
			Type        string `json:"type"`
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
			CreatedAt   string `json:"created_at"`
		} `json:"data"`
		HasMore bool   `json:"has_more"`
		FirstID string `json:"first_id"`
		LastID  string `json:"last_id"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Data) != 2 || out.HasMore || out.FirstID != "claude-a" || out.LastID != "claude-b" {
		t.Fatalf("Anthropic 结构不正确: %s", body)
	}
	if out.Data[0].Type != "model" || out.Data[0].ID != "claude-a" || out.Data[0].DisplayName != "claude-a" || out.Data[0].CreatedAt == "" {
		t.Fatalf("Anthropic 模型项不正确: %s", body)
	}
}

func TestGeminiModelsEndpoint(t *testing.T) {
	env := newEnv(t)
	env.addChannel(t, "a", "http://127.0.0.1:1", 0, []string{"gemini-b", "gemini-a"}, nil, "sk-a")
	gw := newCatalogGateway(t, env, "eg-gemini-token")

	resp, body := getWithHeaders(t, gw.URL+"/v1beta/models", map[string]string{"x-goog-api-key": "eg-gemini-token"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
	var out struct {
		Models []struct {
			Name                       string   `json:"name"`
			BaseModelID                string   `json:"baseModelId"`
			DisplayName                string   `json:"displayName"`
			SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
		} `json:"models"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Models) != 2 || out.Models[0].Name != "models/gemini-a" || out.Models[1].Name != "models/gemini-b" {
		t.Fatalf("Gemini 模型列表不正确: %s", body)
	}
	if out.Models[0].BaseModelID != "gemini-a" || out.Models[0].DisplayName != "gemini-a" || len(out.Models[0].SupportedGenerationMethods) != 2 {
		t.Fatalf("Gemini 模型项不正确: %s", body)
	}

	if resp, _ := getWithHeaders(t, gw.URL+"/v1beta/models", nil); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("无令牌应 401，得到 %d", resp.StatusCode)
	}
}
