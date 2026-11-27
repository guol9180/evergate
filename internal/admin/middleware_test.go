package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// 写接口成功后必须让转发配置快照失效；读接口与失败的写操作不应触发。
func TestInvalidateOnWriteOnlyAfterSuccessfulWrites(t *testing.T) {
	gin.SetMode(gin.TestMode)
	calls := 0
	api := &API{opt: Options{OnConfigChange: func() { calls++ }}}
	r := gin.New()
	g := r.Group("/admin/api")
	g.Use(api.invalidateOnWrite)
	g.GET("/read", func(c *gin.Context) { c.Status(http.StatusOK) })
	g.POST("/write", func(c *gin.Context) { c.Status(http.StatusOK) })
	g.POST("/reject", func(c *gin.Context) { c.Status(http.StatusBadRequest) })

	for _, tc := range []struct {
		method, path string
		want         int
	}{
		{http.MethodGet, "/admin/api/read", 0},
		{http.MethodPost, "/admin/api/write", 1},
		{http.MethodPost, "/admin/api/reject", 1},
	} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))
		if calls != tc.want {
			t.Fatalf("%s %s: invalidations = %d, want %d", tc.method, tc.path, calls, tc.want)
		}
	}
}
