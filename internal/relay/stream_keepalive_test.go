package relay

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestSSEWriterKeepaliveDoesNotEmitDataFrames(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	w := newSSEWriter(c.Writer)
	w.interval = 10 * time.Millisecond
	w.header()

	stop := w.startKeepalive()
	defer stop()

	deadline := time.Now().Add(250 * time.Millisecond)
	for !strings.Contains(rec.Body.String(), ": keepalive\n\n") && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}

	body := rec.Body.String()
	if !strings.Contains(body, ": keepalive\n\n") {
		t.Fatalf("keepalive missing from body %q", body)
	}
	if strings.Contains(body, "data:") {
		t.Fatalf("keepalive must not emit data frames: %q", body)
	}
}
