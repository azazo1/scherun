import { forwardRef } from 'react'
import type { ButtonHTMLAttributes } from 'react'
import { cn } from '@/lib/utils'

/** 按钮变体. */
type Variant = 'primary' | 'secondary' | 'ghost' | 'danger' | 'outline'
/** 按钮尺寸. */
type Size = 'sm' | 'md' | 'lg' | 'icon'

const variantClasses: Record<Variant, string> = {
  primary:
    'bg-[var(--accent)] text-white hover:brightness-110 active:brightness-95 shadow-sm',
  secondary:
    'bg-[var(--surface-sunken)] text-[var(--content)] hover:bg-[var(--border-subtle)]',
  ghost:
    'bg-transparent text-[var(--content-muted)] hover:bg-[var(--surface-sunken)] hover:text-[var(--content)]',
  danger:
    'bg-transparent text-[var(--danger)] hover:bg-[color-mix(in_oklab,var(--danger)_14%,transparent)]',
  outline:
    'border border-[var(--border-subtle)] bg-transparent text-[var(--content-muted)] hover:border-[var(--border-strong)] hover:text-[var(--content)]',
}

const sizeClasses: Record<Size, string> = {
  sm: 'h-8 px-3 text-[13px] gap-1.5',
  md: 'h-9 px-4 text-sm gap-2',
  lg: 'h-10 px-5 text-sm gap-2',
  // 图标按钮: 方形但整个容器都可点击, 不是只有图标本身.
  icon: 'h-9 w-9 justify-center',
}

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: Variant
  size?: Size
}

/** 通用按钮. */
export const Button = forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant = 'secondary', size = 'md', type = 'button', ...props }, ref) => (
    <button
      ref={ref}
      type={type}
      className={cn(
        'inline-flex cursor-pointer items-center rounded-lg font-medium transition-colors',
        'disabled:pointer-events-none disabled:opacity-45',
        variantClasses[variant],
        sizeClasses[size],
        className,
      )}
      {...props}
    />
  ),
)
Button.displayName = 'Button'
