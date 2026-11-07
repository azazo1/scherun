//go:build !notray && darwin

package tray

import "fyne.io/systray"

// setIcon 在 macOS 上使用模板图标.
//
// 模板图标必须是纯黑加 alpha 的单色图, 系统会按菜单栏明暗自动反色,
// 因此不能直接用彩色图标; regular 参数在这里不生效.
func setIcon(template, regular []byte) {
	systray.SetTemplateIcon(template, regular)
}
