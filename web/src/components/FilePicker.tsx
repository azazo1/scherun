import { useCallback, useEffect, useState } from 'react'
import { ChevronRight, File, Folder, FolderUp, HardDrive, Home } from 'lucide-react'
import { appApi } from '@/api/client'
import type { FsListing } from '@/api/types'
import { cn } from '@/lib/utils'
import { Button } from './ui/Button'
import { Dialog } from './ui/Overlay'

interface FilePickerProps {
  open: boolean
  onClose: () => void
  onPick: (path: string) => void
  /** mode 决定可选条目: file 只能选可执行文件, dir 只能选目录. */
  mode?: 'file' | 'dir' | 'any'
  title?: string
}

/**
 * 目录浏览式文件选择器.
 *
 * 浏览器拿不到本机绝对路径, 因此由后端提供目录列表.
 */
export function FilePicker({
  open,
  onClose,
  onPick,
  mode = 'file',
  title = '选择程序',
}: FilePickerProps) {
  const [listing, setListing] = useState<FsListing | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [selected, setSelected] = useState<string | null>(null)

  const load = useCallback(async (path?: string) => {
    setLoading(true)
    setError(null)
    try {
      const result = await appApi.listDir(path)
      setListing(result)
      setSelected(null)
    } catch (err) {
      setError(err instanceof Error ? err.message : '无法读取目录')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    if (open) {
      void load()
    }
  }, [open, load])

  const canSelect = (entry: { is_dir: boolean; selectable: boolean }) => {
    if (mode === 'dir') return entry.is_dir
    if (mode === 'any') return entry.is_dir || entry.selectable
    return !entry.is_dir && entry.selectable
  }

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={title}
      description="浏览本机文件系统, 选择后填入路径"
      width="max-w-2xl"
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            取消
          </Button>
          <Button
            variant="primary"
            disabled={!selected}
            onClick={() => {
              if (selected) {
                onPick(selected)
                onClose()
              }
            }}
          >
            选择
          </Button>
        </>
      }
    >
      <div className="space-y-3">
        {/* 快速跳转. */}
        <div className="flex flex-wrap items-center gap-1">
          {listing?.home ? (
            <Button variant="ghost" size="sm" onClick={() => void load(listing.home)}>
              <Home className="h-3.5 w-3.5" />
              家目录
            </Button>
          ) : null}
          {listing?.parent ? (
            <Button variant="ghost" size="sm" onClick={() => void load(listing.parent)}>
              <FolderUp className="h-3.5 w-3.5" />
              上级
            </Button>
          ) : null}
          {(listing?.roots ?? []).map((root) => (
            <Button
              key={root}
              variant="ghost"
              size="sm"
              onClick={() => void load(root)}
              title={root}
            >
              <HardDrive className="h-3.5 w-3.5" />
              {root === '/' ? '根目录' : root.split('/').pop() || root}
            </Button>
          ))}
        </div>

        {/* 当前路径. */}
        <div className="flex items-center gap-1 overflow-x-auto rounded-lg border border-[var(--border-subtle)] bg-[var(--surface-sunken)] px-2 py-1.5 font-mono text-[11px] text-[var(--content-muted)]">
          {(listing?.path ?? '').split('/').filter(Boolean).map((segment, index, all) => {
            const path = '/' + all.slice(0, index + 1).join('/')
            return (
              <span key={path} className="flex shrink-0 items-center gap-1">
                {index > 0 ? (
                  <ChevronRight className="h-3 w-3 text-[var(--content-faint)]" />
                ) : null}
                <button
                  type="button"
                  className="cursor-pointer hover:text-[var(--content)]"
                  onClick={() => void load(path)}
                >
                  {segment}
                </button>
              </span>
            )
          })}
        </div>

        {/* 目录内容. */}
        <div className="h-72 overflow-y-auto rounded-lg border border-[var(--border-subtle)]">
          {loading ? (
            <p className="p-4 text-sm text-[var(--content-muted)]">加载中...</p>
          ) : error ? (
            <p className="p-4 text-sm text-[var(--danger)]">{error}</p>
          ) : listing && listing.entries.length === 0 ? (
            <p className="p-4 text-sm text-[var(--content-faint)]">此目录为空</p>
          ) : (
            <ul className="divide-y divide-[var(--border-subtle)]">
              {listing?.entries.map((entry) => {
                const selectable = canSelect(entry)
                const isSelected = selected === entry.path
                return (
                  <li key={entry.path}>
                    <div
                      className={cn(
                        'flex items-center gap-2 px-3 py-1.5 text-sm',
                        selectable && 'cursor-pointer hover:bg-[var(--surface-sunken)]',
                        isSelected && 'bg-[var(--accent-soft)]',
                        !selectable && 'opacity-55',
                      )}
                      onClick={() => {
                        if (entry.is_dir && mode !== 'dir') {
                          void load(entry.path)
                        } else if (selectable) {
                          setSelected(entry.path)
                        }
                      }}
                      onDoubleClick={() => {
                        if (entry.is_dir) {
                          void load(entry.path)
                        } else if (selectable) {
                          setSelected(entry.path)
                          onPick(entry.path)
                          onClose()
                        }
                      }}
                    >
                      {entry.is_dir ? (
                        <Folder className="h-4 w-4 shrink-0 text-[var(--accent)]" />
                      ) : (
                        <File
                          className={cn(
                            'h-4 w-4 shrink-0',
                            entry.selectable
                              ? 'text-[var(--success)]'
                              : 'text-[var(--content-faint)]',
                          )}
                        />
                      )}
                      <span className="min-w-0 flex-1 truncate text-[var(--content)]">
                        {entry.name}
                      </span>
                      {entry.selectable && !entry.is_dir ? (
                        <span className="shrink-0 rounded bg-[color-mix(in_oklab,var(--success)_14%,transparent)] px-1 py-px text-[10px] text-[var(--success)]">
                          可执行
                        </span>
                      ) : null}
                    </div>
                  </li>
                )
              })}
            </ul>
          )}
        </div>

        <p className="text-xs text-[var(--content-faint)]">
          单击目录进入, 双击文件直接选中. 隐藏文件默认不显示.
        </p>
      </div>
    </Dialog>
  )
}
