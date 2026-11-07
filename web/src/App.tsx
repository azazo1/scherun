import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  DndContext,
  KeyboardSensor,
  PointerSensor,
  closestCenter,
  useSensor,
  useSensors,
  type DragEndEvent,
} from '@dnd-kit/core'
import {
  SortableContext,
  arrayMove,
  sortableKeyboardCoordinates,
  verticalListSortingStrategy,
} from '@dnd-kit/sortable'
import {
  Activity,
  AlertTriangle,
  Moon,
  Plus,
  Power,
  RefreshCw,
  Search,
  Settings,
  Sun,
} from 'lucide-react'
import { appApi, subscribeEvents, taskApi } from '@/api/client'
import type { Health, Task } from '@/api/types'
import { TaskCard } from '@/components/TaskCard'
import { TaskEditor } from '@/components/TaskEditor'
import { SettingsDialog } from '@/components/SettingsDialog'
import { ToastProvider, useToast } from '@/components/Toast'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Dialog } from '@/components/ui/Overlay'
import { cn } from '@/lib/utils'

/** 主题偏好. */
type Theme = 'dark' | 'light'

/** 应用主体. */
function AppShell() {
  const toast = useToast()
  const [tasks, setTasks] = useState<Task[]>([])
  const [health, setHealth] = useState<Health | null>(null)
  const [connected, setConnected] = useState(false)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [query, setQuery] = useState('')
  const [editing, setEditing] = useState<Task | undefined>(undefined)
  const [editorOpen, setEditorOpen] = useState(false)
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [pendingDelete, setPendingDelete] = useState<Task | null>(null)
  const [quitOpen, setQuitOpen] = useState(false)
  const [busyId, setBusyId] = useState<number | null>(null)
  const [theme, setTheme] = useState<Theme>(() => {
    const stored = localStorage.getItem('scherun-theme')
    return stored === 'light' ? 'light' : 'dark'
  })

  // 记住上次看到的告警, 避免重复弹同一条.
  const seenWarnings = useRef(new Set<string>())

  // 主题应用到根元素.
  useEffect(() => {
    document.documentElement.classList.toggle('light', theme === 'light')
    document.documentElement.classList.toggle('dark', theme === 'dark')
    localStorage.setItem('scherun-theme', theme)
  }, [theme])

  const refresh = useCallback(async () => {
    try {
      const list = await taskApi.list()
      setTasks(list)
      setError(null)
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载任务失败')
    } finally {
      setLoading(false)
    }
  }, [])

  // 首屏拉一次, 之后由 SSE 推送.
  useEffect(() => {
    void refresh()
    appApi
      .health()
      .then(setHealth)
      .catch(() => setHealth(null))
  }, [refresh])

  // 订阅推送.
  useEffect(() => {
    const unsubscribe = subscribeEvents(
      (payload) => {
        if (payload.tasks) {
          setTasks(payload.tasks)
          setLoading(false)
          setError(null)
        }
        for (const warning of payload.warnings ?? []) {
          const key = `${warning.created_at}|${warning.message}`
          if (seenWarnings.current.has(key)) continue
          seenWarnings.current.add(key)
          toast.warning(
            warning.task_name ? `任务告警: ${warning.task_name}` : '任务告警',
            warning.message,
          )
        }
      },
      setConnected,
    )
    return unsubscribe
  }, [toast])

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 4 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  )

  const filtered = useMemo(() => {
    const keyword = query.trim().toLowerCase()
    if (!keyword) return tasks
    return tasks.filter(
      (task) =>
        task.name.toLowerCase().includes(keyword) ||
        task.program.toLowerCase().includes(keyword),
    )
  }, [tasks, query])

  const stats = useMemo(() => {
    const running = tasks.filter((task) => task.status === 'running').length
    const disabled = tasks.filter((task) => !task.enabled).length
    const suspended = tasks.filter((task) => task.status === 'suspended').length
    return { total: tasks.length, running, disabled, suspended }
  }, [tasks])

  const handleDragEnd = async (event: DragEndEvent) => {
    const { active, over } = event
    if (!over || active.id === over.id) return
    const oldIndex = tasks.findIndex((task) => task.id === active.id)
    const newIndex = tasks.findIndex((task) => task.id === over.id)
    if (oldIndex < 0 || newIndex < 0) return

    const next = arrayMove(tasks, oldIndex, newIndex)
    setTasks(next)
    try {
      await taskApi.reorder(next.map((task) => task.id))
    } catch (err) {
      toast.error('调整顺序失败', err instanceof Error ? err.message : undefined)
      void refresh()
    }
  }

  const handleSave = async (payload: Partial<Task>) => {
    await taskApi.save(payload)
    await refresh()
    toast.success(payload.id ? '任务已更新' : '任务已创建')
  }

  const handleDelete = async (task: Task) => {
    setPendingDelete(null)
    try {
      await taskApi.remove(task.id)
      await refresh()
      toast.success('任务已删除')
    } catch (err) {
      toast.error('删除失败', err instanceof Error ? err.message : undefined)
    }
  }

  const handleRun = async (task: Task) => {
    setBusyId(task.id)
    try {
      await taskApi.run(task.id)
      toast.success(`已启动 ${task.name}`)
    } catch (err) {
      toast.error('启动失败', err instanceof Error ? err.message : undefined)
    } finally {
      setBusyId(null)
    }
  }

  const handleStop = async (task: Task) => {
    setBusyId(task.id)
    try {
      await taskApi.stop(task.id)
      toast.success(`已停止 ${task.name}`)
    } catch (err) {
      toast.error('停止失败', err instanceof Error ? err.message : undefined)
    } finally {
      setBusyId(null)
    }
  }

  const handleToggle = async (task: Task, enabled: boolean) => {
    // 乐观更新, 失败时回滚.
    setTasks((current) =>
      current.map((item) =>
        item.id === task.id
          ? { ...item, enabled, status: enabled ? 'idle' : 'disabled' }
          : item,
      ),
    )
    try {
      await taskApi.setEnabled(task.id, enabled)
    } catch (err) {
      toast.error('切换状态失败', err instanceof Error ? err.message : undefined)
      void refresh()
    }
  }

  // 退出应用: 先让后端停止全部任务再退出进程, 页面随后失去连接.
  const handleQuit = async () => {
    setQuitOpen(false)
    try {
      await appApi.exit()
      toast.success('正在退出', '全部任务停止后进程结束, 可以关闭此页面')
    } catch (err) {
      toast.error('退出失败', err instanceof Error ? err.message : undefined)
    }
  }

  const openCreate = () => {
    setEditing(undefined)
    setEditorOpen(true)
  }

  const openEdit = (task: Task) => {
    setEditing(task)
    setEditorOpen(true)
  }

  return (
    <div className="flex min-h-screen flex-col">
      {/* 顶栏. */}
      <header className="sticky top-0 z-30 border-b border-[var(--border-subtle)] bg-[color-mix(in_oklab,var(--surface)_88%,transparent)] backdrop-blur">
        <div className="mx-auto flex max-w-5xl flex-wrap items-center gap-3 px-5 py-3">
          <div className="flex items-center gap-2">
            <span className="flex h-8 w-8 items-center justify-center rounded-lg bg-[var(--accent-soft)] text-[var(--accent)]">
              <Activity className="h-4 w-4" />
            </span>
            <div>
              <h1 className="text-sm font-semibold leading-tight text-[var(--content)]">
                scherun
              </h1>
              <p className="text-[11px] leading-tight text-[var(--content-faint)]">
                后台任务管理
              </p>
            </div>
          </div>

          <div className="relative min-w-40 flex-1">
            <Search className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-[var(--content-faint)]" />
            <Input
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder="搜索任务名称或程序路径"
              className="pl-8"
            />
          </div>

          <div className="flex items-center gap-1">
            <Button
              variant="ghost"
              size="icon"
              aria-label="刷新"
              title="刷新"
              onClick={() => void refresh()}
            >
              <RefreshCw className="h-4 w-4" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              aria-label="切换主题"
              title={theme === 'dark' ? '切换到浅色' : '切换到深色'}
              onClick={() => setTheme(theme === 'dark' ? 'light' : 'dark')}
            >
              {theme === 'dark' ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
            </Button>
            <Button
              variant="ghost"
              size="icon"
              aria-label="设置"
              title="设置"
              onClick={() => setSettingsOpen(true)}
            >
              <Settings className="h-4 w-4" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              aria-label="退出应用"
              title="退出应用"
              onClick={() => setQuitOpen(true)}
            >
              <Power className="h-4 w-4" />
            </Button>
            <Button variant="primary" size="md" onClick={openCreate}>
              <Plus className="h-4 w-4" />
              新建任务
            </Button>
          </div>
        </div>
      </header>

      <main className="mx-auto w-full max-w-5xl flex-1 px-5 py-5">
        {/* 概览. */}
        <div className="mb-5 grid grid-cols-2 gap-2 sm:grid-cols-4">
          <StatCard label="任务总数" value={stats.total} />
          <StatCard label="运行中" value={stats.running} tone="success" />
          <StatCard label="已停用" value={stats.disabled} />
          <StatCard
            label="已挂起"
            value={stats.suspended}
            tone={stats.suspended > 0 ? 'warning' : 'default'}
          />
        </div>

        {error ? (
          <div className="mb-4 flex items-start gap-2 rounded-lg border border-[color-mix(in_oklab,var(--danger)_40%,transparent)] bg-[color-mix(in_oklab,var(--danger)_10%,transparent)] px-3 py-2.5 text-[13px] text-[var(--danger)]">
            <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
            <div className="flex-1">
              <p className="font-medium">无法加载任务</p>
              <p className="mt-0.5 opacity-90">{error}</p>
            </div>
            <Button variant="ghost" size="sm" onClick={() => void refresh()}>
              重试
            </Button>
          </div>
        ) : null}

        {loading ? (
          <div className="space-y-2">
            {[0, 1, 2].map((index) => (
              <div
                key={index}
                className="h-[74px] animate-pulse rounded-xl border border-[var(--border-subtle)] bg-[var(--surface-raised)]"
              />
            ))}
          </div>
        ) : filtered.length === 0 ? (
          <EmptyState hasTasks={tasks.length > 0} onCreate={openCreate} />
        ) : (
          <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
            <SortableContext
              items={filtered.map((task) => task.id)}
              strategy={verticalListSortingStrategy}
            >
              <div className="space-y-2">
                {filtered.map((task) => (
                  <TaskCard
                    key={task.id}
                    task={task}
                    busy={busyId === task.id}
                    onEdit={openEdit}
                    onDelete={setPendingDelete}
                    onRun={handleRun}
                    onStop={handleStop}
                    onToggle={handleToggle}
                  />
                ))}
              </div>
            </SortableContext>
          </DndContext>
        )}
      </main>

      {/* 底部状态栏: 版本号与连接状态. */}
      <footer className="border-t border-[var(--border-subtle)] px-5 py-2">
        <div className="mx-auto flex max-w-5xl items-center justify-between text-[11px] text-[var(--content-faint)]">
          <span>scherun {health?.version ?? 'dev-build'}</span>
          <span className="flex items-center gap-1.5">
            <span
              className={cn(
                'h-1.5 w-1.5 rounded-full',
                connected ? 'bg-[var(--success)]' : 'bg-[var(--warning)]',
              )}
            />
            {connected ? '实时连接' : '正在重连'}
          </span>
        </div>
      </footer>

      <TaskEditor
        open={editorOpen}
        task={editing}
        onClose={() => setEditorOpen(false)}
        onSave={handleSave}
      />
      <SettingsDialog open={settingsOpen} onClose={() => setSettingsOpen(false)} />

      <Dialog
        open={pendingDelete !== null}
        onClose={() => setPendingDelete(null)}
        title="删除任务"
        description="该操作不可撤销"
        width="max-w-sm"
        footer={
          <>
            <Button variant="ghost" onClick={() => setPendingDelete(null)}>
              取消
            </Button>
            <Button
              variant="primary"
              className="bg-[var(--danger)]"
              onClick={() => pendingDelete && void handleDelete(pendingDelete)}
            >
              删除
            </Button>
          </>
        }
      >
        <p className="text-sm text-[var(--content-muted)]">
          确定要删除任务
          <span className="mx-1 font-medium text-[var(--content)]">
            {pendingDelete?.name}
          </span>
          吗? 如果它正在运行, 会被先停止.
        </p>
      </Dialog>

      <Dialog
        open={quitOpen}
        onClose={() => setQuitOpen(false)}
        title="退出 scherun"
        description="托盘常驻程序会一并结束"
        width="max-w-sm"
        footer={
          <>
            <Button variant="ghost" onClick={() => setQuitOpen(false)}>
              取消
            </Button>
            <Button
              variant="primary"
              className="bg-[var(--danger)]"
              onClick={() => void handleQuit()}
            >
              退出
            </Button>
          </>
        }
      >
        <p className="text-sm text-[var(--content-muted)]">
          退出后所有运行中的任务会被停止, 自动触发也会一并结束.
          想让它继续在后台跑, 关闭此页面即可.
        </p>
      </Dialog>
    </div>
  )
}

/** 统计卡片. */
function StatCard({
  label,
  value,
  tone = 'default',
}: {
  label: string
  value: number
  tone?: 'default' | 'success' | 'warning'
}) {
  const color =
    tone === 'success'
      ? 'var(--success)'
      : tone === 'warning'
        ? 'var(--warning)'
        : 'var(--content)'
  return (
    <div className="rounded-lg border border-[var(--border-subtle)] bg-[var(--surface-raised)] px-3 py-2">
      <p className="text-[11px] text-[var(--content-faint)]">{label}</p>
      <p className="text-lg font-semibold leading-tight" style={{ color }}>
        {value}
      </p>
    </div>
  )
}

/** 空状态. */
function EmptyState({ hasTasks, onCreate }: { hasTasks: boolean; onCreate: () => void }) {
  return (
    <div className="flex flex-col items-center justify-center rounded-xl border border-dashed border-[var(--border-strong)] px-6 py-14 text-center">
      <span className="mb-3 flex h-11 w-11 items-center justify-center rounded-full bg-[var(--surface-sunken)] text-[var(--content-faint)]">
        <Activity className="h-5 w-5" />
      </span>
      <p className="text-sm font-medium text-[var(--content)]">
        {hasTasks ? '没有匹配的任务' : '还没有任务'}
      </p>
      <p className="mt-1 max-w-sm text-xs text-[var(--content-faint)]">
        {hasTasks
          ? '换个关键词试试, 或者清空搜索框'
          : '创建一个任务, scherun 会按你选定的触发方式在后台运行它'}
      </p>
      {!hasTasks ? (
        <Button variant="primary" className="mt-4" onClick={onCreate}>
          <Plus className="h-4 w-4" />
          新建第一个任务
        </Button>
      ) : null}
    </div>
  )
}

/** 应用根组件. */
export default function App() {
  return (
    <ToastProvider>
      <AppShell />
    </ToastProvider>
  )
}
