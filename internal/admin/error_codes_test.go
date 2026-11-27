package admin

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// 管理台每个失败信封都必须带稳定 code（前端按 code 本地化）；漏写或退回三参写法在这里被拦下。
// code 与前端字典的一致性由 web/tests/errorCodes.test.ts 扫描本包源码校验。
func TestEveryFailCallCarriesStableCode(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	dir := filepath.Dir(thisFile)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	// importFailure 自带 code 字段，允许变量形式；其余调用点必须写字面量 code
	call := regexp.MustCompile(`\bfail\(c, [^,]+, ("[a-z_]+"|f\.code), `)
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "httpx.Fail(") {
			t.Errorf("%s: use httpx.FailCode with a stable code", entry.Name())
		}
		for i, line := range strings.Split(string(raw), "\n") {
			if strings.Contains(line, "func fail(") || !strings.Contains(line, "fail(c, ") {
				continue
			}
			if !call.MatchString(line) {
				t.Errorf("%s:%d: fail() without stable code: %s", entry.Name(), i+1, strings.TrimSpace(line))
			}
			if match := call.FindStringSubmatch(line); match != nil && match[1] == "" {
				t.Errorf("%s:%d: empty code: %s", entry.Name(), i+1, strings.TrimSpace(line))
			}
		}
	}
}
