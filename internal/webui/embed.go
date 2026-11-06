// Package webui 内嵌前端静态资源.
//
// 构建流程把 web/dist 的内容复制到 dist 目录, 再通过 go:embed 打包进二进制.
// dist 里的产物不进版本库, 只保留一个占位文件让 go:embed 始终可编译;
// 未构建前端时, 服务会返回 Index 里的提示页.
package webui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var embedded embed.FS

// FS 是内嵌静态资源的根.
var FS fs.FS

// Index 是未构建前端时返回的提示页.
const Index = `<!doctype html>
<html lang="zh-CN">
  <head><meta charset="utf-8" /><title>scherun</title></head>
  <body style="font-family: -apple-system, sans-serif; padding: 2rem; line-height: 1.6">
    <h1>前端资源尚未构建</h1>
    <p>请先运行 <code>just web</code>, 然后重新构建并启动 scherun.</p>
  </body>
</html>
`

func init() {
	sub, err := fs.Sub(embedded, "dist")
	if err != nil {
		panic("初始化内嵌前端资源失败: " + err.Error())
	}
	FS = sub
}

// Built 表示是否包含真正的前端构建产物.
func Built() bool {
	_, err := fs.Stat(FS, "index.html")
	return err == nil
}
