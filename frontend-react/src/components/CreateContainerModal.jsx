import React, { useState, useEffect } from 'react'
import { X, Save, Loader2, Plus, Server } from 'lucide-react'
import { containerAPI, dockerHostAPI } from '../api/client.js'

// 从零创建容器弹窗（Portainer 风格）：镜像/名称/端口/环境变量/卷/网络/重启策略 + 目标主机。
// 提交后走任务化创建，onCreated 收到 taskID 供外层轮询进度。
export function CreateContainerModal({ onClose, onCreated }) {
  const [name, setName] = useState('')
  const [image, setImage] = useState('')
  const [ports, setPorts] = useState('')     // 每行一条 8080:80/tcp
  const [env, setEnv] = useState('')          // 每行一条 KEY=VALUE
  const [binds, setBinds] = useState('')      // 每行一条 /host:/container:ro
  const [networkMode, setNetworkMode] = useState('')
  const [restartPolicy, setRestartPolicy] = useState('unless-stopped')
  const [autoPull, setAutoPull] = useState(true)
  const [autoStart, setAutoStart] = useState(true)
  const [hosts, setHosts] = useState([])
  const [hostId, setHostId] = useState('local')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')

  // 加载可用 Docker 主机（用于目标主机下拉）
  useEffect(() => {
    dockerHostAPI.list().then((r) => {
      if (r.data?.code === 200 && Array.isArray(r.data.data)) {
        const enabled = r.data.data.filter((h) => h.enabled)
        setHosts(enabled)
      }
    }).catch(() => {})
  }, [])

  // 把多行文本按非空行拆成数组
  const lines = (s) => s.split('\n').map((x) => x.trim()).filter(Boolean)

  const submit = async () => {
    if (!name.trim() || !image.trim()) {
      setError('容器名和镜像为必填')
      return
    }
    setSubmitting(true)
    setError('')
    try {
      const spec = {
        name: name.trim(),
        image: image.trim(),
        env: lines(env),
        portBindings: lines(ports),
        binds: lines(binds),
        networkMode: networkMode.trim(),
        restartPolicy,
        autoPull,
        autoStart,
        hostId: hostId === 'local' ? '' : hostId,
      }
      const r = await containerAPI.createContainer(spec)
      if (r.data?.code === 200) {
        onCreated?.(r.data.data?.taskID)
        onClose()
      } else {
        setError(r.data?.msg || '创建失败')
      }
    } catch (e) {
      setError(e.response?.data?.msg || e.message || '创建失败')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" onClick={onClose}>
      <div className="w-full max-w-2xl max-h-[90vh] overflow-y-auto rounded-xl bg-white dark:bg-gray-800 p-5 shadow-xl"
        onClick={(e) => e.stopPropagation()}>
        <div className="flex items-center justify-between mb-4">
          <h3 className="text-lg font-semibold text-gray-900 dark:text-gray-100 flex items-center gap-2">
            <Plus className="h-5 w-5" /> 创建容器
          </h3>
          <button onClick={onClose} className="text-gray-400 hover:text-gray-600"><X className="h-5 w-5" /></button>
        </div>

        <CreateForm
          name={name} setName={setName} image={image} setImage={setImage}
          ports={ports} setPorts={setPorts} env={env} setEnv={setEnv}
          binds={binds} setBinds={setBinds} networkMode={networkMode} setNetworkMode={setNetworkMode}
          restartPolicy={restartPolicy} setRestartPolicy={setRestartPolicy}
          autoPull={autoPull} setAutoPull={setAutoPull} autoStart={autoStart} setAutoStart={setAutoStart}
          hosts={hosts} hostId={hostId} setHostId={setHostId}
        />

        {error && <p className="text-sm text-red-500 mt-3">{error}</p>}

        <div className="flex justify-end gap-2 mt-5">
          <button onClick={onClose} className="rounded-lg border border-gray-200 dark:border-gray-700 px-4 py-2 text-sm text-gray-600 dark:text-gray-300">取消</button>
          <button onClick={submit} disabled={submitting}
            className="inline-flex items-center gap-1 rounded-lg bg-blue-600 px-4 py-2 text-sm text-white hover:bg-blue-700 disabled:opacity-60">
            {submitting ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />} 创建
          </button>
        </div>
      </div>
    </div>
  )
}

// 输入框样式
const inp = 'w-full rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900 px-3 py-2 text-sm'
const label = 'block text-sm text-gray-600 dark:text-gray-300 mb-1'

// 表单字段布局
function CreateForm(p) {
  return (
    <div className="space-y-3">
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
        <div>
          <label className={label}>容器名 <span className="text-red-500">*</span></label>
          <input value={p.name} onChange={(e) => p.setName(e.target.value)} placeholder="my-app" className={inp} />
        </div>
        <div>
          <label className={label}>镜像 <span className="text-red-500">*</span></label>
          <input value={p.image} onChange={(e) => p.setImage(e.target.value)} placeholder="nginx:latest" className={`${inp} font-mono`} />
        </div>
      </div>

      {/* 目标 Docker 主机 */}
      <div>
        <label className={label}><Server className="inline h-3.5 w-3.5 mr-1" />目标 Docker 主机</label>
        <select value={p.hostId} onChange={(e) => p.setHostId(e.target.value)} className={inp}>
          <option value="local">本地 Docker</option>
          {p.hosts.filter((h) => h.id !== 'local').map((h) => (
            <option key={h.id} value={h.id}>{h.name}{h.online ? '' : '（离线）'}</option>
          ))}
        </select>
      </div>

      <div>
        <label className={label}>端口映射（每行一条，格式 主机端口:容器端口/协议）</label>
        <textarea value={p.ports} onChange={(e) => p.setPorts(e.target.value)} rows={2}
          placeholder={'8080:80/tcp\n443:443'} className={`${inp} font-mono`} />
      </div>

      <div>
        <label className={label}>环境变量（每行一条 KEY=VALUE）</label>
        <textarea value={p.env} onChange={(e) => p.setEnv(e.target.value)} rows={2}
          placeholder={'TZ=Asia/Shanghai\nPUID=1000'} className={`${inp} font-mono`} />
      </div>

      <div>
        <label className={label}>卷 / 绑定挂载（每行一条 /宿主机:/容器:ro）</label>
        <textarea value={p.binds} onChange={(e) => p.setBinds(e.target.value)} rows={2}
          placeholder={'/data/app:/app\n/etc/localtime:/etc/localtime:ro'} className={`${inp} font-mono`} />
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
        <div>
          <label className={label}>网络模式</label>
          <input value={p.networkMode} onChange={(e) => p.setNetworkMode(e.target.value)} placeholder="bridge / host / 自定义网络" className={inp} />
        </div>
        <div>
          <label className={label}>重启策略</label>
          <select value={p.restartPolicy} onChange={(e) => p.setRestartPolicy(e.target.value)} className={inp}>
            <option value="no">不自动重启 (no)</option>
            <option value="unless-stopped">除非手动停止 (unless-stopped)</option>
            <option value="always">总是重启 (always)</option>
            <option value="on-failure">失败时重启 (on-failure)</option>
          </select>
        </div>
      </div>

      <div className="flex items-center gap-6 pt-1">
        <label className="flex items-center gap-2 text-sm text-gray-600 dark:text-gray-300">
          <input type="checkbox" checked={p.autoPull} onChange={(e) => p.setAutoPull(e.target.checked)} /> 本地无镜像时自动拉取
        </label>
        <label className="flex items-center gap-2 text-sm text-gray-600 dark:text-gray-300">
          <input type="checkbox" checked={p.autoStart} onChange={(e) => p.setAutoStart(e.target.checked)} /> 创建后立即启动
        </label>
      </div>
    </div>
  )
}
