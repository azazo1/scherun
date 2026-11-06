import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { AlertTriangle, CheckCircle2, Info, X, XCircle } from 'lucide-react'
import { cn } from '@/lib/utils'

/** 提示类型. */
type ToastKind = 'success' | 'error' | 'warning' | 'info'

interface ToastItem {
  id: number
  kind: ToastKind
  title: string
  description?: string
}

interface ToastContextValue {
  push: (kind: ToastKind, title: string, description?: string) => void
  success: (title: string, description?: string) => void
  error: (title: string, description?: string) => void
  warning: (title: string, description?: string) => void
}

const ToastContext = createContext<ToastContextValue | null>(null)

const kindStyles: Record<ToastKind, { icon: ReactNode; accent: string }> = {
  success: {
    icon: <CheckCircle2 className="h-4 w-4" />,
    accent: 'var(--success)',
  },
  error: {
    icon: <XCircle className="h-4 w-4" />,
    accent: 'var(--danger)',
  },
  warning: {
    icon: <AlertTriangle className="h-4 w-4" />,
    accent: 'var(--warning)',
  },
  info: {
    icon: <Info className="h-4 w-4" />,
    accent: 'var(--accent)',
  },
}

/** 全局提示容器. */
export function ToastProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<ToastItem[]>([])

  const remove = useCallback((id: number) => {
    setItems((current) => current.filter((item) => item.id !== id))
  }, [])

  const push = useCallback(
    (kind: ToastKind, title: string, description?: string) => {
      const id = Date.now() + Math.random()
      setItems((current) => [...current.slice(-4), { id, kind, title, description }])
    },
    [],
  )

  const value = useMemo<ToastContextValue>(
    () => ({
      push,
      success: (title, description) => push('success', title, description),
      error: (title, description) => push('error', title, description),
      warning: (title, description) => push('warning', title, description),
    }),
    [push],
  )

  return (
    <ToastContext.Provider value={value}>
      {children}
      <div className="pointer-events-none fixed bottom-5 right-5 z-[60] flex w-full max-w-sm flex-col gap-2">
        {items.map((item) => (
          <ToastCard key={item.id} item={item} onClose={() => remove(item.id)} />
        ))}
      </div>
    </ToastContext.Provider>
  )
}

/** 单条提示, 自动消隐. */
function ToastCard({ item, onClose }: { item: ToastItem; onClose: () => void }) {
  const style = kindStyles[item.kind]

  useEffect(() => {
    // 错误提示停留久一些, 方便读完.
    const timeout = window.setTimeout(onClose, item.kind === 'error' ? 8000 : 4500)
    return () => window.clearTimeout(timeout)
  }, [item.id, item.kind, onClose])

  return (
    <div
      role="status"
      className={cn(
        'pointer-events-auto flex items-start gap-3 rounded-lg border p-3',
        'border-[var(--border-subtle)] bg-[var(--surface-raised)] shadow-lg',
      )}
      style={{ borderLeft: `3px solid ${style.accent}` }}
    >
      <span className="mt-0.5 shrink-0" style={{ color: style.accent }}>
        {style.icon}
      </span>
      <div className="min-w-0 flex-1">
        <p className="text-[13px] font-medium text-[var(--content)]">{item.title}</p>
        {item.description ? (
          <p className="mt-0.5 break-words text-xs text-[var(--content-muted)]">
            {item.description}
          </p>
        ) : null}
      </div>
      <button
        type="button"
        onClick={onClose}
        aria-label="关闭提示"
        className="shrink-0 cursor-pointer rounded p-0.5 text-[var(--content-faint)] hover:text-[var(--content)]"
      >
        <X className="h-3.5 w-3.5" />
      </button>
    </div>
  )
}

/** 读取提示上下文. */
export function useToast(): ToastContextValue {
  const context = useContext(ToastContext)
  if (!context) {
    throw new Error('useToast 必须在 ToastProvider 内使用')
  }
  return context
}
