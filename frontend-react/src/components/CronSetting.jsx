import React, { useState, useEffect } from 'react'
import { Clock, Save, CheckCircle, XCircle, Loader2 } from 'lucide-react'
import { scheduleAPI } from '../api/client.js'

// 全局定时设置卡片：所有定时规则共用同一执行时间。
// 提供两种模式——可视化（每天/每隔小时）与高级 cron 手输，覆盖常见场景。
export function CronSetting({ cron, onSaved }) {
  const [mode, setMode] = useState('daily') // daily | interval | advanced
  const [hour, setHour] = useState(4)
  const [minute, setMinute] = useState(30)
  const [everyHours, setEveryHours] = useState(6)
  const [expr, setExpr] = useState(cron || '30 4 * * *')
  const [saving, setSaving] = useState(false)
  const [msg, setMsg] = useState(null) // { ok, text }

  // 外部 cron 变化时，反解析回可视化控件（尽力而为）
  useEffect(() => {
    if (!cron) return
    setExpr(cron)
    const parts = cron.trim().split(/\s+/)
    if (parts.length === 5) {
      const [m, h, dom, , dow] = parts
      // 每隔 N 小时：分固定、时为 */N
      if (dom === '*' && dow === '*' && /^\*\/\d+$/.test(h)) {
        setMode('interval'); setEveryHours(Number(h.slice(2))); setMinute(Number(m) || 0); return
      }
      // 每天 HH:MM
      if (dom === '*' && dow === '*' && /^\d+$/.test(h) && /^\d+$/.test(m)) {
        setMode('daily'); setHour(Number(h)); setMinute(Number(m)); return
      }
      setMode('advanced')
    }
  }, [cron])

  // 由可视化控件推导出最终 cron 表达式
  const buildCron = () => {
    if (mode === 'daily') return `${minute} ${hour} * * *`
    if (mode === 'interval') return `${minute} */${everyHours} * * *`
    return expr.trim()
  }

  const save = async () => {
    const finalCron = buildCron()
    if (!finalCron) { setMsg({ ok: false, text: 'cron 不能为空' }); return }
    setSaving(true); setMsg(null)
    try {
      const resp = await scheduleAPI.saveCron(finalCron)
      if (resp.data?.code === 200) {
        setMsg({ ok: true, text: '已保存，下次按此时间执行' })
        if (onSaved) onSaved(finalCron)
      } else {
        setMsg({ ok: false, text: resp.data?.msg || '保存失败' })
      }
    } catch (e) {
      setMsg({ ok: false, text: '保存失败：' + (e.message || '未知错误') })
    } finally { setSaving(false) }
  }

  const preview = buildCron()

  return (
    <div className="p-5 bg-white dark:bg-gray-800 rounded-xl border border-gray-200 dark:border-gray-700 space-y-4">
      <div className="flex items-center gap-2 text-gray-900 dark:text-white font-semibold">
        <Clock className="h-4 w-4" /> 全局定时设置
      </div>
      <p className="text-xs text-gray-400">所有启用的规则共用这一个执行时间，到点统一依次检查更新。</p>

      {/* 模式切换 */}
      <div className="flex flex-wrap gap-2">
        <ModeBtn active={mode === 'daily'} onClick={() => setMode('daily')} label="每天固定时间" />
        <ModeBtn active={mode === 'interval'} onClick={() => setMode('interval')} label="每隔几小时" />
        <ModeBtn active={mode === 'advanced'} onClick={() => setMode('advanced')} label="高级(cron)" />
      </div>

      {mode === 'daily' && (
        <div className="flex items-center gap-2 text-sm">
          <span>每天</span>
          <NumInput value={hour} min={0} max={23} onChange={setHour} />
          <span>时</span>
          <NumInput value={minute} min={0} max={59} onChange={setMinute} />
          <span>分</span>
        </div>
      )}

      {mode === 'interval' && (
        <div className="flex items-center gap-2 text-sm">
          <span>每隔</span>
          <NumInput value={everyHours} min={1} max={23} onChange={setEveryHours} />
          <span>小时（在第</span>
          <NumInput value={minute} min={0} max={59} onChange={setMinute} />
          <span>分执行）</span>
        </div>
      )}

      {mode === 'advanced' && (
        <div>
          <input value={expr} onChange={(e) => setExpr(e.target.value)}
            className="input font-mono" placeholder="分 时 日 月 周，如 30 4 * * *" />
          <p className="text-xs text-gray-400 mt-1">五段式 cron：分 时 日 月 周。</p>
        </div>
      )}

      <div className="flex flex-wrap items-center gap-3 pt-1">
        <button onClick={save} disabled={saving}
          className="flex items-center gap-1 px-4 py-2 bg-primary-600 text-white rounded-lg hover:bg-primary-700 disabled:opacity-60">
          {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}
          {saving ? '保存中...' : '保存定时'}
        </button>
        <span className="text-xs text-gray-500 font-mono">当前表达式：{preview || '（空）'}</span>
      </div>

      {msg && (
        <div className={`flex items-center gap-2 text-sm px-3 py-2 rounded-lg ${
          msg.ok
            ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-400'
            : 'bg-red-50 text-red-600 dark:bg-red-900/20 dark:text-red-400'}`}>
          {msg.ok ? <CheckCircle className="h-4 w-4" /> : <XCircle className="h-4 w-4" />}
          <span>{msg.text}</span>
        </div>
      )}
    </div>
  )
}

function ModeBtn({ active, onClick, label }) {
  return (
    <button onClick={onClick}
      className={`px-3 py-1.5 rounded-lg text-sm ${active
        ? 'bg-primary-600 text-white'
        : 'bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-300 hover:bg-gray-200 dark:hover:bg-gray-600'}`}>
      {label}
    </button>
  )
}

function NumInput({ value, min, max, onChange }) {
  return (
    <input type="number" value={value} min={min} max={max}
      onChange={(e) => {
        let v = Number(e.target.value)
        if (isNaN(v)) v = min
        onChange(Math.max(min, Math.min(max, v)))
      }}
      className="w-16 px-2 py-1 border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-900 text-center" />
  )
}
