import { useSortable } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import {
  AlertTriangle,
  CircleSlash,
  GripVertical,
  Pause,
  Pencil,
  Play,
  Square,
  Trash2,
} from 'lucide-react'
import type { Task } from '@/api/types'
import { cn, baseName, describeTrigger, isRunning, relativeTime, runningFor } from '@/lib/utils'
import { Button } from './ui/Button'
import { Switch } from './ui/Switch'

interface TaskCardProps {
  task: Task
  onEdit: (task: Task) => void
  onDelete: (task: Task) => void
  onRun: (task: Task) => void
  onStop: (task: Task) => void
  onToggle: (task: Task, enabled: boolean) => void
  busy?: boolean
}

/** 状态圆点. */
function StatusDot({ task }: { task: Task }) {
  const running = isRunning(task)
  const suspended = task.status === 'suspended'
  const paused = task.status === 'paused'
  const disabled = !task.enabled

  const color = suspended
    ? 'var(--warning)'
    : disabled
      ? 'var(--content-faint)'
      : paused
        ? 'var(--content-muted)'
        : running
          ? 'var(--success)'
          : 'var(--content-faint)'

  return (
    <span
      className={cn('h-2.5 w-2.5 shrink-0 rounded-full', running && 'status-pulse')}
      style={{ backgroundColor: color }}
      aria-hidden
    />
  )
}

/** 状态标签. */
function StatusBadge({ task }: { task: Task }) {
  if (task.status === 'suspended') {
    return (
      <span className="inline-flex items-center gap-1 rounded-md bg-[color-mix(in_oklab,var(--warning)_16%,transparent)] px-1.5 py-0.5 text-[11px] font-medium text-[var(--warning)]">
        <AlertTriangle className="h-3 w-3" />
        已挂起
      </span>
    )
  }
  if (isRunning(task)) {
    return (
      <span className="inline-flex items-center gap-1 rounded-md bg-[color-mix(in_oklab,var(--success)_16%,transparent)] px-1.5 py-0.5 text-[11px] font-medium text-[var(--success)]">
        运行中 {runningFor(task.started_at)}
      </span>
    )
  }
  if (!task.enabled) {
    return (
      <span className="inline-flex items-center gap-1 rounded-md bg-[var(--surface-sunken)] px-1.5 py-0.5 text-[11px] font-medium text-[var(--content-faint)]">
        <CircleSlash className="h-3 w-3" />
        已停用
      </span>
    )
  }
  if (task.status === 'paused') {
    return (
      <span className="inline-flex items-center gap-1 rounded-md bg-[var(--surface-sunken)] px-1.5 py-0.5 text-[11px] font-medium text-[var(--content-muted)]">
        <Pause className="h-3 w-3" />
        已暂停
      </span>
    )
  }
  return null
}

/** 单个任务卡片. */
export function TaskCard({
  task,
  onEdit,
  onDelete,
  onRun,
  onStop,
  onToggle,
  busy,
}: TaskCardProps) {
  const { attributes, listeners, setNodeRef, setActivatorNodeRef, transform, transition, isDragging } =
    useSortable({ id: task.id })

  const running = isRunning(task)
  const disabled = !task.enabled

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
  }

  return (
    <div
      ref={setNodeRef}
      style={style}
      className={cn(
        'group relative flex items-center gap-3 rounded-xl border px-3 py-3 transition-colors',
        'border-[var(--border-subtle)] bg-[var(--surface-raised)]',
        'hover:border-[var(--border-strong)]',
        isDragging && 'z-10 opacity-90 shadow-xl',
        disabled && 'opacity-60',
      )}
    >
      {/* 拖动把手: 只有这里能拖, 避免和按钮点击冲突. */}
      <button
        ref={setActivatorNodeRef}
        type="button"
        aria-label="拖动排序"
        className={cn(
          'shrink-0 cursor-grab rounded p-1 text-[var(--content-faint)]',
          'opacity-0 transition-opacity hover:bg-[var(--surface-sunken)] hover:text-[var(--content)]',
          'group-hover:opacity-100 focus-visible:opacity-100 active:cursor-grabbing',
        )}
        {...attributes}
        {...listeners}
      >
        <GripVertical className="h-4 w-4" />
      </button>

      <StatusDot task={task} />

      {/* 主体信息. */}
      <button
        type="button"
        onClick={() => onEdit(task)}
        className="min-w-0 flex-1 cursor-pointer text-left"
      >
        <div className="flex flex-wrap items-center gap-2">
          <span className="truncate text-sm font-medium text-[var(--content)]">
            {task.name}
          </span>
          <StatusBadge task={task} />
        </div>
        <div className="mt-0.5 flex flex-wrap items-center gap-x-2 gap-y-0.5 text-xs text-[var(--content-muted)]">
          <span>{describeTrigger(task.trigger)}</span>
          <span className="text-[var(--content-faint)]">·</span>
          <span className="truncate font-mono text-[11px] text-[var(--content-faint)]">
            {baseName(task.program) || task.program}
          </span>
        </div>
        <div className="mt-1 flex flex-wrap items-center gap-x-2 gap-y-0.5 text-[11px] text-[var(--content-faint)]">
          <span>上次运行 {relativeTime(task.last_run_at)}</span>
          {task.last_exit_code !== undefined ? (
            <span
              className={cn(
                'rounded px-1 py-px font-mono',
                task.last_exit_code === 0
                  ? 'bg-[color-mix(in_oklab,var(--success)_14%,transparent)] text-[var(--success)]'
                  : 'bg-[color-mix(in_oklab,var(--danger)_14%,transparent)] text-[var(--danger)]',
              )}
            >
              exit {task.last_exit_code}
            </span>
          ) : null}
          {!task.runnable ? (
            <span className="inline-flex items-center gap-1 text-[var(--warning)]">
              <AlertTriangle className="h-3 w-3" />
              程序不可执行
            </span>
          ) : null}
          {task.restarts > 0 ? <span>重启 {task.restarts} 次</span> : null}
        </div>
      </button>

      {/* 操作区. */}
      <div className="flex shrink-0 items-center gap-1">
        {running ? (
          <Button
            variant="danger"
            size="icon"
            aria-label="停止任务"
            title="停止"
            disabled={busy}
            onClick={() => onStop(task)}
          >
            <Square className="h-4 w-4" fill="currentColor" />
          </Button>
        ) : (
          <Button
            variant="success"
            size="icon"
            aria-label="运行任务"
            title="运行"
            disabled={busy || disabled}
            onClick={() => onRun(task)}
          >
            <Play className="h-4 w-4" fill="currentColor" />
          </Button>
        )}
        <Button
          variant="ghost"
          size="icon"
          aria-label="编辑任务"
          title="编辑"
          onClick={() => onEdit(task)}
        >
          <Pencil className="h-4 w-4" />
        </Button>
        <Button
          variant="ghost"
          size="icon"
          aria-label="删除任务"
          title="删除"
          className="hover:text-[var(--danger)]"
          onClick={() => onDelete(task)}
        >
          <Trash2 className="h-4 w-4" />
        </Button>
        <div className="mx-1 h-5 w-px bg-[var(--border-subtle)]" aria-hidden />
        <Switch
          checked={task.enabled}
          onCheckedChange={(checked) => onToggle(task, checked)}
          label={task.enabled ? '停用任务' : '启用任务'}
        />
      </div>
    </div>
  )
}
