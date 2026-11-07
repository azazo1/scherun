package localapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/azazo1/scherun/internal/config"
	"github.com/azazo1/scherun/internal/envimport"
	"github.com/azazo1/scherun/internal/scheduler"
	"github.com/azazo1/scherun/internal/store"
	"github.com/azazo1/scherun/internal/task"
)

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
	writeJSON(w, http.StatusOK, s.buildTaskViews(tasks))
}

// taskPayload 是任务保存请求体.
//
// 内嵌 task.Task 后又声明了两个指针字段, 用来区分"没传"和"显式传了 false":
// 直接解码到 bool 时两者都是 false, 会让新建任务默认丢掉环境继承.
// JSON 解码时外层同名字段优先, 因此这两个指针能正确覆盖内嵌结构里的值.
type taskPayload struct {
	task.Task
	UseShellEnv  *bool `json:"use_shell_env,omitempty"`
	UseGlobalEnv *bool `json:"use_global_env,omitempty"`
}

// toTask 把请求体还原成任务定义, 未提供的继承开关按默认开启处理.
func (p taskPayload) toTask() task.Task {
	t := p.Task
	if p.UseShellEnv != nil {
		t.UseShellEnv = *p.UseShellEnv
	} else {
		t.UseShellEnv = true
	}
	if p.UseGlobalEnv != nil {
		t.UseGlobalEnv = *p.UseGlobalEnv
	} else {
		t.UseGlobalEnv = true
	}
	return t
}

// saveTask 新建或更新任务.
func (s *Server) saveTask(w http.ResponseWriter, r *http.Request) {
	var payload taskPayload
	if err := decodeJSON(r, &payload); err != nil {
		writeProblem(w, http.StatusBadRequest, "请求体格式不正确: "+err.Error())
		return
	}
	id, err := s.opts.Scheduler.SaveTask(payload.toTask())
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
	QuietLaunch      bool              `json:"quiet_launch"`
	LogRetentionDays int               `json:"log_retention_days"`
	LocalBind        string            `json:"local_bind"`
	EnvImportEnabled bool              `json:"env_import_enabled"`
	EnvImportCommand string            `json:"env_import_command"`
	EnvVars          map[string]string `json:"env_vars"`
	Version          int               `json:"version"`
}

// buildConfigView 把配置转成对外视图.
func buildConfigView(cfg *config.Config) configView {
	vars := cfg.EnvVars
	if vars == nil {
		vars = map[string]string{}
	}
	return configView{
		QuietLaunch:      cfg.QuietLaunch,
		LogRetentionDays: cfg.LogRetentionDays,
		LocalBind:        cfg.LocalBind,
		EnvImportEnabled: cfg.EnvImportEnabled,
		EnvImportCommand: cfg.EnvImportCommand,
		EnvVars:          vars,
		Version:          cfg.Version,
	}
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
		writeJSON(w, http.StatusOK, buildConfigView(cfg))
	case http.MethodPut, http.MethodPost:
		var payload configView
		if err := decodeJSON(r, &payload); err != nil {
			writeProblem(w, http.StatusBadRequest, "请求体格式不正确: "+err.Error())
			return
		}
		// 导入命令先做存在性检查, 让用户在保存时就得到反馈, 而不是等下次启动才发现填错.
		if payload.EnvImportEnabled {
			if err := envimport.ValidateCommand(envimport.SplitCommand(payload.EnvImportCommand)); err != nil {
				writeProblem(w, http.StatusBadRequest, err.Error())
				return
			}
		}
		next := config.Config{
			QuietLaunch:      payload.QuietLaunch,
			LogRetentionDays: payload.LogRetentionDays,
			LocalBind:        cfg.LocalBind,
			EnvImportEnabled: payload.EnvImportEnabled,
			EnvImportCommand: payload.EnvImportCommand,
			EnvVars:          payload.EnvVars,
		}
		if err := cfg.Update(next); err != nil {
			writeProblem(w, http.StatusBadRequest, err.Error())
			return
		}
		// 导入相关的设置变了就立刻重新导入, 不用等重启.
		if s.opts.ReloadEnv != nil {
			go s.opts.ReloadEnv()
		}
		writeJSON(w, http.StatusOK, buildConfigView(cfg))
	default:
		writeProblem(w, http.StatusMethodNotAllowed, "只支持 GET 与 PUT")
	}
}

// handleEnvStatus 返回登录 shell 环境变量的导入状态.
func (s *Server) handleEnvStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeProblem(w, http.StatusMethodNotAllowed, "只支持 GET")
		return
	}
	if s.opts.EnvStatus == nil {
		writeJSON(w, http.StatusOK, map[string]any{})
		return
	}
	writeJSON(w, http.StatusOK, s.opts.EnvStatus())
}

// handleEnvRefresh 重新执行导入命令.
func (s *Server) handleEnvRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeProblem(w, http.StatusMethodNotAllowed, "只支持 POST")
		return
	}
	if s.opts.ReloadEnv != nil {
		s.opts.ReloadEnv()
	}
	if s.opts.EnvStatus == nil {
		writeJSON(w, http.StatusOK, map[string]any{})
		return
	}
	writeJSON(w, http.StatusOK, s.opts.EnvStatus())
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
