//go:build !notray && !windows

package tray

// platformIcon 在非 Windows 平台直接使用 PNG.
func platformIcon(data []byte) []byte { return data }
