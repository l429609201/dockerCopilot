import React, { useState, useEffect, useCallback } from 'react'
import { Layers, Play, Square, RotateCw, Download, FileEdit, RefreshCw } from 'lucide-react'
import { composeAPI } from '../api/client.js'
import { ComposeEditor } from './ComposeEditor.jsx'
import { ComposeConfigCard } from './ComposeConfigCard.jsx'
import { HostPathMapperCard } from './HostPathMapperCard.jsx'

// Compose 项目管理页面
export function Compose() {
  const [projects, setProjects] = useState([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [editing, setEditing] = useState(null) // { project, filename }
  const [busyId, setBusyId] = useState('')

  const load = useCallback(async () => {
    setLoading(true); setError('')
    try {
      const r = await composeAPI.listProjects()
      if (r.data?.code === 200) setProjects(r.data.data || [])
      else setError(r.data?.msg || '加载失败')
    } catch (e) {
      setError('加载失败：' + (e.message || '未知错误'))
    } finally { setLoading(false) }
  }, [])

  useEffect(() => { load() }, [load])

  // 执行部署动作，处理高风险 409 二次确认
  const doAction = async (project, action) => {
    setBusyId(project.id)
    try {
      let r = await composeAPI.action(project.id, action, false)
      if (r.data?.code === 409) {
        const warnings = (r.data.data?.warnings || []).join('\n')
        if (!confirm(`检测到高风险配置：\n${warnings}\n\n仍要继续 ${action} 吗？`)) {
          setBusyId(''); return
        }
        r = await composeAPI.action(project.id, action, true)
      }
      if (r.data?.code === 200) {
        alert(`已提交 ${action} 任务`)
      } else {
        alert('操作失败：' + (r.data?.msg || '未知错误'))
      }
    } catch (e) {
      alert('操作失败：' + (e.message || '未知错误'))
    } finally { setBusyId('') }
  }

  return (
    <div className="w-full space-y-6">
      {/* 页面头部 */}
      <div className="px-2 sm:px-6 py-4">
        <div className="flex items-center justify-between">
          <h2 className="text-xl font-bold text-gray-900 dark:text-white flex items-center gap-2">
            <Layers className="h-5 w-5" /> Compose 项目
          </h2>
          <button onClick={load} className="flex items-center gap-1 px-3 py-2 bg-gray-100 dark:bg-gray-700 rounded-lg text-sm hover:bg-gray-200">
            <RefreshCw className="h-4 w-4" /> 刷新
          </button>
        </div>
      </div>

      {/* 内容区域 */}
      <div className="px-2 sm:px-6 space-y-4">
        {/* 两张配置卡片：大屏(lg)并排两列，小屏堆叠；items-start 避免等高拉伸 */}
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-4 items-start">
          {/* 扫描配置卡片：保存后自动刷新项目列表 */}
          <ComposeConfigCard onSaved={load} />

          {/* 宿主机路径映射配置：供挂载路径转换使用 */}
          <HostPathMapperCard />
        </div>

        {error && <div className="p-3 bg-red-50 text-red-600 rounded-lg text-sm">{error}</div>}
        {loading && <div className="text-gray-500 text-sm">加载中...</div>}

        <div className="grid gap-3">
        {projects.map((p) => (
          <div key={p.id} className="card">
            <div className="flex items-center justify-between flex-wrap gap-2">
              <div className="min-w-0">
                <div className="font-semibold text-gray-900 dark:text-white truncate">{p.name}</div>
                <div className="text-xs text-gray-500 mt-0.5 truncate">{p.dir}</div>
                <div className="text-xs text-gray-400 mt-0.5">{p.composeFile}</div>
              </div>
              <div className="flex items-center gap-1 flex-shrink-0">
                <ActionBtn disabled={busyId === p.id} onClick={() => doAction(p, 'up')} icon={Play} title="up" color="emerald" />
                <ActionBtn disabled={busyId === p.id} onClick={() => doAction(p, 'down')} icon={Square} title="down" color="red" />
                <ActionBtn disabled={busyId === p.id} onClick={() => doAction(p, 'restart')} icon={RotateCw} title="restart" color="blue" />
                <ActionBtn disabled={busyId === p.id} onClick={() => doAction(p, 'pull')} icon={Download} title="pull" color="gray" />
                <button onClick={() => setEditing({ project: p, filename: p.composeFile })}
                  className="p-2 text-primary-600 hover:bg-primary-50 rounded-lg" title="编辑">
                  <FileEdit className="h-4 w-4" />
                </button>
              </div>
            </div>
          </div>
        ))}
        {!loading && projects.length === 0 && (
          <div className="text-gray-400 text-sm">未发现 Compose 项目。请在上方「Compose 扫描配置」中填写已挂载进容器的项目目录并保存。</div>
        )}
      </div>
      </div>

      {editing && (
        <ComposeEditor project={editing.project} filename={editing.filename}
          onClose={() => setEditing(null)} />
      )}
    </div>
  )
}

function ActionBtn({ onClick, icon: Icon, title, color, disabled }) {
  const colors = {
    emerald: 'text-emerald-600 hover:bg-emerald-50',
    red: 'text-red-600 hover:bg-red-50',
    blue: 'text-blue-600 hover:bg-blue-50',
    gray: 'text-gray-600 hover:bg-gray-100',
  }
  return (
    <button onClick={onClick} disabled={disabled} title={title}
      className={`p-2 rounded-lg disabled:opacity-50 ${colors[color]}`}>
      <Icon className="h-4 w-4" />
    </button>
  )
}
