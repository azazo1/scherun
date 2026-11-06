package tray

import "fmt"

// State 是托盘展示所需的最小状态.
type State struct {
	// Running 是正在运行的任务数.
	Running int
	// Total 是任务总数.
	Total int
	// Failed 是处于挂起状态的任务数.
	Failed int
	// Healthy 表示服务是否正常.
	Healthy bool
}

// TooltipFor 返回托盘提示文本, 第一行是应用名.
func (s State) TooltipFor(app string) string {
	if app == "" {
		app = "scherun"
	}
	return fmt.Sprintf("%s\n运行中 %d / 共 %d", app, s.Running, s.Total)
}

// SummaryItem 返回菜单里的状态行文本.
func (s State) SummaryItem() string {
	if s.Total == 0 {
		return "暂无任务"
	}
	if s.Failed > 0 {
		return fmt.Sprintf("运行中 %d / 共 %d, 挂起 %d", s.Running, s.Total, s.Failed)
	}
	return fmt.Sprintf("运行中 %d / 共 %d", s.Running, s.Total)
}

// IconKind 返回图标颜色类别.
func (s State) IconKind() string {
	switch {
	case s.Failed > 0:
		return "warning"
	case s.Running > 0:
		return "active"
	case s.Total > 0:
		return "idle"
	default:
		return "empty"
	}
}
