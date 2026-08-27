import React, { useState, useEffect, useCallback, useMemo } from 'react'
import { Layers, Play, Square, RotateCw, Download, FileEdit, RefreshCw, FolderOpen, Search } from 'lucide-react'
import { composeAPI } from '../api/client.js'
import { ComposeEditor } from './ComposeEditor.jsx'
import { ComposeFileManager } from './ComposeFileManager.jsx'

// Compose 项目管理页面
export function Compose() {
  const [projects, setProjects] = useState([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [editing, setEditing] = useState(null) // { project, filename }
  const [busyId, setBusyId] = useState('')
  const [showFileManager, setShowFileManager] = useState(false)
  const [searchQuery, setSearchQuery] = useState('') // 搜索关键词

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

  // 根据搜索关键词过滤项目
  const filteredProjects = useMemo(() => {
    if (!searchQuery.trim()) return projects
    const query = searchQuery.toLowerCase()
    return projects.filter(p =>
      p.name.toLowerCase().includes(query) ||
      p.dir.toLowerCase().includes(query) ||
      p.composeFile.toLowerCase().includes(query)
    )
  }, [projects, searchQuery])

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
      <div className="px-2 sm:px-6 py-4 space-y-4">
        <div className="flex items-center justify-between">
          <h2 className="text-xl font-bold text-gray-900 dark:text-white flex items-center gap-2">
            <Layers className="h-5 w-5" /> Compose 项目
          </h2>
          <div className="flex items-center gap-2">
            <button onClick={() => setShowFileManager(true)}
              className="flex items-center gap-1 px-3 py-2 bg-primary-600 text-white rounded-lg text-sm hover:bg-primary-700">
              <FolderOpen className="h-4 w-4" /> 文件管理
            </button>
            <button onClick={load} className="flex items-center gap-1 px-3 py-2 bg-gray-100 dark:bg-gray-700 rounded-lg text-sm hover:bg-gray-200 dark:hover:bg-gray-600">
              <RefreshCw className="h-4 w-4" /> 刷新
            </button>
          </div>
        </div>

        {/* 搜索框 */}
        <div className="relative">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-gray-400" />
          <input
            type="text"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            placeholder="搜索项目名称、路径或配置文件..."
            className="w-full pl-10 pr-4 py-2 bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-600 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary-500"
          />
          {searchQuery && (
            <button
              onClick={() => setSearchQuery('')}
              className="absolute right-3 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
              title="清除搜索"
            >
              ✕
            </button>
          )}
        </div>
      </div>

      {/* 内容区域 */}
      <div className="px-2 sm:px-6 space-y-4">
        {error && <div className="p-3 bg-red-50 text-red-600 rounded-lg text-sm">{error}</div>}
        {loading && <div className="text-gray-500 text-sm">加载中...</div>}

        <div className="grid gap-3">
        {filteredProjects.map((p) => (
          <div key={p.id} className="card">
            <div className="flex items-start justify-between gap-4">
              {/* 左侧：项目信息区 */}
              <div className="min-w-0 flex-1 space-y-1.5">
                <div className="font-semibold text-gray-900 dark:text-white break-words">
                  {p.name}
                </div>
                <div className="text-xs text-gray-500 break-all">
                  {p.dir}
                </div>
                <div className="text-xs text-gray-400 truncate">
                  {p.composeFile}
                </div>
              </div>

              {/* 右侧：操作按钮区 */}
              <div className="flex items-center gap-1 flex-shrink-0">
                <ActionBtn disabled={busyId === p.id} onClick={() => doAction(p, 'up')} icon={Play} title="启动" color="emerald" />
                <ActionBtn disabled={busyId === p.id} onClick={() => doAction(p, 'down')} icon={Square} title="停止" color="red" />
                <ActionBtn disabled={busyId === p.id} onClick={() => doAction(p, 'restart')} icon={RotateCw} title="重启" color="blue" />
                <ActionBtn disabled={busyId === p.id} onClick={() => doAction(p, 'pull')} icon={Download} title="拉取" color="gray" />
                <button onClick={() => setEditing({ project: p, filename: p.composeFile })}
                  className="p-2 text-primary-600 hover:bg-primary-50 dark:hover:bg-primary-900/20 rounded-lg transition-colors" title="编辑">
                  <FileEdit className="h-4 w-4" />
                </button>
              </div>
            </div>
          </div>
        ))}
        {!loading && filteredProjects.length === 0 && projects.length > 0 && (
          <div className="text-gray-400 text-sm">
            未找到匹配 "{searchQuery}" 的项目
          </div>
        )}
        {!loading && projects.length === 0 && (
          <div className="text-gray-400 text-sm">未发现 Compose 项目。请在「设置」页面的「Compose 目录配置」中填写已挂载进容器的项目目录并保存。</div>
        )}
      </div>
      </div>

      {editing && (
        <ComposeEditor project={editing.project} filename={editing.filename}
          onClose={() => setEditing(null)} />
      )}

      {showFileManager && (
        <ComposeFileManager
          onClose={() => setShowFileManager(false)}
          onFileCreated={(filePath) => {
            setShowFileManager(false)
            load() // 刷新项目列表
          }}
        />
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
