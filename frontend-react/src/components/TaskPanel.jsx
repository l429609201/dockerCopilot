import React, { useState } from 'react'
import { Loader2, CheckCircle, XCircle, ChevronDown, ChevronUp, X } from 'lucide-react'
import { useTasks } from '../hooks/useTasks.jsx'
import { cn } from '../utils/cn.js'

// 全局任务浮层：右下角常驻，任意页面都能看到运行中任务及进度。
export function TaskPanel() {
  const { tasks, removeTask } = useTasks()
  const [collapsed, setCollapsed] = useState(false)

  if (tasks.length === 0) return null

  const running = tasks.filter(t => !t.isDone)
  const runningCount = running.length

  return (
    <div className="fixed bottom-4 right-4 z-50 w-80 max-w-[calc(100vw-2rem)]">
      <div className="bg-white dark:bg-gray-800 rounded-xl shadow-2xl border border-gray-200 dark:border-gray-700 overflow-hidden">
        {/* 头部 */}
        <div className="flex items-center justify-between px-4 py-2.5 bg-gradient-to-r from-primary-500 to-primary-600 text-white">
          <div className="flex items-center gap-2">
            {runningCount > 0 && <Loader2 className="h-4 w-4 animate-spin" />}
            <span className="text-sm font-medium">
              {runningCount > 0 ? `${runningCount} 个任务执行中` : '任务完成'}
            </span>
          </div>
          <button onClick={() => setCollapsed(c => !c)} className="p-1 hover:bg-white/20 rounded">
            {collapsed ? <ChevronUp className="h-4 w-4" /> : <ChevronDown className="h-4 w-4" />}
          </button>
        </div>

        {/* 任务列表 */}
        {!collapsed && (
          <div className="max-h-72 overflow-y-auto divide-y divide-gray-100 dark:divide-gray-700">
            {tasks.map(task => (
              <TaskRow key={task.id} task={task} onClose={() => removeTask(task.id)} />
            ))}
          </div>
        )}
      </div>
    </div>
  )
}

function TaskRow({ task, onClose }) {
  const { percentage = 0, message, detailMsg, isDone, failed, title } = task
  return (
    <div className="px-4 py-3">
      <div className="flex items-center justify-between gap-2 mb-1">
        <div className="flex items-center gap-2 min-w-0">
          {!isDone && <Loader2 className="h-3.5 w-3.5 animate-spin text-primary-500 flex-shrink-0" />}
          {isDone && !failed && <CheckCircle className="h-3.5 w-3.5 text-emerald-500 flex-shrink-0" />}
          {isDone && failed && <XCircle className="h-3.5 w-3.5 text-red-500 flex-shrink-0" />}
          <span className="text-sm font-medium text-gray-900 dark:text-white truncate">{title}</span>
        </div>
        <div className="flex items-center gap-1 flex-shrink-0">
          {!isDone && <span className="text-xs text-gray-500">{Math.round(percentage)}%</span>}
          {isDone && (
            <button onClick={onClose} className="p-0.5 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200" title="移除">
              <X className="h-3.5 w-3.5" />
            </button>
          )}
        </div>
      </div>
      {/* 进度条 */}
      {!isDone && (
        <div className="h-1.5 bg-gray-100 dark:bg-gray-700 rounded-full overflow-hidden mb-1">
          <div className="h-full bg-primary-500 transition-all duration-300" style={{ width: `${Math.min(100, percentage)}%` }} />
        </div>
      )}
      {/* 明细/结果 */}
      <p className={cn("text-xs truncate", failed ? "text-red-500" : "text-gray-500 dark:text-gray-400")}
        title={detailMsg || message}>
        {detailMsg || message}
      </p>
    </div>
  )
}
