import React, { useState } from 'react'
import { Save, Loader2, Wand2, Server, FolderOpen } from 'lucide-react'
import { containerAPI } from '../api/client.js'
import { DirectoryPicker } from './DirectoryPicker.jsx'

// Docker Run Command 页签：粘贴 docker run 命令 → 解析预览 → 选目标主机 → 创建。
// 宿主机路径浏览（DirectoryPicker）仅在目标主机为本地 Docker 时提供，用于快速拼 -v 源路径。
export function RunCommandTab({ hosts, onClose, onCreated }) {
  const [command, setCommand] = useState('')
  const [hostId, setHostId] = useState('local')
  const [autoPull, setAutoPull] = useState(true)
  const [autoStart, setAutoStart] = useState(true)
  const [preview, setPreview] = useState(null)   // 解析后的 spec 预览
  const [parsing, setParsing] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')
  const [showPicker, setShowPicker] = useState(false)

  const isLocal = hostId === 'local'

  // 解析命令为可读 spec
  const parse = async () => {
    if (!command.trim()) { setError('请先粘贴 docker run 命令'); return }
    setParsing(true); setError(''); setPreview(null)
    try {
      const r = await containerAPI.parseRunCommand(command.trim())
      if (r.data?.code === 200) {
        setPreview(r.data.data)
      } else {
        setError(r.data?.msg || '解析失败')
      }
    } catch (e) {
      setError(e.response?.data?.msg || e.message || '解析失败')
    } finally { setParsing(false) }
  }

  // 用解析结果创建容器
  const submit = async () => {
    if (!preview?.image) { setError('请先解析出有效的镜像'); return }
    if (!preview?.name) { setError('命令中缺少 --name，请补充容器名'); return }
    setSubmitting(true); setError('')
    try {
      const spec = {
        name: preview.name, image: preview.image,
        env: preview.env || [], portBindings: preview.portBindings || [],
        binds: preview.binds || [], networkMode: preview.networkMode || '',
        restartPolicy: preview.restartPolicy || '', labels: preview.labels || undefined,
        cmd: preview.cmd || [], entrypoint: preview.entrypoint || [],
        autoPull, autoStart, hostId: isLocal ? '' : hostId,
      }
      const r = await containerAPI.createContainer(spec)
      if (r.data?.code === 200) {
        onCreated?.(r.data.data?.taskID); onClose()
      } else {
        setError(r.data?.msg || '创建失败')
      }
    } catch (e) {
      setError(e.response?.data?.msg || e.message || '创建失败')
    } finally { setSubmitting(false) }
  }

  return (
    <div className="space-y-4">
      {/* 目标主机 */}
      <div>
        <label className={label}><Server className="inline h-3.5 w-3.5 mr-1" />目标 Docker 主机</label>
        <select value={hostId} onChange={(e) => { setHostId(e.target.value); setPreview(null) }} className={inp}>
          <option value="local">本地 Docker</option>
          {hosts.filter((h) => h.id !== 'local').map((h) => (
            <option key={h.id} value={h.id}>{h.name}{h.online ? '' : '（离线）'}</option>
          ))}
        </select>
      </div>

      {/* 命令输入 */}
      <div>
        <div className="flex items-center justify-between mb-1">
          <label className={label}>Docker Run 命令</label>
          {isLocal && (
            <button type="button" onClick={() => setShowPicker(true)}
              className="text-xs text-blue-600 hover:text-blue-700 flex items-center gap-1">
              <FolderOpen className="h-3.5 w-3.5" /> 浏览本机目录
            </button>
          )}
        </div>
        <textarea value={command} onChange={(e) => setCommand(e.target.value)} rows={5}
          placeholder={'docker run -d --name nginx \\\n  -p 8080:80 -v /data/html:/usr/share/nginx/html \\\n  --restart unless-stopped nginx:latest'}
          className={`${inp} font-mono text-xs`} spellCheck={false} />
        {!isLocal && (
          <p className="text-xs text-amber-600 mt-1">远程主机：-v 挂载请填写该远程主机上的真实路径（无法浏览远程目录）。</p>
        )}
      </div>

      <button type="button" onClick={parse} disabled={parsing}
        className="inline-flex items-center gap-1.5 rounded-lg bg-gray-100 dark:bg-gray-700 px-4 py-2 text-sm hover:bg-gray-200 dark:hover:bg-gray-600 disabled:opacity-60">
        {parsing ? <Loader2 className="h-4 w-4 animate-spin" /> : <Wand2 className="h-4 w-4" />} 解析命令
      </button>

      {preview && <PreviewCard preview={preview} />}

      <div className="flex items-center gap-6 pt-1">
        <label className="flex items-center gap-2 text-sm text-gray-600 dark:text-gray-300">
          <input type="checkbox" checked={autoPull} onChange={(e) => setAutoPull(e.target.checked)} /> 本地无镜像时自动拉取
        </label>
        <label className="flex items-center gap-2 text-sm text-gray-600 dark:text-gray-300">
          <input type="checkbox" checked={autoStart} onChange={(e) => setAutoStart(e.target.checked)} /> 创建后立即启动
        </label>
      </div>

      {error && <p className="text-sm text-red-500">{error}</p>}

      <div className="flex justify-end gap-2 pt-1">
        <button onClick={onClose} className="rounded-lg border border-gray-200 dark:border-gray-700 px-4 py-2 text-sm text-gray-600 dark:text-gray-300">取消</button>
        <button onClick={submit} disabled={submitting || !preview}
          className="inline-flex items-center gap-1 rounded-lg bg-blue-600 px-4 py-2 text-sm text-white hover:bg-blue-700 disabled:opacity-60">
          {submitting ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />} 创建
        </button>
      </div>

      {showPicker && (
        <DirectoryPicker onClose={() => setShowPicker(false)}
          onSelect={(p) => setCommand((c) => `${c}${c && !c.endsWith(' ') ? ' ' : ''}-v ${p}:`)} />
      )}
    </div>
  )
}

const inp = 'w-full rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900 px-3 py-2 text-sm'
const label = 'block text-sm text-gray-600 dark:text-gray-300 mb-1'

// 解析结果只读预览：让用户确认参数正确后再创建
function PreviewCard({ preview }) {
  const rows = [
    ['容器名', preview.name || <span className="text-red-500">（缺少 --name，请在命令中补充）</span>],
    ['镜像', preview.image],
    ['端口映射', arr(preview.portBindings)],
    ['环境变量', arr(preview.env)],
    ['挂载', arr(preview.binds)],
    ['网络模式', preview.networkMode || '默认'],
    ['重启策略', preview.restartPolicy || '默认'],
    ['启动命令', arr(preview.cmd)],
  ]
  return (
    <div className="rounded-lg border border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-900/50 p-3 space-y-1.5">
      <p className="text-xs font-semibold text-gray-500 mb-1">解析结果（确认无误后点创建）</p>
      {rows.map(([k, v]) => (
        <div key={k} className="grid grid-cols-[80px_1fr] gap-2 text-xs">
          <span className="text-gray-500">{k}</span>
          <span className="font-mono text-gray-800 dark:text-gray-200 break-all">{v}</span>
        </div>
      ))}
    </div>
  )
}

// 数组类字段渲染：空则显示占位
function arr(v) {
  if (!Array.isArray(v) || v.length === 0) return <span className="text-gray-400">—</span>
  return v.join('  ')
}
