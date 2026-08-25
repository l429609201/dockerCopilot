import React, { useState, useRef } from 'react'
import { Save, Loader2, FolderOpen, CheckCircle2, AlertTriangle } from 'lucide-react'
import { composeAPI } from '../api/client.js'
import { DirectoryPicker } from './DirectoryPicker.jsx'
import { HostPathImportButton } from './HostPathImportButton.jsx'

// Docker Compose (YAML) 页签：填工作目录(仅本机) + 项目名 + 文件名 + YAML → 校验 → 部署。
// Compose 由本机 docker compose 执行，故工作目录浏览始终针对本机文件系统。
export function ComposeDeployTab({ onClose, onCreated }) {
  const [workingDir, setWorkingDir] = useState('')
  const [filename, setFilename] = useState('docker-compose.yml')
  const [content, setContent] = useState('')
  const contentRef = useRef(null) // YAML 文本域 ref，用于在光标处插入引入的宿主机路径
  const [msg, setMsg] = useState('')
  const [warnings, setWarnings] = useState([])
  const [validating, setValidating] = useState(false)
  const [deploying, setDeploying] = useState(false)
  const [error, setError] = useState('')
  const [showPicker, setShowPicker] = useState(false)
  const [needConfirm, setNeedConfirm] = useState(false) // 409 高风险待确认

  // 校验 YAML 语法与风险
  const validate = async () => {
    if (!content.trim()) { setError('请先填写 compose 内容'); return }
    setValidating(true); setError(''); setMsg(''); setWarnings([])
    try {
      const r = await composeAPI.validate(content)
      const d = r.data?.data
      if (d?.valid) {
        setWarnings(d.warnings || [])
        setMsg(d.warnings?.length ? '语法正确，但存在风险提示' : '校验通过')
      } else {
        setError('校验失败：' + (d?.error || '未知'))
      }
    } catch (e) {
      setError('校验失败：' + (e.response?.data?.msg || e.message))
    } finally { setValidating(false) }
  }

  // 部署：写入工作目录后 up。confirm=true 表示已确认高风险
  const deploy = async (confirm = false) => {
    if (!workingDir.trim()) { setError('请选择工作目录'); return }
    if (!content.trim()) { setError('请填写 compose 内容'); return }
    setDeploying(true); setError(''); setMsg('')
    try {
      const r = await composeAPI.create({
        workingDir: workingDir.trim(), filename: filename.trim(),
        content, confirmWarnings: confirm,
      })
      if (r.data?.code === 200) {
        onCreated?.(r.data.data?.taskID); onClose()
      } else if (r.data?.code === 409) {
        // 高风险：展示警告并要求二次确认
        setWarnings(r.data.data?.warnings || [])
        setNeedConfirm(true)
        setError('存在高风险配置，请确认后再部署')
      } else {
        setError(r.data?.msg || '部署失败')
      }
    } catch (e) {
      setError(e.response?.data?.msg || e.message || '部署失败')
    } finally { setDeploying(false) }
  }

  // 在 YAML 文本域当前光标位置插入宿主机路径（纯路径），无选区时插入到末尾
  const insertAtCursor = (text) => {
    const ta = contentRef.current
    if (!ta) { setContent((c) => c + text); return }
    const start = ta.selectionStart ?? content.length
    const end = ta.selectionEnd ?? content.length
    const next = content.slice(0, start) + text + content.slice(end)
    setContent(next)
    setNeedConfirm(false)
    // 恢复光标到插入内容之后
    requestAnimationFrame(() => {
      ta.focus()
      const pos = start + text.length
      ta.setSelectionRange(pos, pos)
    })
  }

  return (
    <div className="space-y-4">
      {/* 工作目录 */}
      <div>
        <label className={label}>工作目录（宿主机上的绝对路径）<span className="text-red-500">*</span></label>
        <div className="flex gap-2">
          <input value={workingDir} onChange={(e) => setWorkingDir(e.target.value)}
            placeholder="/compose/my-app" className={`${inp} font-mono flex-1`} />
          <button type="button" onClick={() => setShowPicker(true)}
            className="shrink-0 inline-flex items-center gap-1 rounded-lg border border-gray-200 dark:border-gray-700 px-3 text-sm text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700">
            <FolderOpen className="h-4 w-4" /> 浏览
          </button>
        </div>
        <p className="text-xs text-gray-400 mt-1">目录不存在时将自动创建。Compose 在本机执行。</p>
      </div>

      {/* 文件名 */}
      <div>
        <label className={label}>配置文件名</label>
        <select value={filename} onChange={(e) => setFilename(e.target.value)} className={inp}>
          <option value="docker-compose.yml">docker-compose.yml</option>
          <option value="docker-compose.yaml">docker-compose.yaml</option>
          <option value="compose.yml">compose.yml</option>
          <option value="compose.yaml">compose.yaml</option>
        </select>
      </div>

      {/* YAML 编辑器 */}
      <div>
        <div className="flex items-center justify-between mb-1">
          <label className={label + ' mb-0'}>Compose 内容 (YAML)</label>
          {/* 引入宿主机路径：浏览 /compose 映射目录，转成宿主机真实路径插入光标处。Compose 本机执行故 isLocal 恒真 */}
          <HostPathImportButton isLocal onPick={insertAtCursor} />
        </div>
        <textarea ref={contentRef} value={content} onChange={(e) => { setContent(e.target.value); setNeedConfirm(false) }} rows={12}
          placeholder={'services:\n  web:\n    image: nginx:latest\n    ports:\n      - "8080:80"\n    restart: unless-stopped'}
          className={`${inp} font-mono text-xs`} spellCheck={false} />
      </div>

      {warnings.length > 0 && (
        <div className="p-2 bg-amber-50 dark:bg-amber-900/20 text-amber-700 dark:text-amber-300 rounded-lg text-xs space-y-0.5">
          {warnings.map((w, i) => <div key={i} className="flex items-start gap-1"><AlertTriangle className="h-3.5 w-3.5 mt-0.5 shrink-0" />{w}</div>)}
        </div>
      )}
      {msg && <p className="text-sm text-green-600 flex items-center gap-1"><CheckCircle2 className="h-4 w-4" />{msg}</p>}
      {error && <p className="text-sm text-red-500">{error}</p>}

      <div className="flex justify-end gap-2 pt-1">
        <button onClick={onClose} className="rounded-lg border border-gray-200 dark:border-gray-700 px-4 py-2 text-sm text-gray-600 dark:text-gray-300">取消</button>
        <button onClick={validate} disabled={validating}
          className="rounded-lg bg-gray-100 dark:bg-gray-700 px-4 py-2 text-sm hover:bg-gray-200 dark:hover:bg-gray-600 disabled:opacity-60">
          {validating ? '校验中...' : '校验'}
        </button>
        <button onClick={() => deploy(needConfirm)} disabled={deploying}
          className={`inline-flex items-center gap-1 rounded-lg px-4 py-2 text-sm text-white disabled:opacity-60 ${
            needConfirm ? 'bg-amber-600 hover:bg-amber-700' : 'bg-blue-600 hover:bg-blue-700'}`}>
          {deploying ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}
          {needConfirm ? '确认并部署' : '部署'}
        </button>
      </div>

      {showPicker && (
        <DirectoryPicker initialPath={workingDir} onClose={() => setShowPicker(false)}
          onSelect={(p) => setWorkingDir(p)} />
      )}
    </div>
  )
}

const inp = 'w-full rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900 px-3 py-2 text-sm'
const label = 'block text-sm text-gray-600 dark:text-gray-300 mb-1'
