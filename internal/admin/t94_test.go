package admin

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"evergate/internal/store"
)

// 首启管理员设置：状态、校验、写入、审计与重复调用（T94）。
func TestInitialAdminSetupFlow(t *testing.T) {
	env := newEnvWithoutAdmin(t)

	code, body, _ := env.req(t, "GET", "/admin/api/auth/setup-status", "", "")
	if code != http.StatusOK || !strings.Contains(body, `"setup_required":true`) {
		t.Fatalf("空表应要求初始化: %d %s", code, body)
	}

	if code, body, _ := env.req(t, "POST", "/admin/api/auth/setup", "", `{"username":"admin","password":"short"}`); code != http.StatusBadRequest || !strings.Contains(body, "至少 8 位") {
		t.Fatalf("弱密码应 400: %d %s", code, body)
	}
	if code, body, _ := env.req(t, "POST", "/admin/api/auth/setup", "", `{"username":"a b","password":"Admin1234!"}`); code != http.StatusBadRequest || !strings.Contains(body, "用户名") {
		t.Fatalf("非法用户名应 400: %d %s", code, body)
	}
	if code, body, _ := env.req(t, "POST", "/admin/api/auth/setup", "", `{"username":"admin","password":"abcdefgh"}`); code != http.StatusBadRequest || !strings.Contains(body, "三类") {
		t.Fatalf("字符类别不足应 400: %d %s", code, body)
	}

	code, body, _ = env.req(t, "POST", "/admin/api/auth/setup", "", `{"username":"root.admin","password":"Admin1234!"}`)
	if code != http.StatusOK || !strings.Contains(body, `"setup":true`) {
		t.Fatalf("初始化应成功: %d %s", code, body)
	}

	// 初始化后 directly 登录，不强制改密
	_, loginBody := login(t, env, "root.admin", "Admin1234!")
	if strings.Contains(loginBody, `"must_change_password":true`) {
		t.Fatalf("首启设置后不应强制改密: %s", loginBody)
	}

	var audits []store.AuditLog
	if err := env.db.Order("id desc").Limit(1).Find(&audits).Error; err != nil {
		t.Fatalf("read audit: %v", err)
	}
	if len(audits) != 1 || audits[0].Action != "setup_admin" || audits[0].Username != "root.admin" {
		t.Fatalf("应记录 setup_admin 审计: %+v", audits)
	}

	// 已有管理员：状态 false，重复设置 403
	if code, body, _ := env.req(t, "GET", "/admin/api/auth/setup-status", "", ""); code != http.StatusOK || !strings.Contains(body, `"setup_required":false`) {
		t.Fatalf("已有管理员时状态应为 false: %d %s", code, body)
	}
	if code, body, _ := env.req(t, "POST", "/admin/api/auth/setup", "", `{"username":"other","password":"Admin1234!"}`); code != http.StatusForbidden {
		t.Fatalf("重复初始化应 403: %d %s", code, body)
	}
}

func TestSetupRejectsNonLoopback(t *testing.T) {
	env := newEnvWithoutAdmin(t)
	req := httptest.NewRequest("POST", "/admin/api/auth/setup", strings.NewReader(`{"username":"admin","password":"Admin1234!"}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "203.0.113.9:54321"
	w := httptest.NewRecorder()
	env.engine.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("非回环地址应 403，得到 %d %s", w.Code, w.Body.String())
	}
	if required, _ := store.SetupRequired(env.db); !required {
		t.Fatal("被拒绝的请求不应创建管理员")
	}
}

func TestSetupConcurrentOnlyOneSucceeds(t *testing.T) {
	env := newEnvWithoutAdmin(t)
	const attempts = 5
	codes := make([]int, attempts)
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			req, err := http.NewRequest("POST", env.gw.URL+"/admin/api/auth/setup",
				strings.NewReader(fmt.Sprintf(`{"username":"admin-%d","password":"Admin1234!"}`, i)))
			if err != nil {
				codes[i] = -1
				return
			}
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				codes[i] = -2
				return
			}
			resp.Body.Close()
			codes[i] = resp.StatusCode
		}(i)
	}
	wg.Wait()
	success := 0
	for _, code := range codes {
		if code == http.StatusOK {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("并发初始化应只有一个成功，得到 %v", codes)
	}
}
