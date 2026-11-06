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
export type TaskStatus = 'idle' | 'running' | 'suspended' | 'disabled'

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

/** 应用配置. */
export interface AppConfig {
  quiet_launch: boolean
  log_retention_days: number
  local_bind: string
  version: number
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
}
