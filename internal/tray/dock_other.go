//go:build !darwin && !notray

package tray

// HideDockIcon 在非 macOS 平台无需处理, 保留空实现让调用点不必区分平台.
func HideDockIcon() {}
