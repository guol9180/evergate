package admin

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"evergate/internal/store"
)

// 接口级集成：创建 → 列表 → 下载 → 暂存恢复（坏文件 400 / 真备份 200 且待重启）→ 删除。
func TestBackupEndpointsRoundTrip(t *testing.T) {
	env := newEnv(t)
	cookie := readyCookie(t, env)

	code, body, _ := env.req(t, "POST", "/admin/api/backups", cookie, "")
	if code != http.StatusOK || !strings.Contains(body, `"name":"evergate-`) {
		t.Fatalf("create: %d %s", code, body)
	}
	var created struct {
		Data struct {
			Name    string `json:"name"`
			Size    int64  `json:"size"`
			Version string `json:"version"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	name := created.Data.Name
	if created.Data.Size == 0 {
		t.Fatalf("created size = 0: %s", body)
	}

	code, body, _ = env.req(t, "GET", "/admin/api/backups", cookie, "")
	if code != http.StatusOK || !strings.Contains(body, name) || !strings.Contains(body, `"restart_pending":false`) {
		t.Fatalf("list: %d %s", code, body)
	}

	code, body, _ = env.req(t, "GET", "/admin/api/backups/"+name, cookie, "")
	if code != http.StatusOK || !strings.HasPrefix(body, "SQLite format 3") {
		t.Fatalf("download: %d %.40q", code, body)
	}
	// 路径穿越被路由拒绝（404）；能匹配路由但不符合命名规则的由 ValidBackupName 拒绝（400）
	if code, body, _ := env.req(t, "GET", "/admin/api/backups/..%2Fevergate.db", cookie, ""); code != http.StatusNotFound {
		t.Fatalf("traversal download: %d %s", code, body)
	}
	if code, body, _ := env.req(t, "GET", "/admin/api/backups/evergate-x.db", cookie, ""); code != http.StatusBadRequest {
		t.Fatalf("invalid name download: %d %s", code, body)
	}
	if code, body, _ := env.req(t, "GET", "/admin/api/backups/evergate-20200101T000000Z.db", cookie, ""); code != http.StatusNotFound {
		t.Fatalf("missing download: %d %s", code, body)
	}

	// 坏文件：不是 SQLite → 400 invalid_backup
	code, body, _ = uploadRestore(t, env, cookie, "garbage.db", []byte("definitely not sqlite"))
	if code != http.StatusBadRequest || !strings.Contains(body, "invalid_backup") {
		t.Fatalf("garbage restore: %d %s", code, body)
	}
	if _, err := os.Stat(env.dbPath + ".restore-pending"); !os.IsNotExist(err) {
		t.Fatalf("garbage upload must not stage a restore: %v", err)
	}

	// 真备份：先把数据改掉，再用刚才的备份恢复，暂存成功且提示重启
	if err := env.db.Create(&store.Channel{Name: "post-backup", Type: "openai", BaseURL: "https://x", Models: []string{"m"}}).Error; err != nil {
		t.Fatalf("mutate: %v", err)
	}
	backupBytes, err := os.ReadFile(filepath.Join(store.BackupDir(env.dbPath), name))
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	code, body, _ = uploadRestore(t, env, cookie, name, backupBytes)
	if code != http.StatusOK || !strings.Contains(body, `"restart_required":true`) || !strings.Contains(body, `"safety_backup":"evergate-`) {
		t.Fatalf("restore: %d %s", code, body)
	}
	if _, err := os.Stat(env.dbPath + ".restore-pending"); err != nil {
		t.Fatalf("pending restore missing: %v", err)
	}
	code, body, _ = env.req(t, "GET", "/admin/api/backups", cookie, "")
	if code != http.StatusOK || !strings.Contains(body, `"restart_pending":true`) {
		t.Fatalf("list after restore: %d %s", code, body)
	}

	// 删除：原始备份仍在（恢复用的是上传副本）→ 200；再删 → 404；穿越名 → 400
	if code, body, _ := env.req(t, "DELETE", "/admin/api/backups/"+name, cookie, ""); code != http.StatusOK || !strings.Contains(body, `"name":"`+name+`"`) {
		t.Fatalf("delete: %d %s", code, body)
	}
	if code, body, _ := env.req(t, "DELETE", "/admin/api/backups/"+name, cookie, ""); code != http.StatusNotFound {
		t.Fatalf("second delete: %d %s", code, body)
	}
	if code, body, _ := env.req(t, "DELETE", "/admin/api/backups/evergate-x.db", cookie, ""); code != http.StatusBadRequest {
		t.Fatalf("invalid name delete: %d %s", code, body)
	}
}

func uploadRestore(t *testing.T, env *testEnv, cookie, filename string, content []byte) (int, string, *http.Response) {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("form file: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write part: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	req, _ := http.NewRequest(http.MethodPost, env.gw.URL+"/admin/api/backups/restore", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Cookie", CookieName+"="+cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(data), resp
}
