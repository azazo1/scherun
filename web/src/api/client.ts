/** 与后端通信的客户端. */

import type {
  AppConfig,
  EventsPayload,
  FsListing,
  Health,
  Task,
  Warning,
} from './types'

/** 接口错误, 携带后端返回的可读信息. */
export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

/** 统一请求封装. */
async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers)
  if (init?.body !== undefined && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }
  const response = await fetch(path, {
    ...init,
    headers,
    // 同源 cookie 会自动带上, 显式声明便于排查.
    credentials: 'same-origin',
  })

  if (!response.ok) {
    let message = `请求失败 (${response.status})`
    try {
      const body = (await response.json()) as { error?: string }
      if (body.error) {
        message = body.error
      }
    } catch {
      // 非 JSON 响应, 保留默认信息.
    }
    throw new ApiError(message, response.status)
  }

  if (response.status === 204) {
    return undefined as T
  }
  const text = await response.text()
  if (!text) {
    return undefined as T
  }
  return JSON.parse(text) as T
}

/** 任务相关接口. */
export const taskApi = {
  list: () => request<Task[]>('/api/tasks'),

  save: (task: Partial<Task>) =>
    request<Task>('/api/tasks', {
      method: 'POST',
      body: JSON.stringify(task),
    }),

  remove: (id: number) =>
    request<{ ok: boolean }>(`/api/tasks/${id}`, { method: 'DELETE' }),

  run: (id: number) =>
    request<{ ok: boolean }>(`/api/tasks/${id}/run`, { method: 'POST' }),

  stop: (id: number) =>
    request<{ ok: boolean }>(`/api/tasks/${id}/stop`, { method: 'POST' }),

  setEnabled: (id: number, enabled: boolean) =>
    request<{ ok: boolean }>(`/api/tasks/${id}/enabled`, {
      method: 'POST',
      body: JSON.stringify({ enabled }),
    }),

  reorder: (ids: number[]) =>
    request<{ ok: boolean }>('/api/tasks/reorder', {
      method: 'POST',
      body: JSON.stringify({ ids }),
    }),
}

/** 应用级接口. */
export const appApi = {
  health: () => request<Health>('/api/health'),

  warnings: () => request<Warning[]>('/api/warnings'),

  getConfig: () => request<AppConfig>('/api/config'),

  saveConfig: (config: Partial<AppConfig>) =>
    request<AppConfig>('/api/config', {
      method: 'PUT',
      body: JSON.stringify(config),
    }),

  listDir: (path?: string) =>
    request<FsListing>(
      path ? `/api/fs/list?path=${encodeURIComponent(path)}` : '/api/fs/list',
    ),

  exit: () => request<{ ok: boolean }>('/api/app/exit', { method: 'POST' }),
}

/**
 * 订阅任务状态推送.
 *
 * 用 SSE 而不是轮询: 一个连接覆盖全部任务, 断线时自动重连.
 */
export function subscribeEvents(
  onEvent: (payload: EventsPayload) => void,
  onStateChange?: (connected: boolean) => void,
): () => void {
  let source: EventSource | null = null
  let closed = false

  const connect = () => {
    if (closed) return
    source = new EventSource('/api/events')

    source.onopen = () => onStateChange?.(true)

    source.onmessage = (event) => {
      try {
        onEvent(JSON.parse(event.data) as EventsPayload)
      } catch {
        // 忽略无法解析的消息.
      }
    }

    source.onerror = () => {
      onStateChange?.(false)
      // EventSource 自带重连, 但服务重启后 cookie 可能失效, 因此彻底重建.
      source?.close()
      source = null
      if (!closed) {
        window.setTimeout(connect, 2000)
      }
    }
  }

  connect()

  return () => {
    closed = true
    source?.close()
    source = null
  }
}
