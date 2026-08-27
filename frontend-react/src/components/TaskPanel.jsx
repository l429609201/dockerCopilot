import React, { useState, useRef, useEffect } from 'react'
import { Loader2, CheckCircle, XCircle, X, Ban, Trash2, ListTodo, Activity, ChevronRight, ChevronDown, Layers } from 'lucide-react'
import { useTasks } from '../hooks/useTasks.jsx'
import { progressAPI } from '../api/client.js'
import { cn } from '../utils/cn.js'

// 任务中心：右下角常驻悬浮球，点开从右侧滑出全高抽屉（MoviePilot 智能助手样式）。
// 展示所有后台任务（更新/恢复/镜像/Compose/定时更新/清理）。
// 修改：有新任务时不自动弹开，只显示红色角标提示。
// 支持右滑关闭手势（移动端优化）。
export function TaskPanel() {
  const { tasks, removeTask } = useTasks()
  const [open, setOpen] = useState(false)
  const drawerRef = useRef(null)
  const touchStartX = useRef(0)
  const touchCurrentX = useRef(0)
  const [isDragging, setIsDragging] = useState(false)
  const [dragOffset, setDragOffset] = useState(0)

  const running = tasks.filter(t => !t.isDone)
  const done = tasks.filter(t => t.isDone)
  const runningCount = running.length

  // 取消运行中任务：调用后端 cancel，SSE 会自动刷新状态
  const handleCancel = async (id) => {
    try { await progressAPI.cancelProgress(id) } catch (e) { console.error('取消任务失败:', e) }
  }
  const clearDone = () => done.forEach(t => removeTask(t.id))

  // 右滑关闭手势处理
  useEffect(() => {
    const drawer = drawerRef.current
    if (!drawer || !open) return

    const handleTouchStart = (e) => {
      touchStartX.current = e.touches[0].clientX
      touchCurrentX.current = e.touches[0].clientX
      setIsDragging(true)
    }

    const handleTouchMove = (e) => {
      if (!isDragging) return
      touchCurrentX.current = e.touches[0].clientX
      const diff = touchCurrentX.current - touchStartX.current
      // 仅允许向右滑动（diff > 0）
      if (diff > 0) {
        setDragOffset(diff)
      }
    }

    const handleTouchEnd = () => {
      if (!isDragging) return
      const diff = touchCurrentX.current - touchStartX.current
      // 滑动超过 100px 或速度够快则关闭
      if (diff > 100) {
        setOpen(false)
      }
      setIsDragging(false)
      setDragOffset(0)
    }

    drawer.addEventListener('touchstart', handleTouchStart, { passive: true })
    drawer.addEventListener('touchmove', handleTouchMove, { passive: true })
    drawer.addEventListener('touchend', handleTouchEnd, { passive: true })

    return () => {
      drawer.removeEventListener('touchstart', handleTouchStart)
      drawer.removeEventListener('touchmove', handleTouchMove)
      drawer.removeEventListener('touchend', handleTouchEnd)
    }
  }, [open, isDragging])

  return (
    <>
      {/* 右下角常驻悬浮球
          移动端(<md)底部有胶囊导航栏(约88px)，故抬高到导航栏上方(bottom-24=96px)避免遮挡"设置"按钮；
          桌面端(md+)无底部导航，恢复 bottom-5。 */}
      <button
        onClick={() => setOpen(true)}
        className={cn(
          'fixed right-5 z-40 h-14 w-14 rounded-full text-white shadow-xl flex items-center justify-center transition-all hover:scale-105 active:scale-95',
          'bottom-24 md:bottom-5',
          'bg-gradient-to-br from-primary-500 to-primary-700',
          open && 'opacity-0 pointer-events-none'
        )}
        title="任务中心"
      >
        {runningCount > 0 ? <Loader2 className="h-6 w-6 animate-spin" /> : <ListTodo className="h-6 w-6" />}
        {runningCount > 0 && (
          <span className="absolute -top-1 -right-1 min-w-[20px] h-5 px-1 rounded-full bg-red-500 text-white text-xs font-bold flex items-center justify-center ring-2 ring-white dark:ring-gray-900">
            {runningCount}
          </span>
        )}
      </button>

      {/* 右侧全高抽屉（移除灰色遮罩） */}
      <aside
        ref={drawerRef}
        className={cn(
          'fixed top-0 right-0 z-50 h-full w-full sm:w-[420px] bg-white dark:bg-gray-900 shadow-2xl flex flex-col',
          isDragging ? '' : 'transition-transform duration-300 ease-out',
          open ? 'translate-x-0' : 'translate-x-full'
        )}
        style={isDragging ? { transform: `translateX(${dragOffset}px)` } : {}}
      >
        {/* 顶部标题栏 - 手机端增加安全区域适配，下移避免刘海/状态栏遮挡 */}
        <div className="flex items-center justify-between px-5 py-4 pt-safe bg-gradient-to-r from-primary-500 to-primary-600 text-white">
          <div className="flex items-center gap-3 min-w-0">
            <div className="h-10 w-10 rounded-full bg-white/20 flex items-center justify-center flex-shrink-0">
              <Activity className="h-5 w-5" />
            </div>
            <div className="min-w-0">
              <div className="font-semibold truncate">任务中心</div>
              <div className="text-xs text-white/80 truncate">
                {runningCount > 0 ? `${runningCount} 个任务执行中` : '随时待命'}
              </div>
            </div>
          </div>
          <div className="flex items-center gap-1 flex-shrink-0">
            {done.length > 0 && (
              <button onClick={clearDone} title="清除已完成" className="p-2 hover:bg-white/20 rounded-lg transition-colors">
                <Trash2 className="h-4 w-4" />
              </button>
            )}
            <button onClick={() => setOpen(false)} title="关闭" className="p-2 hover:bg-white/20 rounded-lg transition-colors">
              <X className="h-5 w-5" />
            </button>
          </div>
        </div>

        {/* 任务列表主体 */}
        <div className="flex-1 overflow-y-auto">
          {tasks.length === 0 ? (
            <div className="h-full flex flex-col items-center justify-center text-center px-8 text-gray-400">
              <ListTodo className="h-12 w-12 mb-3 opacity-40" />
              <p className="text-sm">暂无任务</p>
              <p className="text-xs mt-1 text-gray-400/80">容器更新、镜像拉取、定时任务等都会在这里实时显示</p>
            </div>
          ) : (
            <>
              {running.length > 0 && (
                <TaskGroup title="执行中" count={running.length}>
                  {running.map(t => (
                    <TaskRow key={t.id} task={t} onRemove={() => removeTask(t.id)} onCancel={() => handleCancel(t.id)} />
                  ))}
                </TaskGroup>
              )}
              {done.length > 0 && (
                <TaskGroup title="已结束" count={done.length}>
                  {done.map(t => (
                    <TaskRow key={t.id} task={t} onRemove={() => removeTask(t.id)} onCancel={() => handleCancel(t.id)} />
                  ))}
                </TaskGroup>
              )}
            </>
          )}
        </div>
      </aside>
    </>
  )
}

// 任务分组标题（吸顶）
function TaskGroup({ title, count, children }) {
  return (
    <div>
      <div className="sticky top-0 z-10 px-5 py-2 bg-gray-50/95 dark:bg-gray-800/95 backdrop-blur text-xs font-medium text-gray-500 dark:text-gray-400 border-b border-gray-100 dark:border-gray-700">
        {title}（{count}）
      </div>
      <div className="divide-y divide-gray-100 dark:divide-gray-800">{children}</div>
    </div>
  )
}

function TaskRow({ task, onRemove, onCancel }) {
  const { percentage = 0, message, detailMsg, isDone, failed, canceled, title, layers = [] } = task
  // 分层子进度展开状态（有分层数据时才可展开）
  const [expanded, setExpanded] = useState(false)
  const hasLayers = Array.isArray(layers) && layers.length > 0
  return (
    <div className="px-5 py-3 hover:bg-gray-50 dark:hover:bg-gray-800/50 transition-colors">
      <div className="flex items-start justify-between gap-2 mb-1">
        <div className="flex items-center gap-2 min-w-0">
          {/* 展开/折叠按钮：仅有分层数据时显示 */}
          {hasLayers && (
            <button onClick={() => setExpanded(v => !v)} className="p-0.5 -ml-1 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 flex-shrink-0" title={expanded ? '收起分层' : '展开分层'}>
              {expanded ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
            </button>
          )}
          {!isDone && <Loader2 className="h-3.5 w-3.5 animate-spin text-primary-500 flex-shrink-0" />}
          {isDone && !failed && <CheckCircle className="h-3.5 w-3.5 text-emerald-500 flex-shrink-0" />}
          {isDone && failed && <XCircle className="h-3.5 w-3.5 text-red-500 flex-shrink-0" />}
          <span className="text-sm font-medium text-gray-900 dark:text-white break-all">{title}</span>
          {/* 分层数量徽标 */}
          {hasLayers && (
            <span className="inline-flex items-center gap-0.5 px-1 rounded text-[10px] text-gray-500 bg-gray-100 dark:bg-gray-700 flex-shrink-0">
              <Layers className="h-2.5 w-2.5" />{layers.length}
            </span>
          )}
        </div>
        <div className="flex items-center gap-1.5 flex-shrink-0">
          {!isDone && <span className="text-xs text-gray-500 tabular-nums">{Math.round(percentage)}%</span>}
          {!isDone && (
            <button onClick={onCancel} className="p-0.5 text-gray-400 hover:text-red-500" title="取消任务">
              <Ban className="h-3.5 w-3.5" />
            </button>
          )}
          {isDone && (
            <button onClick={onRemove} className="p-0.5 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200" title="移除">
              <X className="h-3.5 w-3.5" />
            </button>
          )}
        </div>
      </div>
      {/* 进度条 */}
      {!isDone && (
        <div className="h-1.5 bg-gray-100 dark:bg-gray-700 rounded-full overflow-hidden mb-1.5">
          <div className="h-full bg-primary-500 transition-all duration-300" style={{ width: `${Math.min(100, percentage)}%` }} />
        </div>
      )}
      {/* 当前阶段：完整换行显示，不截断 */}
      {message && (
        <p className="text-xs text-gray-600 dark:text-gray-300 break-words whitespace-pre-wrap">{message}</p>
      )}
      {/* 明细/结果：完整换行显示，不截断 */}
      {detailMsg && (
        <p className={cn('text-xs mt-0.5 break-words whitespace-pre-wrap',
          failed ? 'text-red-500' : 'text-gray-500 dark:text-gray-400')}>
          {detailMsg}
        </p>
      )}
      {canceled && <p className="text-xs mt-0.5 text-amber-500">已取消</p>}

      {/* 分层子进度：展开后逐层显示，层数随 SSE 数据自动增减 */}
      {hasLayers && expanded && (
        <div className="mt-2 pl-4 border-l-2 border-gray-100 dark:border-gray-700 space-y-1.5">
          {layers.map((ly) => (
            <LayerRow key={ly.id} layer={ly} />
          ))}
        </div>
      )}
    </div>
  )
}

// 单个镜像分层进度行：层ID + 状态 + 字节 + 迷你进度条
function LayerRow({ layer }) {
  const { id, status, current = 0, total = 0, percentage = 0 } = layer
  const done = percentage >= 100
  return (
    <div>
      <div className="flex items-center justify-between gap-2 text-[11px]">
        <span className="font-mono text-gray-500 dark:text-gray-400 flex-shrink-0">{id}</span>
        <span className={cn('truncate flex-1 text-right', done ? 'text-emerald-500' : 'text-gray-500 dark:text-gray-400')}>
          {status}{total > 0 ? ` · ${formatBytes(current)}/${formatBytes(total)}` : ''}
        </span>
      </div>
      <div className="h-1 bg-gray-100 dark:bg-gray-700 rounded-full overflow-hidden mt-0.5">
        <div className={cn('h-full transition-all duration-300', done ? 'bg-emerald-500' : 'bg-primary-400')}
          style={{ width: `${Math.min(100, percentage)}%` }} />
      </div>
    </div>
  )
}

// 字节数格式化为人类可读大小
function formatBytes(bytes) {
  if (!bytes || bytes < 0) return '0B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let n = bytes, i = 0
  while (n >= 1024 && i < units.length - 1) { n /= 1024; i++ }
  return `${n.toFixed(i === 0 ? 0 : 1)}${units[i]}`
}
