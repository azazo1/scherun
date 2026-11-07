//go:build !notray && !darwin && !windows

package tray

import "fyne.io/systray"

// setIcon 在非 macOS, 非 Windows 平台使用彩色图标.
//
// macOS 走 icon_darwin.go 的模板图标, Windows 走 icon_windows.go 的 ICO 包装;
// 这里的三个标签互斥, 少排一个平台就会出现同名函数重复声明.
func setIcon(template, regular []byte) {
	systray.SetIcon(regular)
}
