package relay

import (
	"encoding/json"
	"fmt"

	"github.com/gin-gonic/gin"
)

// ctxBodyRewriter 保存本次请求的请求体重写器；一次解析、按上游模型缓存结果。
const ctxBodyRewriter = "evergate.body_rewriter"

// bodyRewriter 把原始请求体解析成字段表一次，之后每次尝试只替换 model（与流式 usage 开关）再序列化。
// 重试、多 key、同模型多渠道都命中缓存，不再逐次解析 / 序列化整包 JSON。
// 桥接体（Messages / Gemini / Responses 转 chat）不是原始体，直接走无缓存路径。
type bodyRewriter struct {
	original []byte
	// parsed 首次重写时才填充：原生 Gemini 等不重写请求体的路径不付解析成本。
	parsed map[string]json.RawMessage
	// parsedOnce 标记已尝试解析（解析失败时 parsed 为 nil，后续直接走无缓存路径）。
	parsedOnce bool
	// streamOptions 保留原始 stream_options，重建时用它复位，避免不同 injectUsage 互相污染。
	streamOptions json.RawMessage
	cache         map[string][]byte
}

func newBodyRewriter(original []byte) *bodyRewriter {
	return &bodyRewriter{original: original, cache: make(map[string][]byte, 2)}
}

func bodyRewriterFromContext(c *gin.Context) *bodyRewriter {
	value, _ := c.Get(ctxBodyRewriter)
	rw, _ := value.(*bodyRewriter)
	return rw
}

// rewrite 返回该上游模型的请求体；rw 为 nil 或请求体不是原始体时退回逐次重写。
func (rw *bodyRewriter) rewrite(body []byte, upstreamModel string, injectUsage bool) ([]byte, error) {
	if rw == nil || !sameBody(body, rw.original) {
		return rewriteUpstreamBody(body, upstreamModel, injectUsage)
	}
	if !rw.parsedOnce {
		rw.parsedOnce = true
		var wire map[string]json.RawMessage
		if err := json.Unmarshal(rw.original, &wire); err == nil {
			rw.parsed = wire
			rw.streamOptions = wire["stream_options"]
		}
	}
	if rw.parsed == nil {
		return rewriteUpstreamBody(body, upstreamModel, injectUsage)
	}
	key := upstreamModel
	if injectUsage {
		key += "\x00usage"
	}
	if cached, ok := rw.cache[key]; ok {
		return cached, nil
	}

	encodedModel, err := json.Marshal(upstreamModel)
	if err != nil {
		return nil, err
	}
	rw.parsed["model"] = encodedModel
	if err := setStreamUsage(rw.parsed, rw.streamOptions, injectUsage); err != nil {
		return nil, err
	}
	out, err := json.Marshal(rw.parsed)
	if err != nil {
		return nil, err
	}
	rw.cache[key] = out
	return out, nil
}

// rewriteUpstreamBody 是无缓存路径：先替换 model，再按需注入流式 usage。
func rewriteUpstreamBody(body []byte, upstreamModel string, injectUsage bool) ([]byte, error) {
	out, err := replaceModel(body, upstreamModel)
	if err != nil {
		return nil, err
	}
	if !injectUsage {
		return out, nil
	}
	var wire map[string]json.RawMessage
	if err := json.Unmarshal(out, &wire); err != nil {
		return nil, err
	}
	if err := setStreamUsage(wire, wire["stream_options"], true); err != nil {
		return nil, err
	}
	return json.Marshal(wire)
}

// setStreamUsage 按原始 stream_options 重建，injectUsage=false 时恢复原值（或删除注入项）。
func setStreamUsage(wire map[string]json.RawMessage, original json.RawMessage, injectUsage bool) error {
	if !injectUsage {
		if len(original) > 0 {
			wire["stream_options"] = original
		} else {
			delete(wire, "stream_options")
		}
		return nil
	}
	options := make(map[string]json.RawMessage)
	if len(original) > 0 && !isJSONNull(original) {
		if err := json.Unmarshal(original, &options); err != nil {
			return fmt.Errorf("stream_options 必须是 JSON 对象")
		}
	}
	options["include_usage"] = json.RawMessage("true")
	encoded, err := json.Marshal(options)
	if err != nil {
		return err
	}
	wire["stream_options"] = encoded
	return nil
}

// sameBody 判断两个切片是否指向同一段底层数组，用于区分原始体与桥接体（不比较内容）。
func sameBody(a, b []byte) bool {
	return len(a) == len(b) && (len(a) == 0 || &a[0] == &b[0])
}
