import React, { useState, useRef } from 'react'
import { Loader2, CheckCircle, XCircle, X, Ban, Trash2, ListTodo, Activity } from 'lucide-react'
import { useTasks } from '../hooks/useTasks.jsx'
import { progressAPI } from '../api/client.js'
import { cn } from '../utils/cn.js'

// 任务中心：右下角常驻悬浮球，点开从右侧滑出全高抽屉（MoviePilot 智能助手样式）。
// 展示所有后台任务（更新/恢复/镜像/Compose/定时更新/清理）。
// 修改：有新任务时不自动弹开，只显示红色角标提示。
export function TaskPanel() {
  const { tasks, removeTask } = useTasks()
  const [open, setOpen] = useState(false)

  const running = tasks.filter(t => !t.isDone)
  const done = tasks.filter(t => t.isDone)
  const runningCount = running.length

  // 取消运行中任务：调用后端 cancel，SSE 会自动刷新状态
  const handleCancel = async (id) => {
    try { await progressAPI.cancelProgress(id) } catch (e) { console.error('取消任务失败:', e) }
  }
  const clearDone = () => done.forEach(t => removeTask(t.id))

  return (
    <>
      {/* 右下角常驻悬浮球 */}
      <button
        onClick={() => setOpen(true)}
        className={cn(
          'fixed bottom-5 right-5 z-40 h-14 w-14 rounded-full text-white shadow-xl flex items-center justify-center transition-all hover:scale-105 active:scale-95',
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
        className={cn(
          'fixed top-0 right-0 z-50 h-full w-full sm:w-[420px] bg-white dark:bg-gray-900 shadow-2xl flex flex-col transition-transform duration-300 ease-out',
          open ? 'translate-x-0' : 'translate-x-full'
        )}
      >
        {/* 顶部标题栏 */}
        <div className="flex items-center justify-between px-5 py-4 bg-gradient-to-r from-primary-500 to-primary-600 text-white">
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
  const { percentage = 0, message, detailMsg, isDone, failed, canceled, title } = task
  return (
    <div className="px-5 py-3 hover:bg-gray-50 dark:hover:bg-gray-800/50 transition-colors">
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
