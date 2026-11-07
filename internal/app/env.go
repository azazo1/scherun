package app

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/azazo1/scherun/internal/config"
	"github.com/azazo1/scherun/internal/envimport"
	"github.com/azazo1/scherun/internal/task"
)

// envState 缓存从登录 shell 导入的环境变量.
//
// 导入要启动一个交互式 shell, 有一定开销, 所以只在启动时读一次并缓存,
// 由用户在设置里手动刷新. 全局注入的变量直接读配置, 不需要缓存.
type envState struct {
	logger *slog.Logger

	mu      sync.RWMutex
	shell   map[string]string
	command []string
	lastErr error
	loaded  time.Time

	// config 返回当前配置, 用函数而不是值, 保证设置改完立刻生效.
	config func() config.Config
}

// newEnvState 创建环境状态, 并在后台完成首次导入.
//
// 首次导入放在后台, 避免交互式 shell 启动慢拖住应用启动;
// 任务在导入完成前启动时拿不到 shell 环境, 但不会因此失败.
func newEnvState(cfg *config.Config, logger *slog.Logger) *envState {
	state := &envState{
		logger: logger,
		shell:  map[string]string{},
		config: func() config.Config { return *cfg },
	}
	go state.Refresh()
	return state
}

// Layers 返回两层环境变量, 供调度器按任务的开关选择.
func (e *envState) Layers() (shellEnv, globalEnv map[string]string) {
	e.mu.RLock()
	shell := e.shell
	e.mu.RUnlock()

	cfg := e.config()
	global := cfg.EnvVars
	if global == nil {
		global = map[string]string{}
	}
	return shell, global
}

// ForTask 返回某个任务能拿到的环境变量.
//
// 按任务的 UseShellEnv 与 UseGlobalEnv 两个开关独立选择要哪一层,
// 供界面判定程序是否可执行时使用, 判定必须与调度器启动任务时一致.
func (e *envState) ForTask(t task.Task) map[string]string {
	shellEnv, globalEnv := e.Layers()
	return envimport.SelectLayers(shellEnv, globalEnv, t.UseShellEnv, t.UseGlobalEnv)
}

// Status 返回当前导入状态, 供设置界面展示.
func (e *envState) Status() envStatus {
	e.mu.RLock()
	defer e.mu.RUnlock()

	status := envStatus{
		Enabled:    e.config().EnvImportEnabled,
		Command:    envimport.CommandDisplay(e.command),
		VarCount:   len(e.shell),
		PathCount:  countPathEntries(e.shell["PATH"]),
		LastError:  "",
		LoadedAt:   nil,
		DefaultCmd: envimport.CommandDisplay(envimport.DefaultCommand()),
	}
	if e.lastErr != nil {
		status.LastError = e.lastErr.Error()
	}
	if !e.loaded.IsZero() {
		at := e.loaded
		status.LoadedAt = &at
	}
	return status
}

// Refresh 重新执行导入命令并更新缓存.
//
// 导入失败时保留上一次成功的结果, 只记录错误:
// 用户的 shell 配置可能暂时有问题, 不该因此让所有任务失去环境.
func (e *envState) Refresh() {
	cfg := e.config()

	e.mu.Lock()
	e.command = envimport.SplitCommand(cfg.EnvImportCommand)
	if len(e.command) == 0 {
		e.command = envimport.DefaultCommand()
	}
	command := e.command
	e.mu.Unlock()

	if !cfg.EnvImportEnabled {
		e.mu.Lock()
		e.shell = map[string]string{}
		e.lastErr = nil
		e.loaded = time.Now()
		e.mu.Unlock()
		e.logger.Info("环境变量导入已关闭")
		return
	}

	if len(command) == 0 {
		e.mu.Lock()
		e.lastErr = nil
		e.mu.Unlock()
		e.logger.Warn("当前平台没有可用的环境变量导入命令")
		return
	}

	e.logger.Info("开始导入登录 shell 环境变量", "command", envimport.CommandDisplay(command))
	started := time.Now()
	vars, err := envimport.Import(context.Background(), command, 0)
	elapsed := time.Since(started)

	e.mu.Lock()
	defer e.mu.Unlock()
	if err != nil {
		e.lastErr = err
		e.logger.Warn("导入环境变量失败, 保留上一次结果",
			"error", err, "elapsed", elapsed.Round(time.Millisecond))
		return
	}
	e.shell = vars
	e.lastErr = nil
	e.loaded = time.Now()
	e.logger.Info("已导入登录 shell 环境变量",
		"count", len(vars),
		"path_entries", countPathEntries(vars["PATH"]),
		"elapsed", elapsed.Round(time.Millisecond))
}

// envStatus 是环境变量导入的展示状态.
type envStatus struct {
	// Enabled 表示是否开启导入.
	Enabled bool `json:"enabled"`
	// Command 是实际执行的命令.
	Command string `json:"command"`
	// DefaultCmd 是内置的默认命令, 供界面提示.
	DefaultCmd string `json:"default_command"`
	// VarCount 是已导入的变量个数.
	VarCount int `json:"var_count"`
	// PathCount 是导入的 PATH 条目数, 用来判断是否真的拿到了完整路径.
	PathCount int `json:"path_count"`
	// LastError 是最近一次导入的错误, 成功时为空.
	LastError string `json:"last_error,omitempty"`
	// LoadedAt 是最近一次成功导入的时间.
	LoadedAt *time.Time `json:"loaded_at,omitempty"`
}

// countPathEntries 数 PATH 里的条目个数.
func countPathEntries(path string) int {
	if path == "" {
		return 0
	}
	count := 1
	for _, r := range path {
		if r == ':' {
			count++
		}
	}
	return count
}
