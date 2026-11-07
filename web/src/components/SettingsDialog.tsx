import { useEffect, useState } from 'react'
import { appApi } from '@/api/client'
import type { AppConfig, EnvStatus } from '@/api/types'
import { envToList, listToEnv } from '@/lib/utils'
import { Button } from './ui/Button'
import { Field, Input } from './ui/Input'
import { Dialog } from './ui/Overlay'
import { Switch } from './ui/Switch'
import { useToast } from './Toast'

interface SettingsDialogProps {
  open: boolean
  onClose: () => void
}

/** 应用设置. */
export function SettingsDialog({ open, onClose }: SettingsDialogProps) {
  const toast = useToast()
  const [config, setConfig] = useState<AppConfig | null>(null)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  // 全局环境变量的行编辑状态, 保存时再转回对象.
  const [envRows, setEnvRows] = useState<Array<{ key: string; value: string }>>([])
  // 登录 shell 的导入状态, 用于展示实际拿到的 PATH 条目数.
  const [envStatus, setEnvStatus] = useState<EnvStatus | null>(null)
  const [refreshing, setRefreshing] = useState(false)

  useEffect(() => {
    if (!open) return
    setError(null)
    appApi
      .getConfig()
      .then((loaded) => {
        setConfig(loaded)
        setEnvRows(envToList(loaded.env_vars))
      })
      .catch((err: unknown) =>
        setError(err instanceof Error ? err.message : '读取设置失败'),
      )
    appApi
      .envStatus()
      .then(setEnvStatus)
      .catch(() => setEnvStatus(null))
  }, [open])

  const save = async () => {
    if (!config) return
    setSaving(true)
    setError(null)
    try {
      const saved = await appApi.saveConfig({
        quiet_launch: config.quiet_launch,
        log_retention_days: config.log_retention_days,
        env_import_enabled: config.env_import_enabled,
        env_import_command: config.env_import_command,
        env_vars: listToEnv(envRows),
      })
      setConfig(saved)
      setEnvRows(envToList(saved.env_vars))
      // 保存后后端会重新导入, 稍等一下再取状态.
      setTimeout(() => {
        appApi
          .envStatus()
          .then(setEnvStatus)
          .catch(() => undefined)
      }, 600)
      toast.success('设置已保存')
    } catch (err) {
      setError(err instanceof Error ? err.message : '保存设置失败')
    } finally {
      setSaving(false)
    }
  }

  const refreshEnv = async () => {
    setRefreshing(true)
    setError(null)
    try {
      const status = await appApi.refreshEnv()
      setEnvStatus(status)
      if (status.last_error) {
        setError(status.last_error)
      } else {
        toast.success('已重新导入环境变量', `共 ${status.var_count} 个变量`)
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : '重新导入失败')
    } finally {
      setRefreshing(false)
    }
  }

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title="设置"
      description="这些设置保存在配置文件中, 重启后依然有效"
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            取消
          </Button>
          <Button variant="primary" disabled={saving || !config} onClick={() => void save()}>
            {saving ? '保存中...' : '保存'}
          </Button>
        </>
      }
    >
      {error ? (
        <p className="mb-4 rounded-lg bg-[color-mix(in_oklab,var(--danger)_12%,transparent)] px-3 py-2 text-[13px] text-[var(--danger)]">
          {error}
        </p>
      ) : null}

      <div className="space-y-4">
        <div className="flex items-center justify-between rounded-lg border border-[var(--border-subtle)] px-3 py-2.5">
          <div>
            <p className="text-[13px] text-[var(--content)]">后台启动</p>
            <p className="text-xs text-[var(--content-faint)]">
              启动时不自动打开管理界面
            </p>
          </div>
          <Switch
            checked={config?.quiet_launch ?? false}
            onCheckedChange={(checked) =>
              setConfig((current) => (current ? { ...current, quiet_launch: checked } : current))
            }
            label="后台启动"
          />
        </div>

        <Field label="日志保留天数" hint="按天数清理轮转日志, 最小 1 天">
          <Input
            type="number"
            min={1}
            value={config?.log_retention_days ?? 5}
            onChange={(event) =>
              setConfig((current) =>
                current
                  ? { ...current, log_retention_days: Number(event.target.value) }
                  : current,
              )
            }
          />
        </Field>

        <Field label="监听地址" hint="修改后需要重启应用才生效">
          <Input value={config?.local_bind ?? ''} readOnly className="font-mono text-[12px]" />
        </Field>

        {/* 环境变量: 从 Finder 启动时 PATH 只有系统默认值, 靠这里补回来. */}
        <div className="rounded-lg border border-[var(--border-subtle)] px-3 py-2.5">
          <div className="flex items-start justify-between gap-3">
            <div>
              <p className="text-[13px] text-[var(--content)]">继承登录 shell 环境</p>
              <p className="mt-0.5 text-xs text-[var(--content-faint)]">
                从终端以外启动时, PATH 只有系统默认值. 开启后会读取登录 shell 的环境变量,
                让 homebrew 与 SDK 里的命令可用.
              </p>
            </div>
            <Switch
              checked={config?.env_import_enabled ?? true}
              onCheckedChange={(checked) =>
                setConfig((current) =>
                  current ? { ...current, env_import_enabled: checked } : current,
                )
              }
              label="继承登录 shell 环境"
            />
          </div>

          {config?.env_import_enabled ? (
            <div className="mt-3 space-y-2">
              <Field
                label="导入命令"
                hint={
                  envStatus?.default_command
                    ? `留空时使用: ${envStatus.default_command}`
                    : '留空时按 $SHELL 自动推断'
                }
              >
                <Input
                  value={config?.env_import_command ?? ''}
                  placeholder={envStatus?.default_command ?? ''}
                  onChange={(event) =>
                    setConfig((current) =>
                      current ? { ...current, env_import_command: event.target.value } : current,
                    )
                  }
                  className="font-mono text-[12px]"
                />
              </Field>

              <div className="flex items-center justify-between gap-2 rounded-lg bg-[var(--surface-sunken)] px-3 py-2">
                <p className="text-xs text-[var(--content-faint)]">
                  {envStatus
                    ? envStatus.last_error
                      ? `上次导入失败: ${envStatus.last_error}`
                      : `已导入 ${envStatus.var_count} 个变量, PATH 含 ${envStatus.path_count} 个目录`
                    : '尚未获取导入状态'}
                </p>
                <Button
                  variant="ghost"
                  size="sm"
                  disabled={refreshing}
                  onClick={() => void refreshEnv()}
                >
                  {refreshing ? '导入中...' : '立即重新导入'}
                </Button>
              </div>
            </div>
          ) : null}
        </div>

        {/* 全局环境变量: 任务可选择是否继承. */}
        <div className="rounded-lg border border-[var(--border-subtle)] px-3 py-2.5">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-[13px] text-[var(--content)]">全局环境变量</p>
              <p className="mt-0.5 text-xs text-[var(--content-faint)]">
                对所有开启了继承的任务生效, 优先级高于 shell 导入, 低于任务自身的变量.
              </p>
            </div>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setEnvRows((rows) => [...rows, { key: '', value: '' }])}
            >
              添加
            </Button>
          </div>

          {envRows.length === 0 ? (
            <p className="mt-2 text-xs text-[var(--content-faint)]">还没有全局变量</p>
          ) : (
            <div className="mt-2 space-y-1.5">
              {envRows.map((row, index) => (
                <div key={index} className="flex items-center gap-1.5">
                  <Input
                    value={row.key}
                    placeholder="变量名"
                    className="w-40 font-mono text-[12px]"
                    onChange={(event) =>
                      setEnvRows((rows) =>
                        rows.map((item, i) =>
                          i === index ? { ...item, key: event.target.value } : item,
                        ),
                      )
                    }
                  />
                  <Input
                    value={row.value}
                    placeholder="值"
                    className="flex-1 font-mono text-[12px]"
                    onChange={(event) =>
                      setEnvRows((rows) =>
                        rows.map((item, i) =>
                          i === index ? { ...item, value: event.target.value } : item,
                        ),
                      )
                    }
                  />
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label="删除该变量"
                    title="删除该变量"
                    onClick={() => setEnvRows((rows) => rows.filter((_, i) => i !== index))}
                  >
                    ×
                  </Button>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </Dialog>
  )
}
