import React, { useCallback, useEffect, useState } from 'react'
import { DatabaseBackup, Download, RefreshCw, Trash2, AlertTriangle, Loader2 } from 'lucide-react'
import { backupAPI, dockerHostAPI } from '../api/client.js'
import { useTasks } from '../hooks/useTasks.jsx'
import { useProgress } from '../hooks/useProgress.js'

const mountKey = (m) => `${m.type}:${m.type === 'volume' ? m.name : m.source}`
const bytes = (n) => n >= 1073741824 ? `${(n / 1073741824).toFixed(2)} GB` : `${(n / 1048576).toFixed(2)} MB`
const bodyData = (response) => {
  if (response.data?.code !== 200) throw new Error(response.data?.msg || '请求失败')
  return response.data.data
}

// 实际数据归档与旧配置快照分开，避免将配置恢复误用于覆盖持久化目录。
export function DataBackups() {
  const [hosts, setHosts] = useState([])
  const [hostId, setHostId] = useState('local')
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
  const [submitting, setSubmitting] = useState(false)
  const [downloadId, setDownloadId] = useState('')
  const [error, setError] = useState('')
  const [taskId, setTaskId] = useState('')
  const { tasks, addTask } = useTasks()

  const loadArchives = useCallback(async () => {
    try { setArchives(bodyData(await backupAPI.list()) || []) }
    catch (e) { setError(e.response?.data?.msg || e.message) }
  }, [])

  useEffect(() => {
    dockerHostAPI.list().then((r) => setHosts(bodyData(r) || [])).catch((e) => setError(e.message))
    loadArchives()
  }, [loadArchives])

  // 切换实例时丢弃旧资源选择，且忽略已过期的异步响应。
  useEffect(() => {
    let active = true
    setLoading(true); setError(''); setResources(null); setSelected([]); setProjectIds([]); setMounts([])
    backupAPI.resources(hostId).then((r) => {
      if (active) setResources(bodyData(r))
    }).catch((e) => { if (active) setError(e.response?.data?.msg || e.message) })
      .finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [hostId])

  const { progress, isPolling } = useProgress(taskId,
    () => { setSubmitting(false); loadArchives() },
    (e) => { setSubmitting(false); setError(e.detailMsg || e.message || e.msg || '备份失败，请查看任务进度') })

  const toggleContainer = (item) => {
    const removing = selected.includes(item.id)
    const next = removing ? selected.filter((id) => id !== item.id) : [...selected, item.id]
    setSelected(next)
    // 共享挂载按来源去重，只保留已选容器仍在使用的目录。
    const available = new Set((resources?.containers || []).filter((c) => next.includes(c.id))
      .flatMap((c) => (c.mounts || []).filter((m) => m.allowed).map(mountKey)))
    setMounts((prev) => removing ? prev.filter((key) => available.has(key))
      : [...new Set([...prev, ...(item.mounts || []).filter((m) => m.allowed).map(mountKey)])])
  }

  const create = async () => {
    if (stopContainers && !window.confirm('备份期间将停止所选运行中的容器，完成或失败后尝试恢复运行。备份期间请勿重启 Copilot，进程意外退出可能导致容器保持停止。是否继续？')) return
    setSubmitting(true); setError('')
    try {
      const data = bodyData(await backupAPI.create({ hostId, containerIds: selected, projectIds: includeCompose ? projectIds : [], mountKeys: mounts,
        includeData, includeCompose, includeEnv, stopContainers }))
      if (!data?.taskID) throw new Error('后端未返回备份任务编号')
      setTaskId(data.taskID)
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
        try { message = JSON.parse(await e.response.data.text()).msg || message } catch { /* 非 JSON 下载错误保留原始信息。 */ }
      }
      setError(message)
    } finally { setDownloadId('') }
  }

  const remove = async (archive) => {
    if (!window.confirm('确定删除此数据备份？此操作不可恢复。')) return
    try { bodyData(await backupAPI.remove(archive.id)); await loadArchives() }
    catch (e) { setError(e.response?.data?.msg || e.message) }
  }

  const remoteBusy = tasks.some((task) => task.taskType === 'backup' && !task.isDone)
  const busy = submitting || isPolling || remoteBusy
  const selectedContainers = (resources?.containers || []).filter((c) => selected.includes(c.id))
  const selectedMounts = [...new Map(selectedContainers.flatMap((c) => c.mounts || []).map((m) => [mountKey(m), m])).values()]
  const control = 'rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-800 px-3 py-2 text-sm'

  return (
    <section className="px-2 sm:px-6 py-4 space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 className="text-xl font-semibold flex items-center gap-2"><DatabaseBackup className="h-5 w-5" />数据备份</h2>
        <select aria-label="备份实例" value={hostId} onChange={(e) => setHostId(e.target.value)} disabled={busy} className={`${control} max-w-full`}>
          {hosts.length === 0 && <option value="local">本地</option>}
          {hosts.filter((h) => h.enabled !== false).map((h) => <option key={h.id} value={h.id}>{h.name}</option>)}
        </select>
      </div>
      {error && <div role="alert" className="text-sm text-red-600 bg-red-50 dark:bg-red-950/30 p-3 rounded-lg break-words">{error}</div>}
      <div className="flex gap-2 p-3 text-sm text-amber-800 dark:text-amber-200 bg-amber-50 dark:bg-amber-950/30 rounded-lg">
        <AlertTriangle className="h-4 w-4 shrink-0 mt-0.5" />
        <p>在线文件备份不保证数据库一致性。归档可能包含敏感配置，请妥善保护；新数据包不支持自动覆盖恢复。原始文件总量上限 {bytes(resources?.limits?.maxArchiveBytes || 10737418240)}，不包含镜像层、ACL 和扩展属性；含链接或特殊文件的目录将拒绝备份。</p>
      </div>
      {!includeEnv && <p className="text-xs text-gray-500">默认不保存容器运行时环境变量及 .env，恢复业务所需的环境配置需另行保留。</p>}
      <fieldset disabled={busy || loading} className="space-y-3 min-w-0">
        <legend className="font-medium mb-2">容器</legend>
        {loading ? <div className="flex items-center gap-2 text-sm text-gray-500"><Loader2 className="h-4 w-4 animate-spin" />加载资源</div>
          : !(resources?.containers || []).length ? <p className="text-sm text-gray-500">此实例暂无可备份容器</p>
            : <div className="divide-y divide-gray-200 dark:divide-gray-700 border-y border-gray-200 dark:border-gray-700 max-h-64 overflow-y-auto">
              {resources.containers.map((c) => <label key={c.id} className="flex items-start gap-3 py-3 cursor-pointer">
                <input type="checkbox" checked={selected.includes(c.id)} onChange={() => toggleContainer(c)} className="mt-1" />
                <span className="min-w-0"><span className="block text-sm font-medium break-all">{c.name}</span>
                  <span className="block text-xs text-gray-500 break-all">{c.running ? '运行中' : '已停止'} · {(c.mounts || []).length} 个挂载{c.compose?.project ? ` · ${c.compose.project}` : ''}</span></span>
              </label>)}
            </div>}
        <div className="flex flex-wrap gap-x-5 gap-y-2 text-sm">
          <label className="flex gap-2 items-center"><input type="checkbox" checked={includeData} onChange={(e) => setIncludeData(e.target.checked)} />持久化数据</label>
          <label className="flex gap-2 items-center"><input type="checkbox" checked={includeCompose} onChange={(e) => setIncludeCompose(e.target.checked)} />Compose 原文件</label>
          <label className="flex gap-2 items-center"><input type="checkbox" checked={includeEnv} onChange={(e) => setIncludeEnv(e.target.checked)} />包含环境配置（.env / 环境变量）</label>
          <label className="flex gap-2 items-center"><input type="checkbox" checked={stopContainers} onChange={(e) => setStopContainers(e.target.checked)} />停机备份</label>
        </div>
        {includeCompose && (resources?.projects || []).length > 0 && <div className="space-y-2">
          <h3 className="text-sm font-medium">本地 Compose 项目</h3>
          {resources.projects.map((project) => <label key={project.id} className="flex items-start gap-2 text-sm min-w-0">
            <input type="checkbox" className="mt-1" checked={projectIds.includes(project.id)}
              onChange={(e) => setProjectIds((prev) => e.target.checked ? [...prev, project.id] : prev.filter((id) => id !== project.id))} />
            <span className="min-w-0 break-all">{project.name}<span className="block text-xs text-gray-500">{project.dir}</span></span>
          </label>)}
        </div>}
        {includeData && selectedMounts.length > 0 && <div className="space-y-2">
          <h3 className="text-sm font-medium">持久化目录与卷</h3>
          {selectedMounts.map((m) => <label key={mountKey(m)} className="flex items-start gap-2 text-sm min-w-0">
            <input type="checkbox" className="mt-1" disabled={!m.allowed} checked={m.allowed && mounts.includes(mountKey(m))}
              onChange={(e) => setMounts((prev) => e.target.checked ? [...prev, mountKey(m)] : prev.filter((key) => key !== mountKey(m)))} />
            <span className="min-w-0 break-all">{m.type === 'volume' ? m.name : m.source}<span className="text-gray-500"> → {m.destination}</span>
              {!m.allowed && <span className="block text-xs text-amber-600">{m.reason || '此挂载不支持备份'}</span>}</span>
          </label>)}
        </div>}
        {selectedContainers.some((c) => c.compose?.candidate) && includeCompose && <p className="text-xs text-gray-500">Compose 标签路径为候选；原文件必须位于该 Docker 实例宿主机，否则任务会明确报错。</p>}
        {(resources?.warnings || []).map((warning, i) => <p key={i} className="text-sm text-amber-600">{warning}</p>)}
        <button onClick={create} disabled={(!selected.length && !(includeCompose && projectIds.length)) || (!includeCompose && (!includeData || !mounts.length))}
          className="inline-flex items-center gap-2 bg-blue-600 hover:bg-blue-700 text-white px-4 py-2 rounded-lg text-sm disabled:opacity-50">
          {busy ? <Loader2 className="h-4 w-4 animate-spin" /> : <DatabaseBackup className="h-4 w-4" />}创建备份
        </button>
      </fieldset>
      {remoteBusy && !taskId && <p role="status" className="text-sm text-gray-500">数据备份进行中，进度见任务中心</p>}
      {progress && <div role="status" className="space-y-2 text-sm">
        <div className="flex justify-between gap-3"><span className="break-words">{progress.message}</span><span>{progress.percentage}%</span></div>
        <progress value={progress.percentage} max="100" className="w-full h-2" />
        {progress.detailMsg && <p className="text-xs text-gray-500 break-all">{progress.detailMsg}</p>}
      </div>}
      <div className="flex items-center justify-between gap-3 pt-4 border-t border-gray-200 dark:border-gray-700">
        <h3 className="font-medium">数据归档</h3>
        <button onClick={loadArchives} title="刷新备份列表" aria-label="刷新备份列表" className={`${control} p-2`}><RefreshCw className="h-4 w-4" /></button>
      </div>
      {!archives.length && <p className="text-sm text-gray-500">暂无数据归档</p>}
      <div className="divide-y divide-gray-200 dark:divide-gray-700">
        {archives.map((archive) => <div key={archive.id} className="flex flex-wrap items-center justify-between gap-3 py-3">
          <div className="min-w-0 flex-1"><p className="text-sm font-medium break-all">{archive.hostName || archive.hostId} · {new Date(archive.createdAt).toLocaleString()}</p>
            <p className="text-xs text-gray-500 break-all">{bytes(archive.size || 0)} · {archive.id}</p>
            {archive.sha256 && <p className="text-xs font-mono text-gray-500 break-all">SHA-256: {archive.sha256}</p>}
            {(archive.warnings || archive.manifest?.warnings || []).map((w, i) => <p key={i} className="text-xs text-amber-600 break-words">{w}</p>)}</div>
          <div className="flex gap-2 shrink-0"><button onClick={() => download(archive)} disabled={!!downloadId} title="下载数据归档" aria-label="下载数据归档" className={control}>
            {downloadId === archive.id ? <Loader2 className="h-4 w-4 animate-spin" /> : <Download className="h-4 w-4" />}</button>
            <button onClick={() => remove(archive)} title="删除数据归档" aria-label="删除数据归档" className={`${control} text-red-600`}><Trash2 className="h-4 w-4" /></button></div>
        </div>)}
      </div>
    </section>
  )
}
