import React, { useState } from 'react'
import { X, Upload, Link as LinkIcon, Download, Loader2, Package, Image as ImageIcon } from 'lucide-react'
import { imageAPI } from '../api/client.js'
import { cn } from '../utils/cn.js'

// 容器图标编辑面板：效果预览 + 三种方式（本地上传 / 在线URL / 自动获取并持久化）。
// imageName 为绑定的镜像名 key；container 提供端口用于自动获取地址；
// onApplied(url) 在设置成功后回调，用于刷新外层显示。
export function IconEditor({ imageName, container, currentIconUrl, onClose, onApplied }) {
  const [tab, setTab] = useState('upload') // upload | url | auto
  const [preview, setPreview] = useState(currentIconUrl || '')
  const [urlInput, setUrlInput] = useState('')
  const [busy, setBusy] = useState(false)
  const [msg, setMsg] = useState('')
  const fileRef = React.useRef(null)

  // 候选访问地址：宿主机 host + 映射端口/暴露端口
  const host = window.location.hostname || 'localhost'
  const ports = (container?.ports?.length ? container.ports
    : (container?.networkMode === 'host' ? container?.exposedPorts : [])) || []

  const done = (url) => { onApplied?.(url); setBusy(false) }
  const fail = (e) => { setMsg(e?.response?.data?.msg || e?.message || '操作失败'); setBusy(false) }

  // 本地上传
  const handleUpload = async (e) => {
    const file = e.target.files?.[0]
    if (!file) return
    if (file.size > 2 * 1024 * 1024) { setMsg('图片不能超过 2MB'); return }
    setBusy(true); setMsg('')
    try {
      const r = await imageAPI.uploadIcon(file, imageName, container?.name)
      if (r.data.code === 200 || r.data.code === 0) {
        // 后端已统一存 /data/images，通过 /images/ 静态路由访问
        const p = r.data.data ? `/images/${r.data.data}` : preview
        setPreview(p); done(p)
      } else throw new Error(r.data.msg)
    } catch (err) { fail(err) } finally { if (fileRef.current) fileRef.current.value = '' }
  }

  // 在线 URL
  const handleUrl = async () => {
    if (!urlInput.trim()) { setMsg('请输入图标 URL'); return }
    setBusy(true); setMsg('')
    try {
      const r = await imageAPI.setIconUrl(imageName, urlInput.trim())
      if (r.data.code === 200 || r.data.code === 0) { setPreview(urlInput.trim()); done(urlInput.trim()) }
      else throw new Error(r.data.msg)
    } catch (err) { fail(err) }
  }

  // 自动获取并持久化
  const handleAuto = async (accessUrl) => {
    setBusy(true); setMsg('正在抓取图标...')
    try {
      const r = await imageAPI.fetchIcon(imageName, accessUrl)
      if (r.data.code === 200 || r.data.code === 0) { setPreview(r.data.data); setMsg(''); done(r.data.data) }
      else { setMsg(r.data.msg || '未获取到图标'); setBusy(false) }
    } catch (err) { fail(err) }
  }

  return (
    <div className="fixed inset-0 bg-black/50 z-[70] flex items-center justify-center p-4" onClick={onClose}>
      <div className="bg-white dark:bg-gray-800 rounded-xl p-5 w-full max-w-md" onClick={(e) => e.stopPropagation()}>
        <div className="flex items-center justify-between mb-4">
          <h3 className="text-base font-bold text-gray-900 dark:text-white">设置容器图标</h3>
          <button onClick={onClose} className="p-1.5 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 rounded hover:bg-gray-100 dark:hover:bg-gray-700">
            <X className="h-5 w-5" />
          </button>
        </div>

        {/* 效果预览 */}
        <div className="flex items-center gap-3 mb-4 p-3 rounded-lg bg-gray-50 dark:bg-gray-900/40">
          <div className="h-14 w-14 rounded-xl bg-white dark:bg-gray-800 flex items-center justify-center overflow-hidden border border-gray-200 dark:border-gray-700">
            {preview ? (
              <img src={preview} alt="预览" className="h-full w-full object-contain"
                onError={(e) => { e.target.style.display = 'none' }} />
            ) : (
              <Package className="h-6 w-6 text-gray-400" />
            )}
          </div>
          <div className="min-w-0">
            <div className="text-xs text-gray-400">效果预览</div>
            <div className="text-sm text-gray-700 dark:text-gray-200 truncate">{imageName}</div>
          </div>
        </div>

        {/* 方式切换 */}
        <div className="flex gap-1 mb-3 p-1 rounded-lg bg-gray-100 dark:bg-gray-700/50">
          <TabBtn active={tab === 'upload'} onClick={() => setTab('upload')} icon={Upload} label="上传图片" />
          <TabBtn active={tab === 'url'} onClick={() => setTab('url')} icon={LinkIcon} label="在线URL" />
          <TabBtn active={tab === 'auto'} onClick={() => setTab('auto')} icon={Download} label="自动获取" />
        </div>

        {tab === 'upload' && (
          <div>
            <input ref={fileRef} type="file" className="hidden" onChange={handleUpload}
              accept="image/png,image/jpeg,image/webp,image/svg+xml,image/gif" />
            <button onClick={() => fileRef.current?.click()} disabled={busy}
              className="w-full flex items-center justify-center gap-2 px-4 py-2.5 rounded-lg border-2 border-dashed border-gray-300 dark:border-gray-600 text-gray-600 dark:text-gray-300 hover:border-primary-500 disabled:opacity-60">
              <ImageIcon className="h-4 w-4" /> 选择本地图片（≤2MB）
            </button>
          </div>
        )}

        {tab === 'url' && (
          <div className="flex gap-2">
            <input value={urlInput} onChange={(e) => setUrlInput(e.target.value)} placeholder="https://.../icon.png"
              className="flex-1 px-3 py-2 text-sm border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-900" />
            <button onClick={handleUrl} disabled={busy}
              className="px-4 py-2 rounded-lg bg-primary-600 text-white text-sm hover:bg-primary-700 disabled:opacity-60">确定</button>
          </div>
        )}

        {tab === 'auto' && (
          <div>
            <p className="text-xs text-gray-500 mb-2">从容器站点自动抓取图标并保存到服务器（换设备也能显示）。选择访问地址：</p>
            {ports.length > 0 ? (
              <div className="flex flex-wrap gap-2">
                {ports.map((p) => (
                  <button key={p} onClick={() => handleAuto(`http://${host}:${p}`)} disabled={busy}
                    className="px-3 py-1.5 rounded-lg bg-gray-100 dark:bg-gray-700 text-sm text-gray-700 dark:text-gray-200 hover:bg-gray-200 dark:hover:bg-gray-600 disabled:opacity-60">
                    {host}:{p}
                  </button>
                ))}
              </div>
            ) : (
              <AutoManual host={host} busy={busy} onFetch={handleAuto} />
            )}
          </div>
        )}

        {msg && <div className="mt-3 text-xs text-amber-600 dark:text-amber-400 flex items-center gap-1">
          {busy && <Loader2 className="h-3.5 w-3.5 animate-spin" />}{msg}</div>}
      </div>
    </div>
  )
}

// 无端口时手动输入访问地址
function AutoManual({ host, busy, onFetch }) {
  const [v, setV] = useState(`http://${host}:`)
  return (
    <div className="flex gap-2">
      <input value={v} onChange={(e) => setV(e.target.value)} placeholder="http://ip:port"
        className="flex-1 px-3 py-2 text-sm border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-900" />
      <button onClick={() => onFetch(v)} disabled={busy}
        className="px-4 py-2 rounded-lg bg-primary-600 text-white text-sm hover:bg-primary-700 disabled:opacity-60">抓取</button>
    </div>
  )
}

function TabBtn({ active, onClick, icon: Icon, label }) {
  return (
    <button onClick={onClick}
      className={cn('flex-1 flex items-center justify-center gap-1 px-2 py-1.5 rounded-md text-xs font-medium transition-colors',
        active ? 'bg-white dark:bg-gray-800 text-primary-600 dark:text-primary-400 shadow-sm' : 'text-gray-500 dark:text-gray-400')}>
      <Icon className="h-3.5 w-3.5" /> {label}
    </button>
  )
}
