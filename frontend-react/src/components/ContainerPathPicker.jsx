import React, { useState, useEffect, useCallback } from 'react'
import { X, Folder, ArrowUp, RefreshCw, Check, AlertCircle } from 'lucide-react'
import { filesAPI } from '../api/client.js'

// 容器内路径选择器弹窗：浏览指定容器的文件系统，仅选择目录。
// 复用结构化的 filesAPI.list（后端已做防穿越校验），无需解析 ls 文本。
// onSelect(path) 选中当前目录后回调；onClose 关闭。
// hostId 定位容器所属 Docker 主机（多 Docker 管理），远程容器必须传入，
// 否则 exec 请求会打到本地 daemon 报「No such container」。
// 注意：底层通过 docker exec 列目录，要求容器处于运行状态。
export function ContainerPathPicker({ containerId, hostId, initialPath = '/', onSelect, onClose }) {
  const [current, setCurrent] = useState('/') // 当前所在目录
  const [dirs, setDirs] = useState([])        // 当前目录下的子目录（仅目录）
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  // 上一级目录：根目录时为空
  const parent = current === '/' ? '' : '/' + current.split('/').filter(Boolean).slice(0, -1).join('/')

  // 拼接子目录完整路径
  const joinPath = (dir, name) => (dir === '/' ? `/${name}` : `${dir}/${name}`)

  // 加载指定目录，仅保留其中的子目录
  const load = useCallback(async (path) => {
    const p = path || '/'
    setLoading(true)
    setError('')
    try {
      // 传入 hostId，保证远程容器的 exec 列目录请求路由到正确的 Docker 主机
      const r = await filesAPI.list(containerId, p, hostId)
      if (r.data?.code === 200) {
        const entries = r.data.data?.entries || []
        setDirs(entries.filter((e) => e.isDir))
        setCurrent(p)
      } else {
        setError(r.data?.msg || '列目录失败')
      }
    } catch (e) {
      setError('读取目录失败：' + (e.response?.data?.msg || e.message))
    } finally {
      setLoading(false)
    }
  }, [containerId, hostId])

  useEffect(() => { load(initialPath) }, [load, initialPath])

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-[60] p-4">
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow-xl w-full max-w-lg max-h-[80vh] flex flex-col">
        {/* 头部 */}
        <div className="flex items-center justify-between p-4 border-b border-gray-200 dark:border-gray-700">
          <h3 className="text-base font-semibold text-gray-900 dark:text-white flex items-center gap-2">
            <Folder className="h-5 w-5 text-primary-600" /> 选择容器内目录
          </h3>
          <button onClick={onClose} className="p-1.5 text-gray-500 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-lg">
            <X className="h-4 w-4" />
          </button>
        </div>

        {/* 当前路径栏 */}
        <div className="px-4 py-2 border-b border-gray-100 dark:border-gray-700/50 flex items-center gap-2">
          <button
            onClick={() => load(parent || '/')}
            disabled={current === '/' || loading}
            className="p-1.5 rounded-lg text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 disabled:opacity-40 disabled:cursor-not-allowed"
            title="上一级"
          >
            <ArrowUp className="h-4 w-4" />
          </button>
          <code className="flex-1 text-xs bg-gray-50 dark:bg-gray-900 rounded px-2 py-1.5 break-all text-gray-700 dark:text-gray-300">
            {current || '/'}
          </code>
          <button onClick={() => load(current)} disabled={loading}
            className="p-1.5 rounded-lg text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 disabled:opacity-40" title="刷新">
            <RefreshCw className={`h-4 w-4 ${loading ? 'animate-spin' : ''}`} />
          </button>
        </div>

        {/* 目录列表 */}
        <div className="flex-1 overflow-y-auto p-2 min-h-[200px]">
          {error && (
            <div className="m-2 p-3 flex items-start gap-2 bg-red-50 dark:bg-red-900/20 text-red-600 dark:text-red-300 rounded-lg text-sm">
              <AlertCircle className="h-4 w-4 mt-0.5 flex-shrink-0" /><span className="break-all">{error}</span>
            </div>
          )}
          {loading && dirs.length === 0 && (
            <div className="text-center py-10 text-gray-400 text-sm">
              <RefreshCw className="h-6 w-6 animate-spin mx-auto mb-2" /> 加载中...
            </div>
          )}
          {!loading && !error && dirs.length === 0 && (
            <div className="text-center py-10 text-gray-400 text-sm">该目录下没有子目录</div>
          )}
          {dirs.map((d) => (
            <button
              key={d.name}
              onClick={() => load(joinPath(current, d.name))}
              className="w-full flex items-center gap-2 px-3 py-2 text-left rounded-lg hover:bg-gray-100 dark:hover:bg-gray-700/50 text-sm text-gray-700 dark:text-gray-200"
            >
              <Folder className="h-4 w-4 text-amber-500 flex-shrink-0" />
              <span className="truncate">{d.name}</span>
            </button>
          ))}
        </div>

        {/* 底部：选择当前目录 */}
        <div className="flex items-center justify-between gap-3 p-4 border-t border-gray-200 dark:border-gray-700">
          <span className="text-xs text-gray-500 dark:text-gray-400">点击目录进入下一级；需容器运行中</span>
          <div className="flex items-center gap-2">
            <button onClick={onClose} className="px-3 py-2 text-sm text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-lg">取消</button>
            <button
              onClick={() => { onSelect(current); onClose() }}
              disabled={loading}
              className="px-4 py-2 text-sm bg-primary-600 text-white rounded-lg hover:bg-primary-700 disabled:opacity-50 flex items-center gap-1.5"
            >
              <Check className="h-4 w-4" /> 选择此目录
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
