import { useEffect, useState } from 'react'
import { appApi } from '@/api/client'
import type { AppConfig } from '@/api/types'
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

  useEffect(() => {
    if (!open) return
    setError(null)
    appApi
      .getConfig()
      .then(setConfig)
      .catch((err: unknown) =>
        setError(err instanceof Error ? err.message : '读取设置失败'),
      )
  }, [open])

  const save = async () => {
    if (!config) return
    setSaving(true)
    setError(null)
    try {
      const saved = await appApi.saveConfig({
        quiet_launch: config.quiet_launch,
        log_retention_days: config.log_retention_days,
      })
      setConfig(saved)
      toast.success('设置已保存')
      onClose()
    } catch (err) {
      setError(err instanceof Error ? err.message : '保存设置失败')
    } finally {
      setSaving(false)
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
      </div>
    </Dialog>
  )
}
