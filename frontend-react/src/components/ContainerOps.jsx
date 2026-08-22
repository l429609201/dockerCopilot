import React, { useState, useCallback } from 'react'
import { FileText, Terminal as TerminalIcon, RefreshCw } from 'lucide-react'
import { containerAPI } from '../api/client.js'
import { Terminal } from './Terminal.jsx'

// 容器运维弹窗：日志查看 + 命令执行（一次性）
export function ContainerOps({ container, onClose }) {
  const [tab, setTab] = useState('logs')
  return (
    <div className="fixed inset-0 bg-black/50 z-50 flex items-center justify-center p-4">
      <div className="bg-white dark:bg-gray-800 rounded-xl w-full max-w-3xl max-h-[90vh] flex flex-col p-5">
        <div className="flex items-center justify-between mb-3">
          <h3 className="text-lg font-bold text-gray-900 dark:text-white truncate">
            运维 · {container.name}
          </h3>
          <button onClick={onClose} className="text-gray-400 hover:text-gray-600 text-xl leading-none">×</button>
        </div>
        <div className="flex gap-2 mb-3">
          <TabBtn active={tab === 'logs'} onClick={() => setTab('logs')} icon={FileText} label="日志" />
          <TabBtn active={tab === 'exec'} onClick={() => setTab('exec')} icon={TerminalIcon} label="控制台" />
        </div>
        {tab === 'logs' ? <LogsPanel id={container.id} /> : <ExecPanel id={container.id} />}
      </div>
    </div>
  )
}

function TabBtn({ active, onClick, icon: Icon, label }) {
  return (
    <button onClick={onClick}
      className={`flex items-center gap-1 px-3 py-1.5 rounded-lg text-sm ${active ? 'bg-primary-600 text-white' : 'bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-300'}`}>
      <Icon className="h-4 w-4" /> {label}
    </button>
  )
}

// 日志面板
function LogsPanel({ id }) {
  const [logs, setLogs] = useState('')
  const [tail, setTail] = useState(200)
  const [timestamps, setTimestamps] = useState(false)
  const [loading, setLoading] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const r = await containerAPI.getContainerLogs(id, { tail, timestamps })
      setLogs(r.data?.data?.logs || '(无日志)')
    } catch (e) {
      setLogs('读取失败：' + e.message)
    } finally { setLoading(false) }
  }, [id, tail, timestamps])

  React.useEffect(() => { load() }, [load])

  return (
    <div className="flex-1 flex flex-col min-h-0">
      <div className="flex items-center gap-3 mb-2 text-sm">
        <label className="flex items-center gap-1">行数
          <input type="number" value={tail} onChange={(e) => setTail(Number(e.target.value))}
            className="w-20 px-2 py-1 border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-900" />
        </label>
        <label className="flex items-center gap-1">
          <input type="checkbox" checked={timestamps} onChange={(e) => setTimestamps(e.target.checked)} /> 时间戳
        </label>
        <button onClick={load} className="flex items-center gap-1 px-2 py-1 bg-gray-100 dark:bg-gray-700 rounded">
          <RefreshCw className="h-3.5 w-3.5" /> 刷新
        </button>
      </div>
      <pre className="flex-1 min-h-[300px] overflow-auto text-xs font-mono p-3 bg-gray-900 text-gray-100 rounded-lg whitespace-pre-wrap">
        {loading ? '加载中...' : logs}
      </pre>
    </div>
  )
}

// 交互式控制台面板（Portainer 风格）：选 shell + 用户 -> 连接 -> xterm 终端
function ExecPanel({ id }) {
  const [shell, setShell] = useState('/bin/bash')
  const [custom, setCustom] = useState(false)
  const [customCmd, setCustomCmd] = useState('/bin/bash')
  const [user, setUser] = useState('root')
  const [connected, setConnected] = useState(false)
  const [sessionKey, setSessionKey] = useState(0)

  const effectiveCmd = custom ? customCmd : shell

  const connect = () => {
    setSessionKey((k) => k + 1)
    setConnected(true)
  }
  const disconnect = () => setConnected(false)

  return (
    <div className="flex-1 flex flex-col min-h-0">
      {/* 连接配置栏 */}
      <div className="flex flex-wrap items-end gap-3 mb-3">
        <div>
          <label className="block text-xs text-gray-500 mb-1">命令</label>
          {custom ? (
            <input value={customCmd} onChange={(e) => setCustomCmd(e.target.value)}
              className="input font-mono w-48" placeholder="自定义命令" />
          ) : (
            <select value={shell} onChange={(e) => setShell(e.target.value)} className="input w-48">
              <option value="/bin/bash">/bin/bash</option>
              <option value="/bin/sh">/bin/sh</option>
              <option value="/bin/ash">/bin/ash</option>
            </select>
          )}
        </div>
        <label className="flex items-center gap-1 text-sm text-gray-600 dark:text-gray-300 pb-2">
          <input type="checkbox" checked={custom} onChange={(e) => setCustom(e.target.checked)} />
          自定义命令
        </label>
        <div>
          <label className="block text-xs text-gray-500 mb-1">用户</label>
          <input value={user} onChange={(e) => setUser(e.target.value)} className="input w-32" placeholder="root" />
        </div>
        {connected ? (
          <button onClick={disconnect} className="px-4 py-2 bg-red-600 text-white rounded-lg text-sm hover:bg-red-700">断开</button>
        ) : (
          <button onClick={connect} className="px-4 py-2 bg-primary-600 text-white rounded-lg text-sm hover:bg-primary-700">连接</button>
        )}
      </div>

      {connected ? (
        <Terminal key={sessionKey} containerId={id} cmd={effectiveCmd} user={user} />
      ) : (
        <div className="flex-1 min-h-[280px] flex items-center justify-center text-gray-400 text-sm bg-gray-900/50 rounded-lg">
          选择 shell 与用户后点击「连接」进入交互式终端
        </div>
      )}
    </div>
  )
}
