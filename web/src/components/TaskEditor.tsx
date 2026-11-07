import { useEffect, useMemo, useState } from 'react'
import { AlertTriangle, FolderOpen, Plus, Trash2 } from 'lucide-react'
import { appApi } from '@/api/client'
import type { Task, Trigger, TriggerKind } from '@/api/types'
import {
  baseName,
  cn,
  envToList,
  isoToLocalInput,
  listToEnv,
  localInputToISO,
  triggerDescriptions,
  triggerLabels,
} from '@/lib/utils'
import { Button } from './ui/Button'
import { Field, Input } from './ui/Input'
import { Drawer } from './ui/Overlay'
import { Switch } from './ui/Switch'
import { FilePicker } from './FilePicker'

interface TaskEditorProps {
  open: boolean
  task?: Task
  onClose: () => void
  onSave: (task: Partial<Task>) => Promise<void>
}

/** 空表单的初始值. */
function emptyForm(): Partial<Task> {
  return {
    name: '',
    program: '',
    args: [],
    trigger: { kind: 'Manual' },
    enabled: true,
    no_console: false,
    working_dir: '',
    stdin: '',
    stdout: '',
    stderr: '',
    env_vars: {},
    // 默认继承全局与 shell 环境: 从 Finder 启动的应用 PATH 只有系统默认值,
    // 不继承的话 homebrew 与 SDK 里的命令都找不到.
    use_shell_env: true,
    use_global_env: true,
  }
}

/** 触发方式的顺序. */
const triggerOrder: TriggerKind[] = [
  'Manual',
  'Startup',
  'KeepAlive',
  'Routine',
  'Instant',
  'UntilSucceed',
]

/** 任务编辑抽屉. */
export function TaskEditor({ open, task, onClose, onSave }: TaskEditorProps) {
  const [form, setForm] = useState<Partial<Task>>(emptyForm)
  const [args, setArgs] = useState<string[]>([])
  const [envRows, setEnvRows] = useState<Array<{ key: string; value: string }>>([])
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  // 从命令派生任务: 粘贴一条命令, 解析后填进表单, 不直接创建.
  const [commandInput, setCommandInput] = useState('')
  const [parsing, setParsing] = useState(false)
  const [parseWarnings, setParseWarnings] = useState<string[]>([])
  const [picker, setPicker] = useState<null | 'program' | 'working_dir' | 'stdout' | 'stderr' | 'stdin'>(
    null,
  )

  // 打开时用传入的任务重置表单.
  useEffect(() => {
    if (!open) return
    if (task) {
      setForm({
        id: task.id,
        name: task.name,
        program: task.program,
        args: task.args ?? [],
        trigger: task.trigger,
        enabled: task.enabled,
        no_console: task.no_console,
        working_dir: task.working_dir ?? '',
        stdin: task.stdin ?? '',
        stdout: task.stdout ?? '',
        stderr: task.stderr ?? '',
        env_vars: task.env_vars ?? {},
      })
      setArgs(task.args ?? [])
      setEnvRows(envToList(task.env_vars))
    } else {
      const initial = emptyForm()
      setForm(initial)
      setArgs([])
      setEnvRows([])
    }
    setError(null)
    setCommandInput('')
    setParseWarnings([])
  }, [open, task])

  const trigger = form.trigger ?? { kind: 'Manual' }

  const updateTrigger = (patch: Partial<Trigger>) => {
    setForm((current) => ({
      ...current,
      trigger: { ...(current.trigger ?? { kind: 'Manual' }), ...patch },
    }))
  }

  const canSave = useMemo(
    () => Boolean(form.name?.trim()) && Boolean(form.program?.trim()) && !saving,
    [form.name, form.program, saving],
  )

  // handleParseCommand 把命令拆成任务字段填进表单.
  //
  // 只预填, 不保存: 用户可以继续改, 再自己点 "创建任务".
  const handleParseCommand = async () => {
    const raw = commandInput.trim()
    if (!raw) return
    setParsing(true)
    setParseWarnings([])
    setError(null)
    try {
      const parsed = await appApi.parseCommand(raw)
      setForm((current) => ({
        ...current,
        // 名称留空时用程序名兜底, 让用户少填一项.
        name: current.name?.trim() ? current.name : baseName(parsed.program),
        program: parsed.program,
        working_dir: parsed.working_dir ?? current.working_dir,
        stdin: parsed.stdin ?? '',
        stdout: parsed.stdout ?? '',
        stderr: parsed.stderr ?? '',
        env_vars: parsed.env_vars ?? {},
      }))
      setArgs(parsed.args ?? [])
      setEnvRows(envToList(parsed.env_vars))
      setParseWarnings(parsed.warnings ?? [])
    } catch (err) {
      setError(err instanceof Error ? err.message : '命令解析失败')
    } finally {
      setParsing(false)
    }
  }

  const handleSubmit = async () => {
    if (!canSave) return
    setSaving(true)
    setError(null)
    try {
      const payload: Partial<Task> = {
        ...form,
        args: args.filter((arg) => arg !== ''),
        env_vars: listToEnv(envRows),
        working_dir: form.working_dir || undefined,
        stdin: form.stdin || undefined,
        stdout: form.stdout || undefined,
        stderr: form.stderr || undefined,
      }
      await onSave(payload)
      onClose()
    } catch (err) {
      setError(err instanceof Error ? err.message : '保存失败')
    } finally {
      setSaving(false)
    }
  }

  const pickerMode = picker === 'working_dir' ? 'dir' : 'file'

  return (
    <>
      <Drawer
        open={open}
        onClose={onClose}
        title={task ? '编辑任务' : '新建任务'}
        description={task ? '修改后立即生效, 正在运行的任务会按新定义重启' : '配置一个后台任务'}
        footer={
          <>
            <Button variant="ghost" onClick={onClose}>
              取消
            </Button>
            <Button variant="primary" disabled={!canSave} onClick={() => void handleSubmit()}>
              {saving ? '保存中...' : task ? '保存修改' : '创建任务'}
            </Button>
          </>
        }
      >
        {error ? (
          <div className="rounded-lg border border-[color-mix(in_oklab,var(--danger)_40%,transparent)] bg-[color-mix(in_oklab,var(--danger)_12%,transparent)] px-3 py-2 text-[13px] text-[var(--danger)]">
            {error}
          </div>
        ) : null}

        {/* 从命令派生. */}
        <section className="space-y-2 rounded-lg border border-[var(--border-subtle)] bg-[var(--surface-sunken)] p-3">
          <div>
            <h3 className="text-[13px] font-semibold text-[var(--content)]">从命令填充</h3>
            <p className="mt-0.5 text-[11px] text-[var(--content-faint)]">
              粘贴一条命令, 自动拆出程序, 参数, 环境变量与重定向; 只填表不创建
            </p>
          </div>
          <div className="flex gap-2">
            <Input
              value={commandInput}
              placeholder={`docker run --rm -v /tmp/d:/d -e MODE=prod nginx:latest`}
              className="font-mono text-[12px]"
              onChange={(event) => setCommandInput(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === 'Enter') {
                  event.preventDefault()
                  void handleParseCommand()
                }
              }}
            />
            <Button
              variant="outline"
              size="md"
              disabled={parsing || !commandInput.trim()}
              onClick={() => void handleParseCommand()}
            >
              {parsing ? '解析中' : '解析'}
            </Button>
          </div>
          {parseWarnings.length > 0 ? (
            <ul className="space-y-1">
              {parseWarnings.map((warning, index) => (
                <li
                  key={index}
                  className="flex items-start gap-1.5 text-[11px] text-[var(--warning)]"
                >
                  <AlertTriangle className="mt-0.5 h-3 w-3 shrink-0" />
                  <span>{warning}</span>
                </li>
              ))}
            </ul>
          ) : null}
        </section>

        {/* 基础信息. */}
        <section className="space-y-4">
          <h3 className="text-[13px] font-semibold uppercase tracking-wide text-[var(--content-faint)]">
            基础信息
          </h3>
          <Field label="任务名称">
            <Input
              value={form.name ?? ''}
              placeholder="例如: 同步笔记"
              onChange={(event) => setForm({ ...form, name: event.target.value })}
            />
          </Field>
          <Field label="程序路径" hint="可执行文件, macOS 也可以选择 .app 目录">
            <div className="flex gap-2">
              <Input
                value={form.program ?? ''}
                placeholder="/usr/local/bin/example"
                className="font-mono text-[12px]"
                onChange={(event) => setForm({ ...form, program: event.target.value })}
              />
              <Button variant="outline" size="md" onClick={() => setPicker('program')}>
                <FolderOpen className="h-4 w-4" />
                浏览
              </Button>
            </div>
          </Field>
          <Field label="启动目录" hint="留空时使用程序所在目录">
            <div className="flex gap-2">
              <Input
                value={form.working_dir ?? ''}
                placeholder="留空使用程序所在目录"
                className="font-mono text-[12px]"
                onChange={(event) => setForm({ ...form, working_dir: event.target.value })}
              />
              <Button variant="outline" size="md" onClick={() => setPicker('working_dir')}>
                <FolderOpen className="h-4 w-4" />
                浏览
              </Button>
            </div>
          </Field>
        </section>

        {/* 触发方式. */}
        <section className="space-y-3">
          <h3 className="text-[13px] font-semibold uppercase tracking-wide text-[var(--content-faint)]">
            触发方式
          </h3>
          <div className="grid grid-cols-2 gap-2 sm:grid-cols-3">
            {triggerOrder.map((kind) => (
              <button
                key={kind}
                type="button"
                onClick={() => updateTrigger({ kind })}
                className={cn(
                  'cursor-pointer rounded-lg border px-3 py-2 text-left transition-colors',
                  trigger.kind === kind
                    ? 'border-[var(--accent)] bg-[var(--accent-soft)]'
                    : 'border-[var(--border-subtle)] hover:border-[var(--border-strong)]',
                )}
              >
                <span
                  className={cn(
                    'block text-[13px] font-medium',
                    trigger.kind === kind
                      ? 'text-[var(--accent)]'
                      : 'text-[var(--content)]',
                  )}
                >
                  {triggerLabels[kind]}
                </span>
                <span className="mt-0.5 block text-[11px] leading-tight text-[var(--content-faint)]">
                  {triggerDescriptions[kind]}
                </span>
              </button>
            ))}
          </div>

          {trigger.kind === 'Routine' ? (
            <Field label="间隔秒数" hint={`每 ${trigger.interval_seconds ?? 0} 秒运行一次`}>
              <Input
                type="number"
                min={1}
                value={trigger.interval_seconds ?? 60}
                onChange={(event) =>
                  updateTrigger({ interval_seconds: Number(event.target.value) })
                }
              />
            </Field>
          ) : null}

          {trigger.kind === 'Instant' ? (
            <Field label="执行时间">
              <Input
                type="datetime-local"
                value={isoToLocalInput(trigger.run_at)}
                onChange={(event) =>
                  updateTrigger({ run_at: localInputToISO(event.target.value) })
                }
              />
            </Field>
          ) : null}
        </section>

        {/* 参数. */}
        <section className="space-y-3">
          <div className="flex items-center justify-between">
            <h3 className="text-[13px] font-semibold uppercase tracking-wide text-[var(--content-faint)]">
              启动参数
            </h3>
            <Button variant="ghost" size="sm" onClick={() => setArgs([...args, ''])}>
              <Plus className="h-3.5 w-3.5" />
              添加
            </Button>
          </div>
          {args.length === 0 ? (
            <p className="text-xs text-[var(--content-faint)]">暂无参数</p>
          ) : (
            <div className="space-y-2">
              {args.map((arg, index) => (
                <div key={index} className="flex gap-2">
                  <Input
                    value={arg}
                    placeholder={`参数 ${index + 1}`}
                    className="font-mono text-[12px]"
                    onChange={(event) => {
                      const next = [...args]
                      next[index] = event.target.value
                      setArgs(next)
                    }}
                  />
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label="删除参数"
                    onClick={() => setArgs(args.filter((_, i) => i !== index))}
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </div>
              ))}
            </div>
          )}
        </section>

        {/* 环境变量. */}
        <section className="space-y-3">
          {/* 继承开关: 决定这个任务能拿到哪些外部环境. */}
          <div className="space-y-2 rounded-lg border border-[var(--border-subtle)] px-3 py-2.5">
            <div className="flex items-center justify-between gap-3">
              <div>
                <p className="text-[13px] text-[var(--content)]">继承登录 shell 环境</p>
                <p className="mt-0.5 text-xs text-[var(--content-faint)]">
                  拿到 shell 里的 PATH 等变量, 让 adb, node 这类命令可直接调用
                </p>
              </div>
              <Switch
                checked={form.use_shell_env ?? true}
                onCheckedChange={(checked) =>
                  setForm((current) => ({ ...current, use_shell_env: checked }))
                }
                label="继承登录 shell 环境"
              />
            </div>
            <div className="flex items-center justify-between gap-3 border-t border-[var(--border-subtle)] pt-2">
              <div>
                <p className="text-[13px] text-[var(--content)]">继承全局环境变量</p>
                <p className="mt-0.5 text-xs text-[var(--content-faint)]">
                  使用设置里填写的全局变量, 可用设置页统一管理
                </p>
              </div>
              <Switch
                checked={form.use_global_env ?? true}
                onCheckedChange={(checked) =>
                  setForm((current) => ({ ...current, use_global_env: checked }))
                }
                label="继承全局环境变量"
              />
            </div>
          </div>

          <div className="flex items-center justify-between">
            <h3 className="text-[13px] font-semibold uppercase tracking-wide text-[var(--content-faint)]">
              任务环境变量
            </h3>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setEnvRows([...envRows, { key: '', value: '' }])}
            >
              <Plus className="h-3.5 w-3.5" />
              添加
            </Button>
          </div>
          {envRows.length === 0 ? (
            <p className="text-xs text-[var(--content-faint)]">暂无环境变量</p>
          ) : (
            <div className="space-y-2">
              {envRows.map((row, index) => (
                <div key={index} className="flex gap-2">
                  <Input
                    value={row.key}
                    placeholder="变量名"
                    className="font-mono text-[12px]"
                    onChange={(event) => {
                      const next = [...envRows]
                      next[index] = { ...next[index], key: event.target.value }
                      setEnvRows(next)
                    }}
                  />
                  <Input
                    value={row.value}
                    placeholder="变量值"
                    className="font-mono text-[12px]"
                    onChange={(event) => {
                      const next = [...envRows]
                      next[index] = { ...next[index], value: event.target.value }
                      setEnvRows(next)
                    }}
                  />
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label="删除环境变量"
                    onClick={() => setEnvRows(envRows.filter((_, i) => i !== index))}
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </div>
              ))}
            </div>
          )}
        </section>

        {/* 文件重定向. */}
        <section className="space-y-4">
          <h3 className="text-[13px] font-semibold uppercase tracking-wide text-[var(--content-faint)]">
            文件重定向
          </h3>
          <Field label="标准输出" hint="留空则不重定向">
            <div className="flex gap-2">
              <Input
                value={form.stdout ?? ''}
                placeholder="日志输出文件"
                className="font-mono text-[12px]"
                onChange={(event) => setForm({ ...form, stdout: event.target.value })}
              />
              <Button variant="outline" size="md" onClick={() => setPicker('stdout')}>
                <FolderOpen className="h-4 w-4" />
                浏览
              </Button>
            </div>
          </Field>
          <Field label="标准错误" hint="留空则不重定向">
            <div className="flex gap-2">
              <Input
                value={form.stderr ?? ''}
                placeholder="错误输出文件"
                className="font-mono text-[12px]"
                onChange={(event) => setForm({ ...form, stderr: event.target.value })}
              />
              <Button variant="outline" size="md" onClick={() => setPicker('stderr')}>
                <FolderOpen className="h-4 w-4" />
                浏览
              </Button>
            </div>
          </Field>
          <Field label="标准输入" hint="留空则不重定向">
            <div className="flex gap-2">
              <Input
                value={form.stdin ?? ''}
                placeholder="输入文件"
                className="font-mono text-[12px]"
                onChange={(event) => setForm({ ...form, stdin: event.target.value })}
              />
              <Button variant="outline" size="md" onClick={() => setPicker('stdin')}>
                <FolderOpen className="h-4 w-4" />
                浏览
              </Button>
            </div>
          </Field>
        </section>

        {/* 其他选项. */}
        <section className="space-y-3">
          <h3 className="text-[13px] font-semibold uppercase tracking-wide text-[var(--content-faint)]">
            其他
          </h3>
          <div className="flex items-center justify-between rounded-lg border border-[var(--border-subtle)] px-3 py-2.5">
            <div>
              <p className="text-[13px] text-[var(--content)]">启用任务</p>
              <p className="text-xs text-[var(--content-faint)]">停用后不会按触发器自动运行</p>
            </div>
            <Switch
              checked={form.enabled ?? true}
              onCheckedChange={(checked) => setForm({ ...form, enabled: checked })}
              label="启用任务"
            />
          </div>
          <div className="flex items-center justify-between rounded-lg border border-[var(--border-subtle)] px-3 py-2.5">
            <div>
              <p className="text-[13px] text-[var(--content)]">隐藏控制台</p>
              <p className="text-xs text-[var(--content-faint)]">仅 Windows 有效</p>
            </div>
            <Switch
              checked={form.no_console ?? false}
              onCheckedChange={(checked) => setForm({ ...form, no_console: checked })}
              label="隐藏控制台"
            />
          </div>
        </section>
      </Drawer>

      <FilePicker
        open={picker !== null}
        mode={pickerMode}
        title={picker === 'working_dir' ? '选择启动目录' : '选择文件'}
        onClose={() => setPicker(null)}
        onPick={(path) => {
          if (picker === 'program') setForm((current) => ({ ...current, program: path }))
          else if (picker === 'working_dir')
            setForm((current) => ({ ...current, working_dir: path }))
          else if (picker === 'stdout') setForm((current) => ({ ...current, stdout: path }))
          else if (picker === 'stderr') setForm((current) => ({ ...current, stderr: path }))
          else if (picker === 'stdin') setForm((current) => ({ ...current, stdin: path }))
        }}
      />
    </>
  )
}
