//go:build !notray && !darwin

package tray

import "fyne.io/systray"

// platformIcon 在非 Windows 平台直接使用 PNG.
func platformIcon(data []byte) []byte { return data }

// setIcon 在非 macOS 平台使用彩色图标.
//
// Windows 与 Linux 的托盘不按模板图标处理, 传模板图反而会得到一个纯黑图标,
// 因此这里直接走 SetIcon.
func setIcon(template, regular []byte) {
	systray.SetIcon(regular)
}
