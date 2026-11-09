package relay

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"evergate/internal/store"
)

func newAuthEnv(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	gin.SetMode(gin.TestMode)
	e := gin.New()
	e.GET("/v1/models", TokenAuth(db), func(c *gin.Context) {
		id, _ := tokenFromContext(c)
		c.JSON(200, gin.H{"token_id": id})
	})
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return e, db
}

func TestTokenAuthMiddleware(t *testing.T) {
	e, db := newAuthEnv(t)
	plain := "eg-test-token-123"
	sum := sha256.Sum256([]byte(plain))
	if err := db.Create(&store.Token{Name: "t1", KeyHash: hex.EncodeToString(sum[:]), Enabled: true}).Error; err != nil {
		t.Fatal(err)
	}
	gw := httptest.NewServer(e)
	defer gw.Close()

	do := func(auth string) *http.Response {
		req, _ := http.NewRequest("GET", gw.URL+"/v1/models", nil)
		if auth != "" {
			req.Header.Set("Authorization", "Bearer "+auth)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}
	// 无 header / 错 token → 401
	if resp := do(""); resp.StatusCode != 401 {
		t.Fatalf("无 token: %d", resp.StatusCode)
	}
	if resp := do("wrong"); resp.StatusCode != 401 {
		t.Fatalf("错 token: %d", resp.StatusCode)
	}
	// 正确 token → 通过且注入身份
	resp := do(plain)
	if resp.StatusCode != 200 {
		t.Fatalf("正确 token: %d", resp.StatusCode)
	}
	resp.Body.Close()
	// 禁用后 → 401
	db.Model(&store.Token{}).Where("id = 1").Update("enabled", false)
	if resp := do(plain); resp.StatusCode != 401 {
		t.Fatalf("禁用 token: %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestTokenAuthExpired(t *testing.T) {
	e, db := newAuthEnv(t)
	plain := "eg-expired-token"
	sum := sha256.Sum256([]byte(plain))
	expired := time.Now().Add(-time.Hour)
	if err := db.Create(&store.Token{Name: "t2", KeyHash: hex.EncodeToString(sum[:]), Enabled: true, ExpiredAt: &expired}).Error; err != nil {
		t.Fatal(err)
	}
	gw := httptest.NewServer(e)
	defer gw.Close()
	req, _ := http.NewRequest("GET", gw.URL+"/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+plain)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 401 {
		t.Fatalf("过期 token 应 401: %d", resp.StatusCode)
	}
}

func TestLogWriterBatchAndDrop(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	w := NewLogWriter(db, 8, nil)
	for i := 0; i < 20; i++ {
		w.Submit(&store.RequestLog{Model: "m", StatusCode: 200})
	}
	w.Close() // Close 刷出缓冲并等待落库
	var count int64
	db.Model(&store.RequestLog{}).Count(&count)
	if count == 0 {
		t.Fatal("Close 后日志应已落库")
	}
	// Close 后 Submit 不 panic
	w.Submit(&store.RequestLog{Model: "late"})
}
