import React, { useState, useEffect, useCallback } from 'react'
import { KeyRound, Plus, Trash2, Pencil, RefreshCw } from 'lucide-react'
import { registryAPI } from '../api/client.js'

// 凭据类型元信息：图标、展示名、默认地址、是否需要手填地址、是否支持次数查询。
const REGISTRY_TYPES = {
  dockerhub: { label: 'Docker Hub', icon: '🐳', defaultRegistry: 'docker.io', editableRegistry: false, supportRateLimit: true },
  github: { label: 'GitHub (ghcr.io)', icon: '🐙', defaultRegistry: 'ghcr.io', editableRegistry: false, supportRateLimit: false },
  custom: { label: '自定义仓库', icon: '📦', defaultRegistry: '', editableRegistry: true, supportRateLimit: false },
}

// normalizeType 归一化类型，兼容旧数据（无类型视为 custom）。
const normalizeType = (t) => (REGISTRY_TYPES[t] ? t : 'custom')

// Registry 凭据管理卡片（密码脱敏显示，不回显明文）。
// 自洽组件：内部自行加载与刷新数据，可放置在任意页面（如设置页）。
// onChanged 可选，供外部（如定时更新页）在凭据变更后同步刷新自己的下拉。
export function RegistrySection({ onChanged }) {
  const [registries, setRegistries] = useState([])
  const [loading, setLoading] = useState(false)
  // editing: null=未编辑; 'new'=新增; 其他=正在编辑的凭据ID
  const [editing, setEditing] = useState(null)
  const [form, setForm] = useState({ id: '', type: 'dockerhub', name: '', registry: 'docker.io', username: '', password: '', note: '' })
  // rateLimits: { [credId]: { loading, supported, limit, remaining, message } }
  const [rateLimits, setRateLimits] = useState({})

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const r = await registryAPI.list()
      setRegistries(r.data?.data || [])
    } catch (e) {
      console.error('加载仓库凭据失败:', e)
    } finally { setLoading(false) }
  }, [])

  useEffect(() => { load() }, [load])

  const reset = () => {
    setForm({ id: '', type: 'dockerhub', name: '', registry: 'docker.io', username: '', password: '', note: '' })
    setEditing(null)
  }

  // 打开新增表单：默认 Docker Hub 类型
  const startAdd = () => {
    setForm({ id: '', type: 'dockerhub', name: '', registry: 'docker.io', username: '', password: '', note: '' })
    setEditing('new')
  }

  // 打开编辑表单：预填基本信息，密码留空（表示不修改）
  const startEdit = (r) => {
    const type = normalizeType(r.type)
    setForm({ id: r.id, type, name: r.name, registry: r.registry || REGISTRY_TYPES[type].defaultRegistry, username: r.username, password: '', note: r.note || '' })
    setEditing(r.id)
  }

  // 切换凭据类型：非自定义类型自动套用默认地址
  const changeType = (type) => {
    const meta = REGISTRY_TYPES[type]
    setForm((f) => ({ ...f, type, registry: meta.editableRegistry ? f.registry : meta.defaultRegistry }))
  }

  // 查询单个 Docker Hub 凭据的剩余拉取次数
  const queryRateLimit = async (id) => {
    setRateLimits((s) => ({ ...s, [id]: { ...s[id], loading: true } }))
    try {
      const r = await registryAPI.rateLimit(id)
      setRateLimits((s) => ({ ...s, [id]: { loading: false, ...(r.data?.data || {}) } }))
    } catch (e) {
      setRateLimits((s) => ({ ...s, [id]: { loading: false, supported: false, message: e.message || '查询失败' } }))
    }
  }

  // 变更后刷新自身，并通知外部（如有）
  const notifyChanged = () => { load(); if (onChanged) onChanged() }

  const save = async () => {
    if (!form.name || !form.username) { alert('名称和用户名必填'); return }
    try { await registryAPI.save(form); reset(); notifyChanged() }
    catch (e) { alert('保存失败：' + (e.message || '未知错误')) }
  }

  const remove = async (id) => {
    if (!confirm('确认删除该凭据？')) return
    try { await registryAPI.remove(id); notifyChanged() } catch (e) { alert('删除失败：' + e.message) }
  }

  const isEdit = editing && editing !== 'new'

  return (
    <div className="p-5 bg-white dark:bg-gray-800 rounded-xl border border-gray-200 dark:border-gray-700 space-y-4">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2 text-gray-900 dark:text-white font-semibold">
          <KeyRound className="h-4 w-4" /> 仓库凭据
        </div>
        <button onClick={startAdd}
          className="flex items-center gap-1 px-3 py-1.5 bg-gray-100 dark:bg-gray-700 rounded-lg text-sm hover:bg-gray-200 dark:hover:bg-gray-600">
          <Plus className="h-4 w-4" /> 添加
        </button>
      </div>

      {loading && <div className="text-gray-500 text-sm">加载中...</div>}

      <div className="grid gap-2">
        {registries.map((r) => {
          const type = normalizeType(r.type)
          const meta = REGISTRY_TYPES[type]
          const rl = rateLimits[r.id]
          return (
          <div key={r.id} className="p-3 bg-white dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700 flex items-center justify-between gap-3">
            {/* 左侧：类型标签 + 名称/地址/用户名 + 说明 */}
            <div className="text-sm min-w-0 flex-1">
              <div className="flex items-center gap-2 flex-wrap">
                <span className="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-xs bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-300">
                  {meta.icon} {meta.label}
                </span>
                <span className="font-medium text-gray-900 dark:text-white">{r.name}</span>
                <span className="text-gray-400">{r.registry || 'docker.io'}</span>
                <span className="text-gray-500">{r.username} / {r.password}</span>
              </div>
              {r.note && <div className="text-xs text-gray-400 mt-1 truncate">说明：{r.note}</div>}
            </div>

            {/* 中间：Docker Hub 剩余拉取次数（仅该类型显示，手动刷新） */}
            {meta.supportRateLimit && (
              <div className="flex items-center gap-1.5 flex-shrink-0 text-xs">
                {rl?.loading ? (
                  <span className="text-gray-400">查询中...</span>
                ) : rl?.supported ? (
                  <span className={`font-medium ${rl.remaining <= 20 ? 'text-red-500' : rl.remaining <= 50 ? 'text-amber-500' : 'text-green-600 dark:text-green-400'}`}>
                    剩余 {rl.limit < 0 ? '无限制' : `${rl.remaining}/${rl.limit}`}
                  </span>
                ) : rl ? (
                  <span className="text-gray-400" title={rl.message}>—</span>
                ) : (
                  <span className="text-gray-400">拉取次数</span>
                )}
                <button onClick={() => queryRateLimit(r.id)} title="刷新剩余拉取次数" disabled={rl?.loading}
                  className="p-1 text-gray-500 hover:text-primary-600 hover:bg-gray-100 dark:hover:bg-gray-700 rounded disabled:opacity-50">
                  <RefreshCw className={`h-3.5 w-3.5 ${rl?.loading ? 'animate-spin' : ''}`} />
                </button>
              </div>
            )}

            {/* 右侧：编辑 / 删除 */}
            <div className="flex items-center gap-1 flex-shrink-0">
              <button onClick={() => startEdit(r)} title="编辑" className="p-2 text-primary-600 hover:bg-primary-50 dark:hover:bg-primary-900/20 rounded-lg">
                <Pencil className="h-4 w-4" />
              </button>
              <button onClick={() => remove(r.id)} title="删除" className="p-2 text-red-600 hover:bg-red-50 dark:hover:bg-red-900/20 rounded-lg">
                <Trash2 className="h-4 w-4" />
              </button>
            </div>
          </div>
        )})}
        {registries.length === 0 && !loading && <div className="text-gray-400 text-sm">暂无凭据</div>}
      </div>

      {editing && (
        <div className="mt-3 p-4 bg-gray-50 dark:bg-gray-800/60 rounded-lg space-y-2">
          <div className="text-sm font-medium text-gray-700 dark:text-gray-300">{isEdit ? '编辑凭据' : '新增凭据'}</div>
          {/* 类型选择：切换后自动套用默认地址 */}
          <div className="flex gap-2">
            {Object.entries(REGISTRY_TYPES).map(([key, meta]) => (
              <button key={key} type="button" onClick={() => changeType(key)}
                className={`flex-1 px-2 py-1.5 rounded-lg text-sm border transition-colors ${form.type === key
                  ? 'border-primary-500 bg-primary-50 dark:bg-primary-900/20 text-primary-700 dark:text-primary-300'
                  : 'border-gray-200 dark:border-gray-700 text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700'}`}>
                {meta.icon} {meta.label}
              </button>
            ))}
          </div>
          <input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} className="input" placeholder="名称" />
          {/* 仅自定义类型手填地址；Docker Hub/GitHub 使用固定地址 */}
          {REGISTRY_TYPES[form.type].editableRegistry ? (
            <input value={form.registry} onChange={(e) => setForm({ ...form, registry: e.target.value })} className="input" placeholder="仓库地址，如 registry.example.com" />
          ) : (
            <div className="text-xs text-gray-400 px-1">仓库地址：{REGISTRY_TYPES[form.type].defaultRegistry}</div>
          )}
          <input value={form.username} onChange={(e) => setForm({ ...form, username: e.target.value })} className="input" placeholder="用户名" />
          <input type="password" value={form.password} onChange={(e) => setForm({ ...form, password: e.target.value })} className="input"
            placeholder={isEdit ? '密码 / 访问令牌（留空表示不修改）' : '密码 / 访问令牌'} />
          <input value={form.note} onChange={(e) => setForm({ ...form, note: e.target.value })} className="input" placeholder="说明 / 备注（该凭据的用途，可选）" />
          <div className="flex justify-end gap-2">
            <button onClick={reset} className="px-3 py-1.5 text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-lg text-sm">取消</button>
            <button onClick={save} className="px-3 py-1.5 bg-primary-600 text-white rounded-lg text-sm hover:bg-primary-700">保存</button>
          </div>
        </div>
      )}
    </div>
  )
}
