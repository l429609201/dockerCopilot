import React, { useState, useEffect } from 'react'
import { Play, Square, RotateCcw, Upload, RefreshCw, FileText, TerminalSquare, FolderOpen, MoreVertical, Edit3, Activity, Network, Info, Trash2 } from 'lucide-react'
import { cn } from '../utils/cn.js'
import { formatRunningTime } from '../utils/format.js'
import { stateLabel, stateDotColor } from '../utils/containerState.js'
import { ContainerStats } from './ContainerStats.jsx'

// 容器列表行：横向一条展示（图标 + 名称/镜像 + 资源 + 状态 + 操作按钮）
// 所有交互通过 props 回调，保持与卡片视图一致的行为。
export function ContainerListRow({
  container, iconUrl, selected, batchMode, actionState, stat,
  onOpen, onToggleSelect, onAction, onUpdate, onOps, onFiles, onEdit, onProcess, onDelete,
}) {
  const running = container.status === 'running'
  const loading = actionState?.loading
  // 更新进度：仅 action==='update' 且加载中时显示进度条（与卡片模式一致）
  const updating = loading && actionState?.action === 'update'
  const pct = Math.min(100, Math.max(0, actionState?.percentage || 0))
  const [menuOpen, setMenuOpen] = useState(false)
  // 图标加载失败标记：失败后回退到首字母占位，避免直接隐藏留空
  const [iconError, setIconError] = useState(false)
  // iconUrl 变化时重置失败标记（如 favicon 后续才抓取到），让新地址有机会重新加载
  useEffect(() => { setIconError(false) }, [iconUrl])

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
        "relative overflow-hidden flex items-center gap-3 px-3 py-2.5 bg-white dark:bg-gray-800 border rounded-xl cursor-pointer transition-all hover:shadow-sm",
        selected ? "border-primary-400 dark:border-primary-600 ring-1 ring-primary-300" : "border-gray-200 dark:border-gray-700"
      )}
    >
      {/* 整行背景进度条：更新时整行背景按百分比渐变填充 + shimmer 微光动画（与卡片模式一致） */}
      {updating && (
        <div className="absolute inset-0 pointer-events-none rounded-xl overflow-hidden z-0">
          <div
            className="absolute top-0 left-0 bottom-0 bg-gradient-to-r from-primary-500/25 via-primary-400/25 to-primary-500/25 transition-all duration-500 ease-out"
            style={{ width: `${pct}%` }}
          >
            <div className="absolute inset-0 bg-gradient-to-r from-transparent via-white/10 to-transparent"
              style={{ backgroundSize: '200% 100%', animation: 'shimmer 2s infinite linear' }} />
          </div>
        </div>
      )}

      {/* 批量选择框 */}
      {batchMode && (
        <input type="checkbox" checked={selected} readOnly
          className="rounded border-gray-300 flex-shrink-0" />
      )}

      {/* 图标：有 url 且未加载失败时显示图片，否则回退首字母占位 */}
      <div className="flex-shrink-0">
        {iconUrl && !iconError ? (
          <img
            src={iconUrl}
            alt={container.name}
            className="h-9 w-9 rounded-lg object-cover"
            onError={() => setIconError(true)}
            loading="lazy"
          />
        ) : (
          <div className="h-9 w-9 rounded-lg bg-gradient-to-r from-blue-500 to-purple-600 flex items-center justify-center text-white text-sm font-bold">
            {(container.name || '?').charAt(0).toUpperCase()}
          </div>
        )}
      </div>

      {/* 状态圆点：按 Docker 原生 State 上色并提示具体状态，不再笼统显示「已停止」 */}
      <span className={cn("flex-shrink-0 w-2.5 h-2.5 rounded-full", stateDotColor(container.status))}
        title={stateLabel(container.status)} />

      {/* 名称 + 镜像 */}
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-1.5 min-w-0">
          <div className="font-medium text-gray-900 dark:text-white truncate">{container.name}</div>
          {/* 来源主机标识：非本地容器才展示 */}
          {container.hostName && container.hostId && container.hostId !== 'local' && (
            <span className="flex-shrink-0 inline-flex items-center gap-0.5 rounded bg-purple-100 dark:bg-purple-900/40 text-purple-600 dark:text-purple-300 px-1.5 py-0.5 text-[10px] font-medium">
              <Network className="h-2.5 w-2.5" />
              {container.hostName}
            </span>
          )}
        </div>
        <div className="text-xs text-gray-500 dark:text-gray-400 truncate">{container.usingImage}</div>
      </div>

      {/* 端口映射（中大屏显示）：最多 2 条，多余折叠为 +N */}
      {Array.isArray(container.portMappings) && container.portMappings.length > 0 && (
        <div className="hidden md:flex items-center gap-1 flex-shrink-0 max-w-[200px]" title={container.portMappings.join('\n')}>
          {container.portMappings.slice(0, 2).map((m) => (
            <span key={m} className="inline-flex items-center rounded bg-blue-50 dark:bg-blue-900/30 text-blue-600 dark:text-blue-300 px-1.5 py-0.5 text-[10px] font-mono whitespace-nowrap">
              {m}
            </span>
          ))}
          {container.portMappings.length > 2 && (
            <span className="text-[10px] text-gray-400">+{container.portMappings.length - 2}</span>
          )}
        </div>
      )}

      {/* 资源监控：CPU%+内存%（大屏），流量（超大屏），仅运行中显示 */}
      {running && (
        <div className="hidden lg:block flex-shrink-0">
          <ContainerStats stat={stat} variant="list" />
        </div>
      )}

      {/* 运行时间/状态（中大屏显示）：非运行态展示具体状态名 */}
      <div className="hidden md:block text-xs text-gray-500 dark:text-gray-400 flex-shrink-0 w-28 text-right truncate">
        {running ? `运行 ${formatRunningTime(container.runningTime)}` : stateLabel(container.status)}
      </div>

      {/* 操作按钮区（relative z-10 保证浮在整行背景进度条之上） */}
      {!batchMode && (
        <div className="flex items-center gap-1 flex-shrink-0 relative z-10" onClick={(e) => e.stopPropagation()}>
          {/* 大屏(md+)：显示所有按钮 */}
          <div className="hidden md:flex items-center gap-1">
            {/* 详情：打开容器详情弹窗 */}
            <IconBtn onClick={() => onOpen(container)} icon={Info} title="详情" color="gray" />
            {/* 日志 / 控制台 / 文件管理 快捷入口 - 添加颜色区分 */}
            {onOps && (
              <>
                <IconBtn onClick={() => onOps('logs')} icon={FileText} title="查看日志" color="blue" />
                <IconBtn onClick={() => onOps('exec')} icon={TerminalSquare} title="终端" color="purple" />
                {onFiles && <IconBtn onClick={onFiles} icon={FolderOpen} title="文件管理" color="yellow" />}
                {onEdit && <IconBtn onClick={() => onEdit({ ...container, ID: container.id })} icon={Edit3} title="编辑容器" color="orange" />}
                {onProcess && <IconBtn onClick={() => onProcess({ ...container, ID: container.id })} icon={Activity} title="查看进程" color="green" />}
              </>
            )}
            {loading ? (
              updating ? (
                // 更新中：细进度条 + 百分比 + 拉取明细（与卡片模式信息行同款）
                <div className="flex flex-col gap-0.5 px-2 py-1 min-w-[200px] max-w-[260px]" title={actionState.detailMsg || ''}>
                  <div className="flex items-center gap-2">
                    <div className="flex-1 h-1.5 bg-gray-200 dark:bg-gray-700 rounded-full overflow-hidden">
                      <div className="h-full bg-gradient-to-r from-blue-500 to-blue-600 transition-all duration-300 rounded-full"
                        style={{ width: `${pct}%` }} />
                    </div>
                    <span className="text-xs font-medium text-blue-600 dark:text-blue-400 min-w-[3ch] text-right">
                      {Math.round(pct)}%
                    </span>
                  </div>
                  {actionState.detailMsg && (
                    <span className="text-[10px] text-gray-500 dark:text-gray-400 truncate">{actionState.detailMsg}</span>
                  )}
                </div>
              ) : (
                // 其它操作（启动/停止/重启/删除）：保持简洁的旋转图标 + 文字
                <span className="flex items-center gap-1 px-2 py-1 text-xs text-primary-600 dark:text-primary-400 max-w-[220px]">
                  <RefreshCw className="h-4 w-4 animate-spin flex-shrink-0" />
                  <span className="truncate">处理中</span>
                </span>
              )
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
                {onDelete && <IconBtn onClick={onDelete} icon={Trash2} title="删除容器" color="red" />}
              </>
            )}
          </div>

          {/* 小屏(<md)：三点菜单 */}
          <div className="md:hidden">
            {loading ? (
              updating ? (
                // 更新中：迷你进度条 + 百分比（小屏空间有限，省略明细文字）
                <div className="flex items-center gap-1.5 px-2 py-1 w-[92px]" title={actionState.detailMsg || ''}>
                  <div className="flex-1 h-1.5 bg-gray-200 dark:bg-gray-700 rounded-full overflow-hidden">
                    <div className="h-full bg-gradient-to-r from-blue-500 to-blue-600 transition-all duration-300 rounded-full"
                      style={{ width: `${pct}%` }} />
                  </div>
                  <span className="text-[10px] font-medium text-blue-600 dark:text-blue-400 min-w-[3ch] text-right">{Math.round(pct)}%</span>
                </div>
              ) : (
                <span className="flex items-center gap-1 px-2 py-1 text-xs text-primary-600 dark:text-primary-400">
                  <RefreshCw className="h-4 w-4 animate-spin flex-shrink-0" />
                  <span className="truncate">处理中</span>
                </span>
              )
            ) : (
              <>
                <button
                  onClick={(e) => { e.stopPropagation(); setMenuOpen(!menuOpen) }}
                  className="p-2 text-gray-500 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-lg transition-colors">
                  <MoreVertical className="h-4 w-4" />
                </button>
                {menuOpen && (
                  <>
                    {/* 遮罩：点击关闭菜单 */}
                    <div className="fixed inset-0 z-10" onClick={() => setMenuOpen(false)} />
                    {/* 下拉菜单 */}
                    <div className="absolute right-0 top-full mt-1 bg-white dark:bg-gray-800 rounded-lg shadow-lg border border-gray-200 dark:border-gray-700 py-1 z-20 min-w-[140px]">
                      <MenuItem onClick={() => { onOpen(container); setMenuOpen(false) }} icon={Info} text="详情" />
                      <div className="h-px bg-gray-200 dark:bg-gray-700 my-1" />
                      {running ? (
                        <>
                          <MenuItem onClick={() => { onAction(container.id, 'stop'); setMenuOpen(false) }} icon={Square} text="停止" />
                          <MenuItem onClick={() => { onAction(container.id, 'restart'); setMenuOpen(false) }} icon={RotateCcw} text="重启" />
                        </>
                      ) : (
                        <MenuItem onClick={() => { onAction(container.id, 'start'); setMenuOpen(false) }} icon={Play} text="启动" />
                      )}
                      <MenuItem onClick={() => { onUpdate(container.id); setMenuOpen(false) }} icon={Upload} text="更新" />
                      {onOps && (
                        <>
                          <div className="h-px bg-gray-200 dark:bg-gray-700 my-1" />
                          <MenuItem onClick={() => { onOps('logs'); setMenuOpen(false) }} icon={FileText} text="日志" />
                          <MenuItem onClick={() => { onOps('exec'); setMenuOpen(false) }} icon={TerminalSquare} text="终端" />
                          {onFiles && <MenuItem onClick={() => { onFiles(); setMenuOpen(false) }} icon={FolderOpen} text="文件管理" />}
                        </>
                      )}
                      {onDelete && (
                        <>
                          <div className="h-px bg-gray-200 dark:bg-gray-700 my-1" />
                          <MenuItem onClick={() => { onDelete(); setMenuOpen(false) }} icon={Trash2} text="删除容器" danger />
                        </>
                      )}
                    </div>
                  </>
                )}
              </>
            )}
          </div>
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

// 下拉菜单项组件；danger 为 true 时用红色（危险操作，如删除）
function MenuItem({ onClick, icon: Icon, text, danger }) {
  return (
    <button
      onClick={onClick}
      className={cn(
        "w-full flex items-center gap-2 px-3 py-2 text-sm transition-colors",
        danger
          ? "text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/20"
          : "text-gray-700 dark:text-gray-200 hover:bg-gray-100 dark:hover:bg-gray-700"
      )}>
      <Icon className="h-4 w-4 flex-shrink-0" />
      <span>{text}</span>
    </button>
  )
}
