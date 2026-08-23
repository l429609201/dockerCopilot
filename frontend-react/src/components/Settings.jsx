import React, { useState, useEffect } from 'react'
import { Settings as SettingsIcon, Save, Send, CheckCircle, XCircle, Loader2, Eye, EyeOff } from 'lucide-react'
import { botAPI } from '../api/client.js'
import { RegistrySection } from './RegistrySection.jsx'

// 解码后端用登录令牌 XOR 混淆后的 Base64 明文 Token。
// key 为当前登录 JWT 令牌字符串（与后端混淆时所用密钥一致），失败时返回空串。
function deobfuscateToken(obf, key) {
  if (!obf || !key) return ''
  try {
    const bin = atob(obf) // Base64 -> 原始字节字符串
    const kb = key
    let out = ''
    for (let i = 0; i < bin.length; i++) {
      out += String.fromCharCode(bin.charCodeAt(i) ^ kb.charCodeAt(i % kb.length))
    }
    return out
  } catch (e) {
    return ''
  }
}

// 设置页面：Telegram Bot 配置（Token 默认隐藏，可点击眼睛查看明文）
export function Settings() {
  const [cfg, setCfg] = useState({
    enabled: false, token: '', allowedChatIds: [], proxy: '',
    pollIntervalSec: 3, notifyUpdate: true, updateCheckIntervalMinutes: 30,
  })
  const [chatIdsText, setChatIdsText] = useState('')
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [msg, setMsg] = useState('')
  const [testing, setTesting] = useState(false)
  const [testMsg, setTestMsg] = useState(null) // { ok: boolean, text: string }
  const [showToken, setShowToken] = useState(false) // Token 明文显隐切换

  useEffect(() => {
    (async () => {
      setLoading(true)
      try {
        const r = await botAPI.getConfig()
        const d = r.data?.data || {}
        // 用登录令牌解码后端混淆的明文 Token，回填到输入框（默认以密码形式隐藏）
        const jwt = localStorage.getItem('docker_copilot_token') || ''
        const plainToken = deobfuscateToken(d.tokenObf, jwt)
        setCfg({
          enabled: !!d.enabled, token: plainToken,
          allowedChatIds: d.allowedChatIds || [], proxy: d.proxy || '',
          pollIntervalSec: d.pollIntervalSec || 3, notifyUpdate: !!d.notifyUpdate,
          updateCheckIntervalMinutes: d.updateCheckIntervalMinutes || 30,
        })
        setChatIdsText((d.allowedChatIds || []).join(', '))
      } catch (e) {
        setMsg('加载失败：' + e.message)
      } finally { setLoading(false) }
    })()
  }, [])

  // 解析白名单文本为数字数组，供保存和测试复用
  const parseChatIds = () =>
    chatIdsText.split(',').map((s) => s.trim()).filter(Boolean).map(Number).filter((n) => !isNaN(n))

  const save = async () => {
    setSaving(true); setMsg('')
    try {
      await botAPI.saveConfig({ ...cfg, allowedChatIds: parseChatIds() })
      setMsg('已保存')
    } catch (e) {
      setMsg('保存失败：' + (e.message || '未知错误'))
    } finally { setSaving(false) }
  }

  // 发送测试消息：把当前表单值一并传给后端（Token 为空则后端用已存值）
  const test = async () => {
    setTesting(true); setTestMsg(null)
    try {
      const resp = await botAPI.testConfig({
        ...cfg,
        allowedChatIds: parseChatIds(),
      })
      const d = resp.data || {}
      if (d.code === 200) {
        setTestMsg({ ok: true, text: d.msg || '测试消息已发送' })
      } else {
        setTestMsg({ ok: false, text: d.msg || '测试失败' })
      }
    } catch (e) {
      setTestMsg({ ok: false, text: '测试失败：' + (e.message || '未知错误') })
    } finally { setTesting(false) }
  }

  const set = (k, v) => setCfg((c) => ({ ...c, [k]: v }))

  return (
    <div className="w-full space-y-6">
      {/* 页面头部 */}
      <div className="px-2 sm:px-6 py-4">
        <h2 className="text-xl font-bold text-gray-900 dark:text-white flex items-center gap-2">
          <SettingsIcon className="h-5 w-5" /> 设置
        </h2>
      </div>

      {/* 内容区域 */}
      <div className="px-2 sm:px-6 space-y-6">
        <div className="card space-y-4">
        <div className="flex items-center gap-2 text-gray-900 dark:text-white font-semibold">
          <Send className="h-4 w-4" /> Telegram 机器人
        </div>
        {loading && <div className="text-gray-500 text-sm">加载中...</div>}

        <label className="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
          <input type="checkbox" checked={cfg.enabled} onChange={(e) => set('enabled', e.target.checked)} className="rounded" />
          启用 Bot
        </label>

        <div>
          <label className="block text-sm font-medium mb-1 text-gray-700 dark:text-gray-300">Bot Token（默认隐藏，点击右侧眼睛查看；留空表示不修改）</label>
          <div className="relative">
            <input
              type={showToken ? 'text' : 'password'}
              value={cfg.token}
              onChange={(e) => set('token', e.target.value)}
              className="input pr-10"
              placeholder="123456:ABC-..."
            />
            {/* 显隐切换按钮 */}
            <button
              type="button"
              onClick={() => setShowToken((v) => !v)}
              className="absolute inset-y-0 right-0 flex items-center px-3 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200"
              title={showToken ? '隐藏 Token' : '查看 Token'}
            >
              {showToken ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
            </button>
          </div>
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
          <div>
            <label className="block text-sm font-medium mb-1 text-gray-700 dark:text-gray-300">更新检测周期(分钟)</label>
            <input type="number" value={cfg.updateCheckIntervalMinutes} onChange={(e) => set('updateCheckIntervalMinutes', Number(e.target.value))} className="input" placeholder="30" />
            <p className="text-xs text-gray-500 mt-1">内置更新检测推送周期，留空或0使用默认30分钟</p>
          </div>
        </div>

        <label className="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
          <input type="checkbox" checked={cfg.notifyUpdate} onChange={(e) => set('notifyUpdate', e.target.checked)} className="rounded" />
          推送更新/定时任务通知
        </label>

        <div className="flex flex-wrap items-center gap-3 pt-2">
          <button onClick={save} disabled={saving}
            className="flex items-center gap-1 px-4 py-2 bg-primary-600 text-white rounded-lg hover:bg-primary-700 disabled:opacity-60">
            <Save className="h-4 w-4" /> {saving ? '保存中...' : '保存'}
          </button>
          {/* 测试连接：向白名单会话发送一条测试消息，验证 Token/代理/白名单是否可达 */}
          <button onClick={test} disabled={testing}
            className="flex items-center gap-1 px-4 py-2 border border-primary-600 text-primary-600 dark:text-primary-400 rounded-lg hover:bg-primary-50 dark:hover:bg-primary-900/20 disabled:opacity-60">
            {testing ? <Loader2 className="h-4 w-4 animate-spin" /> : <Send className="h-4 w-4" />}
            {testing ? '发送中...' : '发送测试消息'}
          </button>
          {msg && <span className="text-sm text-gray-500">{msg}</span>}
        </div>

        {/* 测试结果反馈 */}
        {testMsg && (
          <div className={`flex items-center gap-2 text-sm px-3 py-2 rounded-lg ${
            testMsg.ok
              ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-400'
              : 'bg-red-50 text-red-600 dark:bg-red-900/20 dark:text-red-400'}`}>
            {testMsg.ok ? <CheckCircle className="h-4 w-4 flex-shrink-0" /> : <XCircle className="h-4 w-4 flex-shrink-0" />}
            <span>{testMsg.text}</span>
          </div>
        )}

        <p className="text-xs text-gray-400">
          提示：测试会使用当前填写的 Token（留空则用已保存的）向白名单 Chat ID 发送一条消息。请先确保已填写白名单 Chat ID。
        </p>
      </div>

      {/* 仓库凭据卡片：从定时更新页迁移至此，供拉取私有镜像使用 */}
      <RegistrySection />
      </div>
    </div>
  )
}
