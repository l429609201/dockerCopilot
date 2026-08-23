import React, { useState, useEffect } from 'react'
import { X, RefreshCw } from 'lucide-react'
import { containerAPI } from '../api/client.js'

// 容器进程查看弹窗：调用 docker top 展示进程列表（PID/USER/CPU/COMMAND 等）。
export function ContainerProcessModal({ container, onClose }) {
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [titles, setTitles] = useState([])
  const [processes, setProcesses] = useState([])

  const load = async () => {
    setLoading(true)
    setError('')
    try {
      const r = await containerAPI.topContainer(container.ID)
      const d = r.data?.data || {}
      setTitles(d.titles || [])
      setProcesses(d.processes || [])
    } catch (e) {
      setError('获取进程列表失败：' + (e.response?.data?.msg || e.message))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { load() }, [container.ID])

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50 p-4">
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow-xl w-full max-w-4xl max-h-[85vh] flex flex-col">
        {/* 头部 */}
        <div className="flex items-center justify-between p-4 border-b border-gray-200 dark:border-gray-700">
          <h3 className="text-lg font-semibold text-gray-900 dark:text-white">
            🔍 容器进程 - {container.Names?.[0]?.replace(/^\//, '') || container.ID.slice(0, 12)}
          </h3>
          <div className="flex items-center gap-2">
            <button onClick={load} disabled={loading}
              className="p-2 text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-lg disabled:opacity-50"
              title="刷新">
              <RefreshCw className={`h-4 w-4 ${loading ? 'animate-spin' : ''}`} />
            </button>
            <button onClick={onClose}
              className="p-2 text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-lg">
              <X className="h-4 w-4" />
            </button>
          </div>
        </div>

        {/* 内容区 */}
        <div className="flex-1 overflow-auto p-4">
          {error && (
            <div className="p-3 bg-red-50 dark:bg-red-900/20 text-red-700 dark:text-red-300 rounded-lg mb-4">
              {error}
            </div>
          )}

          {loading && processes.length === 0 && (
            <div className="text-center text-gray-500 py-8">
              <RefreshCw className="h-8 w-8 animate-spin mx-auto mb-2" />
              加载中...
            </div>
          )}

          {!loading && processes.length === 0 && !error && (
            <div className="text-center text-gray-400 py-8">
              暂无运行中的进程
            </div>
          )}

          {processes.length > 0 && (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead className="bg-gray-50 dark:bg-gray-900/50 sticky top-0">
                  <tr>
                    {titles.map((title, i) => (
                      <th key={i} className="px-3 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                        {title}
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
                  {processes.map((proc, idx) => (
                    <tr key={idx} className="hover:bg-gray-50 dark:hover:bg-gray-700/50">
                      {proc.map((cell, cellIdx) => (
                        <td key={cellIdx} className="px-3 py-2 text-gray-900 dark:text-gray-100 font-mono text-xs whitespace-nowrap">
                          {cell}
                        </td>
                      ))}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>

        {/* 底部说明 */}
        <div className="p-3 border-t border-gray-200 dark:border-gray-700 text-xs text-gray-500 dark:text-gray-400">
          💡 进程列表通过 <code className="px-1 py-0.5 bg-gray-100 dark:bg-gray-700 rounded">docker top</code> 获取，显示容器内所有运行中的进程。
        </div>
      </div>
    </div>
  )
}
