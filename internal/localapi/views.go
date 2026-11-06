package localapi

import (
	"time"

	"github.com/azazo1/scherun/internal/proc"
	"github.com/azazo1/scherun/internal/scheduler"
	"github.com/azazo1/scherun/internal/task"
)

// taskView 是返回给界面的任务视图, 合并了定义与运行状态.
type taskView struct {
	task.Task
	Status    scheduler.Status `json:"status"`
	PID       int              `json:"pid,omitempty"`
	StartedAt *time.Time       `json:"started_at,omitempty"`
	Restarts  int              `json:"restarts"`
	Failures  int              `json:"failures"`
	// Runnable 表示程序当前是否可执行, 供界面提前提示.
	Runnable bool `json:"runnable"`
}

// buildTaskViews 把任务定义与调度状态合并成界面视图.
//
// 列表接口与 SSE 推送共用这一处构造: 两边各自拼一份时极易漏字段,
// 曾经就漏掉 Runnable, 导致任务在运行时界面却显示 "程序不可执行".
func (s *Server) buildTaskViews(tasks []task.Task) []taskView {
	snapshots := s.opts.Scheduler.Snapshots()
	views := make([]taskView, 0, len(tasks))
	for _, t := range tasks {
		snap, ok := snapshots[t.ID]
		if !ok || snap.TaskID == 0 {
			snap = scheduler.Snapshot{TaskID: t.ID, Status: scheduler.StatusIdle}
		}
		views = append(views, taskView{
			Task:      t,
			Status:    snap.Status,
			PID:       snap.PID,
			StartedAt: snap.StartedAt,
			Restarts:  snap.Restarts,
			Failures:  snap.Failures,
			Runnable:  proc.Runnable(t.Program),
		})
	}
	return views
}
