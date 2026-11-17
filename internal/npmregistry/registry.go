// Package npmregistry 定义自更新下载与版本检查共用的 npm registry 默认链。
package npmregistry

import (
	"net/url"
	"strings"
)

const (
	// Mirror 是国内可用的 npm 镜像，优先使用。
	Mirror = "https://registry.npmmirror.com"
	// Official 是 npm 官方 registry，作为规范源兜底。
	Official = "https://registry.npmjs.org"
	// Package 是 EverGate npm 主包名，版本与 git tag 同源。
	Package = "@heguoliang/evergate"
	// UpdatesPackage 包含完整安装资产，仅供内置更新器使用。
	UpdatesPackage = "@heguoliang/evergate-updates"
)

// Default 返回默认下载/检查链：镜像优先，官方兜底。
func Default() []string {
	return []string{Mirror, Official}
}

// Normalize 去掉空项、末尾斜杠与重复项，保持声明顺序。
func Normalize(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, registry := range in {
		registry = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(registry), "/"))
		if registry == "" || seen[registry] {
			continue
		}
		seen[registry] = true
		out = append(out, registry)
	}
	return out
}

// Host 返回 registry 的主机名，用于日志与界面展示。
func Host(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		return u.Host
	}
	return raw
}
