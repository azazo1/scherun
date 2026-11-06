import { useCallback, useEffect, useMemo, useState } from 'react'
import { ChevronRight, Eye, File, Folder, FolderUp, HardDrive, Home } from 'lucide-react'
import { appApi } from '@/api/client'
import type { FsListing } from '@/api/types'
import { deriveFilter } from '@/lib/path-filter'
import { cn } from '@/lib/utils'
import { Button } from './ui/Button'
import { Input } from './ui/Input'
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
  // 路径输入框的内容, 与浏览到的目录保持同步, 也允许用户直接改写.
  const [pathInput, setPathInput] = useState('')
  // 是否列出隐藏条目, 打开 ~/.local/bin 这类路径时必须开启.
  const [showHidden, setShowHidden] = useState(false)

  const load = useCallback(
    // keepInput 为真时保留用户正在输入的路径.
    //
    // 自动切换隐藏显示时会重新加载当前目录, 此时若同步输入框,
    // 用户刚敲下的 "/." 会被覆盖成当前目录, 后缀直接丢失.
    async (path?: string, hidden = showHidden, keepInput = false) => {
      setLoading(true)
      setError(null)
      try {
        const result = await appApi.listDir(path, hidden)
        setListing(result)
        // 请求的是文件时后端会把它放在 selected 里, 直接作为待选项.
        setSelected(result.selected ?? null)
        if (!keepInput) {
          setPathInput(result.selected ?? result.path)
        }
      } catch (err) {
        setError(err instanceof Error ? err.message : '无法读取目录')
      } finally {
        setLoading(false)
      }
    },
    [showHidden],
  )

  // filter 是输入框里最后一段的文本, 用于就地过滤当前目录.
  //
  // 用户正在逐字输入路径时, 把它当成前缀过滤, 而不是每次都去请求目录,
  // 这样输入过程中列表会实时收窄, 回车才真正跳转.
  const { filter, isHidden: filterIsHidden } = useMemo(
    () => deriveFilter(pathInput, listing?.path),
    [pathInput, listing?.path],
  )

  // 最后一段以点开头时, 说明用户在找隐藏条目, 自动把隐藏项显示出来.
  useEffect(() => {
    if (filterIsHidden && !showHidden) {
      setShowHidden(true)
      // 保留输入: 用户正在敲路径, 不能被重新加载覆盖.
      void load(listing?.path, true, true)
    }
  }, [filterIsHidden, showHidden, listing?.path, load])

  // visibleEntries 按最后一段过滤当前目录.
  const visibleEntries = useMemo(() => {
    const entries = listing?.entries ?? []
    if (!filter) return entries
    const needle = filter.toLowerCase()
    return entries.filter((entry) => entry.name.toLowerCase().startsWith(needle))
  }, [listing?.entries, filter])

  // submitPath 把输入框里的路径交给后端解析.
  //
  // 不靠扩展名猜是文件还是目录: /usr/local/bin/python3 这类可执行文件
  // 没有扩展名, 猜错就会跳到它的父目录. 统一由后端 stat 后判定,
  // 若是文件则回填 selected, 由用户确认后再点 "选择".
  const submitPath = () => {
    const raw = pathInput.trim()
    if (!raw) return
    void load(raw)
  }

  useEffect(() => {
    if (open) {
      setShowHidden(false)
      setPathInput('')
      void load(undefined, false)
    }
    // 只在打开时重置, load 的身份变化不应重新触发.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open])

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
          <Button
            variant={showHidden ? 'secondary' : 'ghost'}
            size="sm"
            title="显示以点开头的隐藏文件与目录"
            onClick={() => {
              const next = !showHidden
              setShowHidden(next)
              // 只切换隐藏条目的显示, 不动用户已经输入的路径.
              void load(listing?.path, next, true)
            }}
          >
            <Eye className="h-3.5 w-3.5" />
            {showHidden ? '隐藏已显示' : '显示隐藏'}
          </Button>
        </div>

        {/* 路径输入: 可以直接粘贴或手输绝对路径, 回车跳转. */}
        <div className="flex gap-2">
          <Input
            value={pathInput}
            placeholder="直接输入或粘贴绝对路径, 回车跳转"
            className="font-mono text-[12px]"
            onChange={(event) => setPathInput(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === 'Enter') {
                event.preventDefault()
                submitPath()
              }
            }}
          />
          <Button variant="outline" size="md" onClick={submitPath}>
            跳转
          </Button>
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
          ) : visibleEntries.length === 0 ? (
            <p className="p-4 text-sm text-[var(--content-faint)]">
              {listing && listing.entries.length > 0
                ? `没有匹配 "${filter}" 的条目`
                : '此目录为空'}
            </p>
          ) : (
            <ul className="divide-y divide-[var(--border-subtle)]">
              {visibleEntries.map((entry) => {
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
