import React, { useState, useCallback } from 'react'
import { FileText, Terminal, RefreshCw } from 'lucide-react'
import { containerAPI } from '../api/client.js'

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
          <TabBtn active={tab === 'exec'} onClick={() => setTab('exec')} icon={Terminal} label="命令" />
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

// 命令执行面板
function ExecPanel({ id }) {
  const [cmd, setCmd] = useState('ls -al')
  const [output, setOutput] = useState('')
  const [running, setRunning] = useState(false)

  const run = async () => {
    const parts = cmd.trim().split(/\s+/).filter(Boolean)
    if (parts.length === 0) return
    setRunning(true); setOutput('执行中...')
    try {
      const r = await containerAPI.execContainer(id, parts)
      const d = r.data?.data
      setOutput(`退出码: ${d?.exitCode}\n\n${d?.output || ''}`)
    } catch (e) {
      setOutput('执行失败：' + (e.message || '未知错误'))
    } finally { setRunning(false) }
  }

  return (
    <div className="flex-1 flex flex-col min-h-0">
      <div className="flex gap-2 mb-2">
        <input value={cmd} onChange={(e) => setCmd(e.target.value)} className="input font-mono"
          placeholder="如：ls -al  （参数以空格分隔，不支持管道）" />
        <button onClick={run} disabled={running}
          className="px-4 py-2 bg-primary-600 text-white rounded-lg text-sm hover:bg-primary-700 disabled:opacity-60">
          执行
        </button>
      </div>
      <pre className="flex-1 min-h-[280px] overflow-auto text-xs font-mono p-3 bg-gray-900 text-gray-100 rounded-lg whitespace-pre-wrap">
        {output}
      </pre>
      <p className="text-xs text-gray-400 mt-1">提示：命令通过 Docker Exec API 执行，参数按空格拆分，不支持 shell 管道/重定向。</p>
    </div>
  )
}
