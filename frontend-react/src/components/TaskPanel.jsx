import React, { useState, useRef, useCallback } from 'react'
import { Loader2, CheckCircle, XCircle, ChevronDown, ChevronUp, X, Ban, Maximize2, Minimize2, Trash2, GripHorizontal } from 'lucide-react'
import { useTasks } from '../hooks/useTasks.jsx'
import { progressAPI } from '../api/client.js'
import { cn } from '../utils/cn.js'

// 全局任务浮层：可拖动、可放大/折叠、明细完整显示、支持取消运行中任务。
export function TaskPanel() {
  const { tasks, removeTask } = useTasks()
  const [collapsed, setCollapsed] = useState(false)
  const [expanded, setExpanded] = useState(false)
  const [pos, setPos] = useState(null) // 拖动后生效的 {x,y}；null 时默认右下角
  const panelRef = useRef(null)
  const dragRef = useRef(null)

  // 拖动：按住标题栏移动整个浮层，位置限制在视口内
  const onDragStart = useCallback((e) => {
    if (e.target.closest('button')) return // 点按钮不触发拖动
    const rect = panelRef.current?.getBoundingClientRect()
    if (!rect) return
    dragRef.current = { dx: e.clientX - rect.left, dy: e.clientY - rect.top, w: rect.width, h: rect.height }
    const onMove = (ev) => {
      const { dx, dy, w, h } = dragRef.current
      const x = Math.min(Math.max(0, ev.clientX - dx), window.innerWidth - w)
      const y = Math.min(Math.max(0, ev.clientY - dy), window.innerHeight - h)
      setPos({ x, y })
    }
    const onUp = () => {
      window.removeEventListener('mousemove', onMove)
      window.removeEventListener('mouseup', onUp)
    }
    window.addEventListener('mousemove', onMove)
    window.addEventListener('mouseup', onUp)
  }, [])

  if (tasks.length === 0) return null

  const running = tasks.filter(t => !t.isDone)
  const runningCount = running.length
  const doneCount = tasks.length - runningCount

  // 取消运行中任务：调用后端 cancel，轮询会自动把状态刷新为已取消/已完成
  const handleCancel = async (id) => {
    try { await progressAPI.cancelProgress(id) } catch (e) { console.error('取消任务失败:', e) }
  }
  // 一键清除所有已完成任务
  const clearDone = () => tasks.filter(t => t.isDone).forEach(t => removeTask(t.id))

  // 拖动后改用 left/top 绝对定位，否则默认钉在右下角
  const posStyle = pos
    ? { left: pos.x, top: pos.y, right: 'auto', bottom: 'auto' }
    : { right: 16, bottom: 16 }
  const widthCls = expanded ? 'w-[34rem]' : 'w-96'

  return (
    <div ref={panelRef} className={cn('fixed z-50 max-w-[calc(100vw-2rem)]', widthCls)} style={posStyle}>
      <div className="bg-white dark:bg-gray-800 rounded-xl shadow-2xl border border-gray-200 dark:border-gray-700 overflow-hidden flex flex-col">
        {/* 头部：按住可拖动 */}
        <div onMouseDown={onDragStart}
          className="flex items-center justify-between px-4 py-2.5 bg-gradient-to-r from-primary-500 to-primary-600 text-white cursor-move select-none">
          <div className="flex items-center gap-2 min-w-0">
            <GripHorizontal className="h-4 w-4 opacity-70 flex-shrink-0" />
            {runningCount > 0 && <Loader2 className="h-4 w-4 animate-spin flex-shrink-0" />}
            <span className="text-sm font-medium truncate">
              {runningCount > 0 ? `${runningCount} 个任务执行中` : `全部完成（${tasks.length}）`}
            </span>
          </div>
          <div className="flex items-center gap-1 flex-shrink-0">
            {doneCount > 0 && (
              <button onClick={clearDone} title="清除已完成" className="p-1 hover:bg-white/20 rounded">
                <Trash2 className="h-4 w-4" />
              </button>
            )}
            <button onClick={() => setExpanded(v => !v)} title={expanded ? '缩小' : '放大'} className="p-1 hover:bg-white/20 rounded">
              {expanded ? <Minimize2 className="h-4 w-4" /> : <Maximize2 className="h-4 w-4" />}
            </button>
            <button onClick={() => setCollapsed(c => !c)} title={collapsed ? '展开' : '折叠'} className="p-1 hover:bg-white/20 rounded">
              {collapsed ? <ChevronUp className="h-4 w-4" /> : <ChevronDown className="h-4 w-4" />}
            </button>
          </div>
        </div>

        {/* 任务列表 */}
        {!collapsed && (
          <div className={cn('overflow-y-auto divide-y divide-gray-100 dark:divide-gray-700',
            expanded ? 'max-h-[70vh]' : 'max-h-96')}>
            {tasks.map(task => (
              <TaskRow key={task.id} task={task}
                onRemove={() => removeTask(task.id)}
                onCancel={() => handleCancel(task.id)} />
            ))}
          </div>
        )}
      </div>
    </div>
  )
}

function TaskRow({ task, onRemove, onCancel }) {
  const { percentage = 0, message, detailMsg, isDone, failed, canceled, title } = task
  return (
    <div className="px-4 py-3">
      <div className="flex items-start justify-between gap-2 mb-1">
        <div className="flex items-center gap-2 min-w-0">
          {!isDone && <Loader2 className="h-3.5 w-3.5 animate-spin text-primary-500 flex-shrink-0" />}
          {isDone && !failed && <CheckCircle className="h-3.5 w-3.5 text-emerald-500 flex-shrink-0" />}
          {isDone && failed && <XCircle className="h-3.5 w-3.5 text-red-500 flex-shrink-0" />}
          <span className="text-sm font-medium text-gray-900 dark:text-white break-all">{title}</span>
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
    </div>
  )
}
