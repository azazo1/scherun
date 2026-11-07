// Package task 定义后台任务的数据模型与触发方式.
package task

import (
	"encoding/json"
	"fmt"
	"time"
)

// TriggerKind 是触发方式.
type TriggerKind string

// 支持的触发方式.
const (
	// TriggerManual 只手动运行.
	TriggerManual TriggerKind = "Manual"
	// TriggerStartup 应用启动时运行一次.
	TriggerStartup TriggerKind = "Startup"
	// TriggerKeepAlive 进程退出后立即重启, 随应用启动.
	TriggerKeepAlive TriggerKind = "KeepAlive"
	// TriggerRoutine 按固定间隔重复运行.
	TriggerRoutine TriggerKind = "Routine"
	// TriggerInstant 在指定时间点运行一次.
	TriggerInstant TriggerKind = "Instant"
	// TriggerUntilSucceed 反复运行直到退出码为 0.
	TriggerUntilSucceed TriggerKind = "UntilSucceed"
)

// AllTriggerKinds 按展示顺序列出全部触发方式.
var AllTriggerKinds = []TriggerKind{
	TriggerManual,
	TriggerStartup,
	TriggerKeepAlive,
	TriggerRoutine,
	TriggerInstant,
	TriggerUntilSucceed,
}

// Valid 判断触发方式是否为已知值.
func (k TriggerKind) Valid() bool {
	for _, known := range AllTriggerKinds {
		if k == known {
			return true
		}
	}
	return false
}

// Trigger 描述触发方式及其参数.
type Trigger struct {
	Kind TriggerKind `json:"kind"`

	// IntervalSeconds 仅 Routine 使用, 表示两次运行之间的间隔秒数.
	IntervalSeconds int `json:"interval_seconds,omitempty"`

	// RunAt 仅 Instant 使用, 表示计划执行的时间点.
	RunAt *time.Time `json:"run_at,omitempty"`
}

// Validate 校验触发参数.
func (t Trigger) Validate() error {
	if !t.Kind.Valid() {
		return fmt.Errorf("未知的触发方式: %q", t.Kind)
	}
	switch t.Kind {
	case TriggerRoutine:
		if t.IntervalSeconds < 1 {
			return fmt.Errorf("定时重复的间隔必须大于等于 1 秒")
		}
	case TriggerInstant:
		if t.RunAt == nil {
			return fmt.Errorf("定时一次必须指定执行时间")
		}
	}
	return nil
}

// Describe 返回触发方式的中文简述, 供日志与界面复用.
func (t Trigger) Describe() string {
	switch t.Kind {
	case TriggerRoutine:
		return fmt.Sprintf("每 %d 秒", t.IntervalSeconds)
	case TriggerInstant:
		if t.RunAt == nil {
			return "定时一次"
		}
		return "于 " + t.RunAt.Local().Format("2006-01-02 15:04") + " 运行一次"
	case TriggerStartup:
		return "启动时运行一次"
	case TriggerKeepAlive:
		return "保持运行"
	case TriggerUntilSucceed:
		return "重复运行直到成功"
	default:
		return "手动运行"
	}
}

// Task 是一个后台任务定义.
type Task struct {
	ID        int64    `json:"id"`
	SortOrder int64    `json:"sort_order"`
	Name      string   `json:"name"`
	Program   string   `json:"program"`
	Args      []string `json:"args"`

	Stdin  string `json:"stdin,omitempty"`
	Stdout string `json:"stdout,omitempty"`
	Stderr string `json:"stderr,omitempty"`

	WorkingDir string            `json:"working_dir,omitempty"`
	EnvVars    map[string]string `json:"env_vars,omitempty"`

	// UseShellEnv 为真时任务继承从登录 shell 导入的环境变量.
	//
	// 默认开启: 从 Finder 启动的应用 PATH 只有系统默认值, 不继承的话
	// homebrew 与用户目录下的命令都找不到.
	UseShellEnv bool `json:"use_shell_env"`

	// UseGlobalEnv 为真时任务继承设置里手工填写的全局环境变量.
	//
	// 与 UseShellEnv 分开控制: 想用全局注入但不要 shell 那套环境时,
	// 可以只关掉前者. 任务自己的 EnvVars 始终生效, 且优先级最高.
	UseGlobalEnv bool `json:"use_global_env"`

	Trigger Trigger `json:"trigger"`
	Enabled bool    `json:"enabled"`

	// NoConsole 仅 Windows 有效, 为真时隐藏控制台窗口.
	NoConsole bool `json:"no_console"`

	LastExitCode *int       `json:"last_exit_code,omitempty"`
	LastRunAt    *time.Time `json:"last_run_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// Validate 校验任务定义.
func (t Task) Validate() error {
	if t.Name == "" {
		return fmt.Errorf("任务名称不能为空")
	}
	if t.Program == "" {
		return fmt.Errorf("程序路径不能为空")
	}
	if err := t.Trigger.Validate(); err != nil {
		return err
	}
	if t.Trigger.Kind == TriggerInstant && t.Trigger.RunAt == nil {
		return fmt.Errorf("定时一次必须指定执行时间")
	}
	return nil
}

// EncodeArgs 把参数列表序列化成 JSON 字符串.
func EncodeArgs(args []string) string {
	if len(args) == 0 {
		return "[]"
	}
	buf, err := json.Marshal(args)
	if err != nil {
		return "[]"
	}
	return string(buf)
}

// DecodeArgs 解析参数 JSON, 解析失败时返回空列表.
func DecodeArgs(raw string) []string {
	if raw == "" {
		return nil
	}
	var args []string
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return nil
	}
	return args
}

// EncodeEnv 把环境变量序列化成 JSON 字符串.
func EncodeEnv(env map[string]string) string {
	if len(env) == 0 {
		return "{}"
	}
	buf, err := json.Marshal(env)
	if err != nil {
		return "{}"
	}
	return string(buf)
}

// DecodeEnv 解析环境变量 JSON, 解析失败时返回空 map.
func DecodeEnv(raw string) map[string]string {
	if raw == "" {
		return nil
	}
	var env map[string]string
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		return nil
	}
	return env
}
