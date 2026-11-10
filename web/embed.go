// Package web 提供 go:embed 嵌入的前端管理台构建产物。
// dist 随仓库提交（约 400KB），保证后端开发者无需 Node 环境即可构建单二进制。
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// DistFS 以 dist 为根的文件系统，可直接交给 http.FileServerFS 使用。
var DistFS, _ = fs.Sub(dist, "dist")
