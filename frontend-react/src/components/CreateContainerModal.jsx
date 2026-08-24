import React, { useState, useEffect } from 'react'
import { X, Plus, Terminal, FileCode } from 'lucide-react'
import { dockerHostAPI } from '../api/client.js'
import { RunCommandTab } from './RunCommandTab.jsx'
import { ComposeDeployTab } from './ComposeDeployTab.jsx'

// 创建容器弹窗：仅提供两种业界标准方式——Docker Run 命令 / Docker Compose (YAML)。
// 支持引入宿主机路径（仅本机 Docker 生效）。提交后返回 taskID 供外层轮询进度。
export function CreateContainerModal({ onClose, onCreated }) {
  const [activeTab, setActiveTab] = useState('compose') // compose | run，默认 Compose
  const [hosts, setHosts] = useState([])

  // 加载可用 Docker 主机（用于 Run 页签的目标主机下拉）
  useEffect(() => {
    dockerHostAPI.list().then((r) => {
      if (r.data?.code === 200 && Array.isArray(r.data.data)) {
        setHosts(r.data.data.filter((h) => h.enabled))
      }
    }).catch(() => {})
  }, [])

  const tabs = [
    { id: 'compose', label: 'Docker Compose (YAML)', icon: FileCode },
    { id: 'run', label: 'Docker Run Command', icon: Terminal },
  ]

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" onClick={onClose}>
      <div className="w-full max-w-3xl max-h-[92vh] overflow-hidden flex flex-col rounded-xl bg-white dark:bg-gray-800 shadow-xl"
        onClick={(e) => e.stopPropagation()}>
        {/* 头部 */}
        <div className="flex items-center justify-between px-5 py-4 border-b border-gray-200 dark:border-gray-700">
          <h3 className="text-lg font-semibold text-gray-900 dark:text-gray-100 flex items-center gap-2">
            <Plus className="h-5 w-5" /> 创建容器
          </h3>
          <button onClick={onClose} className="text-gray-400 hover:text-gray-600"><X className="h-5 w-5" /></button>
        </div>

        {/* 标签切换 */}
        <div className="flex gap-1 px-5 pt-3 border-b border-gray-200 dark:border-gray-700">
          {tabs.map((t) => {
            const Icon = t.icon
            const on = activeTab === t.id
            return (
              <button key={t.id} onClick={() => setActiveTab(t.id)}
                className={`flex items-center gap-1.5 px-4 py-2 text-sm font-medium rounded-t-lg border-b-2 -mb-px transition-colors ${
                  on ? 'border-blue-600 text-blue-600 dark:text-blue-400'
                     : 'border-transparent text-gray-500 hover:text-gray-700 dark:hover:text-gray-300'
                }`}>
                <Icon className="h-4 w-4" /> {t.label}
              </button>
            )
          })}
        </div>

        {/* 页签内容 */}
        <div className="flex-1 overflow-y-auto p-5">
          {activeTab === 'run'
            ? <RunCommandTab hosts={hosts} onClose={onClose} onCreated={onCreated} />
            : <ComposeDeployTab onClose={onClose} onCreated={onCreated} />}
        </div>
      </div>
    </div>
  )
}
