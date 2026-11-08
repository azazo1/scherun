/** 与后端 task 包保持一致的类型定义. */

/** 触发方式. */
export type TriggerKind =
  | 'Manual'
  | 'Startup'
  | 'KeepAlive'
  | 'Routine'
  | 'Instant'
  | 'UntilSucceed'

/** 触发方式及其参数. */
export interface Trigger {
  kind: TriggerKind
  interval_seconds?: number
  run_at?: string
}

/** 任务运行状态. */
export type TaskStatus = 'idle' | 'running' | 'suspended' | 'paused' | 'disabled'

/** 任务定义与运行状态的合并视图. */
export interface Task {
  id: number
  sort_order: number
  name: string
  program: string
  args: string[]
  stdin?: string
  stdout?: string
  stderr?: string
  working_dir?: string
  env_vars?: Record<string, string>
  /** 是否继承从登录 shell 导入的环境变量. */
  use_shell_env: boolean
  /** 是否继承设置里填写的全局环境变量. */
  use_global_env: boolean
  trigger: Trigger
  enabled: boolean
  no_console: boolean
  last_exit_code?: number
  last_run_at?: string
  created_at: string
  updated_at: string

  // 运行状态
  status: TaskStatus
  pid?: number
  started_at?: string
  restarts: number
  failures: number
  runnable: boolean
}

/** 运行时告警. */
export interface Warning {
  task_id?: number
  task_name?: string
  message: string
  created_at: string
}

/** SSE 推送的事件. */
export interface EventsPayload {
  tasks?: Task[]
  warnings?: Warning[]
}

/** 命令解析结果. */
export interface ParsedCommand {
  program: string
  args: string[]
  working_dir?: string
  env_vars?: Record<string, string>
  stdin?: string
  stdout?: string
  stderr?: string
  warnings?: string[]
  resolved: boolean
}

/** 应用配置. */
export interface AppConfig {
  quiet_launch: boolean
  log_retention_days: number
  local_bind: string
  /** 是否在启动时从登录 shell 导入环境变量. */
  env_import_enabled: boolean
  /** 导入命令, 留空时按 $SHELL 自动推断. */
  env_import_command: string
  /** 全局注入的环境变量, 任务可选择是否继承. */
  env_vars: Record<string, string>
  version: number
}

/** 登录 shell 环境变量的导入状态. */
export interface EnvStatus {
  enabled: boolean
  command: string
  default_command: string
  var_count: number
  path_count: number
  last_error?: string
  loaded_at?: string
}

/** 健康检查响应. */
export interface Health {
  version: string
  commit: string
  started_at: string
  url: string
  task_count: number
}

/** 目录浏览条目. */
export interface FsEntry {
  name: string
  path: string
  is_dir: boolean
  executable: boolean
  selectable: boolean
  size?: number
}

/** 目录浏览响应. */
export interface FsListing {
  path: string
  parent?: string
  entries: FsEntry[]
  home: string
  roots: string[]
  show_hidden: boolean
  selected?: string
}
