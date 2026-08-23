import React, { useState, useEffect } from 'react'
import { X, Plus, Trash2, Save } from 'lucide-react'
import { containerAPI } from '../api/client.js'

// 容器编辑弹窗：编辑端口映射、环境变量、重启策略（任务化重建）。
// 复用已有后端 EditLogic，EditSpec 支持 Image/Env/RestartPolicy/PortBindings/KeepOld。
export function ContainerEditModal({ container, onClose, onSuccess }) {
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  // 表单状态
  const [form, setForm] = useState({
    image: '',
    restartPolicy: 'unless-stopped',
    keepOld: false,
    env: [], // [{key, value}]
    ports: [], // [{host, container, proto}]
  })

  const set = (k, v) => setForm((f) => ({ ...f, [k]: v }))

  // 加载容器 Inspect 数据回填表单
  useEffect(() => {
    (async () => {
      setLoading(true)
      try {
        const r = await containerAPI.inspectContainer(container.ID)
        const cfg = r.data?.data || {}
        // 镜像
        set('image', cfg.Config?.Image || '')
        // 重启策略
        set('restartPolicy', cfg.HostConfig?.RestartPolicy?.Name || 'unless-stopped')
        // 环境变量（过滤 PATH 等系统变量）
        const envs = (cfg.Config?.Env || []).map((line) => {
          const idx = line.indexOf('=')
          return idx > 0 ? { key: line.slice(0, idx), value: line.slice(idx + 1) } : { key: line, value: '' }
        }).filter((e) => !e.key.startsWith('PATH') && !e.key.startsWith('HOSTNAME'))
        set('env', envs)
        // 端口映射（从 HostConfig.PortBindings 解析）
        const bindings = cfg.HostConfig?.PortBindings || {}
        const ports = []
        for (const [containerPort, hostBindings] of Object.entries(bindings)) {
          if (!hostBindings || hostBindings.length === 0) continue
          const [port, proto] = containerPort.split('/')
          ports.push({
            host: hostBindings[0].HostPort || '',
            container: port,
            proto: proto || 'tcp',
          })
        }
        set('ports', ports)
      } catch (e) {
        setError('加载容器配置失败：' + (e.response?.data?.msg || e.message))
      } finally {
        setLoading(false)
      }
    })()
  }, [container.ID])

  // 提交编辑（转为 EditSpec 格式）
  const submit = async () => {
    setSaving(true)
    setError('')
    try {
      /* PortBindings 格式: ["hostPort:containerPort/proto", ...] */
      const portBindings = form.ports.map((p) => `${p.host}:${p.container}/${p.proto}`)
      /* Env 格式: ["KEY=VALUE", ...] */
      const env = form.env.filter((e) => e.key).map((e) => `${e.key}=${e.value}`)
      const spec = {
        image: form.image || undefined,
        env: env.length > 0 ? env : undefined,
        restartPolicy: form.restartPolicy,
        portBindings: portBindings.length > 0 ? portBindings : undefined,
        keepOld: form.keepOld,
      }
      await containerAPI.editContainer(container.ID, spec)
      alert('编辑任务已提交，容器将重建')
      onSuccess?.()
      onClose()
    } catch (e) {
      setError('提交失败：' + (e.response?.data?.msg || e.message))
    } finally {
      setSaving(false)
    }
  }

  // 端口操作
  const addPort = () => set('ports', [...form.ports, { host: '', container: '', proto: 'tcp' }])
  const removePort = (idx) => set('ports', form.ports.filter((_, i) => i !== idx))
  const updatePort = (idx, k, v) => {
    const updated = [...form.ports]
    updated[idx][k] = v
    set('ports', updated)
  }

  // 环境变量操作
  const addEnv = () => set('env', [...form.env, { key: '', value: '' }])
  const removeEnv = (idx) => set('env', form.env.filter((_, i) => i !== idx))
  const updateEnv = (idx, k, v) => {
    const updated = [...form.env]
    updated[idx][k] = v
    set('env', updated)
  }

  if (loading) {
    return (
      <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
        <div className="bg-white dark:bg-gray-800 rounded-lg p-8 text-center">
          <div className="text-gray-600 dark:text-gray-400">加载配置中...</div>
        </div>
      </div>
    )
  }

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50 p-4 overflow-y-auto">
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow-xl w-full max-w-3xl my-8">
        {/* 头部 */}
        <div className="flex items-center justify-between p-4 border-b border-gray-200 dark:border-gray-700">
          <h3 className="text-lg font-semibold text-gray-900 dark:text-white">
            ✏️ 编辑容器 - {container.Names?.[0]?.replace(/^\//, '') || container.ID.slice(0, 12)}
          </h3>
          <button onClick={onClose}
            className="p-2 text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-lg">
            <X className="h-4 w-4" />
          </button>
        </div>

        {/* 表单区（内容将在下一次编辑中补充） */}
        <div className="p-4 space-y-4 max-h-[70vh] overflow-y-auto">
          {error && (
            <div className="p-3 bg-red-50 dark:bg-red-900/20 text-red-700 dark:text-red-300 rounded-lg">
              {error}
            </div>
          )}

          {/* 镜像（只读提示，编辑镜像用"更新"功能） */}
          <Field label="镜像">
            <input value={form.image} readOnly className="input bg-gray-50 dark:bg-gray-900 cursor-not-allowed" />
            <p className="text-xs text-gray-500 mt-1">💡 镜像修改请使用"更新"功能，此处仅供查看</p>
          </Field>

          {/* 端口映射 */}
          <Field label="端口映射">
            <div className="space-y-2">
              {form.ports.map((p, i) => (
                <div key={i} className="flex items-center gap-2">
                  <input placeholder="主机端口" value={p.host} onChange={(e) => updatePort(i, 'host', e.target.value)}
                    className="input flex-1" />
                  <span className="text-gray-500">→</span>
                  <input placeholder="容器端口" value={p.container} onChange={(e) => updatePort(i, 'container', e.target.value)}
                    className="input flex-1" />
                  <select value={p.proto} onChange={(e) => updatePort(i, 'proto', e.target.value)} className="input w-24">
                    <option value="tcp">TCP</option>
                    <option value="udp">UDP</option>
                  </select>
                  <button onClick={() => removePort(i)} className="p-2 text-red-600 hover:bg-red-50 dark:hover:bg-red-900/20 rounded-lg">
                    <Trash2 className="h-4 w-4" />
                  </button>
                </div>
              ))}
              <button onClick={addPort} className="w-full py-2 border-2 border-dashed border-gray-300 dark:border-gray-600 rounded-lg text-gray-600 dark:text-gray-400 hover:bg-gray-50 dark:hover:bg-gray-700/50 flex items-center justify-center gap-2">
                <Plus className="h-4 w-4" /> 添加端口
              </button>
            </div>
          </Field>

          {/* 环境变量 */}
          <Field label="环境变量">
            <div className="space-y-2">
              {form.env.map((e, i) => (
                <div key={i} className="flex items-center gap-2">
                  <input placeholder="变量名" value={e.key} onChange={(ev) => updateEnv(i, 'key', ev.target.value)}
                    className="input flex-1" />
                  <span className="text-gray-500">=</span>
                  <input placeholder="值" value={e.value} onChange={(ev) => updateEnv(i, 'value', ev.target.value)}
                    className="input flex-1" />
                  <button onClick={() => removeEnv(i)} className="p-2 text-red-600 hover:bg-red-50 dark:hover:bg-red-900/20 rounded-lg">
                    <Trash2 className="h-4 w-4" />
                  </button>
                </div>
              ))}
              <button onClick={addEnv} className="w-full py-2 border-2 border-dashed border-gray-300 dark:border-gray-600 rounded-lg text-gray-600 dark:text-gray-400 hover:bg-gray-50 dark:hover:bg-gray-700/50 flex items-center justify-center gap-2">
                <Plus className="h-4 w-4" /> 添加变量
              </button>
            </div>
          </Field>

          {/* 重启策略 */}
          <Field label="重启策略">
            <select value={form.restartPolicy} onChange={(e) => set('restartPolicy', e.target.value)} className="input">
              <option value="no">不重启</option>
              <option value="always">总是重启</option>
              <option value="unless-stopped">除非手动停止</option>
              <option value="on-failure">仅失败时重启</option>
            </select>
          </Field>

          {/* 保留旧容器 */}
          <label className="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
            <input type="checkbox" checked={form.keepOld} onChange={(e) => set('keepOld', e.target.checked)} className="rounded" />
            保留旧容器（重建后不删除旧容器）
          </label>

          <div className="p-3 bg-yellow-50 dark:bg-yellow-900/20 text-yellow-700 dark:text-yellow-300 text-sm rounded-lg">
            ⚠️ 保存将<b>重建容器</b>（停止→删除→按新配置创建→启动），数据卷不受影响。
          </div>
        </div>

        {/* 底部按钮 */}
        <div className="flex items-center justify-end gap-3 p-4 border-t border-gray-200 dark:border-gray-700">
          <button onClick={onClose} className="px-4 py-2 text-gray-600 hover:bg-gray-100 rounded-lg">取消</button>
          <button onClick={submit} disabled={saving}
            className="px-4 py-2 bg-primary-600 text-white rounded-lg hover:bg-primary-700 disabled:opacity-50 flex items-center gap-2">
            <Save className="h-4 w-4" />
            {saving ? '提交中...' : '保存并重建'}
          </button>
        </div>
      </div>
    </div>
  )
}

function Field({ label, children }) {
  return (
    <div>
      <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">{label}</label>
      {children}
    </div>
  )
}
