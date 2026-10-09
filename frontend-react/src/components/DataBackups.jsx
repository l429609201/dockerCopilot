import React, { useCallback, useEffect, useRef, useState } from 'react'
import { DatabaseBackup, Download, RefreshCw, Trash2, AlertTriangle, Loader2, Plus, X, Search, Folder, Box, ChevronRight, FileCode, Info } from 'lucide-react'
import { backupAPI, dockerHostAPI } from '../api/client.js'
import { useTasks } from '../hooks/useTasks.jsx'
import { useProgress } from '../hooks/useProgress.js'

const mountKey = (m) => `${m.type}:${m.type === 'volume' ? m.name : m.source}`
const bytes = (n) => n >= 1073741824 ? `${(n / 1073741824).toFixed(2)} GB` : `${(n / 1048576).toFixed(2)} MB`
const bodyData = (r) => {
  if (r.data?.code !== 200) throw new Error(r.data?.msg || '请求失败')
  return r.data.data
}
const inputClass = 'w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-900 px-3 py-2 text-sm outline-none focus:border-blue-500 focus:ring-2 focus:ring-blue-500/15'
const iconClass = 'inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-lg text-gray-500 hover:bg-gray-100 dark:hover:bg-gray-700 disabled:opacity-40'
const primaryClass = 'inline-flex items-center justify-center gap-2 rounded-lg bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-40'

// 归档列表与创建流程分离，资源列表始终限制高度，长路径不撑开布局。
export function DataBackups() {
  const [hosts, setHosts] = useState([])
  const [hostId, setHostId] = useState('local')
  const [hostFilter, setHostFilter] = useState('all')
  const [resources, setResources] = useState(null)
  const [selected, setSelected] = useState([])
  const [projectIds, setProjectIds] = useState([])
  const [mounts, setMounts] = useState([])
  const [archives, setArchives] = useState([])
  const [includeData, setIncludeData] = useState(true)
  const [includeCompose, setIncludeCompose] = useState(true)
  const [includeEnv, setIncludeEnv] = useState(false)
  const [stopContainers, setStopContainers] = useState(false)
  const [loading, setLoading] = useState(true)
  const [archiveLoading, setArchiveLoading] = useState(true)
  const [submitting, setSubmitting] = useState(false)
  const [downloadId, setDownloadId] = useState('')
  const [error, setError] = useState('')
  const [taskId, setTaskId] = useState('')
  const [creating, setCreating] = useState(false)
  const [resourceTab, setResourceTab] = useState('containers')
  const [search, setSearch] = useState('')
  const [archiveSearch, setArchiveSearch] = useState('')
  const [detail, setDetail] = useState(null)
  const [deleteTarget, setDeleteTarget] = useState(null)
  const [deleting, setDeleting] = useState(false)
  const [stopConfirm, setStopConfirm] = useState(false)
  const { tasks, addTask } = useTasks()
  const hostName = (id) => hosts.find((h) => h.id === id)?.name || (id === 'local' ? '本地' : id)

  const loadArchives = useCallback(async () => {
    setArchiveLoading(true)
    try { setArchives(bodyData(await backupAPI.list()) || []) }
    catch (e) { setError(e.response?.data?.msg || e.message) }
    finally { setArchiveLoading(false) }
  }, [])
  useEffect(() => {
    dockerHostAPI.list().then((r) => setHosts(bodyData(r) || [])).catch((e) => setError(e.message))
    loadArchives()
  }, [loadArchives])

  // 实例变更立即清空选择，过期响应不允许覆盖当前实例的数据。
  useEffect(() => {
    if (!creating) return
    let active = true
    setLoading(true); setError(''); setResources(null); setSelected([]); setProjectIds([]); setMounts([]); setSearch('')
    backupAPI.resources(hostId).then((r) => { if (active) setResources(bodyData(r)) })
      .catch((e) => { if (active) setError(e.response?.data?.msg || e.message) })
      .finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [hostId, creating])

  const { progress, isPolling } = useProgress(taskId,
    () => { setSubmitting(false); loadArchives() },
    (e) => { setSubmitting(false); setError(e.detailMsg || e.message || e.msg || '备份失败') })
  const busy = submitting || isPolling || tasks.some((t) => t.taskType === 'backup' && !t.isDone)

  const setContainers = (next) => {
    const available = new Set((resources?.containers || []).filter((c) => next.includes(c.id))
      .flatMap((c) => (c.mounts || []).filter((m) => m.allowed).map(mountKey)))
    const added = (resources?.containers || []).filter((c) => next.includes(c.id) && !selected.includes(c.id))
      .flatMap((c) => (c.mounts || []).filter((m) => m.allowed).map(mountKey))
    setSelected(next)
    setMounts((prev) => [...new Set([...prev.filter((key) => available.has(key)), ...added])])
  }
  const create = async (confirmed = false) => {
    if (stopContainers && !confirmed) { setStopConfirm(true); return }
    setStopConfirm(false); setSubmitting(true); setError('')
    try {
      const data = bodyData(await backupAPI.create({ hostId, containerIds: selected, projectIds: includeCompose ? projectIds : [],
        mountKeys: mounts, includeData, includeCompose, includeEnv, stopContainers }))
      if (!data?.taskID) throw new Error('后端未返回备份任务编号')
      setTaskId(data.taskID); setCreating(false)
      addTask({ id: data.taskID, title: '数据备份', onDone: loadArchives })
    } catch (e) { setError(e.response?.data?.msg || e.message); setSubmitting(false) }
  }
  const download = async (archive) => {
    setDownloadId(archive.id); setError('')
    try {
      const response = await backupAPI.download(archive.id)
      const url = URL.createObjectURL(response.data)
      const link = document.createElement('a')
      link.href = url; link.download = `backup-${archive.id}.tar.gz`
      document.body.appendChild(link); link.click(); link.remove()
      setTimeout(() => URL.revokeObjectURL(url), 60000)
    } catch (e) {
      let message = e.message
      if (e.response?.data instanceof Blob) {
        try { message = JSON.parse(await e.response.data.text()).msg || message } catch { /* 非 JSON 错误保留原信息。 */ }
      }
      setError(message)
    } finally { setDownloadId('') }
  }
  const remove = async () => {
    if (!deleteTarget || deleting) return
    setDeleting(true)
    try { bodyData(await backupAPI.remove(deleteTarget.id)); setDeleteTarget(null); await loadArchives() }
    catch (e) { setError(e.response?.data?.msg || e.message) }
    finally { setDeleting(false) }
  }

  const selectedContainers = (resources?.containers || []).filter((c) => selected.includes(c.id))
  const selectedMounts = [...new Map(selectedContainers.flatMap((c) => c.mounts || []).map((m) => [mountKey(m), m])).values()]
  const query = search.trim().toLowerCase()
  const filteredContainers = (resources?.containers || []).filter((c) => `${c.name} ${c.compose?.project || ''}`.toLowerCase().includes(query))
  const filteredProjects = (resources?.projects || []).filter((p) => `${p.name} ${p.dir}`.toLowerCase().includes(query))
  const visibleArchives = archives.filter((a) => (hostFilter === 'all' || a.hostId === hostFilter)
    && `${a.id} ${hostName(a.hostId)} ${a.createdAt}`.toLowerCase().includes(archiveSearch.toLowerCase()))
  const canCreate = !busy && !loading && ((includeData && mounts.length > 0) || (includeCompose && (projectIds.length > 0 || selectedContainers.some((c) => c.compose?.candidate))))
  const closeCreate = () => { if (!submitting) { setCreating(false); setStopConfirm(false) } }
  const openCreate = () => { setResourceTab('containers'); setSearch(''); setError(''); setStopConfirm(false); setCreating(true) }

  // 弹窗打开期间锁定页面滚动，并提供 Escape 退出；提交中不打断请求。
  useEffect(() => {
    if (!creating && !detail && !deleteTarget) return
    const old = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    const key = (e) => {
      if (e.key !== 'Escape' || submitting || deleting) return
      setCreating(false); setDetail(null); setDeleteTarget(null); setStopConfirm(false)
    }
    document.addEventListener('keydown', key)
    return () => { document.body.style.overflow = old; document.removeEventListener('keydown', key) }
  }, [creating, detail, deleteTarget, submitting, deleting])

  return (
    <section className="w-full px-2 sm:px-6 py-4 text-gray-900 dark:text-gray-100">
      <header className="flex flex-wrap items-center justify-between gap-3 pb-5">
        <div className="flex items-center gap-3"><DatabaseBackup className="h-6 w-6 text-blue-600" /><h2 className="text-xl font-semibold">备份</h2><span className="text-sm text-gray-500">{archives.length} 个归档 · {bytes(archives.reduce((sum, a) => sum + (a.size || 0), 0))}</span></div>
        <button onClick={openCreate} disabled={busy} className={primaryClass}><Plus className="h-4 w-4" />新建备份</button>
      </header>
      {error && !creating && <Notice>{error}</Notice>}
      {(busy || progress) && <div role="status" className="mb-4 border-l-2 border-blue-500 bg-blue-50 dark:bg-blue-950/20 px-4 py-3">
        <div className="flex items-center justify-between gap-3 text-sm"><span className="break-words">{progress?.message || '备份任务进行中'}</span>{progress && <span className="shrink-0 tabular-nums">{progress.percentage}%</span>}</div>
        {busy && <progress value={progress?.percentage || 0} max="100" className="mt-2 h-1.5 w-full" />}
      </div>}
      <div className="flex flex-wrap items-center gap-3 border-y border-gray-200 dark:border-gray-700 py-3">
        <div className="relative flex-1 min-w-[160px] max-w-sm"><Search className="absolute left-3 top-2.5 h-4 w-4 text-gray-400" /><input aria-label="搜索归档" value={archiveSearch} onChange={(e) => setArchiveSearch(e.target.value)} placeholder="搜索归档" className={`${inputClass} pl-9`} /></div>
        <select aria-label="筛选实例" value={hostFilter} onChange={(e) => setHostFilter(e.target.value)} className={`${inputClass} w-auto max-w-full`}><option value="all">全部实例</option>{[...new Set([...hosts.map((h) => h.id), ...archives.map((a) => a.hostId)])].map((id) => <option key={id} value={id}>{hostName(id)}</option>)}</select>
        <button onClick={loadArchives} disabled={archiveLoading} title="刷新归档" aria-label="刷新归档" className={iconClass}><RefreshCw className={`h-4 w-4 ${archiveLoading ? 'animate-spin' : ''}`} /></button>
      </div>
      <div className="overflow-x-auto">
        <table className="w-full min-w-[620px] text-left text-sm">
          <thead className="text-xs text-gray-500 bg-gray-50 dark:bg-gray-800/50"><tr>{['备份时间', '实例', '内容', '大小', '操作'].map((label) => <th key={label} className="px-3 py-3 font-medium">{label}</th>)}</tr></thead>
          <tbody className="divide-y divide-gray-200 dark:divide-gray-700">{visibleArchives.map((a) => <tr key={a.id} className="hover:bg-gray-50 dark:hover:bg-gray-800/40">
            <td className="px-3 py-4"><button onClick={() => setDetail(a)} className="text-left hover:text-blue-600"><span className="block font-medium">{new Date(a.createdAt).toLocaleString()}</span><span className="block text-xs text-gray-400 font-mono mt-1">{a.id.slice(0, 8)}</span></button></td>
            <td className="px-3 py-4 max-w-[160px] break-words">{hostName(a.hostId)}</td>
            <td className="px-3 py-4 text-gray-500">{(a.items || []).filter((i) => i.kind === 'mount').length} 个挂载 · {(a.items || []).filter((i) => i.kind === 'compose').length} 个文件</td>
            <td className="px-3 py-4 tabular-nums whitespace-nowrap">{bytes(a.size || 0)}</td>
            <td className="px-3 py-4"><div className="flex gap-1"><button onClick={() => setDetail(a)} title="归档详情" aria-label="归档详情" className={iconClass}><Info className="h-4 w-4" /></button><button onClick={() => download(a)} disabled={!!downloadId} title="下载归档" aria-label="下载归档" className={iconClass}>{downloadId === a.id ? <Loader2 className="h-4 w-4 animate-spin" /> : <Download className="h-4 w-4" />}</button><button onClick={() => { setError(''); setDeleteTarget(a) }} title="删除归档" aria-label="删除归档" className={`${iconClass} hover:text-red-600`}><Trash2 className="h-4 w-4" /></button></div></td>
          </tr>)}</tbody>
        </table>
      </div>
      {archiveLoading && !archives.length ? <div className="flex items-center justify-center gap-2 py-16 text-sm text-gray-500"><Loader2 className="h-4 w-4 animate-spin" />加载归档</div> : !visibleArchives.length && <div className="flex flex-col items-center py-16 text-center"><DatabaseBackup className="h-9 w-9 text-gray-300 dark:text-gray-600 mb-3" /><h3 className="font-medium">{archives.length ? '没有匹配的归档' : '暂无备份归档'}</h3>{!archives.length && <button onClick={openCreate} disabled={busy} className="mt-4 inline-flex items-center gap-1.5 text-sm text-blue-600 disabled:opacity-40"><Plus className="h-4 w-4" />新建备份</button>}</div>}

      {creating && <Modal title="新建备份" onClose={closeCreate} wide>
        <div className="flex min-h-0 flex-1 flex-col lg:flex-row overflow-y-auto lg:overflow-hidden">
          <div className="min-w-0 flex-1 px-5 py-4 lg:overflow-y-auto">
            {error && <Notice>{error}</Notice>}
            <label className="block text-xs font-medium text-gray-500 mb-2" htmlFor="backup-host">实例</label>
            <select id="backup-host" value={hostId} onChange={(e) => setHostId(e.target.value)} disabled={submitting} className={inputClass}>{!hosts.length && <option value="local">本地</option>}{hosts.filter((h) => h.enabled !== false).map((h) => <option key={h.id} value={h.id}>{h.name}</option>)}</select>
            <div className="flex gap-5 mt-5 border-b border-gray-200 dark:border-gray-700" role="tablist" aria-label="备份资源">{[['containers', '容器', Box], ['projects', 'Compose 项目', FileCode]].map(([key, label, Icon]) => <button key={key} role="tab" aria-selected={resourceTab === key} onClick={() => { setResourceTab(key); setSearch('') }} className={`flex items-center gap-2 pb-3 text-sm border-b-2 ${resourceTab === key ? 'border-blue-600 text-blue-600' : 'border-transparent text-gray-500'}`}><Icon className="h-4 w-4" />{label}<span className="text-xs">{key === 'containers' ? selected.length : projectIds.length}</span></button>)}</div>
            <div className="flex items-center gap-3 py-3"><div className="relative flex-1"><Search className="absolute left-3 top-2.5 h-4 w-4 text-gray-400" /><input aria-label="搜索资源" placeholder={resourceTab === 'containers' ? '搜索容器' : '搜索项目或路径'} value={search} onChange={(e) => setSearch(e.target.value)} className={`${inputClass} pl-9`} /></div>
              <button disabled={loading || submitting} onClick={() => { if (resourceTab === 'containers') setContainers([...new Set([...selected, ...filteredContainers.map((c) => c.id)])]); else { setProjectIds([...new Set([...projectIds, ...filteredProjects.map((p) => p.id)])]); setIncludeCompose(true) } }} className="text-xs text-blue-600 whitespace-nowrap disabled:opacity-40">全选</button>
              <button disabled={submitting} onClick={() => resourceTab === 'containers' ? setContainers([]) : setProjectIds([])} className="text-xs text-gray-500 whitespace-nowrap">清空</button>
            </div>
            <fieldset disabled={loading || submitting} className="h-60 sm:h-72 overflow-y-auto border-y border-gray-200 dark:border-gray-700 divide-y divide-gray-100 dark:divide-gray-800">
              {loading ? <div className="flex justify-center items-center gap-2 h-full text-sm text-gray-500"><Loader2 className="h-4 w-4 animate-spin" />加载资源</div> : resourceTab === 'containers' ? filteredContainers.length ? filteredContainers.map((c) => <label key={c.id} className={`flex items-start gap-3 p-3 cursor-pointer hover:bg-gray-50 dark:hover:bg-gray-800 ${selected.includes(c.id) ? 'bg-blue-50/60 dark:bg-blue-950/20' : ''}`}><input type="checkbox" className="mt-1 accent-blue-600" checked={selected.includes(c.id)} onChange={() => setContainers(selected.includes(c.id) ? selected.filter((id) => id !== c.id) : [...selected, c.id])} /><Box className="h-4 w-4 text-gray-400 mt-0.5 shrink-0" /><span className="min-w-0 flex-1"><span className="block text-sm font-medium break-all">{c.name}</span><span className="block text-xs text-gray-500 mt-1">{(c.mounts || []).length} 个挂载{c.compose?.project ? ` · ${c.compose.project}` : ''}</span></span><span className={`text-xs shrink-0 ${c.running ? 'text-emerald-600' : 'text-gray-400'}`}>{c.running ? '运行中' : '已停止'}</span></label>) : <EmptyResource />
                : filteredProjects.length ? filteredProjects.map((p) => <label key={p.id} className={`flex items-start gap-3 p-3 cursor-pointer hover:bg-gray-50 dark:hover:bg-gray-800 ${projectIds.includes(p.id) ? 'bg-blue-50/60 dark:bg-blue-950/20' : ''}`}><input type="checkbox" className="mt-1 accent-blue-600" checked={projectIds.includes(p.id)} onChange={(e) => { setIncludeCompose(true); setProjectIds(e.target.checked ? [...projectIds, p.id] : projectIds.filter((id) => id !== p.id)) }} /><Folder className="h-4 w-4 text-gray-400 mt-0.5 shrink-0" /><span className="min-w-0"><span className="block text-sm font-medium break-all">{p.name}</span><span className="block text-xs text-gray-500 mt-1 break-all">{p.dir}</span></span></label>) : <EmptyResource />}
            </fieldset>
            {includeData && selectedMounts.length > 0 && <details className="mt-4" open><summary className="cursor-pointer text-sm font-medium">持久化目录与卷 <span className="text-gray-400 font-normal">({mounts.length}/{selectedMounts.length})</span></summary><fieldset disabled={submitting} className="mt-2 max-h-40 overflow-y-auto divide-y divide-gray-100 dark:divide-gray-800">{selectedMounts.map((m) => <label key={mountKey(m)} className="flex items-start gap-2 py-2 text-xs"><input type="checkbox" className="mt-0.5 accent-blue-600" disabled={!m.allowed} checked={!!m.allowed && mounts.includes(mountKey(m))} onChange={(e) => setMounts((prev) => e.target.checked ? [...new Set([...prev, mountKey(m)])] : prev.filter((key) => key !== mountKey(m)))} /><span className="min-w-0 break-all">{m.type === 'volume' ? m.name : m.source}<span className="block text-gray-400 mt-0.5">{m.allowed ? m.destination : m.reason || '不支持此挂载'}</span></span></label>)}</fieldset></details>}
          </div>
          <aside className="w-full lg:w-72 shrink-0 border-t lg:border-t-0 lg:border-l border-gray-200 dark:border-gray-700 px-5 py-4 bg-gray-50 dark:bg-gray-900/40 lg:overflow-y-auto">
            <h3 className="text-sm font-semibold mb-4">备份内容</h3>
            <fieldset disabled={submitting} className="space-y-4"><Option checked={includeData} onChange={setIncludeData} label="持久化数据" /><Option checked={includeCompose} onChange={setIncludeCompose} label="Compose 原文件" /><Option checked={includeEnv} onChange={setIncludeEnv} label="环境配置" note=".env 与容器环境变量，可能含密钥" /><Option checked={stopContainers} onChange={setStopContainers} label="停机备份" note="完成后尝试恢复原运行状态" /></fieldset>
            <div className="mt-5 border-t border-gray-200 dark:border-gray-700 pt-4 text-xs text-gray-500 space-y-2"><div className="flex justify-between"><span>容器</span><span>{selected.length}</span></div><div className="flex justify-between"><span>项目</span><span>{includeCompose ? projectIds.length : 0}</span></div><div className="flex justify-between"><span>数据挂载</span><span>{includeData ? mounts.length : 0}</span></div><div className="flex justify-between"><span>归档格式</span><span>tar.gz</span></div></div>
            <p className="mt-5 text-xs leading-5 text-amber-700 dark:text-amber-400 flex gap-2"><AlertTriangle className="h-4 w-4 shrink-0 mt-0.5" />{stopContainers ? '备份期间请勿重启 Copilot。共享目录的运行容器需一并选择。' : '在线文件备份不保证数据库一致性。'}</p>
            <details className="mt-3 text-xs text-gray-500"><summary className="cursor-pointer">限制与注意事项</summary><div className="mt-2 space-y-2 leading-5"><p>原始数据与压缩包各限 {bytes(resources?.limits?.maxArchiveBytes || 10737418240)}；不支持链接、特殊文件、ACL、扩展属性和镜像层。</p><p>Compose 原文件需在所选实例宿主机可读；本地扫描项目读取已配置目录。外部依赖不会自动收集。</p><p>{includeEnv ? '归档包含敏感环境配置，请妥善保护。' : '未包含运行时环境变量及 .env，恢复时需另行配置。'}数据归档不提供自动覆盖恢复。</p></div></details>
          </aside>
        </div>
        <footer className="shrink-0 flex flex-wrap items-center justify-between gap-3 border-t border-gray-200 dark:border-gray-700 px-5 py-3 bg-white dark:bg-gray-800"><span className="text-xs text-gray-500">{selected.length + (includeCompose ? projectIds.length : 0)} 个资源已选</span><div className="flex gap-2"><button onClick={closeCreate} disabled={submitting} className="px-4 py-2 text-sm text-gray-500">取消</button><button onClick={() => create()} disabled={!canCreate} className={primaryClass}>{submitting ? <Loader2 className="h-4 w-4 animate-spin" /> : <DatabaseBackup className="h-4 w-4" />}创建备份</button></div></footer>
        {stopConfirm && <div className="absolute inset-0 z-10 flex items-center justify-center bg-black/35 p-5"><div role="alertdialog" aria-label="确认停机备份" className="max-w-sm rounded-lg bg-white dark:bg-gray-800 p-5 shadow-xl"><h3 className="font-semibold">确认停机备份</h3><p className="text-sm text-gray-500 leading-6 my-4">将停止所选运行容器，并在任务结束后尝试恢复。Copilot 意外退出可能导致容器保持停止。</p><div className="flex justify-end gap-3"><button onClick={() => setStopConfirm(false)} className="text-sm text-gray-500">取消</button><button onClick={() => create(true)} className={primaryClass}>确认并备份</button></div></div></div>}
      </Modal>}

      {detail && <Modal title="归档详情" onClose={() => setDetail(null)}><div className="p-5 overflow-y-auto space-y-4 text-sm"><dl className="space-y-3"><dt className="text-gray-500 text-xs">归档编号</dt><dd className="font-mono break-all">{detail.id}</dd><dt className="text-gray-500 text-xs">实例与时间</dt><dd>{hostName(detail.hostId)} · {new Date(detail.createdAt).toLocaleString()}</dd><dt className="text-gray-500 text-xs">SHA-256</dt><dd className="text-xs font-mono break-all">{detail.sha256 || '未提供'}</dd></dl><h4 className="font-medium pt-3 border-t border-gray-200 dark:border-gray-700">归档内容</h4>{(detail.items || []).map((item, i) => <div key={i} className="text-xs break-all"><span className="text-gray-400">{item.kind}</span><p className="mt-1">{item.source}</p><p className="text-gray-400 mt-1 flex items-start gap-1"><ChevronRight className="h-3 w-3 shrink-0" />{item.archivePath}</p></div>)}<details className="text-xs text-gray-500"><summary className="cursor-pointer">备份注意事项</summary>{(detail.warnings || []).map((w, i) => <p key={i} className="mt-2 leading-5">{w}</p>)}</details></div><footer className="p-4 border-t border-gray-200 dark:border-gray-700 flex justify-end"><button onClick={() => download(detail)} disabled={!!downloadId} className={primaryClass}><Download className="h-4 w-4" />下载归档</button></footer></Modal>}
      {deleteTarget && <Modal title="删除归档" onClose={() => { if (!deleting) setDeleteTarget(null) }}><div className="p-5 text-sm text-gray-500">删除 {new Date(deleteTarget.createdAt).toLocaleString()} 的备份归档？此操作不可恢复。{error && <div className="mt-3"><Notice>{error}</Notice></div>}</div><footer className="flex justify-end gap-3 p-4 border-t border-gray-200 dark:border-gray-700"><button disabled={deleting} onClick={() => setDeleteTarget(null)} className="text-sm text-gray-500">取消</button><button disabled={deleting} onClick={remove} className="inline-flex gap-2 items-center rounded-lg bg-red-600 text-white px-4 py-2 text-sm">{deleting ? <Loader2 className="h-4 w-4 animate-spin" /> : <Trash2 className="h-4 w-4" />}删除</button></footer></Modal>}
    </section>
  )
}

function Modal({ title, children, onClose, wide = false }) {
  const dialogRef = useRef(null)
  // 将键盘焦点约束到当前弹窗，关闭后回到触发按钮。
  useEffect(() => {
    const previous = document.activeElement
    const dialog = dialogRef.current
    const focusable = () => [...dialog.querySelectorAll('button, input, select, summary, [tabindex="0"]')]
      .filter((node) => !node.disabled && node.getClientRects().length)
    focusable()[0]?.focus()
    const trap = (event) => {
      if (event.key !== 'Tab') return
      const nodes = focusable()
      const first = nodes[0], last = nodes[nodes.length - 1]
      if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus() }
      else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus() }
    }
    dialog.addEventListener('keydown', trap)
    return () => { dialog.removeEventListener('keydown', trap); previous?.focus?.() }
  }, [])
  return <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-3 sm:p-6" onClick={(e) => { if (e.target === e.currentTarget) onClose() }}><div ref={dialogRef} role="dialog" aria-modal="true" aria-label={title} className={`relative flex flex-col w-full max-h-[90dvh] rounded-lg bg-white dark:bg-gray-800 shadow-xl overflow-hidden ${wide ? 'max-w-5xl h-[min(780px,90dvh)]' : 'max-w-xl'}`}><header className="shrink-0 flex items-center justify-between px-5 py-4 border-b border-gray-200 dark:border-gray-700"><h3 className="text-base font-semibold">{title}</h3><button onClick={onClose} className={iconClass} title="关闭" aria-label="关闭"><X className="h-4 w-4" /></button></header>{children}</div></div>
}
function Option({ checked, onChange, label, note }) {
  return <label className="flex items-start gap-3 cursor-pointer"><input type="checkbox" className="mt-0.5 accent-blue-600" checked={checked} onChange={(e) => onChange(e.target.checked)} /><span className="min-w-0 text-sm">{label}{note && <span className="block text-xs text-gray-500 mt-1 leading-5">{note}</span>}</span></label>
}
function Notice({ children }) {
  return <div role="alert" className="mb-3 rounded-lg bg-red-50 dark:bg-red-950/30 px-3 py-2 text-sm text-red-600 break-words">{children}</div>
}
function EmptyResource() {
  return <div className="flex items-center justify-center h-full text-sm text-gray-400">没有可选资源</div>
}
