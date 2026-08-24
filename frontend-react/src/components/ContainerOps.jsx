import React, { useState, useCallback, useMemo } from 'react'
import { FileText, Terminal as TerminalIcon, RefreshCw, Maximize2, Minimize2, X, Search, Download } from 'lucide-react'
import { containerAPI } from '../api/client.js'
import { Terminal } from './Terminal.jsx'
import { cn } from '../utils/cn.js'

// 【已废弃】容器运维弹窗：日志查看 + 命令执行（一次性）。支持最大化/全屏。
// 为了更好的用户体验，已拆分为 ContainerLogs 和 ContainerConsole 两个独立组件
export function ContainerOps({ container, onClose, initialTab = 'logs' }) {
  const [tab, setTab] = useState(initialTab === 'exec' ? 'exec' : 'logs')
  const [fullscreen, setFullscreen] = useState(false)
  return (
    <div className={cn('fixed inset-0 bg-black/50 z-50 flex items-center justify-center',
      fullscreen ? 'p-0' : 'p-4')}>
      <div className={cn('bg-white dark:bg-gray-800 flex flex-col',
        fullscreen
          ? 'w-screen h-screen max-w-none max-h-none rounded-none p-4'
          : 'w-full max-w-3xl max-h-[90vh] rounded-xl p-5')}>
        <div className="flex items-center justify-between mb-3">
          <h3 className="text-lg font-bold text-gray-900 dark:text-white truncate">
            运维 · {container.name}
          </h3>
          <div className="flex items-center gap-1">
            <button onClick={() => setFullscreen(v => !v)} title={fullscreen ? '还原' : '全屏'}
              className="p-1.5 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 rounded hover:bg-gray-100 dark:hover:bg-gray-700">
              {fullscreen ? <Minimize2 className="h-4.5 w-4.5" /> : <Maximize2 className="h-4.5 w-4.5" />}
            </button>
            <button onClick={onClose} title="关闭"
              className="p-1.5 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 rounded hover:bg-gray-100 dark:hover:bg-gray-700">
              <X className="h-5 w-5" />
            </button>
          </div>
        </div>
        <div className="flex gap-2 mb-3">
          <TabBtn active={tab === 'logs'} onClick={() => setTab('logs')} icon={FileText} label="日志" />
          <TabBtn active={tab === 'exec'} onClick={() => setTab('exec')} icon={TerminalIcon} label="终端" />
        </div>
        {tab === 'logs' ? <LogsPanel id={container.id} name={container.name} hostId={container.hostId} /> : <ExecPanel id={container.id} fullscreen={fullscreen} hostId={container.hostId} />}
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

// 【新】独立的日志查看弹窗
export function ContainerLogs({ container, onClose }) {
  const [fullscreen, setFullscreen] = useState(false)
  return (
    <div className={cn('fixed inset-0 bg-black/50 z-50 flex items-center justify-center',
      fullscreen ? 'p-0' : 'p-4')}>
      <div className={cn('bg-white dark:bg-gray-800 flex flex-col',
        fullscreen
          ? 'w-screen h-screen max-w-none max-h-none rounded-none p-4'
          : 'w-full max-w-3xl max-h-[90vh] rounded-xl p-5')}>
        <div className="flex items-center justify-between mb-3">
          <h3 className="text-lg font-bold text-gray-900 dark:text-white truncate flex items-center gap-2">
            <FileText className="h-5 w-5 text-sky-600 dark:text-sky-400" />
            日志 · {container.name}
          </h3>
          <div className="flex items-center gap-1">
            <button onClick={() => setFullscreen(v => !v)} title={fullscreen ? '还原' : '全屏'}
              className="p-1.5 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 rounded hover:bg-gray-100 dark:hover:bg-gray-700">
              {fullscreen ? <Minimize2 className="h-4.5 w-4.5" /> : <Maximize2 className="h-4.5 w-4.5" />}
            </button>
            <button onClick={onClose} title="关闭"
              className="p-1.5 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 rounded hover:bg-gray-100 dark:hover:bg-gray-700">
              <X className="h-5 w-5" />
            </button>
          </div>
        </div>
        <LogsPanel id={container.id} name={container.name} hostId={container.hostId} />
      </div>
    </div>
  )
}

// 【新】独立的控制台弹窗
export function ContainerConsole({ container, onClose }) {
  const [fullscreen, setFullscreen] = useState(false)
  return (
    <div className={cn('fixed inset-0 bg-black/50 z-50 flex items-center justify-center',
      fullscreen ? 'p-0' : 'p-4')}>
      <div className={cn('bg-white dark:bg-gray-800 flex flex-col',
        fullscreen
          ? 'w-screen h-screen max-w-none max-h-none rounded-none p-4'
          : 'w-full max-w-3xl max-h-[90vh] rounded-xl p-5')}>
        <div className="flex items-center justify-between mb-3">
          <h3 className="text-lg font-bold text-gray-900 dark:text-white truncate flex items-center gap-2">
            <TerminalIcon className="h-5 w-5 text-teal-600 dark:text-teal-400" />
            终端 · {container.name}
          </h3>
          <div className="flex items-center gap-1">
            <button onClick={() => setFullscreen(v => !v)} title={fullscreen ? '还原' : '全屏'}
              className="p-1.5 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 rounded hover:bg-gray-100 dark:hover:bg-gray-700">
              {fullscreen ? <Minimize2 className="h-4.5 w-4.5" /> : <Maximize2 className="h-4.5 w-4.5" />}
            </button>
            <button onClick={onClose} title="关闭"
              className="p-1.5 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 rounded hover:bg-gray-100 dark:hover:bg-gray-700">
              <X className="h-5 w-5" />
            </button>
          </div>
        </div>
        <ExecPanel id={container.id} fullscreen={fullscreen} hostId={container.hostId} />
      </div>
    </div>
  )
}

// 转义正则特殊字符，避免搜索词包含 . * 等导致高亮异常
function escapeRegExp(s) {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

// 将一行日志按关键词（不区分大小写）拆分并高亮匹配片段
function highlightLine(text, keyword) {
  if (!keyword) return text
  const parts = text.split(new RegExp(`(${escapeRegExp(keyword)})`, 'gi'))
  return parts.map((part, i) =>
    part.toLowerCase() === keyword.toLowerCase()
      ? <mark key={i} className="bg-yellow-400 text-black rounded px-0.5">{part}</mark>
      : part
  )
}

// 日志面板：支持行数/时间戳、关键词搜索过滤+高亮、日志下载
function LogsPanel({ id, name, hostId }) {
  const [logs, setLogs] = useState('')
  const [tail, setTail] = useState(200)
  const [timestamps, setTimestamps] = useState(false)
  const [loading, setLoading] = useState(false)
  const [search, setSearch] = useState('') // 搜索关键词

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const r = await containerAPI.getContainerLogs(id, { tail, timestamps }, hostId)
      setLogs(r.data?.data?.logs || '(无日志)')
    } catch (e) {
      setLogs('读取失败：' + e.message)
    } finally { setLoading(false) }
  }, [id, tail, timestamps, hostId])

  React.useEffect(() => { load() }, [load])

  // 按关键词过滤出需要展示的行（不区分大小写），空关键词时展示全部
  const shownLines = useMemo(() => {
    const lines = logs.split('\n')
    const kw = search.trim()
    if (!kw) return lines
    const lower = kw.toLowerCase()
    return lines.filter((l) => l.toLowerCase().includes(lower))
  }, [logs, search])

  // 下载当前完整日志为 .log 文件（不受搜索过滤影响，导出全部内容）
  const download = () => {
    const blob = new Blob([logs], { type: 'text/plain;charset=utf-8' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    const ts = new Date().toISOString().replace(/[:.]/g, '-')
    a.href = url
    a.download = `${(name || id).slice(0, 40)}_${ts}.log`
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    URL.revokeObjectURL(url)
  }

  const kw = search.trim()

  return (
    <div className="flex-1 flex flex-col min-h-0">
      <div className="flex flex-wrap items-center gap-3 mb-2 text-sm">
        <label className="flex items-center gap-1">行数
          <input type="number" value={tail} onChange={(e) => setTail(Number(e.target.value))}
            className="w-20 px-2 py-1 border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-900" />
        </label>
        <label className="flex items-center gap-1">
          <input type="checkbox" checked={timestamps} onChange={(e) => setTimestamps(e.target.checked)} /> 时间戳
        </label>
        {/* 搜索框：实时过滤并高亮匹配行 */}
        <div className="relative flex-1 min-w-[160px]">
          <Search className="absolute left-2 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-gray-400" />
          <input value={search} onChange={(e) => setSearch(e.target.value)} placeholder="搜索日志…"
            className="w-full pl-7 pr-2 py-1 border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-900" />
        </div>
        <button onClick={load} className="flex items-center gap-1 px-2 py-1 bg-gray-100 dark:bg-gray-700 rounded">
          <RefreshCw className="h-3.5 w-3.5" /> 刷新
        </button>
        <button onClick={download} title="下载日志"
          className="flex items-center gap-1 px-2 py-1 bg-gray-100 dark:bg-gray-700 rounded">
          <Download className="h-3.5 w-3.5" /> 下载
        </button>
      </div>
      {kw && (
        <div className="text-xs text-gray-500 mb-1">匹配 {shownLines.length} 行</div>
      )}
      <pre className="flex-1 min-h-[300px] overflow-auto text-xs font-mono p-3 bg-gray-900 text-gray-100 rounded-lg whitespace-pre-wrap">
        {loading
          ? '加载中...'
          : (kw && shownLines.length === 0)
            ? '(无匹配行)'
            : shownLines.map((line, i) => (
                <div key={i}>{highlightLine(line, kw)}</div>
              ))}
      </pre>
    </div>
  )
}

// 交互式控制台面板（Portainer 风格）：选 shell + 用户 -> 连接 -> xterm 终端
function ExecPanel({ id, fullscreen, hostId }) {
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
        <Terminal key={sessionKey} containerId={id} cmd={effectiveCmd} user={user} fullscreen={fullscreen} hostId={hostId} />
      ) : (
        <div className="flex-1 min-h-[280px] flex items-center justify-center text-gray-400 text-sm bg-gray-900/50 rounded-lg">
          选择 shell 与用户后点击「连接」进入交互式终端
        </div>
      )}
    </div>
  )
}
