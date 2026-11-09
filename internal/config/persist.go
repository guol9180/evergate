package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"strconv"

	"gopkg.in/yaml.v3"
)

// Persist 将 security 密钥写回 path。已有文件按行替换空值以保留注释；
// 文件不存在或无法按行替换时整体 YAML 重写（注释丢失但密钥不丢）。
func (c Config) Persist(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("read config for persist: %w", err)
		}
		out, mErr := yaml.Marshal(c)
		if mErr != nil {
			return fmt.Errorf("marshal config: %w", mErr)
		}
		return os.WriteFile(path, out, 0o600)
	}
	text := string(data)
	replaced := 0
	for _, p := range []struct{ key, val string }{
		{"master_key", c.Security.MasterKey},
		{"session_secret", c.Security.SessionSecret},
	} {
		if p.val == "" {
			continue
		}
		re := regexp.MustCompile(`(?m)^(\s*` + regexp.QuoteMeta(p.key) + `:\s*)""`)
		if re.MatchString(text) {
			text = re.ReplaceAllString(text, "${1}"+strconv.Quote(p.val))
			replaced++
		}
	}
	if replaced == 0 {
		out, mErr := yaml.Marshal(c)
		if mErr != nil {
			return fmt.Errorf("marshal config: %w", mErr)
		}
		return os.WriteFile(path, out, 0o600)
	}
	return os.WriteFile(path, []byte(text), 0o600)
}
