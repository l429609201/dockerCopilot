import React from 'react'
import { Play, Square, RotateCcw, Upload, RefreshCw, FileText, TerminalSquare, FolderOpen } from 'lucide-react'
import { cn } from '../utils/cn.js'
import { formatRunningTime } from '../utils/format.js'
import { ContainerStats } from './ContainerStats.jsx'

// 容器列表行：横向一条展示（图标 + 名称/镜像 + 资源 + 状态 + 操作按钮）
// 所有交互通过 props 回调，保持与卡片视图一致的行为。
export function ContainerListRow({
  container, iconUrl, selected, batchMode, actionState, stat,
  onOpen, onToggleSelect, onAction, onUpdate, onOps, onFiles,
}) {
  const running = container.status === 'running'
  const loading = actionState?.loading

  return (
    <div
      onClick={(e) => {
        if (e.metaKey || e.ctrlKey || batchMode) {
          e.stopPropagation(); onToggleSelect(container.id)
        } else {
          onOpen(container)
        }
      }}
      className={cn(
        "flex items-center gap-3 px-3 py-2.5 bg-white dark:bg-gray-800 border rounded-xl cursor-pointer transition-all hover:shadow-sm",
        selected ? "border-primary-400 dark:border-primary-600 ring-1 ring-primary-300" : "border-gray-200 dark:border-gray-700"
      )}
    >
      {/* 批量选择框 */}
      {batchMode && (
        <input type="checkbox" checked={selected} readOnly
          className="rounded border-gray-300 flex-shrink-0" />
      )}

      {/* 图标 */}
      <div className="flex-shrink-0">
        {iconUrl ? (
          <img src={iconUrl} alt={container.name}
            className="h-9 w-9 rounded-lg object-cover"
            onError={(e) => { e.target.style.display = 'none' }} />
        ) : (
          <div className="h-9 w-9 rounded-lg bg-gradient-to-r from-blue-500 to-purple-600 flex items-center justify-center text-white text-sm font-bold">
            {(container.name || '?').charAt(0).toUpperCase()}
          </div>
        )}
      </div>

      {/* 状态圆点 */}
      <span className={cn("flex-shrink-0 w-2.5 h-2.5 rounded-full",
        running ? "bg-emerald-500" : "bg-gray-400")} title={running ? '运行中' : '已停止'} />

      {/* 名称 + 镜像 */}
      <div className="min-w-0 flex-1">
        <div className="font-medium text-gray-900 dark:text-white truncate">{container.name}</div>
        <div className="text-xs text-gray-500 dark:text-gray-400 truncate">{container.usingImage}</div>
      </div>

      {/* 资源监控：CPU%+内存%（大屏），流量（超大屏），仅运行中显示 */}
      {running && (
        <div className="hidden lg:block flex-shrink-0">
          <ContainerStats stat={stat} variant="list" />
        </div>
      )}

      {/* 运行时间/状态（中大屏显示） */}
      <div className="hidden md:block text-xs text-gray-500 dark:text-gray-400 flex-shrink-0 w-28 text-right truncate">
        {running ? `运行 ${formatRunningTime(container.runningTime)}` : '已停止'}
      </div>

      {/* 操作按钮 */}
      {!batchMode && (
        <div className="flex items-center gap-1 flex-shrink-0" onClick={(e) => e.stopPropagation()}>
          {/* 日志 / 控制台 快捷入口 */}
          {onOps && (
            <>
              <IconBtn onClick={() => onOps('logs')} icon={FileText} title="查看日志" color="gray" />
              <IconBtn onClick={() => onOps('exec')} icon={TerminalSquare} title="控制台" color="gray" />
              {onFiles && <IconBtn onClick={onFiles} icon={FolderOpen} title="文件管理" color="gray" />}
            </>
          )}
          {loading ? (
            <span className="flex items-center gap-1 px-2 py-1 text-xs text-primary-600 dark:text-primary-400 max-w-[220px]"
              title={actionState.detailMsg || ''}>
              <RefreshCw className="h-4 w-4 animate-spin flex-shrink-0" />
              <span className="truncate">
                {actionState.action === 'update' && actionState.percentage ? `${Math.round(actionState.percentage)}%` : '处理中'}
                {actionState.detailMsg ? ` · ${actionState.detailMsg}` : ''}
              </span>
            </span>
          ) : (
            <>
              {running ? (
                <>
                  <IconBtn onClick={() => onAction(container.id, 'stop')} icon={Square} title="停止" color="red" />
                  <IconBtn onClick={() => onAction(container.id, 'restart')} icon={RotateCcw} title="重启" color="blue" />
                </>
              ) : (
                <IconBtn onClick={() => onAction(container.id, 'start')} icon={Play} title="启动" color="green" />
              )}
              <IconBtn onClick={() => onUpdate(container.id)} icon={Upload} title="更新"
                color={container.haveUpdate ? "yellow" : "purple"} />
            </>
          )}
        </div>
      )}
    </div>
  )
}

function IconBtn({ onClick, icon: Icon, title, color }) {
  const colors = {
    red: "text-red-600 hover:bg-red-50 dark:hover:bg-red-900/20",
    blue: "text-blue-600 hover:bg-blue-50 dark:hover:bg-blue-900/20",
    green: "text-green-600 hover:bg-green-50 dark:hover:bg-green-900/20",
    purple: "text-purple-600 hover:bg-purple-50 dark:hover:bg-purple-900/20",
    yellow: "text-yellow-600 hover:bg-yellow-50 dark:hover:bg-yellow-900/20",
    gray: "text-gray-500 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-700",
  }
  return (
    <button onClick={onClick} title={title}
      className={cn("p-2 rounded-lg transition-colors active:scale-95", colors[color])}>
      <Icon className="h-4 w-4" />
    </button>
  )
}
