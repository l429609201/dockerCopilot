import React, { useState, useEffect, useCallback } from 'react'
import { KeyRound, Plus, Trash2, Pencil } from 'lucide-react'
import { registryAPI } from '../api/client.js'

// Registry 凭据管理卡片（密码脱敏显示，不回显明文）。
// 自洽组件：内部自行加载与刷新数据，可放置在任意页面（如设置页）。
// onChanged 可选，供外部（如定时更新页）在凭据变更后同步刷新自己的下拉。
export function RegistrySection({ onChanged }) {
  const [registries, setRegistries] = useState([])
  const [loading, setLoading] = useState(false)
  // editing: null=未编辑; 'new'=新增; 其他=正在编辑的凭据ID
  const [editing, setEditing] = useState(null)
  const [form, setForm] = useState({ id: '', name: '', registry: '', username: '', password: '' })

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
    setForm({ id: '', name: '', registry: '', username: '', password: '' })
    setEditing(null)
  }

  // 打开新增表单
  const startAdd = () => {
    setForm({ id: '', name: '', registry: '', username: '', password: '' })
    setEditing('new')
  }

  // 打开编辑表单：预填基本信息，密码留空（表示不修改）
  const startEdit = (r) => {
    setForm({ id: r.id, name: r.name, registry: r.registry || '', username: r.username, password: '' })
    setEditing(r.id)
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
        {registries.map((r) => (
          <div key={r.id} className="p-3 bg-white dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700 flex items-center justify-between gap-2">
            <div className="text-sm min-w-0">
              <span className="font-medium text-gray-900 dark:text-white">{r.name}</span>
              <span className="text-gray-400 ml-2">{r.registry || 'docker.io'}</span>
              <span className="text-gray-500 ml-2">{r.username} / {r.password}</span>
            </div>
            <div className="flex items-center gap-1 flex-shrink-0">
              <button onClick={() => startEdit(r)} title="编辑" className="p-2 text-primary-600 hover:bg-primary-50 dark:hover:bg-primary-900/20 rounded-lg">
                <Pencil className="h-4 w-4" />
              </button>
              <button onClick={() => remove(r.id)} title="删除" className="p-2 text-red-600 hover:bg-red-50 dark:hover:bg-red-900/20 rounded-lg">
                <Trash2 className="h-4 w-4" />
              </button>
            </div>
          </div>
        ))}
        {registries.length === 0 && !loading && <div className="text-gray-400 text-sm">暂无凭据</div>}
      </div>

      {editing && (
        <div className="mt-3 p-4 bg-gray-50 dark:bg-gray-800/60 rounded-lg space-y-2">
          <div className="text-sm font-medium text-gray-700 dark:text-gray-300">{isEdit ? '编辑凭据' : '新增凭据'}</div>
          <input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} className="input" placeholder="名称" />
          <input value={form.registry} onChange={(e) => setForm({ ...form, registry: e.target.value })} className="input" placeholder="仓库地址（留空为 Docker Hub）" />
          <input value={form.username} onChange={(e) => setForm({ ...form, username: e.target.value })} className="input" placeholder="用户名" />
          <input type="password" value={form.password} onChange={(e) => setForm({ ...form, password: e.target.value })} className="input"
            placeholder={isEdit ? '密码 / 访问令牌（留空表示不修改）' : '密码 / 访问令牌'} />
          <div className="flex justify-end gap-2">
            <button onClick={reset} className="px-3 py-1.5 text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-lg text-sm">取消</button>
            <button onClick={save} className="px-3 py-1.5 bg-primary-600 text-white rounded-lg text-sm hover:bg-primary-700">保存</button>
          </div>
        </div>
      )}
    </div>
  )
}
