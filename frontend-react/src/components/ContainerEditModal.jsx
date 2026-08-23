import React, { useState, useEffect } from 'react'
import { X, Plus, Trash2, Save, AlertTriangle } from 'lucide-react'
import { containerAPI } from '../api/client.js'

// Tab 定义：常规 / 网络 / 挂载 / 环境变量 / 资源 / 标签&命令
const TABS = [
  { key: 'general', label: '常规' },
  { key: 'network', label: '网络' },
  { key: 'mounts', label: '挂载' },
  { key: 'env', label: '环境变量' },
  { key: 'resources', label: '资源' },
  { key: 'labels', label: '标签 & 命令' },
]

// 容器编辑弹窗：按 Tab 分区编辑端口/网络/挂载/环境/资源/标签命令（任务化重建）。
// 后端 EditSpec 支持全部字段，未提供字段保留原容器配置。
export function ContainerEditModal({ container, onClose, onSuccess }) {
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [tab, setTab] = useState('general')
  // 表单状态（覆盖 6 个 Tab 的所有可编辑字段）
  const [form, setForm] = useState({
    image: '',
    restartPolicy: 'unless-stopped',
    keepOld: false,
    env: [],        // [{key, value}]
    ports: [],      // [{host, container, proto}]
    binds: [],      // [{source, target, mode}]
    networkMode: 'bridge',
    labels: [],     // [{key, value}]
    cmd: '',        // 空格分隔的启动命令（提交时拆分）
    entrypoint: '', // 空格分隔的入口点
    memoryMB: 0,    // 内存限制(MB)，0=不限制
    cpus: 0,        // CPU 核数，0=不限制
  })

  const set = (k, v) => setForm((f) => ({ ...f, [k]: v }))

  // 加载容器 Inspect 数据回填表单
  useEffect(() => {
    (async () => {
      setLoading(true)
      try {
        const r = await containerAPI.inspectContainer(container.ID)
        const cfg = r.data?.data || {}
        const hc = cfg.HostConfig || {}
        const cc = cfg.Config || {}
        // 环境变量（过滤 PATH 等系统变量）
        const envs = (cc.Env || []).map((line) => {
          const idx = line.indexOf('=')
          return idx > 0 ? { key: line.slice(0, idx), value: line.slice(idx + 1) } : { key: line, value: '' }
        }).filter((e) => !e.key.startsWith('PATH') && !e.key.startsWith('HOSTNAME'))
        // 端口映射
        const ports = []
        for (const [containerPort, hostBindings] of Object.entries(hc.PortBindings || {})) {
          if (!hostBindings || hostBindings.length === 0) continue
          const [port, proto] = containerPort.split('/')
          ports.push({ host: hostBindings[0].HostPort || '', container: port, proto: proto || 'tcp' })
        }
        // 挂载（从 HostConfig.Binds 解析 "source:target:mode"）
        const binds = (hc.Binds || []).map((b) => {
          const segs = b.split(':')
          return { source: segs[0] || '', target: segs[1] || '', mode: segs[2] || 'rw' }
        })
        // 标签（过滤 Docker/Compose 内部标签，避免误改）
        const labels = Object.entries(cc.Labels || {})
          .filter(([k]) => !k.startsWith('com.docker.') && !k.startsWith('org.opencontainers.'))
          .map(([key, value]) => ({ key, value }))
        // 资源限制换算：字节→MB、NanoCPUs→核数
        const memoryMB = hc.Memory ? Math.round(hc.Memory / 1048576) : 0
        const cpus = hc.NanoCpus ? +(hc.NanoCpus / 1e9).toFixed(2) : 0
        setForm({
          image: cc.Image || '',
          restartPolicy: hc.RestartPolicy?.Name || 'unless-stopped',
          keepOld: false,
          env: envs,
          ports,
          binds,
          networkMode: hc.NetworkMode || 'bridge',
          labels,
          cmd: (cc.Cmd || []).join(' '),
          entrypoint: (cc.Entrypoint || []).join(' '),
          memoryMB,
          cpus,
        })
      } catch (e) {
        setError('加载容器配置失败：' + (e.response?.data?.msg || e.message))
      } finally {
        setLoading(false)
      }
    })()
  }, [container.ID])

  // 提交编辑（转为后端 EditSpec 格式）
  const submit = async () => {
    setSaving(true)
    setError('')
    try {
      // 端口："hostPort:containerPort/proto"
      const portBindings = form.ports.filter((p) => p.host && p.container)
        .map((p) => `${p.host}:${p.container}/${p.proto}`)
      // 环境变量："KEY=VALUE"
      const env = form.env.filter((e) => e.key).map((e) => `${e.key}=${e.value}`)
      // 挂载："source:target:mode"
      const binds = form.binds.filter((b) => b.source && b.target)
        .map((b) => `${b.source}:${b.target}:${b.mode || 'rw'}`)
      // 标签转对象
      const labels = {}
      form.labels.filter((l) => l.key).forEach((l) => { labels[l.key] = l.value })
      // 命令/入口点按空格拆分（简单分词，复杂命令建议用 exec）
      const cmd = form.cmd.trim() ? form.cmd.trim().split(/\s+/) : []
      const entrypoint = form.entrypoint.trim() ? form.entrypoint.trim().split(/\s+/) : []
      // 资源换算：MB→字节、核数→NanoCPUs
      const memory = form.memoryMB > 0 ? form.memoryMB * 1048576 : 0
      const nanoCpus = form.cpus > 0 ? Math.round(form.cpus * 1e9) : 0

      const spec = {
        image: form.image || undefined,
        restartPolicy: form.restartPolicy,
        keepOld: form.keepOld,
        env,
        portBindings,
        binds,
        networkMode: form.networkMode || undefined,
        labels,
        cmd,
        entrypoint,
        memory,
        memorySwap: memory > 0 ? memory : 0, // swap 与内存一致，避免只限内存导致 swap 无限
        nanoCpus,
        confirmWarnings: true,
      }
      await containerAPI.editContainer(container.ID, spec)
      alert('编辑任务已提交，容器将重建')
      onSuccess?.()
      onClose()
    } catch (e) {
      setError('提交失败：' + (e.response?.data?.msg || e.message))
    } finally {
      setSaving(false)
    }
  }

  // 通用列表项操作工厂：add/remove/update 一套
  const listOps = (key, emptyItem) => ({
    add: () => set(key, [...form[key], { ...emptyItem }]),
    remove: (idx) => set(key, form[key].filter((_, i) => i !== idx)),
    update: (idx, k, v) => {
      const updated = form[key].map((it, i) => (i === idx ? { ...it, [k]: v } : it))
      set(key, updated)
    },
  })
  const portOps = listOps('ports', { host: '', container: '', proto: 'tcp' })
  const envOps = listOps('env', { key: '', value: '' })
  const bindOps = listOps('binds', { source: '', target: '', mode: 'rw' })
  const labelOps = listOps('labels', { key: '', value: '' })

  if (loading) {
    return (
      <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
        <div className="bg-white dark:bg-gray-800 rounded-lg p-8 text-center">
          <div className="text-gray-600 dark:text-gray-400">加载配置中...</div>
        </div>
      </div>
    )
  }

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50 p-4 overflow-y-auto">
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow-xl w-full max-w-3xl my-8">
        {/* 头部 */}
        <div className="flex items-center justify-between p-4 border-b border-gray-200 dark:border-gray-700">
          <h3 className="text-lg font-semibold text-gray-900 dark:text-white">
            ✏️ 编辑容器 - {container.name || container.Names?.[0]?.replace(/^\//, '') || (container.ID || '').slice(0, 12)}
          </h3>
          <button onClick={onClose}
            className="p-2 text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-lg">
            <X className="h-4 w-4" />
          </button>
        </div>

        {/* Tab 导航条（横向滚动，容纳 6 个分区） */}
        <div className="flex gap-1 px-4 pt-3 border-b border-gray-200 dark:border-gray-700 overflow-x-auto">
          {TABS.map((t) => (
            <button key={t.key} onClick={() => setTab(t.key)}
              className={`px-3 py-2 text-sm whitespace-nowrap border-b-2 -mb-px transition-colors ${
                tab === t.key
                  ? 'border-primary-600 text-primary-600 font-medium'
                  : 'border-transparent text-gray-500 hover:text-gray-700 dark:hover:text-gray-300'}`}>
              {t.label}
            </button>
          ))}
        </div>

        {/* 表单区：按当前 Tab 渲染 */}
        <div className="p-4 space-y-4 max-h-[65vh] overflow-y-auto">
          {error && (
            <div className="p-3 bg-red-50 dark:bg-red-900/20 text-red-700 dark:text-red-300 rounded-lg">
              {error}
            </div>
          )}

          {/* ===== 常规 ===== */}
          {tab === 'general' && (
            <>
              <Field label="镜像">
                <input value={form.image} readOnly className="input bg-gray-50 dark:bg-gray-900 cursor-not-allowed" />
                <p className="text-xs text-gray-500 mt-1">💡 镜像修改请使用"更新"功能，此处仅供查看</p>
              </Field>
              <Field label="重启策略">
                <select value={form.restartPolicy} onChange={(e) => set('restartPolicy', e.target.value)} className="input">
                  <option value="no">不重启</option>
                  <option value="always">总是重启</option>
                  <option value="unless-stopped">除非手动停止</option>
                  <option value="on-failure">仅失败时重启</option>
                </select>
              </Field>
              <label className="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
                <input type="checkbox" checked={form.keepOld} onChange={(e) => set('keepOld', e.target.checked)} className="rounded" />
                保留旧容器（重建后不删除旧容器）
              </label>
            </>
          )}

          {/* ===== 网络 ===== */}
          {tab === 'network' && (
            <>
              <Field label="网络模式">
                <input value={form.networkMode} onChange={(e) => set('networkMode', e.target.value)}
                  className="input" placeholder="bridge / host / none / 自定义网络名" />
                <p className="text-xs text-gray-500 mt-1">常用：bridge（默认）、host（共享主机网络）、none（无网络），或填自定义网络名。</p>
              </Field>
              <Field label="端口映射">
                <KVList items={form.ports} ops={portOps} addLabel="添加端口"
                  render={(p, i) => (
                    <>
                      <input placeholder="主机端口" value={p.host} onChange={(e) => portOps.update(i, 'host', e.target.value)} className="input flex-1" />
                      <span className="text-gray-500">→</span>
                      <input placeholder="容器端口" value={p.container} onChange={(e) => portOps.update(i, 'container', e.target.value)} className="input flex-1" />
                      <select value={p.proto} onChange={(e) => portOps.update(i, 'proto', e.target.value)} className="input w-20">
                        <option value="tcp">TCP</option>
                        <option value="udp">UDP</option>
                      </select>
                    </>
                  )} />
                <p className="text-xs text-amber-600 dark:text-amber-400 mt-1">⚠️ host 网络模式下端口映射不生效。</p>
              </Field>
            </>
          )}

          {/* ===== 挂载 ===== */}
          {tab === 'mounts' && (
            <>
              <div className="flex items-start gap-2 p-3 bg-red-50 dark:bg-red-900/20 text-red-700 dark:text-red-300 text-sm rounded-lg">
                <AlertTriangle className="h-4 w-4 flex-shrink-0 mt-0.5" />
                <span>修改挂载会改变数据绑定关系。若挂载源路径变化，容器内数据可能<b>看起来"丢失"</b>（实为指向了新路径）。请确认宿主机路径正确后再保存。</span>
              </div>
              <Field label="卷 / 绑定挂载">
                <KVList items={form.binds} ops={bindOps} addLabel="添加挂载"
                  render={(b, i) => (
                    <>
                      <input placeholder="宿主机路径" value={b.source} onChange={(e) => bindOps.update(i, 'source', e.target.value)} className="input flex-1" />
                      <span className="text-gray-500">:</span>
                      <input placeholder="容器内路径" value={b.target} onChange={(e) => bindOps.update(i, 'target', e.target.value)} className="input flex-1" />
                      <select value={b.mode} onChange={(e) => bindOps.update(i, 'mode', e.target.value)} className="input w-20">
                        <option value="rw">读写</option>
                        <option value="ro">只读</option>
                      </select>
                    </>
                  )} />
              </Field>
            </>
          )}

          {/* ===== 环境变量 ===== */}
          {tab === 'env' && (
            <Field label="环境变量">
              <KVList items={form.env} ops={envOps} addLabel="添加变量"
                render={(e, i) => (
                  <>
                    <input placeholder="变量名" value={e.key} onChange={(ev) => envOps.update(i, 'key', ev.target.value)} className="input flex-1" />
                    <span className="text-gray-500">=</span>
                    <input placeholder="值" value={e.value} onChange={(ev) => envOps.update(i, 'value', ev.target.value)} className="input flex-1" />
                  </>
                )} />
            </Field>
          )}

          {/* ===== 资源 ===== */}
          {tab === 'resources' && (
            <>
              <Field label="内存限制（MB，0 = 不限制）">
                <input type="number" min={0} value={form.memoryMB}
                  onChange={(e) => set('memoryMB', Math.max(0, Number(e.target.value) || 0))} className="input" />
              </Field>
              <Field label="CPU 限额（核数，0 = 不限制）">
                <input type="number" min={0} step={0.1} value={form.cpus}
                  onChange={(e) => set('cpus', Math.max(0, Number(e.target.value) || 0))} className="input" />
                <p className="text-xs text-gray-500 mt-1">例如 1.5 表示最多使用 1.5 个 CPU 核心。</p>
              </Field>
            </>
          )}

          {/* ===== 标签 & 命令 ===== */}
          {tab === 'labels' && (
            <>
              <Field label="标签（Labels）">
                <KVList items={form.labels} ops={labelOps} addLabel="添加标签"
                  render={(l, i) => (
                    <>
                      <input placeholder="键" value={l.key} onChange={(e) => labelOps.update(i, 'key', e.target.value)} className="input flex-1" />
                      <span className="text-gray-500">=</span>
                      <input placeholder="值" value={l.value} onChange={(e) => labelOps.update(i, 'value', e.target.value)} className="input flex-1" />
                    </>
                  )} />
              </Field>
              <Field label="启动命令（Cmd）">
                <input value={form.cmd} onChange={(e) => set('cmd', e.target.value)}
                  className="input font-mono" placeholder="留空使用镜像默认，如：npm start" />
              </Field>
              <Field label="入口点（Entrypoint）">
                <input value={form.entrypoint} onChange={(e) => set('entrypoint', e.target.value)}
                  className="input font-mono" placeholder="留空使用镜像默认，如：/docker-entrypoint.sh" />
                <p className="text-xs text-amber-600 dark:text-amber-400 mt-1">⚠️ 命令/入口点填错可能导致容器无法启动，按空格分词。</p>
              </Field>
            </>
          )}

          <div className="p-3 bg-yellow-50 dark:bg-yellow-900/20 text-yellow-700 dark:text-yellow-300 text-sm rounded-lg">
            ⚠️ 保存将<b>重建容器</b>（停止→删除→按新配置创建→启动），数据卷不受影响。所有 Tab 的改动会一并提交。
          </div>
        </div>

        {/* 底部按钮 */}
        <div className="flex items-center justify-end gap-3 p-4 border-t border-gray-200 dark:border-gray-700">
          <button onClick={onClose} className="px-4 py-2 text-gray-600 hover:bg-gray-100 rounded-lg">取消</button>
          <button onClick={submit} disabled={saving}
            className="px-4 py-2 bg-primary-600 text-white rounded-lg hover:bg-primary-700 disabled:opacity-50 flex items-center gap-2">
            <Save className="h-4 w-4" />
            {saving ? '提交中...' : '保存并重建'}
          </button>
        </div>
      </div>
    </div>
  )
}

function Field({ label, children }) {
  return (
    <div>
      <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">{label}</label>
      {children}
    </div>
  )
}

// 通用增删列表：每行由 render 渲染字段，末尾带删除按钮，底部带添加按钮。
// items 当前列表；ops={add,remove,update}；render(item,index)=>行内字段。
function KVList({ items, ops, render, addLabel }) {
  return (
    <div className="space-y-2">
      {items.map((it, i) => (
        <div key={i} className="flex items-center gap-2">
          {render(it, i)}
          <button onClick={() => ops.remove(i)}
            className="p-2 text-red-600 hover:bg-red-50 dark:hover:bg-red-900/20 rounded-lg flex-shrink-0">
            <Trash2 className="h-4 w-4" />
          </button>
        </div>
      ))}
      <button onClick={ops.add}
        className="w-full py-2 border-2 border-dashed border-gray-300 dark:border-gray-600 rounded-lg text-gray-600 dark:text-gray-400 hover:bg-gray-50 dark:hover:bg-gray-700/50 flex items-center justify-center gap-2">
        <Plus className="h-4 w-4" /> {addLabel}
      </button>
    </div>
  )
}
