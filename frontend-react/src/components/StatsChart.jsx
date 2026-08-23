import React, { useEffect, useRef, useState } from 'react'
import { Cpu, MemoryStick } from 'lucide-react'

// 实时资源折线图：CPU% 与 内存% 两条线，保留最近 maxPoints 个采样点滚动。
// 轻量 SVG 自绘，不依赖第三方图表库。stat 来自 useContainerStats 的单容器采样。
export function StatsChart({ stat, maxPoints = 40 }) {
  const [points, setPoints] = useState([]) // [{ cpu, mem }]
  const lastRef = useRef(null)

  // 每当采到新数据就追加一个点（用对象引用变化判断，避免重复）
  useEffect(() => {
    if (!stat) return
    if (lastRef.current === stat) return
    lastRef.current = stat
    setPoints((prev) => {
      const next = [...prev, {
        cpu: Math.max(0, stat.cpuPercent ?? 0),
        mem: Math.max(0, stat.memPercent ?? 0),
      }]
      return next.slice(-maxPoints)
    })
  }, [stat, maxPoints])

  const w = 560, h = 120, pad = 4
  // Y 轴上限：至少 100%，CPU 可能超 100（多核），动态取最大值向上取整到 20 的倍数
  const maxVal = Math.max(100, ...points.map((p) => Math.max(p.cpu, p.mem)))
  const yMax = Math.ceil(maxVal / 20) * 20

  const toPath = (key) => {
    if (points.length === 0) return ''
    const stepX = points.length > 1 ? (w - pad * 2) / (points.length - 1) : 0
    return points.map((p, i) => {
      const x = pad + i * stepX
      const y = h - pad - (p[key] / yMax) * (h - pad * 2)
      return `${i === 0 ? 'M' : 'L'}${x.toFixed(1)},${y.toFixed(1)}`
    }).join(' ')
  }

  const cur = points[points.length - 1] || { cpu: 0, mem: 0 }

  return (
    <div className="rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-3">
      <div className="flex items-center justify-between mb-2">
        <span className="text-sm font-medium text-gray-700 dark:text-gray-200">实时资源</span>
        <div className="flex items-center gap-4 text-xs tabular-nums">
          <span className="flex items-center gap-1 text-blue-500">
            <Cpu className="h-3.5 w-3.5" /> CPU {cur.cpu.toFixed(1)}%
          </span>
          <span className="flex items-center gap-1 text-emerald-500">
            <MemoryStick className="h-3.5 w-3.5" /> 内存 {cur.mem.toFixed(1)}%
          </span>
        </div>
      </div>
      {points.length === 0 ? (
        <div className="h-[120px] flex items-center justify-center text-xs text-gray-400">
          正在采集数据...
        </div>
      ) : (
        <svg viewBox={`0 0 ${w} ${h}`} className="w-full" style={{ height: h }} preserveAspectRatio="none">
          {/* 网格线 */}
          {[0.25, 0.5, 0.75].map((r) => (
            <line key={r} x1={pad} y1={h - pad - r * (h - pad * 2)} x2={w - pad} y2={h - pad - r * (h - pad * 2)}
              stroke="currentColor" strokeWidth="0.5" className="text-gray-200 dark:text-gray-700" />
          ))}
          <path d={toPath('cpu')} fill="none" stroke="#3b82f6" strokeWidth="1.5" />
          <path d={toPath('mem')} fill="none" stroke="#10b981" strokeWidth="1.5" />
        </svg>
      )}
      <div className="text-[10px] text-gray-400 mt-1 text-right">纵轴上限 {yMax}%（每 3 秒采样）</div>
    </div>
  )
}
