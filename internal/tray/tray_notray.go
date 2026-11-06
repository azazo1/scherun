//go:build notray

// Package tray 在 notray 构建中提供空实现, 保证无 cgo 环境可编译.
package tray

import (
	"context"
	"log/slog"
)

// Actions 是托盘菜单的回调.
type Actions struct {
	// Open 打开或聚焦管理界面.
	Open func()
	// Quit 走统一退出路径.
	Quit func()
}

// Tray 是无托盘构建的占位控制器.
type Tray struct {
	state  State
	logger *slog.Logger
}

// New 创建占位托盘控制器.
func New(appName, version string, actions Actions, logger *slog.Logger) *Tray {
	return &Tray{logger: logger}
}

// Available 表示当前构建不包含托盘能力.
func Available() bool { return false }

// Update 在无托盘构建中只记录状态.
func (t *Tray) Update(state State) { t.state = state }

// Run 在无托盘构建中阻塞到 ctx 取消.
func (t *Tray) Run(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	<-ctx.Done()
	return nil
}

// HideDockIcon 在无托盘构建中不做任何事.
func HideDockIcon() {}
