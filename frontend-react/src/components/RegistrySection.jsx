import React, { useState } from 'react'
import { KeyRound, Plus, Trash2 } from 'lucide-react'
import { registryAPI } from '../api/client.js'

// Registry 凭据管理区块（密码脱敏显示，不回显明文）
export function RegistrySection({ registries, onChanged }) {
  const [adding, setAdding] = useState(false)
  const [form, setForm] = useState({ name: '', registry: '', username: '', password: '' })

  const reset = () => { setForm({ name: '', registry: '', username: '', password: '' }); setAdding(false) }

  const save = async () => {
    if (!form.name || !form.username) { alert('名称和用户名必填'); return }
    try { await registryAPI.save(form); reset(); onChanged() }
    catch (e) { alert('保存失败：' + (e.message || '未知错误')) }
  }

  const remove = async (id) => {
    if (!confirm('确认删除该凭据？')) return
    try { await registryAPI.remove(id); onChanged() } catch (e) { alert('删除失败：' + e.message) }
  }

  return (
    <div className="mt-8">
      <div className="flex items-center justify-between mb-3">
        <h3 className="text-lg font-bold text-gray-900 dark:text-white flex items-center gap-2">
          <KeyRound className="h-5 w-5" /> 仓库凭据
        </h3>
        <button onClick={() => setAdding(true)}
          className="flex items-center gap-1 px-3 py-1.5 bg-gray-100 dark:bg-gray-700 rounded-lg text-sm hover:bg-gray-200">
          <Plus className="h-4 w-4" /> 添加
        </button>
      </div>

      <div className="grid gap-2">
        {registries.map((r) => (
          <div key={r.id} className="p-3 bg-white dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700 flex items-center justify-between">
            <div className="text-sm">
              <span className="font-medium text-gray-900 dark:text-white">{r.name}</span>
              <span className="text-gray-400 ml-2">{r.registry || 'docker.io'}</span>
              <span className="text-gray-500 ml-2">{r.username} / {r.password}</span>
            </div>
            <button onClick={() => remove(r.id)} className="p-2 text-red-600 hover:bg-red-50 rounded-lg">
              <Trash2 className="h-4 w-4" />
            </button>
          </div>
        ))}
        {registries.length === 0 && <div className="text-gray-400 text-sm">暂无凭据</div>}
      </div>

      {adding && (
        <div className="mt-3 p-4 bg-gray-50 dark:bg-gray-800/60 rounded-lg space-y-2">
          <input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} className="input" placeholder="名称" />
          <input value={form.registry} onChange={(e) => setForm({ ...form, registry: e.target.value })} className="input" placeholder="仓库地址（留空为 Docker Hub）" />
          <input value={form.username} onChange={(e) => setForm({ ...form, username: e.target.value })} className="input" placeholder="用户名" />
          <input type="password" value={form.password} onChange={(e) => setForm({ ...form, password: e.target.value })} className="input" placeholder="密码 / 访问令牌" />
          <div className="flex justify-end gap-2">
            <button onClick={reset} className="px-3 py-1.5 text-gray-600 hover:bg-gray-100 rounded-lg text-sm">取消</button>
            <button onClick={save} className="px-3 py-1.5 bg-primary-600 text-white rounded-lg text-sm hover:bg-primary-700">保存</button>
          </div>
        </div>
      )}
    </div>
  )
}
