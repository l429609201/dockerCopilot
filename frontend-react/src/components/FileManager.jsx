import React, { useState, useEffect, useCallback, useRef } from 'react'
import {
  Folder, File as FileIcon, ArrowLeft, RefreshCw, Upload, FolderPlus,
  Maximize2, Minimize2, X, Home, ChevronRight, Loader2, Download, Trash2, Edit3, Save,
} from 'lucide-react'
import { filesAPI } from '../api/client.js'
import { cn } from '../utils/cn.js'

// 容器文件管理器弹窗：面包屑导航、目录表格、右键菜单、上传/下载/编辑。
// 所有路径安全（防穿越）由后端统一校验，前端仅做交互。
export function FileManager({ container, onClose }) {
  const [fullscreen, setFullscreen] = useState(false)
  const [path, setPath] = useState('/')
  const [entries, setEntries] = useState([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [menu, setMenu] = useState(null) // 右键菜单 { x, y, entry }
  const [editing, setEditing] = useState(null) // 文本编辑 { name, path, content }
  const fileInputRef = useRef(null)
  const id = container.id

  const load = useCallback(async (p) => {
    setLoading(true); setError('')
    try {
      const r = await filesAPI.list(id, p)
      if (r.data?.code === 200) {
        setEntries(r.data.data.entries || [])
      } else {
        setError(r.data?.msg || '列目录失败'); setEntries([])
      }
    } catch (e) {
      setError(e.response?.data?.msg || e.message); setEntries([])
    } finally { setLoading(false) }
  }, [id])

  useEffect(() => { load(path) }, [path, load])

  // 路径拼接
  const joinPath = (dir, name) => (dir === '/' ? `/${name}` : `${dir}/${name}`)
  const segments = path === '/' ? [] : path.split('/').filter(Boolean)

  const enterDir = (name) => setPath(joinPath(path, name))
  const goUp = () => {
    if (path === '/') return
    const parts = path.split('/').filter(Boolean)
    parts.pop()
    setPath('/' + parts.join('/'))
  }
  const goCrumb = (idx) => setPath('/' + segments.slice(0, idx + 1).join('/'))

  // 下载
  const handleDownload = async (entry) => {
    try {
      const r = await filesAPI.download(id, joinPath(path, entry.name))
      const url = URL.createObjectURL(r.data)
      const a = document.createElement('a')
      a.href = url; a.download = entry.name; a.click()
      URL.revokeObjectURL(url)
    } catch (e) { alert('下载失败：' + (e.response?.data?.msg || e.message)) }
  }

  // 删除
  const handleDelete = async (entry) => {
    if (!confirm(`确定删除 ${entry.name}${entry.isDir ? '（含其下所有内容）' : ''}？此操作不可恢复`)) return
    try {
      await filesAPI.remove(id, joinPath(path, entry.name))
      load(path)
    } catch (e) { alert('删除失败：' + (e.response?.data?.msg || e.message)) }
  }

  // 重命名
  const handleRename = async (entry) => {
    const newName = prompt('新名称', entry.name)
    if (!newName || newName === entry.name) return
    try {
      await filesAPI.rename(id, joinPath(path, entry.name), joinPath(path, newName))
      load(path)
    } catch (e) { alert('重命名失败：' + (e.response?.data?.msg || e.message)) }
  }

  // 新建目录
  const handleMkdir = async () => {
    const name = prompt('新目录名')
    if (!name) return
    try {
      await filesAPI.mkdir(id, path, name)
      load(path)
    } catch (e) { alert('创建失败：' + (e.response?.data?.msg || e.message)) }
  }

  // 上传
  const handleUpload = async (e) => {
    const file = e.target.files?.[0]
    if (!file) return
    try {
      await filesAPI.upload(id, path, file)
      load(path)
    } catch (err) { alert('上传失败：' + (err.response?.data?.msg || err.message)) }
    finally { e.target.value = '' }
  }

  // 打开文本编辑
  const handleEdit = async (entry) => {
    try {
      const r = await filesAPI.read(id, joinPath(path, entry.name))
      if (r.data?.code === 200) {
        setEditing({ name: entry.name, path: joinPath(path, entry.name), content: r.data.data.content, truncated: r.data.data.truncated })
      } else { alert(r.data?.msg || '读取失败') }
    } catch (e) { alert('读取失败：' + (e.response?.data?.msg || e.message)) }
  }

  return (
    <div className={cn('fixed inset-0 bg-black/50 z-50 flex items-center justify-center', fullscreen ? 'p-0' : 'p-4')}
      onClick={() => setMenu(null)}>
      <div className={cn('bg-white dark:bg-gray-800 flex flex-col',
        fullscreen ? 'w-screen h-screen rounded-none p-4' : 'w-full max-w-4xl h-[85vh] rounded-xl p-5')}>
        {/* 头部 */}
        <div className="flex items-center justify-between mb-3">
          <h3 className="text-lg font-bold text-gray-900 dark:text-white truncate">
            文件管理 · {container.name}
          </h3>
          <div className="flex items-center gap-1">
            <button onClick={() => setFullscreen(v => !v)} title={fullscreen ? '还原' : '全屏'}
              className="p-1.5 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 rounded hover:bg-gray-100 dark:hover:bg-gray-700">
              {fullscreen ? <Minimize2 className="h-4.5 w-4.5" /> : <Maximize2 className="h-4.5 w-4.5" />}
            </button>
            <button onClick={onClose} title="关闭"
              className="p-1.5 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 rounded hover:bg-gray-100 dark:hover:bg-gray-700">
              <X className="h-5 w-5" />
            </button>
          </div>
        </div>

        <FileToolbar
          segments={segments} onHome={() => setPath('/')} onCrumb={goCrumb}
          onUp={goUp} onRefresh={() => load(path)} onMkdir={handleMkdir}
          onUpload={() => fileInputRef.current?.click()} canUp={path !== '/'} />
        <input ref={fileInputRef} type="file" className="hidden" onChange={handleUpload} />

        <FileTable
          entries={entries} loading={loading} error={error}
          onEnter={enterDir}
          onContext={(e, entry) => { e.preventDefault(); setMenu({ x: e.clientX, y: e.clientY, entry }) }} />

        <div className="mt-2 text-xs text-gray-400">
          提示：右键点击文件可打开操作菜单 · 共 {entries.length} 个项目
        </div>
      </div>

      {menu && (
        <FileContextMenu menu={menu} onClose={() => setMenu(null)}
          onDownload={handleDownload} onEdit={handleEdit}
          onRename={handleRename} onDelete={handleDelete} />
      )}
      {editing && (
        <TextEditor id={id} editing={editing} onClose={() => setEditing(null)} onSaved={() => { setEditing(null); load(path) }} />
      )}
    </div>
  )
}

// 工具栏：面包屑 + 返回上级 + 刷新 + 新建目录 + 上传
function FileToolbar({ segments, onHome, onCrumb, onUp, onRefresh, onMkdir, onUpload, canUp }) {
  return (
    <div className="flex items-center gap-2 mb-3 flex-wrap">
      <button onClick={onUp} disabled={!canUp} title="上级"
        className="p-1.5 rounded-lg border border-gray-200 dark:border-gray-700 disabled:opacity-40 hover:bg-gray-100 dark:hover:bg-gray-700">
        <ArrowLeft className="h-4 w-4" />
      </button>
      {/* 面包屑 */}
      <div className="flex items-center gap-1 flex-1 min-w-0 overflow-x-auto text-sm">
        <button onClick={onHome} className="p-1 rounded hover:bg-gray-100 dark:hover:bg-gray-700 text-gray-600 dark:text-gray-300">
          <Home className="h-4 w-4" />
        </button>
        {segments.map((s, i) => (
          <React.Fragment key={i}>
            <ChevronRight className="h-3.5 w-3.5 text-gray-400 flex-shrink-0" />
            <button onClick={() => onCrumb(i)} className="px-1 rounded hover:bg-gray-100 dark:hover:bg-gray-700 text-gray-700 dark:text-gray-200 truncate">
              {s}
            </button>
          </React.Fragment>
        ))}
      </div>
      <button onClick={onRefresh} title="刷新" className="p-1.5 rounded-lg border border-gray-200 dark:border-gray-700 hover:bg-gray-100 dark:hover:bg-gray-700">
        <RefreshCw className="h-4 w-4" />
      </button>
      <button onClick={onMkdir} title="新建目录" className="p-1.5 rounded-lg border border-gray-200 dark:border-gray-700 hover:bg-gray-100 dark:hover:bg-gray-700">
        <FolderPlus className="h-4 w-4" />
      </button>
      <button onClick={onUpload} className="flex items-center gap-1 px-3 py-1.5 rounded-lg bg-primary-600 text-white text-sm hover:bg-primary-700">
        <Upload className="h-4 w-4" /> 上传
      </button>
    </div>
  )
}

// 文件表格
function FileTable({ entries, loading, error, onEnter, onContext }) {
  return (
    <div className="flex-1 min-h-0 overflow-auto border border-gray-100 dark:border-gray-700 rounded-lg">
      <table className="w-full text-sm">
        <thead className="sticky top-0 bg-gray-50 dark:bg-gray-800 text-gray-500 dark:text-gray-400">
          <tr className="text-left">
            <th className="px-3 py-2 font-medium">文件名</th>
            <th className="px-3 py-2 font-medium w-24">大小</th>
            <th className="px-3 py-2 font-medium w-40 hidden sm:table-cell">修改时间</th>
            <th className="px-3 py-2 font-medium w-32 hidden md:table-cell">权限</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-gray-100 dark:divide-gray-800">
          {loading ? (
            <tr><td colSpan={4} className="px-3 py-8 text-center text-gray-400"><Loader2 className="h-5 w-5 animate-spin inline" /> 加载中</td></tr>
          ) : error ? (
            <tr><td colSpan={4} className="px-3 py-8 text-center text-red-500">{error}</td></tr>
          ) : entries.length === 0 ? (
            <tr><td colSpan={4} className="px-3 py-8 text-center text-gray-400">空目录</td></tr>
          ) : entries.map((e) => (
            <tr key={e.name}
              onContextMenu={(ev) => onContext(ev, e)}
              onDoubleClick={() => e.isDir && onEnter(e.name)}
              className="hover:bg-gray-50 dark:hover:bg-gray-700/50 cursor-default">
              <td className="px-3 py-2">
                <button onClick={() => e.isDir && onEnter(e.name)}
                  className={cn('flex items-center gap-2 text-left', e.isDir && 'text-primary-600 dark:text-primary-400 hover:underline')}>
                  {e.isDir ? <Folder className="h-4 w-4 flex-shrink-0 text-amber-500" /> : <FileIcon className="h-4 w-4 flex-shrink-0 text-gray-400" />}
                  <span className="break-all">{e.name}</span>
                  {e.isLink && e.link && <span className="text-xs text-gray-400">→ {e.link}</span>}
                </button>
              </td>
              <td className="px-3 py-2 text-gray-500 tabular-nums">{e.isDir ? '-' : formatSize(e.size)}</td>
              <td className="px-3 py-2 text-gray-500 hidden sm:table-cell">{e.modTime}</td>
              <td className="px-3 py-2 text-gray-400 font-mono text-xs hidden md:table-cell">{e.mode}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

function formatSize(n) {
  if (n < 1024) return n + ' B'
  if (n < 1024 * 1024) return (n / 1024).toFixed(1) + ' KB'
  if (n < 1024 * 1024 * 1024) return (n / 1024 / 1024).toFixed(1) + ' MB'
  return (n / 1024 / 1024 / 1024).toFixed(2) + ' GB'
}

// 右键操作菜单
function FileContextMenu({ menu, onClose, onDownload, onEdit, onRename, onDelete }) {
  const { x, y, entry } = menu
  // 防止菜单超出视口右/下边界
  const style = { left: Math.min(x, window.innerWidth - 160), top: Math.min(y, window.innerHeight - 200) }
  const act = (fn) => { onClose(); fn(entry) }
  return (
    <div className="fixed z-[60] w-40 py-1 bg-white dark:bg-gray-800 rounded-lg shadow-xl border border-gray-200 dark:border-gray-700 text-sm"
      style={style} onClick={(e) => e.stopPropagation()}>
      {!entry.isDir && (
        <>
          <MenuItem icon={Download} label="下载" onClick={() => act(onDownload)} />
          <MenuItem icon={Edit3} label="编辑文本" onClick={() => act(onEdit)} />
        </>
      )}
      <MenuItem icon={Edit3} label="重命名" onClick={() => act(onRename)} />
      <MenuItem icon={Trash2} label="删除" danger onClick={() => act(onDelete)} />
    </div>
  )
}

function MenuItem({ icon: Icon, label, onClick, danger }) {
  return (
    <button onClick={onClick}
      className={cn('flex items-center gap-2 w-full px-3 py-1.5 text-left hover:bg-gray-100 dark:hover:bg-gray-700',
        danger ? 'text-red-600 dark:text-red-400' : 'text-gray-700 dark:text-gray-200')}>
      <Icon className="h-4 w-4" /> {label}
    </button>
  )
}

// 文本编辑器弹窗
function TextEditor({ id, editing, onClose, onSaved }) {
  const [content, setContent] = useState(editing.content)
  const [saving, setSaving] = useState(false)

  const save = async () => {
    setSaving(true)
    try {
      await filesAPI.write(id, editing.path, content)
      onSaved()
    } catch (e) { alert('保存失败：' + (e.response?.data?.msg || e.message)) }
    finally { setSaving(false) }
  }

  return (
    <div className="fixed inset-0 bg-black/50 z-[70] flex items-center justify-center p-4" onClick={onClose}>
      <div className="bg-white dark:bg-gray-800 rounded-xl p-5 w-full max-w-3xl h-[80vh] flex flex-col" onClick={(e) => e.stopPropagation()}>
        <div className="flex items-center justify-between mb-3">
          <h3 className="text-base font-bold text-gray-900 dark:text-white truncate">编辑 · {editing.name}</h3>
          <button onClick={onClose} className="p-1.5 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 rounded hover:bg-gray-100 dark:hover:bg-gray-700">
            <X className="h-5 w-5" />
          </button>
        </div>
        {editing.truncated && (
          <div className="mb-2 px-3 py-1.5 text-xs rounded-lg bg-amber-50 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300">
            文件超过 1MB 预览上限，内容已截断，保存将覆盖为当前显示内容，请谨慎操作。
          </div>
        )}
        <textarea value={content} onChange={(e) => setContent(e.target.value)} spellCheck={false}
          className="flex-1 min-h-0 w-full p-3 font-mono text-xs bg-gray-900 text-gray-100 rounded-lg resize-none outline-none" />
        <div className="flex justify-end gap-2 mt-3">
          <button onClick={onClose} className="px-4 py-2 rounded-lg border border-gray-200 dark:border-gray-700 text-sm text-gray-600 dark:text-gray-300">取消</button>
          <button onClick={save} disabled={saving}
            className="flex items-center gap-1 px-4 py-2 rounded-lg bg-primary-600 text-white text-sm hover:bg-primary-700 disabled:opacity-60">
            {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />} 保存
          </button>
        </div>
      </div>
    </div>
  )
}
