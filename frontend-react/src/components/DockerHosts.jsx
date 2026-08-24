import React, { useState, useEffect, useCallback } from 'react'
import { dockerHostAPI } from '../api/client.js'
import { Server, Plus, Trash2, RefreshCw, Wifi, WifiOff, Loader2, Save, X, HardDrive, AlertCircle } from 'lucide-react'

// 多 Docker 管理页面：第一个恒为本地主机（不可删、地址固定），其余为远程 tcp:// 主机。
export function DockerHosts() {
  const [hosts, setHosts] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)
  const [editing, setEditing] = useState(null) // 正在编辑/新建的主机对象
  const [pingState, setPingState] = useState({}) // { [id]: 'testing'|'ok'|'fail' }

  const load = useCallback(async () => {
    try {
      setLoading(true)
      setError(null)
      const resp = await dockerHostAPI.list()
      if (resp.data.code === 200) {
        setHosts(Array.isArray(resp.data.data) ? resp.data.data : [])
      } else {
        setError(resp.data.msg || '加载主机列表失败')
      }
    } catch (err) {
      setError(err.message || '加载主机列表失败')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { load() }, [load])

  // 连通性测试
  const testPing = async (id) => {
    setPingState((s) => ({ ...s, [id]: 'testing' }))
    try {
      const resp = await dockerHostAPI.ping(id)
      const online = resp.data?.data?.online
      setPingState((s) => ({ ...s, [id]: online ? 'ok' : 'fail' }))
    } catch {
      setPingState((s) => ({ ...s, [id]: 'fail' }))
    }
  }

  const remove = async (host) => {
    if (!window.confirm(`确定删除远程主机「${host.name}」？该操作不影响远程主机本身。`)) return
    try {
      const resp = await dockerHostAPI.remove(host.id)
      if (resp.data.code === 200) {
        await load()
      } else {
        alert(resp.data.msg || '删除失败')
      }
    } catch (err) {
      alert(err.message || '删除失败')
    }
  }

  return (
    <div className="max-w-4xl mx-auto space-y-4">
      <HostsHeader onAdd={() => setEditing({ id: '', name: '', address: 'tcp://', enabled: true, note: '' })} onRefresh={load} />
      {error && (
        <div className="flex items-center gap-2 rounded-lg bg-red-50 dark:bg-red-900/20 px-4 py-3 text-sm text-red-600 dark:text-red-400">
          <AlertCircle className="h-4 w-4" /> {error}
        </div>
      )}
      {loading ? (
        <div className="flex items-center justify-center py-12 text-gray-400">
          <Loader2 className="h-5 w-5 animate-spin mr-2" /> 加载中...
        </div>
      ) : (
        <div className="space-y-3">
          {hosts.map((h) => (
            <HostRow key={h.id} host={h} pingState={pingState[h.id]}
              onEdit={() => setEditing({ ...h })} onDelete={() => remove(h)} onPing={() => testPing(h.id)} />
          ))}
        </div>
      )}
      {editing && (
        <HostEditModal host={editing} onClose={() => setEditing(null)}
          onSaved={async () => { setEditing(null); await load() }} />
      )}
    </div>
  )
}

// 页面标题栏
function HostsHeader({ onAdd, onRefresh }) {
  return (
    <div className="flex items-center justify-between">
      <div>
        <h2 className="text-xl font-semibold text-gray-900 dark:text-gray-100 flex items-center gap-2">
          <Server className="h-5 w-5" /> 多 Docker 管理
        </h2>
        <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
          管理本地与远程 Docker 主机，容器页将按来源聚合展示
        </p>
      </div>
      <div className="flex items-center gap-2">
        <button onClick={onRefresh}
          className="inline-flex items-center gap-1 rounded-lg border border-gray-200 dark:border-gray-700 px-3 py-2 text-sm text-gray-600 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-800">
          <RefreshCw className="h-4 w-4" /> 刷新
        </button>
        <button onClick={onAdd}
          className="inline-flex items-center gap-1 rounded-lg bg-blue-600 px-3 py-2 text-sm text-white hover:bg-blue-700">
          <Plus className="h-4 w-4" /> 添加主机
        </button>
      </div>
    </div>
  )
}

// 单个主机行
function HostRow({ host, pingState, onEdit, onDelete, onPing }) {
  const isLocal = host.local || host.type === 'local'
  const online = pingState ? pingState === 'ok' : host.online
  return (
    <div className="flex items-center justify-between rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 px-4 py-3">
      <div className="flex items-center gap-3 min-w-0">
        <div className={`flex h-9 w-9 items-center justify-center rounded-lg ${isLocal ? 'bg-blue-50 dark:bg-blue-900/30 text-blue-600' : 'bg-purple-50 dark:bg-purple-900/30 text-purple-600'}`}>
          {isLocal ? <HardDrive className="h-5 w-5" /> : <Server className="h-5 w-5" />}
        </div>
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <span className="font-medium text-gray-900 dark:text-gray-100 truncate">{host.name}</span>
            {isLocal && <span className="text-xs rounded bg-blue-100 dark:bg-blue-900/40 text-blue-600 px-1.5 py-0.5">本地</span>}
            {!host.enabled && <span className="text-xs rounded bg-gray-100 dark:bg-gray-700 text-gray-500 px-1.5 py-0.5">已禁用</span>}
          </div>
          <div className="text-xs text-gray-400 truncate font-mono">{host.address}</div>
        </div>
      </div>
      <div className="flex items-center gap-2 flex-shrink-0">
        <span className={`inline-flex items-center gap-1 text-xs ${online ? 'text-green-600' : 'text-gray-400'}`}>
          {pingState === 'testing' ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : online ? <Wifi className="h-3.5 w-3.5" /> : <WifiOff className="h-3.5 w-3.5" />}
          {pingState === 'testing' ? '测试中' : online ? '在线' : '离线'}
        </span>
        <button onClick={onPing} title="连通性测试"
          className="rounded-lg border border-gray-200 dark:border-gray-700 p-1.5 text-gray-500 hover:bg-gray-50 dark:hover:bg-gray-700">
          <RefreshCw className="h-4 w-4" />
        </button>
        <button onClick={onEdit}
          className="rounded-lg border border-gray-200 dark:border-gray-700 px-2.5 py-1.5 text-sm text-gray-600 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-700">
          编辑
        </button>
        {!isLocal && (
          <button onClick={onDelete} title="删除"
            className="rounded-lg border border-red-200 dark:border-red-900/40 p-1.5 text-red-500 hover:bg-red-50 dark:hover:bg-red-900/20">
            <Trash2 className="h-4 w-4" />
          </button>
        )}
      </div>
    </div>
  )
}

// 主机新建/编辑弹窗。本地主机仅可改名/备注，地址与类型锁定。
function HostEditModal({ host, onClose, onSaved }) {
  const isLocal = host.local || host.type === 'local' || host.id === 'local'
  const [name, setName] = useState(host.name || '')
  const [address, setAddress] = useState(host.address || 'tcp://')
  const [enabled, setEnabled] = useState(host.enabled !== false)
  const [note, setNote] = useState(host.note || '')
  // 自定义请求头：转成 [{key,value,dirty}] 行编辑。后端回显的 value 是脱敏串，
  // 保留原值时提交空串（dirty=false 表示未改动）。
  const [headers, setHeaders] = useState(() =>
    Object.entries(host.headers || {}).map(([key]) => ({ key, value: '', dirty: false, existed: true }))
  )
  const [saving, setSaving] = useState(false)
  const [msg, setMsg] = useState(null)

  const setHeaderField = (i, field, val) => {
    setHeaders((rows) => rows.map((r, idx) => idx === i ? { ...r, [field]: val, dirty: field === 'value' ? true : r.dirty } : r))
  }
  const addHeader = () => setHeaders((rows) => [...rows, { key: '', value: '', dirty: true, existed: false }])
  const removeHeader = (i) => setHeaders((rows) => rows.filter((_, idx) => idx !== i))

  const save = async () => {
    if (!name.trim()) { setMsg('名称不能为空'); return }
    if (!isLocal && !/^tcp:\/\/.+/.test(address.trim())) { setMsg('远程地址需形如 tcp://ip:2375'); return }
    // 组装 headers：未改动的已存在项提交空值（后端保留原值）；新增/改动项提交实际值
    const headerMap = {}
    for (const h of headers) {
      const k = h.key.trim()
      if (!k) continue
      headerMap[k] = h.dirty ? h.value : ''
    }
    setSaving(true); setMsg(null)
    try {
      const resp = await dockerHostAPI.save({ id: host.id || '', name: name.trim(), address: address.trim(), enabled, note, headers: headerMap })
      if (resp.data.code === 200) { await onSaved() }
      else { setMsg(resp.data.msg || '保存失败') }
    } catch (err) { setMsg(err.message || '保存失败') }
    finally { setSaving(false) }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4" onClick={onClose}>
      <div className="w-full max-w-md rounded-xl bg-white dark:bg-gray-800 p-5 shadow-xl" onClick={(e) => e.stopPropagation()}>
        <div className="flex items-center justify-between mb-4">
          <h3 className="text-lg font-semibold text-gray-900 dark:text-gray-100">{host.id ? '编辑主机' : '添加远程主机'}</h3>
          <button onClick={onClose} className="text-gray-400 hover:text-gray-600"><X className="h-5 w-5" /></button>
        </div>
        <div className="space-y-3">
          <div>
            <label className="block text-sm text-gray-600 dark:text-gray-300 mb-1">名称</label>
            <input value={name} onChange={(e) => setName(e.target.value)} placeholder="如：家里的 NAS"
              className="w-full rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900 px-3 py-2 text-sm" />
          </div>
          <div>
            <label className="block text-sm text-gray-600 dark:text-gray-300 mb-1">连接地址</label>
            <input value={address} onChange={(e) => setAddress(e.target.value)} disabled={isLocal}
              placeholder="tcp://192.168.1.10:2375"
              className="w-full rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900 px-3 py-2 text-sm font-mono disabled:opacity-60" />
            {isLocal
              ? <p className="text-xs text-gray-400 mt-1">本地主机固定使用 unix socket，地址不可修改</p>
              : <p className="text-xs text-amber-500 mt-1">⚠ tcp 为明文无认证连接，请仅在可信内网使用</p>}
          </div>
          {!isLocal && (
            <label className="flex items-center gap-2 text-sm text-gray-600 dark:text-gray-300">
              <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} /> 启用该主机
            </label>
          )}
          {/* 自定义请求头：仅远程主机，用于经反向代理/网关鉴权的场景 */}
          {!isLocal && (
            <div>
              <div className="flex items-center justify-between mb-1">
                <label className="text-sm text-gray-600 dark:text-gray-300">自定义请求头</label>
                <button type="button" onClick={addHeader}
                  className="inline-flex items-center gap-1 text-xs text-blue-600 hover:text-blue-700">
                  <Plus className="h-3 w-3" /> 添加
                </button>
              </div>
              {headers.length === 0 && (
                <p className="text-xs text-gray-400">用于经反向代理/网关鉴权的 Docker API（如 Authorization、X-Api-Key）</p>
              )}
              <div className="space-y-2">
                {headers.map((h, i) => (
                  <div key={i} className="flex items-center gap-2">
                    <input value={h.key} onChange={(e) => setHeaderField(i, 'key', e.target.value)}
                      placeholder="Header 名，如 Authorization"
                      className="flex-1 rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900 px-2 py-1.5 text-sm font-mono" />
                    <input value={h.value} onChange={(e) => setHeaderField(i, 'value', e.target.value)}
                      placeholder={h.existed && !h.dirty ? '（已设置，留空不改）' : 'Header 值'}
                      className="flex-1 rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900 px-2 py-1.5 text-sm font-mono" />
                    <button type="button" onClick={() => removeHeader(i)}
                      className="rounded-lg border border-red-200 dark:border-red-900/40 p-1.5 text-red-500 hover:bg-red-50 dark:hover:bg-red-900/20">
                      <Trash2 className="h-3.5 w-3.5" />
                    </button>
                  </div>
                ))}
              </div>
            </div>
          )}
          <div>
            <label className="block text-sm text-gray-600 dark:text-gray-300 mb-1">备注</label>
            <input value={note} onChange={(e) => setNote(e.target.value)}
              className="w-full rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900 px-3 py-2 text-sm" />
          </div>
          {msg && <p className="text-sm text-red-500">{msg}</p>}
        </div>
        <div className="flex justify-end gap-2 mt-5">
          <button onClick={onClose} className="rounded-lg border border-gray-200 dark:border-gray-700 px-4 py-2 text-sm text-gray-600 dark:text-gray-300">取消</button>
          <button onClick={save} disabled={saving}
            className="inline-flex items-center gap-1 rounded-lg bg-blue-600 px-4 py-2 text-sm text-white hover:bg-blue-700 disabled:opacity-60">
            {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />} 保存
          </button>
        </div>
      </div>
    </div>
  )
}
