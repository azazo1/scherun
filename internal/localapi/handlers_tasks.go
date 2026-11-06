package localapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/azazo1/scherun/internal/config"
	"github.com/azazo1/scherun/internal/proc"
	"github.com/azazo1/scherun/internal/scheduler"
	"github.com/azazo1/scherun/internal/store"
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
	Runnable  bool             `json:"runnable"`
}

// healthResponse 是健康检查响应.
type healthResponse struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	StartedAt string `json:"started_at"`
	URL       string `json:"url"`
	TaskCount int    `json:"task_count"`
}

// handleHealth 返回服务基本信息.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeProblem(w, http.StatusMethodNotAllowed, "只支持 GET")
		return
	}
	// 健康检查不需要鉴权, 但同样受请求围栏约束, 且不泄露任何敏感内容.
	if !trustedRequest(r) {
		writeProblem(w, http.StatusForbidden, "请求来源不可信")
		return
	}
	tasks, err := s.opts.DB.ListTasks()
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, healthResponse{
		Version:   s.opts.Version,
		Commit:    s.opts.Commit,
		StartedAt: s.started.Format(time.RFC3339),
		URL:       s.BaseURL(),
		TaskCount: len(tasks),
	})
}

// handleTasks 处理列表与创建/更新.
func (s *Server) handleTasks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listTasks(w)
	case http.MethodPost, http.MethodPut:
		s.saveTask(w, r)
	default:
		writeProblem(w, http.StatusMethodNotAllowed, "只支持 GET 与 POST")
	}
}

// listTasks 返回任务列表.
func (s *Server) listTasks(w http.ResponseWriter) {
	tasks, err := s.opts.DB.ListTasks()
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, err.Error())
		return
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
			Runnable:  proc.Runnable(t.Program),
		})
	}
	writeJSON(w, http.StatusOK, views)
}

// saveTask 新建或更新任务.
func (s *Server) saveTask(w http.ResponseWriter, r *http.Request) {
	var payload task.Task
	if err := decodeJSON(r, &payload); err != nil {
		writeProblem(w, http.StatusBadRequest, "请求体格式不正确: "+err.Error())
		return
	}
	id, err := s.opts.Scheduler.SaveTask(payload)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	saved, err := s.opts.DB.GetTask(id)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

// handleTaskByID 处理单个任务的操作.
//
// 路径形式: /api/tasks/{id} 与 /api/tasks/{id}/{action}.
func (s *Server) handleTaskByID(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/tasks/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeProblem(w, http.StatusNotFound, "缺少任务 id")
		return
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "任务 id 无效")
		return
	}

	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			t, err := s.opts.DB.GetTask(id)
			if err != nil {
				s.writeTaskError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, t)
		case http.MethodDelete:
			if err := s.opts.Scheduler.DeleteTask(id); err != nil {
				s.writeTaskError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		default:
			writeProblem(w, http.StatusMethodNotAllowed, "只支持 GET 与 DELETE")
		}
		return
	}

	if r.Method != http.MethodPost {
		writeProblem(w, http.StatusMethodNotAllowed, "只支持 POST")
		return
	}

	switch parts[1] {
	case "run":
		if err := s.opts.Scheduler.RunTask(id); err != nil {
			writeProblem(w, http.StatusConflict, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	case "stop":
		if err := s.opts.Scheduler.StopTask(id); err != nil {
			writeProblem(w, http.StatusConflict, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	case "enabled":
		var payload struct {
			Enabled *bool `json:"enabled"`
		}
		if err := decodeJSON(r, &payload); err != nil || payload.Enabled == nil {
			writeProblem(w, http.StatusBadRequest, "缺少 enabled 字段")
			return
		}
		if err := s.opts.Scheduler.SetEnabled(id, *payload.Enabled); err != nil {
			s.writeTaskError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		writeProblem(w, http.StatusNotFound, "未知的任务操作: "+parts[1])
	}
}

// handleReorder 重排任务顺序.
func (s *Server) handleReorder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeProblem(w, http.StatusMethodNotAllowed, "只支持 POST")
		return
	}
	var payload struct {
		IDs []int64 `json:"ids"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		writeProblem(w, http.StatusBadRequest, "请求体格式不正确: "+err.Error())
		return
	}
	if err := s.opts.DB.ReorderTasks(payload.IDs); err != nil {
		writeProblem(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleWarnings 取走运行时告警.
func (s *Server) handleWarnings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeProblem(w, http.StatusMethodNotAllowed, "只支持 GET")
		return
	}
	warnings := s.opts.Scheduler.TakeWarnings()
	if warnings == nil {
		warnings = []scheduler.Warning{}
	}
	writeJSON(w, http.StatusOK, warnings)
}

// configView 是配置的对外视图.
type configView struct {
	QuietLaunch      bool   `json:"quiet_launch"`
	LogRetentionDays int    `json:"log_retention_days"`
	LocalBind        string `json:"local_bind"`
	Version          int    `json:"version"`
}

// handleConfig 读取或保存配置.
func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	cfg := s.opts.Config
	if cfg == nil {
		writeProblem(w, http.StatusInternalServerError, "配置未初始化")
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, configView{
			QuietLaunch:      cfg.QuietLaunch,
			LogRetentionDays: cfg.LogRetentionDays,
			LocalBind:        cfg.LocalBind,
			Version:          cfg.Version,
		})
	case http.MethodPut, http.MethodPost:
		var payload configView
		if err := decodeJSON(r, &payload); err != nil {
			writeProblem(w, http.StatusBadRequest, "请求体格式不正确: "+err.Error())
			return
		}
		next := config.Config{
			QuietLaunch:      payload.QuietLaunch,
			LogRetentionDays: payload.LogRetentionDays,
			LocalBind:        cfg.LocalBind,
		}
		if err := cfg.Update(next); err != nil {
			writeProblem(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, configView{
			QuietLaunch:      cfg.QuietLaunch,
			LogRetentionDays: cfg.LogRetentionDays,
			LocalBind:        cfg.LocalBind,
			Version:          cfg.Version,
		})
	default:
		writeProblem(w, http.StatusMethodNotAllowed, "只支持 GET 与 PUT")
	}
}

// handleExit 处理界面发起的退出.
func (s *Server) handleExit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeProblem(w, http.StatusMethodNotAllowed, "只支持 POST")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	if s.opts.OnExit != nil {
		// 先让响应写出去, 再触发退出.
		go s.opts.OnExit()
	}
}

// handleFocus 处理第二实例的聚焦请求.
func (s *Server) handleFocus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeProblem(w, http.StatusMethodNotAllowed, "只支持 POST")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	if s.opts.OnOpen != nil {
		go s.opts.OnOpen()
	}
}

// writeTaskError 把存储层错误映射成合适的 HTTP 状态.
func (s *Server) writeTaskError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrTaskNotFound) {
		writeProblem(w, http.StatusNotFound, "任务不存在")
		return
	}
	writeProblem(w, http.StatusInternalServerError, err.Error())
}
