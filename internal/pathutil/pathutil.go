// Package pathutil 提供路径规整的共享实现.
//
// 用户在各种输入框里都会写 ~, 而操作系统并不认识它: 直接拿去打开会建出
// 字面名为 "~" 的目录, 或者报路径不存在. 因此所有接受路径的入口都必须
// 先经过 ExpandHome, 而不是各自实现一份.
package pathutil

import (
	"os"
	"path/filepath"
	"strings"
)

// ExpandHome 把路径开头的 ~ 展开成用户家目录.
//
// 只认开头的 ~ 与 ~/, 路径中间出现的 ~ 按字面处理.
// 无法取得家目录时原样返回, 由后续的打开操作给出真实错误.
func ExpandHome(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
		return path
	}
	// 同时接受 ~/ 与 Windows 的 ~\.
	if !strings.HasPrefix(path, "~/") && !strings.HasPrefix(path, `~\`) {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[2:])
}

// Resolve 把路径规整成可直接使用的形式.
//
// 先展开开头的 ~, 再按基准目录把相对路径转成绝对路径.
// base 为空时相对路径保持不变, 交给调用方决定如何解释.
func Resolve(base, path string) string {
	path = ExpandHome(path)
	if path == "" {
		return path
	}
	if filepath.IsAbs(path) || base == "" {
		return path
	}
	return filepath.Join(base, path)
}
