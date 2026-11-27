package relay

import (
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"evergate/internal/ratelimit"
	"evergate/internal/store"
)

// TokenLimits 在鉴权之后执行下游令牌的 RPM 与并发上限。
// 超限返回 OpenAI 格式 429（type = rate_limit_exceeded）并写入请求日志；
// 并发名额在请求结束（含流式写完之后）释放。
func TokenLimits(limiter *ratelimit.Limiter, logWriter *LogWriter) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := tokenSnapshotFromContext(c)
		if token == nil || limiter == nil {
			c.Next()
			return
		}
		if token.RPMLimit > 0 {
			if retryAfter, ok := limiter.AllowRPM(token.ID, token.RPMLimit); !ok {
				rejectRateLimit(c, logWriter, *token, "rpm_limit_exceeded",
					"token requests per minute limit exceeded", retryAfter)
				return
			}
		}
		if token.ConcurrencyLimit > 0 {
			release, ok := limiter.Acquire(token.ID, token.ConcurrencyLimit)
			if !ok {
				rejectRateLimit(c, logWriter, *token, "concurrency_limit_exceeded",
					"token concurrency limit exceeded", time.Second)
				return
			}
			defer release()
		}
		c.Next()
	}
}

func rejectRateLimit(c *gin.Context, writer *LogWriter, token store.Token, code, message string, retryAfter time.Duration) {
	if writer != nil {
		writer.Submit(&store.RequestLog{
			TokenID: token.ID, TokenName: token.Name, StatusCode: http.StatusTooManyRequests,
			ErrorType: code, ErrorMessage: message, IP: c.ClientIP(), UserAgent: c.Request.UserAgent(),
		})
	}
	seconds := int(math.Ceil(retryAfter.Seconds()))
	if seconds < 1 {
		seconds = 1
	}
	c.Header("Retry-After", strconv.Itoa(seconds))
	c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
		"error": gin.H{"message": message, "type": "rate_limit_exceeded", "code": code},
	})
}
