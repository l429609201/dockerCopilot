import React, { useState, useEffect, useCallback } from 'react'
import { X, Folder, ArrowUp, RefreshCw, Check, AlertCircle } from 'lucide-react'
import { composeAPI } from '../api/client.js'

// 目录选择器弹窗：浏览 DC 自身文件系统（宿主机已挂载进容器的目录），仅选择目录。
// onSelect(path) 选中当前目录后回调；onClose 关闭。initialPath 可选起始路径。
export function DirectoryPicker({ initialPath = '', onSelect, onClose }) {
  const [current, setCurrent] = useState('') // 当前所在目录
  const [parent, setParent] = useState('')   // 上一级目录（空表示已在根）
  const [dirs, setDirs] = useState([])       // 当前目录下的子目录
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  // 加载指定目录的子目录列表。
  // 若指定路径不可访问（如宿主机真实路径在 DC 容器内不存在），自动回退到根目录。
  const load = useCallback(async (path, allowFallback = false) => {
    setLoading(true)
    setError('')
    try {
      const r = await composeAPI.browse(path || '')
      const d = r.data?.data || {}
      // 后端 Browse 返回 code != 200 时表示路径不可访问
      if (r.data?.code !== 200 && allowFallback && path) {
        // 首次加载 initialPath 不可访问时，回退到根目录浏览
        return load('', false)
      }
      setCurrent(d.path || '')
      setParent(d.parent || '')
      setDirs(Array.isArray(d.dirs) ? d.dirs : [])
    } catch (e) {
      if (allowFallback && path) {
        // 网络/解析异常也回退到根目录
        return load('', false)
      }
      setError('读取目录失败：' + (e.response?.data?.msg || e.message))
    } finally {
      setLoading(false)
    }
  }, [])

  // 首次加载使用 allowFallback=true：initialPath 不可访问时自动回退根目录
  useEffect(() => { load(initialPath, true) }, [load, initialPath])

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-[60] p-4">
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow-xl w-full max-w-lg max-h-[80vh] flex flex-col">
        {/* 头部 */}
        <div className="flex items-center justify-between p-4 border-b border-gray-200 dark:border-gray-700">
          <div>
            <h3 className="text-base font-semibold text-gray-900 dark:text-white flex items-center gap-2">
              <Folder className="h-5 w-5 text-primary-600" /> 浏览 DockerCopilot 容器内目录
            </h3>
            <p className="text-xs text-amber-600 dark:text-amber-400 mt-1">
              ⚠️ 这里显示的是 DockerCopilot 容器内的目录，不是宿主机真实路径
            </p>
          </div>
          <button onClick={onClose} className="p-1.5 text-gray-500 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-lg">
            <X className="h-4 w-4" />
          </button>
        </div>

        {/* 当前路径栏 */}
        <div className="px-4 py-2 border-b border-gray-100 dark:border-gray-700/50 flex items-center gap-2">
          <button
            onClick={() => load(parent)}
            disabled={!parent || loading}
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
          {dirs.map((name) => {
            // 后端 /compose/browse 返回的 dirs 是「目录名字符串数组」（非对象），
            // 需用当前目录 current 拼出完整子路径再进入。
            const childPath = (current === '/' || current === '')
              ? `/${name}`
              : `${current.replace(/\/$/, '')}/${name}`
            return (
              <button
                key={name}
                onClick={() => load(childPath)}
                className="w-full flex items-center gap-2 px-3 py-2 text-left rounded-lg hover:bg-gray-100 dark:hover:bg-gray-700/50 text-sm text-gray-700 dark:text-gray-200"
              >
                <Folder className="h-4 w-4 text-amber-500 flex-shrink-0" />
                <span className="truncate">{name}</span>
              </button>
            )
          })}
        </div>

        {/* 底部：选择当前目录 */}
        <div className="flex items-center justify-between gap-3 p-4 border-t border-gray-200 dark:border-gray-700">
          <span className="text-xs text-gray-500 dark:text-gray-400">点击目录进入下一级</span>
          <div className="flex items-center gap-2">
            <button onClick={onClose} className="px-3 py-2 text-sm text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-lg">取消</button>
            <button
              onClick={() => { if (current) { onSelect(current); onClose() } }}
              disabled={!current || loading}
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
