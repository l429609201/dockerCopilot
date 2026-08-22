import React, { useState, useEffect } from 'react'
import { Settings as SettingsIcon, Save, Send } from 'lucide-react'
import { botAPI } from '../api/client.js'

// 设置页面：目前包含 Telegram Bot 配置（Token 脱敏，不回显明文）
export function Settings() {
  const [cfg, setCfg] = useState({
    enabled: false, token: '', allowedChatIds: [], proxy: '',
    pollIntervalSec: 3, notifyUpdate: true,
  })
  const [chatIdsText, setChatIdsText] = useState('')
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [msg, setMsg] = useState('')

  useEffect(() => {
    (async () => {
      setLoading(true)
      try {
        const r = await botAPI.getConfig()
        const d = r.data?.data || {}
        setCfg({
          enabled: !!d.enabled, token: '', // 脱敏返回不回填明文，留空表示不修改
          allowedChatIds: d.allowedChatIds || [], proxy: d.proxy || '',
          pollIntervalSec: d.pollIntervalSec || 3, notifyUpdate: !!d.notifyUpdate,
        })
        setChatIdsText((d.allowedChatIds || []).join(', '))
      } catch (e) {
        setMsg('加载失败：' + e.message)
      } finally { setLoading(false) }
    })()
  }, [])

  const save = async () => {
    setSaving(true); setMsg('')
    const allowedChatIds = chatIdsText.split(',').map((s) => s.trim()).filter(Boolean).map(Number).filter((n) => !isNaN(n))
    try {
      await botAPI.saveConfig({ ...cfg, allowedChatIds })
      setMsg('已保存')
    } catch (e) {
      setMsg('保存失败：' + (e.message || '未知错误'))
    } finally { setSaving(false) }
  }

  const set = (k, v) => setCfg((c) => ({ ...c, [k]: v }))

  return (
    <div className="space-y-6 max-w-2xl">
      <h2 className="text-xl font-bold text-gray-900 dark:text-white flex items-center gap-2">
        <SettingsIcon className="h-5 w-5" /> 设置
      </h2>

      <div className="p-5 bg-white dark:bg-gray-800 rounded-xl border border-gray-200 dark:border-gray-700 space-y-4">
        <div className="flex items-center gap-2 text-gray-900 dark:text-white font-semibold">
          <Send className="h-4 w-4" /> Telegram 机器人
        </div>
        {loading && <div className="text-gray-500 text-sm">加载中...</div>}

        <label className="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
          <input type="checkbox" checked={cfg.enabled} onChange={(e) => set('enabled', e.target.checked)} className="rounded" />
          启用 Bot
        </label>

        <div>
          <label className="block text-sm font-medium mb-1 text-gray-700 dark:text-gray-300">Bot Token（留空表示不修改）</label>
          <input type="password" value={cfg.token} onChange={(e) => set('token', e.target.value)} className="input" placeholder="123456:ABC-..." />
        </div>

        <div>
          <label className="block text-sm font-medium mb-1 text-gray-700 dark:text-gray-300">白名单 Chat ID（逗号分隔）</label>
          <input value={chatIdsText} onChange={(e) => setChatIdsText(e.target.value)} className="input" placeholder="123456789, 987654321" />
        </div>

        <div className="grid grid-cols-2 gap-3">
          <div>
            <label className="block text-sm font-medium mb-1 text-gray-700 dark:text-gray-300">代理（可选）</label>
            <input value={cfg.proxy} onChange={(e) => set('proxy', e.target.value)} className="input" placeholder="http://host:port" />
          </div>
          <div>
            <label className="block text-sm font-medium mb-1 text-gray-700 dark:text-gray-300">轮询间隔(秒)</label>
            <input type="number" value={cfg.pollIntervalSec} onChange={(e) => set('pollIntervalSec', Number(e.target.value))} className="input" />
          </div>
        </div>

        <label className="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
          <input type="checkbox" checked={cfg.notifyUpdate} onChange={(e) => set('notifyUpdate', e.target.checked)} className="rounded" />
          推送更新/定时任务通知
        </label>

        <div className="flex items-center gap-3 pt-2">
          <button onClick={save} disabled={saving}
            className="flex items-center gap-1 px-4 py-2 bg-primary-600 text-white rounded-lg hover:bg-primary-700 disabled:opacity-60">
            <Save className="h-4 w-4" /> {saving ? '保存中...' : '保存'}
          </button>
          {msg && <span className="text-sm text-gray-500">{msg}</span>}
        </div>
      </div>
    </div>
  )
}
