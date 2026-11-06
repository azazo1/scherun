//go:build !notray

// Package tray 提供系统托盘图标与菜单.
//
// 交互约定:
//   - 左键单击: 打开或聚焦管理界面.
//   - 右键: 弹出菜单, 首项是禁用的应用名与版本, 末项是可用的退出.
//
// 托盘事件循环必须占用主 goroutine, HTTP 服务与调度器在后台运行.
package tray

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"fyne.io/systray"
)

// Actions 是托盘菜单的回调.
type Actions struct {
	// Open 打开或聚焦管理界面.
	Open func()
	// Quit 走统一退出路径.
	Quit func()
}

// Tray 是托盘控制器.
type Tray struct {
	appName string
	version string
	actions Actions
	logger  *slog.Logger

	mu    sync.Mutex
	state State
	menu  *menuItems
	ready bool
}

// menuItems 保存菜单项句柄.
type menuItems struct {
	title  *systray.MenuItem
	status *systray.MenuItem
	open   *systray.MenuItem
	quit   *systray.MenuItem
}

// New 创建托盘控制器.
func New(appName, version string, actions Actions, logger *slog.Logger) *Tray {
	if appName == "" {
		appName = "scherun"
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Tray{appName: appName, version: version, actions: actions, logger: logger}
}

// Available 表示当前构建包含托盘能力.
func Available() bool { return true }

// Update 更新托盘展示的状态; 可以在任意 goroutine 调用.
func (t *Tray) Update(state State) {
	t.mu.Lock()
	t.state = state
	t.mu.Unlock()
	t.apply()
}

// Run 占用主 goroutine 运行托盘事件循环, ctx 取消时退出.
func (t *Tray) Run(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		return nil
	}
	exited := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			systray.Quit()
		case <-exited:
		}
	}()
	systray.Run(t.onReady, func() {
		t.mu.Lock()
		t.ready = false
		t.mu.Unlock()
		close(exited)
	})
	return nil
}

// onReady 在托盘就绪后构建菜单.
func (t *Tray) onReady() {
	// 左键单击直接打开界面, 右键弹出菜单.
	systray.SetOnTapped(func() {
		t.logger.Info("托盘左键单击", "action", "open")
		if t.actions.Open != nil {
			t.actions.Open()
		}
	})
	systray.SetOnSecondaryTapped(func() {
		t.logger.Debug("托盘右键单击", "action", "menu")
	})

	m := &menuItems{}
	// 首项显示应用名与版本, 禁用不可点.
	m.title = systray.AddMenuItem(fmt.Sprintf("%s %s", t.appName, t.version), "应用名称与版本")
	m.title.Disable()
	m.status = systray.AddMenuItem("正在启动", "任务运行概况")
	m.status.Disable()
	systray.AddSeparator()
	m.open = systray.AddMenuItem("打开管理界面", "在浏览器中打开本地管理界面")
	systray.AddSeparator()
	m.quit = systray.AddMenuItem("退出", "退出 scherun")

	t.mu.Lock()
	t.menu = m
	t.ready = true
	t.mu.Unlock()

	go t.loop("open", m.open.ClickedCh, t.actions.Open)
	go t.loop("quit", m.quit.ClickedCh, t.actions.Quit)

	t.apply()
}

// loop 把菜单点击转成回调调用.
func (t *Tray) loop(name string, ch <-chan struct{}, fn func()) {
	if fn == nil {
		return
	}
	for range ch {
		t.logger.Info("托盘菜单点击", "item", name)
		fn()
	}
}

// apply 把状态刷新到图标与菜单.
func (t *Tray) apply() {
	t.mu.Lock()
	ready := t.ready
	state := t.state
	m := t.menu
	t.mu.Unlock()
	if !ready || m == nil {
		return
	}

	systray.SetIcon(platformIcon(iconPNG(state.IconKind())))
	systray.SetTooltip(state.TooltipFor(t.appName))
	m.status.SetTitle(state.SummaryItem())
}
