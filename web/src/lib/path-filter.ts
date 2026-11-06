/** 路径输入框的过滤规则, 与 FilePicker 共用. */

/** 去掉末尾分隔符, 便于比较两个路径是否指向同一目录. */
export function normalizePath(path: string): string {
  const trimmed = path.trim()
  if (trimmed.length <= 1) return trimmed
  return trimmed.replace(/[\\/]+$/, '')
}

/** 过滤结果. */
export interface PathFilter {
  /** 用于前缀过滤的文本, 空串表示不过滤. */
  filter: string
  /** 最后一段是否以点开头, 表示用户正在找隐藏条目. */
  isHidden: boolean
}

/**
 * deriveFilter 由输入框内容与当前目录推导过滤条件.
 *
 * 输入框内容与当前浏览目录一致时不过滤: 每次加载成功都会把输入框同步成
 * 当前目录, 若照常取最后一段, 会把列表过滤成只剩与目录同名的那一项.
 */
export function deriveFilter(input: string, currentDir?: string): PathFilter {
  const raw = input.trim()
  if (!raw) return { filter: '', isHidden: false }

  if (currentDir && normalizePath(raw) === normalizePath(currentDir)) {
    return { filter: '', isHidden: false }
  }

  const segments = raw.split(/[\\/]/)
  const last = segments[segments.length - 1] ?? ''
  return { filter: last, isHidden: last.startsWith('.') }
}
