import { forwardRef } from 'react'
import type { InputHTMLAttributes, TextareaHTMLAttributes } from 'react'
import { cn } from '@/lib/utils'

const baseField =
  'w-full min-w-0 rounded-lg border border-[var(--border-subtle)] bg-[var(--surface-sunken)] ' +
  'px-3 py-2 text-sm text-[var(--content)] placeholder:text-[var(--content-faint)] ' +
  'transition-colors hover:border-[var(--border-strong)] ' +
  'focus:border-[var(--accent)] focus:outline-none disabled:opacity-50'

/** 单行输入框. */
export const Input = forwardRef<HTMLInputElement, InputHTMLAttributes<HTMLInputElement>>(
  ({ className, ...props }, ref) => (
    <input ref={ref} className={cn(baseField, 'h-9', className)} {...props} />
  ),
)
Input.displayName = 'Input'

/** 多行输入框. */
export const Textarea = forwardRef<
  HTMLTextAreaElement,
  TextareaHTMLAttributes<HTMLTextAreaElement>
>(({ className, ...props }, ref) => (
  <textarea ref={ref} className={cn(baseField, 'resize-y', className)} {...props} />
))
Textarea.displayName = 'Textarea'

/** 表单字段容器. */
export function Field({
  label,
  hint,
  children,
  className,
}: {
  label: string
  hint?: string
  children: React.ReactNode
  className?: string
}) {
  return (
    <label className={cn('block space-y-1.5', className)}>
      <span className="text-[13px] font-medium text-[var(--content-muted)]">
        {label}
      </span>
      {children}
      {hint ? (
        <span className="block text-xs text-[var(--content-faint)]">{hint}</span>
      ) : null}
    </label>
  )
}
