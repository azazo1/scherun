import { clsx, type ClassValue } from 'clsx'
import { twMerge } from 'tailwind-merge'
import type { Task, Trigger, TriggerKind } from '@/api/types'

/** 合并 Tailwind 类名. */
export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

/** 触发方式的中文名. */
export const triggerLabels: Record<TriggerKind, string> = {
  Manual: '手动',
  Startup: '启动时',
  KeepAlive: '保持运行',
  Routine: '定时重复',
  Instant: '定时一次',
  UntilSucceed: '直到成功',
}

/** 触发方式的说明文案. */
export const triggerDescriptions: Record<TriggerKind, string> = {
  Manual: '只在点击运行时执行',
  Startup: '应用启动时运行一次',
  KeepAlive: '进程退出后自动重启',
  Routine: '按固定间隔重复执行',
  Instant: '在指定时刻运行一次',
  UntilSucceed: '重复执行直到退出码为 0',
}

/** 生成触发方式的一行描述. */
export function describeTrigger(trigger: Trigger): string {
  switch (trigger.kind) {
    case 'Routine':
      return `每 ${trigger.interval_seconds ?? 0} 秒`
    case 'Instant':
      return trigger.run_at
        ? `于 ${formatDateTime(trigger.run_at)} 运行一次`
        : '定时一次'
    default:
      return triggerDescriptions[trigger.kind]
  }
}

/** 把 ISO 时间格式化成易读形式. */
export function formatDateTime(iso?: string): string {
  if (!iso) return '-'
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return '-'
  const now = new Date()
  const sameDay = date.toDateString() === now.toDateString()
  const time = date.toLocaleTimeString('zh-CN', {
    hour: '2-digit',
    minute: '2-digit',
  })
  if (sameDay) return `今天 ${time}`
  const sameYear = date.getFullYear() === now.getFullYear()
  const day = date.toLocaleDateString('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    ...(sameYear ? {} : { year: 'numeric' }),
  })
  return `${day} ${time}`
}

/** 相对时间描述, 例如 "3 分钟前". */
export function relativeTime(iso?: string): string {
  if (!iso) return '从未运行'
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return '从未运行'
  const seconds = Math.floor((Date.now() - date.getTime()) / 1000)
  if (seconds < 0) return '刚刚'
  if (seconds < 60) return `${seconds} 秒前`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes} 分钟前`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours} 小时前`
  const days = Math.floor(hours / 24)
  if (days < 30) return `${days} 天前`
  return formatDateTime(iso)
}

/** 计算运行时长. */
export function runningFor(startedAt?: string): string {
  if (!startedAt) return ''
  const start = new Date(startedAt).getTime()
  if (Number.isNaN(start)) return ''
  const seconds = Math.max(0, Math.floor((Date.now() - start) / 1000))
  if (seconds < 60) return `${seconds} 秒`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes} 分 ${seconds % 60} 秒`
  const hours = Math.floor(minutes / 60)
  return `${hours} 小时 ${minutes % 60} 分`
}

/** 取出路径的最后一段, 作为程序显示名. */
export function baseName(path?: string): string {
  if (!path) return ''
  const normalized = path.replace(/[\\/]+$/, '')
  const parts = normalized.split(/[\\/]/)
  return parts[parts.length - 1] ?? normalized
}

/** 把环境变量对象转成可编辑的列表. */
export function envToList(
  env?: Record<string, string>,
): Array<{ key: string; value: string }> {
  if (!env) return []
  return Object.entries(env).map(([key, value]) => ({ key, value }))
}

/** 把可编辑列表转回环境变量对象, 忽略空键. */
export function listToEnv(
  list: Array<{ key: string; value: string }>,
): Record<string, string> {
  const result: Record<string, string> = {}
  for (const { key, value } of list) {
    const trimmed = key.trim()
    if (trimmed) {
      result[trimmed] = value
    }
  }
  return result
}

/** 判断任务当前是否正在运行. */
export function isRunning(task: Task): boolean {
  return task.status === 'running'
}

/** 把 datetime-local 输入值转成 ISO 字符串. */
export function localInputToISO(value: string): string | undefined {
  if (!value) return undefined
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return undefined
  return date.toISOString()
}

/** 把 ISO 时间转成 datetime-local 输入值. */
export function isoToLocalInput(iso?: string): string {
  if (!iso) return ''
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return ''
  const offset = date.getTimezoneOffset() * 60000
  return new Date(date.getTime() - offset).toISOString().slice(0, 16)
}
