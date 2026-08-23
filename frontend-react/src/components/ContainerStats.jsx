import React from 'react'
import { Cpu, MemoryStick, ArrowDown, ArrowUp } from 'lucide-react'
import { formatBytes } from '../utils/format.js'

// 容器资源展示。variant='card' 全量(CPU/内存/↑↓)，variant='list' 精简(CPU%+内存%)。
// stat 为 useContainerStats 返回的单个容器采样，可能为 undefined（未运行/未采到）。
export function ContainerStats({ stat, variant = 'card' }) {
  if (!stat) {
    // 未运行或暂无数据时，列表模式占位保持对齐，卡片模式不渲染
    return variant === 'list' ? <span className="text-xs text-gray-300 dark:text-gray-600">—</span> : null
  }

  const cpu = `${(stat.cpuPercent ?? 0).toFixed(1)}%`
  const memPct = `${(stat.memPercent ?? 0).toFixed(1)}%`

  if (variant === 'list') {
    return (
      <div className="flex items-center gap-2.5 text-xs text-gray-500 dark:text-gray-400 tabular-nums">
        <span className="flex items-center gap-1" title="CPU 使用率">
          <Cpu className="h-3.5 w-3.5 text-blue-500" />{cpu}
        </span>
        <span className="flex items-center gap-1" title="内存使用率">
          <MemoryStick className="h-3.5 w-3.5 text-emerald-500" />{memPct}
        </span>
        {/* 流量：仅超大屏显示，避免列表行拥挤 */}
        <span className="hidden xl:flex items-center gap-1" title="下行流量">
          <ArrowDown className="h-3.5 w-3.5 text-cyan-500" />{formatBytes(stat.netRxBytes)}
        </span>
        <span className="hidden xl:flex items-center gap-1" title="上行流量">
          <ArrowUp className="h-3.5 w-3.5 text-orange-500" />{formatBytes(stat.netTxBytes)}
        </span>
      </div>
    )
  }

  // 卡片全量
  return (
    <div className="grid grid-cols-2 gap-x-3 gap-y-1.5 text-xs">
      <Metric icon={Cpu} color="text-blue-500" label="CPU" value={cpu} />
      <Metric icon={MemoryStick} color="text-emerald-500" label="内存"
        value={`${formatBytes(stat.memUsed)} · ${memPct}`} />
      <Metric icon={ArrowDown} color="text-cyan-500" label="下行" value={formatBytes(stat.netRxBytes)} />
      <Metric icon={ArrowUp} color="text-orange-500" label="上行" value={formatBytes(stat.netTxBytes)} />
    </div>
  )
}

function Metric({ icon: Icon, color, label, value }) {
  return (
    <div className="flex items-center gap-1.5 min-w-0">
      <Icon className={`h-3.5 w-3.5 flex-shrink-0 ${color}`} />
      <span className="text-gray-400 dark:text-gray-500 flex-shrink-0">{label}</span>
      <span className="text-gray-700 dark:text-gray-200 truncate tabular-nums">{value}</span>
    </div>
  )
}
