package localapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/azazo1/scherun/internal/scheduler"
	"github.com/azazo1/scherun/internal/task"
)

// sseEvent 是推送给前端的事件.
type sseEvent struct {
	// Tasks 是任务列表快照, 每秒推送一次.
	Tasks []taskView `json:"tasks,omitempty"`
	// Warnings 是取走的告警, 有新告警时出现.
	Warnings []scheduler.Warning `json:"warnings,omitempty"`
}

// handleEvents 通过 SSE 持续推送任务状态与告警.
//
// 用 SSE 而不是轮询: 一个连接就能覆盖全部任务, 前端也不必为每个任务单独请求.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeProblem(w, http.StatusMethodNotAllowed, "只支持 GET")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeProblem(w, http.StatusInternalServerError, "当前连接不支持流式推送")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	// 先立即推送一次, 避免界面等待首个 tick.
	s.sendEvent(w, flusher, true)

	for {
		select {
		case <-r.Context().Done():
			// 客户端关闭了页面或连接.
			return
		case <-s.draining:
			// 服务开始关闭: 主动退出, 让 http.Server.Shutdown 能及时返回.
			return
		case <-s.done:
			return
		case <-ticker.C:
			if !s.sendEvent(w, flusher, false) {
				return
			}
		}
	}
}

// sendEvent 组装并写出一条事件, 连接已断开时返回 false.
func (s *Server) sendEvent(w http.ResponseWriter, flusher http.Flusher, includeWarnings bool) bool {
	tasks, err := s.opts.DB.ListTasks()
	if err != nil {
		// 数据库临时不可用时跳过本轮, 不让连接断开.
		return true
	}
	snapshots := s.opts.Scheduler.Snapshots()
	views := make([]taskView, 0, len(tasks))
	for _, t := range tasks {
		snap := snapshots[t.ID]
		if snap.TaskID == 0 {
			snap = scheduler.Snapshot{TaskID: t.ID, Status: scheduler.StatusIdle}
		}
		views = append(views, taskView{
			Task:      t,
			Status:    snap.Status,
			PID:       snap.PID,
			StartedAt: snap.StartedAt,
			Restarts:  snap.Restarts,
			Failures:  snap.Failures,
		})
	}

	event := sseEvent{Tasks: views}
	if includeWarnings {
		event.Warnings = s.opts.Scheduler.TakeWarnings()
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return true
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
		return false
	}
	flusher.Flush()
	return true
}

// 保证 task 包被引用 (taskView 内嵌 task.Task).
var _ = task.Task{}
