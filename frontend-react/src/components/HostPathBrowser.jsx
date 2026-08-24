import React, { useState, useEffect } from 'react'
import { hostPathAPI } from '../api/client.js'
import { Folder, AlertCircle, RefreshCw } from 'lucide-react'

/**
 * 宿主机路径浏览器 - 只读式访问
 * 允许浏览宿主机上挂载到容器的目录（只读）
 */
export function HostPathBrowser() {
  const [mappings, setMappings] = useState([])
  const [selectedMapping, setSelectedMapping] = useState(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)

  // 加载路径映射配置
  const loadMappings = async () => {
    try {
      setLoading(true)
      setError(null)
      const response = await hostPathAPI.getMappings()

      if (response.data.code === 200) {
        const data = response.data.data
        if (data.enabled && data.mappings && data.mappings.length > 0) {
          setMappings(data.mappings)
          // 默认选中第一个映射
          if (!selectedMapping) {
            setSelectedMapping(data.mappings[0])
          }
        } else {
          setError('宿主机路径映射功能未启用或无可用映射')
        }
      } else {
        setError(response.data.msg || '加载映射配置失败')
      }
    } catch (err) {
      console.error('Failed to load mappings:', err)
      setError('加载映射配置失败: ' + (err.message || '未知错误'))
    } finally {
      setLoading(false)
    }
  }

  // 组件挂载时加载
  useEffect(() => {
    loadMappings()
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  if (loading) {
    return (
      <div className="flex items-center justify-center min-h-[400px]">
        <RefreshCw className="w-8 h-8 animate-spin text-blue-500" />
        <span className="ml-3 text-gray-600 dark:text-gray-400">加载中...</span>
      </div>
    )
  }

  if (error) {
    return (
      <div className="flex flex-col items-center justify-center min-h-[400px] p-6">
        <AlertCircle className="w-12 h-12 text-red-500 mb-4" />
        <p className="text-red-600 dark:text-red-400 text-center mb-4">{error}</p>
        <p className="text-sm text-gray-500 dark:text-gray-400 text-center mb-4">
          请在配置文件中启用宿主机路径映射功能并添加映射规则
        </p>
        <button
          onClick={loadMappings}
          className="px-4 py-2 bg-blue-500 text-white rounded-lg hover:bg-blue-600 transition-colors"
        >
          重新加载
        </button>
      </div>
    )
  }

  if (mappings.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center min-h-[400px] p-6">
        <Folder className="w-12 h-12 text-gray-400 mb-4" />
        <p className="text-gray-600 dark:text-gray-400 text-center">
          暂无可用的宿主机路径映射
        </p>
      </div>
    )
  }

  return (
    <div className="space-y-4">
      {/* 路径映射选择器 */}
      <div className="bg-white dark:bg-gray-800 rounded-xl p-4 shadow-sm border border-gray-200 dark:border-gray-700">
        <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
          选择映射路径
        </label>
        <select
          value={selectedMapping?.id || ''}
          onChange={(e) => {
            const mapping = mappings.find(m => m.id === e.target.value)
            setSelectedMapping(mapping)
          }}
          className="w-full px-4 py-2 bg-gray-50 dark:bg-gray-900 border border-gray-300 dark:border-gray-600 
                   rounded-lg text-gray-900 dark:text-gray-100 focus:outline-none focus:ring-2 
                   focus:ring-blue-500 transition-colors"
        >
          {mappings.map(mapping => (
            <option key={mapping.id} value={mapping.id}>
              {mapping.description || mapping.id} - {mapping.containerPath} → {mapping.hostPath}
            </option>
          ))}
        </select>
        {selectedMapping?.readOnly && (
          <p className="mt-2 text-sm text-amber-600 dark:text-amber-400">
            ⚠️ 此路径为只读模式
          </p>
        )}
      </div>

      {/* 文件管理器 - 使用宿主机 API */}
      {selectedMapping && (
        <div className="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700 p-4">
          <div className="text-sm text-gray-600 dark:text-gray-400 mb-4">
            <p><strong>容器路径:</strong> {selectedMapping.containerPath}</p>
            <p><strong>宿主机路径:</strong> {selectedMapping.hostPath}</p>
            {selectedMapping.readOnly && (
              <p className="text-amber-600 dark:text-amber-400 mt-2">⚠️ 只读模式</p>
            )}
          </div>
          {/* TODO: 实现宿主机文件浏览器界面 */}
          <div className="text-center py-8 text-gray-500 dark:text-gray-400">
            宿主机文件浏览器功能开发中...
          </div>
        </div>
      )}
    </div>
  )
}
