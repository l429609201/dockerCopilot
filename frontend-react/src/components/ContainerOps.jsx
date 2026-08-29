import React, { useState, useCallback, useMemo } from 'react'
import { FileText, Terminal as TerminalIcon, RefreshCw, Maximize2, Minimize2, X, Search, Download, ArrowDownToLine } from 'lucide-react'
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
      fullscreen ? 'p-0' : 'p-2 sm:p-4')}>
      <div className={cn('bg-white dark:bg-gray-800 flex flex-col',
        fullscreen
          ? 'w-screen h-screen max-w-none max-h-none rounded-none'
          // 日志是等宽宽内容（HTTP 行含长 UA + 三列布局），max-w-3xl(768px) 过窄导致大量截断贴边，
          // 放宽到 max-w-6xl(1152px)，宽屏下有充足横向空间，窄屏仍由 w-full 自适应。
          : 'w-full max-w-6xl max-h-[90vh] rounded-xl')}>
        <div className="flex items-center justify-between p-3 sm:p-5 border-b border-gray-200 dark:border-gray-700">
          <h3 className="text-base sm:text-lg font-bold text-gray-900 dark:text-white truncate flex items-center gap-2">
            <FileText className="h-4 w-4 sm:h-5 sm:w-5 text-sky-600 dark:text-sky-400 flex-shrink-0" />
            <span className="truncate">日志 · {container.name}</span>
          </h3>
          <div className="flex items-center gap-1 flex-shrink-0">
            <button onClick={() => setFullscreen(v => !v)} title={fullscreen ? '还原' : '全屏'}
              className="p-1.5 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 rounded hover:bg-gray-100 dark:hover:bg-gray-700">
              {fullscreen ? <Minimize2 className="h-4 w-4 sm:h-4.5 sm:w-4.5" /> : <Maximize2 className="h-4 w-4 sm:h-4.5 sm:w-4.5" />}
            </button>
            <button onClick={onClose} title="关闭"
              className="p-1.5 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 rounded hover:bg-gray-100 dark:hover:bg-gray-700">
              <X className="h-4 w-4 sm:h-5 sm:w-5" />
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
      fullscreen ? 'p-0' : 'p-2 sm:p-4')}>
      <div className={cn('bg-white dark:bg-gray-800 flex flex-col',
        fullscreen
          ? 'w-screen h-screen max-w-none max-h-none rounded-none'
          // 终端输出同为等宽宽内容，与日志弹窗保持一致放宽到 max-w-6xl(1152px)。
          : 'w-full max-w-6xl max-h-[90vh] rounded-xl')}>
        <div className="flex items-center justify-between p-3 sm:p-5 border-b border-gray-200 dark:border-gray-700">
          <h3 className="text-base sm:text-lg font-bold text-gray-900 dark:text-white truncate flex items-center gap-2">
            <TerminalIcon className="h-4 w-4 sm:h-5 sm:w-5 text-teal-600 dark:text-teal-400 flex-shrink-0" />
            <span className="truncate">终端 · {container.name}</span>
          </h3>
          <div className="flex items-center gap-1 flex-shrink-0">
            <button onClick={() => setFullscreen(v => !v)} title={fullscreen ? '还原' : '全屏'}
              className="p-1.5 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 rounded hover:bg-gray-100 dark:hover:bg-gray-700">
              {fullscreen ? <Minimize2 className="h-4 w-4 sm:h-4.5 sm:w-4.5" /> : <Maximize2 className="h-4 w-4 sm:h-4.5 sm:w-4.5" />}
            </button>
            <button onClick={onClose} title="关闭"
              className="p-1.5 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 rounded hover:bg-gray-100 dark:hover:bg-gray-700">
              <X className="h-4 w-4 sm:h-5 sm:w-5" />
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

// Go 标准库 log 包的默认格式：2026/08/26 09:55:36 消息内容
// net/http 等内部组件绕过了 go-zero 的 logx，只能在这里单独识别
const STDLIB_LOG_RE = /^(\d{4}\/\d{2}\/\d{2} \d{2}:\d{2}:\d{2}(?:\.\d+)?)\s+([\s\S]*)$/

// Docker 容器 stdout 常见的 ISO 时间戳前缀：2026-08-26T09:55:36.123456789Z 消息内容
const ISO_PREFIX_RE = /^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:?\d{2})?)\s+([\s\S]*)$/

// 从非结构化文本里推断日志级别，让 error/warn 也能上色
function guessLevel(text) {
  const s = text.toLowerCase()
  if (/\b(panic|fatal)\b/.test(s)) return 'fatal'
  if (/\b(error|err|failed|failure|unauthorized|forbidden)\b/.test(s)) return 'error'
  if (/\b(warn|warning|unsolicited|superfluous|deprecated)\b/.test(s)) return 'warn'
  if (/\bdebug\b/.test(s)) return 'debug'
  return 'info'
}

// 百分号转义片段：连续的 %XX，用于识别被编码过的 URL 参数
const PCT_ENCODED_RE = /(?:%[0-9A-Fa-f]{2})+/g

// 解码日志中被百分号转义的片段。
// go-zero 记录的 authURL 经 url.Values.Encode() 处理后，
// scope 会变成 repository%3Alibrary%2Fpostgres%3Apull 之类，可读性很差。
// 仅替换 %XX 片段而非整串 decodeURIComponent，避免正文里的 % 触发异常；
// 单个片段解码失败时保留原文，不影响其余内容。
function decodePctEscapes(text) {
  if (!text || text.indexOf('%') === -1) return text
  return text.replace(PCT_ENCODED_RE, (seg) => {
    try {
      const decoded = decodeURIComponent(seg)
      // 解码出控制字符说明原文本就不是 URL 编码，保留原样
      return /[\u0000-\u001f\u007f]/.test(decoded) ? seg : decoded
    } catch {
      return seg
    }
  })
}

// 解析单行结构化日志（dockerCopilot 自身使用 go-zero 的 JSON 日志格式）
// 兼容 @timestamp/ts/time、content/msg/message 等常见字段名
// 非 JSON 行会再尝试标准库 log 与 ISO 前缀两种格式，都不匹配才返回 null 走原文展示
function parseLogLine(line) {
  const t = line.trim()
  if (!t) return null
  if (!t.startsWith('{') || !t.endsWith('}')) {
    const m = STDLIB_LOG_RE.exec(t) || ISO_PREFIX_RE.exec(t)
    if (!m) return null
    const content = m[2].trim()
    if (!content) return null
    // 级别判定用原文，避免解码后新增的关键词干扰推断
    return { time: m[1], caller: '', content: decodePctEscapes(content), level: guessLevel(content) }
  }
  try {
    const o = JSON.parse(t)
    if (!o || typeof o !== 'object') return null
    const time = o['@timestamp'] || o.ts || o.time || o.timestamp || ''
    const raw = o.content ?? o.msg ?? o.message ?? ''
    let content = typeof raw === 'string' ? raw : JSON.stringify(raw)
    if (!time && !o.caller && !content) return null
    let level = String(o.level || 'info').toLowerCase()
    // go-zero 无 Warn 级别，后端以 "warn:" 前缀标注，这里还原成 warn 并剥掉前缀
    const pm = /^(warn|warning)\s*[:：]\s*/i.exec(content)
    if (pm) {
      level = 'warn'
      content = content.slice(pm[0].length)
    }
    return {
      time: String(time),
      caller: o.caller ? String(o.caller) : '',
      content: decodePctEscapes(content),
      level,
    }
  } catch {
    return null
  }
}

// 时间只保留 时:分:秒.毫秒，完整时间戳通过 title 提示，避免占满横向空间
// 同时支持 ISO 的 T 分隔符与标准库 log 的空格分隔符
function formatLogTime(v) {
  const m = /[T ](\d{2}:\d{2}:\d{2})(\.\d+)?/.exec(v)
  if (m) return m[1] + (m[2] ? m[2].slice(0, 4) : '')
  return v
}

// 日志级别对应的文字颜色，让 error/warn 能从大段 info 里跳出来
const LEVEL_COLOR = {
  error: 'text-red-400',
  fatal: 'text-red-400',
  warn: 'text-amber-400',
  warning: 'text-amber-400',
  info: 'text-sky-400',
  debug: 'text-gray-500',
}

// 单行结构化日志：时间 | 位置 | 内容 三列对齐
// 正文默认单行截断避免超长 User-Agent 刷屏，点击整行可展开/收起完整内容
function StructuredLogRow({ obj, kw }) {
  const [expanded, setExpanded] = useState(false)
  return (
    <div
      onClick={() => setExpanded((v) => !v)}
      className="group flex items-baseline gap-x-4 px-4 py-1.5 border-l-2 border-transparent hover:border-sky-500/60 hover:bg-gray-800/50 cursor-pointer transition-colors"
      title={expanded ? '点击收起' : '点击展开完整内容'}
    >
      {/* 时间列：固定宽度 + 等宽数字，保证纵向对齐 */}
      <span className="shrink-0 w-[74px] text-gray-500 tabular-nums" title={obj.time}>
        {formatLogTime(obj.time)}
      </span>
      {/* 位置列：左对齐（尾部溢出才省略）+ 弱化配色，默认极淡、hover 提亮，避免抢占正文视线 */}
      <span
        className={cn(
          'shrink-0 w-44 truncate transition-colors',
          obj.caller ? 'text-gray-600 group-hover:text-violet-400' : 'text-gray-700'
        )}
        title={obj.caller || '无调用位置信息'}
      >
        {obj.caller ? highlightLine(obj.caller, kw) : '—'}
      </span>
      {/* 内容列：默认单行截断，展开后完整换行显示 */}
      <span
        className={cn(
          'flex-1 min-w-0',
          expanded ? 'whitespace-pre-wrap break-all' : 'truncate',
          LEVEL_COLOR[obj.level] || 'text-gray-100'
        )}
      >
        {highlightLine(obj.content, kw)}
      </span>
    </div>
  )
}

// 日志面板：支持行数/时间戳、关键词搜索过滤+高亮、日志下载
function LogsPanel({ id, name, hostId }) {
  const [logs, setLogs] = useState('')
  const [tail, setTail] = useState(200)
  const [timestamps, setTimestamps] = useState(false)
  const [loading, setLoading] = useState(false)
  const [search, setSearch] = useState('') // 搜索关键词
  const [pretty, setPretty] = useState(true) // 是否结构化展示（仅对 JSON 日志生效）
  const [autoScroll, setAutoScroll] = useState(true) // 自动滚动到最新一行，默认开启
  const scrollRef = React.useRef(null) // 日志滚动容器

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

  // 用户手动向上滚动时自动关闭跟随，滚回底部（20px 容差）时重新开启
  const onScroll = useCallback(() => {
    const el = scrollRef.current
    if (!el) return
    const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 20
    setAutoScroll(atBottom)
  }, [])

  // 按关键词过滤出需要展示的行（不区分大小写），空关键词时展示全部
  const shownLines = useMemo(() => {
    const lines = logs.split('\n')
    const kw = search.trim()
    if (!kw) return lines
    const lower = kw.toLowerCase()
    return lines.filter((l) => l.toLowerCase().includes(lower))
  }, [logs, search])

  // 逐行解析，并判断整体是否为结构化日志（过半行可解析才启用三列视图）
  const { rows, structured } = useMemo(() => {
    const parsedRows = shownLines.map((raw) => ({ raw, obj: parseLogLine(raw) }))
    const valid = parsedRows.filter((r) => r.raw.trim())
    const okCount = valid.filter((r) => r.obj).length
    return { rows: parsedRows, structured: valid.length > 0 && okCount / valid.length > 0.5 }
  }, [shownLines])

  // 日志内容变化后，若开启自动滚动则跟随到最新一行
  React.useEffect(() => {
    if (!autoScroll) return
    const el = scrollRef.current
    if (el) el.scrollTop = el.scrollHeight
  }, [rows, autoScroll, loading])

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
    // 补横向 + 底部内边距，与 header 的 p-3 sm:p-5 对齐，避免工具栏和深色日志区直接贴弹窗外框边缘。
    <div className="flex-1 flex flex-col min-h-0 px-3 sm:px-5 pb-3 sm:pb-5">
      <div className="flex flex-wrap items-center gap-3 mb-2 text-sm">
        <label className="flex items-center gap-1">行数
          <input type="number" value={tail} onChange={(e) => setTail(Number(e.target.value))}
            className="w-20 px-2 py-1 border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-900" />
        </label>
        <label className="flex items-center gap-1">
          <input type="checkbox" checked={timestamps} onChange={(e) => setTimestamps(e.target.checked)} /> 时间戳
        </label>
        {/* 结构化展示开关：仅当日志本身是 JSON 格式时才有意义 */}
        {structured && (
          <label className="flex items-center gap-1" title="将 JSON 日志解析为 时间 / 位置 / 内容 三列">
            <input type="checkbox" checked={pretty} onChange={(e) => setPretty(e.target.checked)} /> 解析
          </label>
        )}
        {/* 搜索框：实时过滤并高亮匹配行 */}
        <div className="relative flex-1 min-w-[160px]">
          <Search className="absolute left-2 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-gray-400" />
          <input value={search} onChange={(e) => setSearch(e.target.value)} placeholder="搜索日志…"
            className="w-full pl-7 pr-2 py-1 border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-900" />
        </div>
        {/* 自动滚动：开启后每次日志更新都跟随到最新一行 */}
        <button onClick={() => setAutoScroll(v => !v)}
          title={autoScroll ? '自动滚动已开启，点击关闭' : '自动滚动已关闭，点击开启'}
          className={cn('flex items-center gap-1 px-2 py-1 rounded',
            autoScroll
              ? 'bg-sky-100 text-sky-700 dark:bg-sky-900/50 dark:text-sky-300'
              : 'bg-gray-100 dark:bg-gray-700')}>
          <ArrowDownToLine className="h-3.5 w-3.5" /> 自动滚动
        </button>
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
      <div ref={scrollRef} onScroll={onScroll}
        className="flex-1 min-h-[300px] overflow-auto text-xs font-mono py-2 bg-gray-900 text-gray-100 rounded-lg leading-relaxed">
        {loading
          ? <div className="px-4 py-2">加载中...</div>
          : (kw && shownLines.length === 0)
            ? <div className="px-4 py-2">(无匹配行)</div>
            : rows.map(({ raw, obj }, i) => (
                (structured && pretty && obj) ? (
                  <StructuredLogRow key={i} obj={obj} kw={kw} />
                ) : (
                  <div key={i} className="whitespace-pre-wrap break-all px-4 py-1 hover:bg-gray-800/50">{highlightLine(raw, kw)}</div>
                )
              ))}
      </div>
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
    // 与 header 的 p-3 sm:p-5 对齐补内边距，避免配置栏和终端区贴弹窗外框边缘。
    <div className="flex-1 flex flex-col min-h-0 px-3 sm:px-5 pb-3 sm:pb-5">
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
