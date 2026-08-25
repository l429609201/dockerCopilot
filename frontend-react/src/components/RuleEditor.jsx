import React, { useState, useEffect, useMemo } from 'react'
import { Search, Check, Clock, Server } from 'lucide-react'
import { containerAPI } from '../api/client.js'
import { useToast } from '../hooks/useToast.jsx'

// 按主机ID查主机名，找不到回退 ID
function hostNameOf(hostList, hostId) {
  const h = hostList.find((x) => x.id === hostId)
  return h ? h.name : hostId
}

// 任务类型选项：自动更新 / 自动清理镜像 / 自动备份
const TASK_TYPES = [
  { value: 'update', label: '自动更新', desc: '按策略更新选中容器的镜像' },
  { value: 'prune', label: '自动清理镜像', desc: '定时清理无tag或未使用的镜像' },
  { value: 'backup', label: '自动备份', desc: '定时备份所有容器配置为JSON' },
]

// 将标准五段式 cron 反解析为可视化模式（尽力而为），供编辑回填。
// 返回 { mode, hour, minute, everyHours }。无法识别时回退 advanced 模式。
function parseCronToVisual(cron) {
  const def = { mode: 'daily', hour: 4, minute: 30, everyHours: 6 }
  if (!cron) return def
  const parts = cron.trim().split(/\s+/)
  if (parts.length !== 5) return { ...def, mode: 'advanced' }
  const [m, h, dom, , dow] = parts
  // 每隔 N 小时：时为 */N，日、周为 *
  if (dom === '*' && dow === '*' && /^\*\/\d+$/.test(h)) {
    return { mode: 'interval', hour: 4, minute: Number(m) || 0, everyHours: Number(h.slice(2)) || 6 }
  }
  // 每天 HH:MM
  if (dom === '*' && dow === '*' && /^\d+$/.test(h) && /^\d+$/.test(m)) {
    return { mode: 'daily', hour: Number(h), minute: Number(m), everyHours: 6 }
  }
  return { ...def, mode: 'advanced' }
}

// 定时规则编辑弹窗
export function RuleEditor({ rule, registries, onCancel, onSave }) {
  const toast = useToast() // 卡片式消息提示，替代原生 alert
  // 由规则已有 cron 反解析出可视化初值
  const initVisual = parseCronToVisual(rule.cron)
  const [form, setForm] = useState({
    ...rule,
    // 任务类型，默认 update（兼容历史数据无 type 的情况）
    type: rule.type || 'update',
    // 镜像清理范围，默认 dangling
    pruneMode: rule.pruneMode || 'dangling',
    // 已选容器名集合（数组），从规则初始化（历史字段，仍用于兼容展示）
    containerNames: rule.containerNames || [],
    // 精确到「主机+容器名」的更新目标；历史规则无此字段时由 containerNames 兜底转换（视为本地）
    containerTargets: (rule.containerTargets && rule.containerTargets.length > 0)
      ? rule.containerTargets
      : (rule.containerNames || []).map((n) => ({ hostId: 'local', name: n })),
    // prune/backup 的目标主机列表；为空默认本地
    hostIds: rule.hostIds || [],
    // 该规则独立的 cron 定时（默认每天 04:30）
    cron: rule.cron || '30 4 * * *',
  })
  // cron 可视化编辑状态：模式 + 各输入项
  const [cronMode, setCronMode] = useState(initVisual.mode)
  const [cronHour, setCronHour] = useState(initVisual.hour)
  const [cronMinute, setCronMinute] = useState(initVisual.minute)
  const [cronEvery, setCronEvery] = useState(initVisual.everyHours)
  const [cronExpr, setCronExpr] = useState(rule.cron || '30 4 * * *')
  // 从后端拉取的可选容器名列表
  const [allContainers, setAllContainers] = useState([])
  const [loadErr, setLoadErr] = useState('')
  const [keyword, setKeyword] = useState('')

  const set = (key, val) => setForm((f) => ({ ...f, [key]: val }))

  // 拉取容器列表供勾选（保留主机信息，用于按主机分组）
  useEffect(() => {
    (async () => {
      try {
        const resp = await containerAPI.getContainers()
        const code = resp.data?.code
        if (code === 200 || code === 0) {
          const items = (resp.data?.data || [])
            .filter((c) => c.name)
            .map((c) => ({ name: c.name, hostId: c.hostId || 'local', hostName: c.hostName || '本地 Docker' }))
          setAllContainers(items)
        } else {
          setLoadErr(resp.data?.msg || '获取容器列表失败')
        }
      } catch (e) {
        setLoadErr('获取容器列表失败：' + (e.message || '未知错误'))
      }
    })()
  }, [])

  // 按关键词过滤并按主机分组：返回 [{hostId, hostName, items:[{name,...}]}]
  const groupedContainers = useMemo(() => {
    const kw = keyword.trim().toLowerCase()
    const list = kw ? allContainers.filter((c) => c.name.toLowerCase().includes(kw)) : allContainers
    const groups = new Map()
    for (const c of list) {
      if (!groups.has(c.hostId)) groups.set(c.hostId, { hostId: c.hostId, hostName: c.hostName, items: [] })
      groups.get(c.hostId).items.push(c)
    }
    // 每组内按容器名排序
    for (const g of groups.values()) g.items.sort((a, b) => a.name.localeCompare(b.name))
    return Array.from(groups.values())
  }, [allContainers, keyword])

  // 可选主机列表（供 prune/backup 主机多选）
  const hostList = useMemo(() => {
    const m = new Map()
    for (const c of allContainers) if (!m.has(c.hostId)) m.set(c.hostId, c.hostName)
    return Array.from(m, ([id, name]) => ({ id, name }))
  }, [allContainers])

  // 由可视化控件推导出最终 cron 表达式
  const buildCron = () => {
    if (cronMode === 'daily') return `${cronMinute} ${cronHour} * * *`
    if (cronMode === 'interval') return `${cronMinute} */${cronEvery} * * *`
    return cronExpr.trim()
  }
  // 可视化控件变化时，实时同步到 form.cron
  const cronPreview = buildCron()
  useEffect(() => {
    setForm((f) => ({ ...f, cron: cronPreview }))
  }, [cronPreview])

  // 判断某主机的某容器是否已选中
  const isTargetChecked = (hostId, name) =>
    form.containerTargets.some((t) => t.hostId === hostId && t.name === name)

  // 切换某个容器（主机+名）的选中状态
  const toggle = (hostId, name) => {
    setForm((f) => {
      const has = f.containerTargets.some((t) => t.hostId === hostId && t.name === name)
      return {
        ...f,
        containerTargets: has
          ? f.containerTargets.filter((t) => !(t.hostId === hostId && t.name === name))
          : [...f.containerTargets, { hostId, name }],
      }
    })
  }

  // 移除已选项
  const removeSelected = (hostId, name) =>
    setForm((f) => ({ ...f, containerTargets: f.containerTargets.filter((t) => !(t.hostId === hostId && t.name === name)) }))

  // 切换 prune/backup 的目标主机
  const toggleHost = (hostId) => {
    setForm((f) => {
      const has = f.hostIds.includes(hostId)
      return { ...f, hostIds: has ? f.hostIds.filter((h) => h !== hostId) : [...f.hostIds, hostId] }
    })
  }

  const submit = () => {
    if (!form.name) {
      toast.warning('规则名称必填')
      return
    }
    // 仅"自动更新"类型需要选择容器；清理/备份无需选容器
    if (form.type === 'update' && form.containerTargets.length === 0) {
      toast.warning('请至少选择一个容器')
      return
    }
    // 校验定时表达式非空（高级模式手输可能清空）
    const finalCron = buildCron()
    if (!finalCron) {
      toast.warning('请设置该规则的执行时间（cron 不能为空）')
      return
    }
    // 同步 containerNames（取所选目标的容器名去重）以兼容后端历史字段
    const names = Array.from(new Set(form.containerTargets.map((t) => t.name)))
    onSave({ ...form, cron: finalCron, containerNames: names })
  }

  return (
    <div className="fixed inset-0 bg-black/50 z-50 flex items-center justify-center p-4">
      <div className="bg-white dark:bg-gray-800 rounded-xl w-full max-w-lg max-h-[90vh] overflow-y-auto p-6 space-y-4">
        <h3 className="text-lg font-bold text-gray-900 dark:text-white">
          {rule.id ? '编辑规则' : '新建规则'}
        </h3>

        <Field label="规则名称">
          <input value={form.name} onChange={(e) => set('name', e.target.value)}
            className="input" placeholder="如：每日凌晨更新" />
        </Field>

        {/* 任务类型选择 */}
        <Field label="任务类型">
          <div className="grid grid-cols-3 gap-2">
            {TASK_TYPES.map((t) => (
              <button type="button" key={t.value} onClick={() => set('type', t.value)}
                className={`px-2 py-2 rounded-lg text-sm border text-center transition-colors ${
                  form.type === t.value
                    ? 'bg-primary-600 border-primary-600 text-white'
                    : 'border-gray-300 dark:border-gray-600 text-gray-700 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-700/50'}`}>
                {t.label}
              </button>
            ))}
          </div>
          <p className="text-xs text-gray-400 mt-1.5">
            {TASK_TYPES.find((t) => t.value === form.type)?.desc}
          </p>
        </Field>

        {/* 该规则独立的执行时间设置 */}
        <Field label="执行时间">
          <div className="rounded-lg border border-gray-200 dark:border-gray-700 p-3 space-y-3">
            {/* 模式切换 */}
            <div className="flex flex-wrap gap-2">
              <CronModeBtn active={cronMode === 'daily'} onClick={() => setCronMode('daily')} label="每天固定时间" />
              <CronModeBtn active={cronMode === 'interval'} onClick={() => setCronMode('interval')} label="每隔几小时" />
              <CronModeBtn active={cronMode === 'advanced'} onClick={() => setCronMode('advanced')} label="高级(cron)" />
            </div>

            {cronMode === 'daily' && (
              <div className="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
                <span>每天</span>
                <CronNum value={cronHour} min={0} max={23} onChange={setCronHour} />
                <span>时</span>
                <CronNum value={cronMinute} min={0} max={59} onChange={setCronMinute} />
                <span>分</span>
              </div>
            )}

            {cronMode === 'interval' && (
              <div className="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
                <span>每隔</span>
                <CronNum value={cronEvery} min={1} max={23} onChange={setCronEvery} />
                <span>小时（在第</span>
                <CronNum value={cronMinute} min={0} max={59} onChange={setCronMinute} />
                <span>分执行）</span>
              </div>
            )}

            {cronMode === 'advanced' && (
              <div>
                <input value={cronExpr} onChange={(e) => setCronExpr(e.target.value)}
                  className="input font-mono" placeholder="分 时 日 月 周，如 30 4 * * *" />
                <p className="text-xs text-gray-400 mt-1">五段式 cron：分 时 日 月 周。</p>
              </div>
            )}

            <div className="flex items-center gap-1.5 text-xs text-gray-500 font-mono pt-1 border-t border-gray-100 dark:border-gray-700/50">
              <Clock className="h-3.5 w-3.5" /> 当前表达式：{cronPreview || '（空）'}
            </div>
          </div>
        </Field>

        {/* 自动清理镜像：清理范围选择 */}
        {form.type === 'prune' && (
          <Field label="清理范围">
            <select value={form.pruneMode} onChange={(e) => set('pruneMode', e.target.value)} className="input">
              <option value="dangling">仅无tag悬空镜像（&lt;none&gt;）</option>
              <option value="unused">所有未使用镜像</option>
            </select>
            <p className="text-xs text-gray-400 mt-1.5">
              {form.pruneMode === 'unused' ? '将清理所有未被任何容器使用的镜像，请谨慎使用。' : '仅清理无标签的悬空镜像，较为安全。'}
            </p>
          </Field>
        )}

        {/* prune/backup 的目标主机多选（多 Docker 管理） */}
        {(form.type === 'prune' || form.type === 'backup') && hostList.length > 1 && (
          <Field label={`目标主机（已选 ${form.hostIds.length || 1} 个，不选默认仅本地）`}>
            <div className="border border-gray-200 dark:border-gray-700 rounded-lg divide-y divide-gray-100 dark:divide-gray-700">
              {hostList.map((h) => {
                const checked = form.hostIds.includes(h.id)
                return (
                  <button type="button" key={h.id} onClick={() => toggleHost(h.id)}
                    className="w-full flex items-center gap-2 px-3 py-2 text-sm text-left hover:bg-gray-50 dark:hover:bg-gray-700/50">
                    <span className={`flex items-center justify-center h-4 w-4 rounded border flex-shrink-0 ${
                      checked ? 'bg-primary-600 border-primary-600 text-white' : 'border-gray-300 dark:border-gray-600'}`}>
                      {checked && <Check className="h-3 w-3" />}
                    </span>
                    <Server className="h-3.5 w-3.5 text-gray-400" />
                    <span className="text-gray-800 dark:text-gray-200 truncate">{h.name}</span>
                  </button>
                )
              })}
            </div>
            <p className="text-xs text-gray-400 mt-1.5">勾选要执行的 Docker 主机；不勾选则默认仅本地主机。</p>
          </Field>
        )}

        {/* 自动备份：说明 */}
        {form.type === 'backup' && (
          <div className="p-3 rounded-lg bg-blue-50 dark:bg-blue-900/20 text-sm text-blue-700 dark:text-blue-300">
            💾 备份将导出所选主机<b>所有容器</b>的配置为 JSON 文件，保存到备份目录，无需选择容器。
          </div>
        )}

        {/* 自动更新：容器选择 + 拉取凭据 */}
        {form.type === 'update' && (
        <>
        <Field label={`选择容器（已选 ${form.containerTargets.length} 个）`}>
          {/* 已选标签：非本地带主机名后缀 */}
          {form.containerTargets.length > 0 && (
            <div className="flex flex-wrap gap-1.5 mb-2">
              {form.containerTargets.map((t) => (
                <span key={`${t.hostId}|${t.name}`} className="inline-flex items-center gap-1 pl-2 pr-1 py-0.5 text-xs rounded-full bg-primary-100 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300">
                  {t.name}{t.hostId && t.hostId !== 'local' ? `@${hostNameOf(hostList, t.hostId)}` : ''}
                  <button type="button" onClick={() => removeSelected(t.hostId, t.name)}
                    className="hover:text-red-500 leading-none text-sm">×</button>
                </span>
              ))}
            </div>
          )}

          {/* 搜索框 */}
          <div className="relative mb-2">
            <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-4 w-4 text-gray-400" />
            <input value={keyword} onChange={(e) => setKeyword(e.target.value)}
              className="input pl-8" placeholder="搜索容器名..." />
          </div>

          {loadErr && <div className="text-xs text-red-500 mb-2">{loadErr}</div>}

          {/* 可勾选列表：按主机分组 */}
          <div className="max-h-52 overflow-y-auto border border-gray-200 dark:border-gray-700 rounded-lg">
            {groupedContainers.length === 0 && (
              <div className="px-3 py-4 text-center text-xs text-gray-400">
                {allContainers.length === 0 ? '暂无容器或加载中...' : '无匹配容器'}
              </div>
            )}
            {groupedContainers.map((g) => (
              <div key={g.hostId}>
                {/* 主机分组标题 */}
                <div className="sticky top-0 px-3 py-1.5 bg-gray-100 dark:bg-gray-700/60 text-xs font-medium text-gray-600 dark:text-gray-300 flex items-center gap-1.5">
                  <Server className="h-3.5 w-3.5" /> {g.hostName}
                  <span className="text-gray-400">（{g.items.length}）</span>
                </div>
                <div className="divide-y divide-gray-100 dark:divide-gray-700">
                  {g.items.map((c) => {
                    const checked = isTargetChecked(g.hostId, c.name)
                    return (
                      <button type="button" key={`${g.hostId}|${c.name}`} onClick={() => toggle(g.hostId, c.name)}
                        className="w-full flex items-center gap-2 px-3 py-2 text-sm text-left hover:bg-gray-50 dark:hover:bg-gray-700/50">
                        <span className={`flex items-center justify-center h-4 w-4 rounded border flex-shrink-0 ${
                          checked ? 'bg-primary-600 border-primary-600 text-white' : 'border-gray-300 dark:border-gray-600'}`}>
                          {checked && <Check className="h-3 w-3" />}
                        </span>
                        <span className="text-gray-800 dark:text-gray-200 truncate">{c.name}</span>
                      </button>
                    )
                  })}
                </div>
              </div>
            ))}
          </div>
          <p className="text-xs text-gray-400 mt-1.5">容器按所属 Docker 主机分组，勾选精确到某主机的某容器。</p>
        </Field>

        <Field label="拉取凭据">
          <select value={form.registryId} onChange={(e) => set('registryId', e.target.value)} className="input">
            <option value="">匿名拉取</option>
            <option value="auto">自适应（按镜像自动匹配凭证）</option>
            {registries.map((r) => <option key={r.id} value={r.id}>{r.name}</option>)}
          </select>
          {form.registryId === 'auto' && (
            <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
              更新时按每个容器镜像所属仓库（如 ghcr.io、私有仓库）自动匹配已保存的凭证，匹配不到则匿名拉取。
            </p>
          )}
        </Field>
        </>
        )}

        {/* 更新专有策略开关 */}
        {form.type === 'update' && (
          <div className="grid grid-cols-2 gap-2">
            <CheckField label="仅有更新才更新" checked={form.onlyWhenUpdate} onChange={(v) => set('onlyWhenUpdate', v)} />
            <CheckField label="跳过无tag/digest镜像" checked={form.skipInvalidTag} onChange={(v) => set('skipInvalidTag', v)} />
            <CheckField label="保留旧容器" checked={form.keepOldContainer} onChange={(v) => set('keepOldContainer', v)} />
          </div>
        )}

        {/* 通用开关：启用 + 通知（所有类型共用） */}
        <div className="grid grid-cols-2 gap-2">
          <CheckField label="启用" checked={form.enabled} onChange={(v) => set('enabled', v)} />
          <CheckField label="开始通知" checked={form.notifyOnStart} onChange={(v) => set('notifyOnStart', v)} />
          <CheckField label="完成通知" checked={form.notifyOnDone} onChange={(v) => set('notifyOnDone', v)} />
          <CheckField label="失败通知" checked={form.notifyOnError} onChange={(v) => set('notifyOnError', v)} />
        </div>

        <div className="flex justify-end gap-2 pt-2">
          <button onClick={onCancel} className="px-4 py-2 text-gray-600 hover:bg-gray-100 rounded-lg">取消</button>
          <button onClick={submit} className="px-4 py-2 bg-primary-600 text-white rounded-lg hover:bg-primary-700">保存</button>
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

function CheckField({ label, checked, onChange }) {
  return (
    <label className="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300 cursor-pointer">
      <input type="checkbox" checked={!!checked} onChange={(e) => onChange(e.target.checked)}
        className="rounded border-gray-300" />
      {label}
    </label>
  )
}

// cron 模式切换按钮
function CronModeBtn({ active, onClick, label }) {
  return (
    <button type="button" onClick={onClick}
      className={`px-3 py-1.5 rounded-lg text-sm ${active
        ? 'bg-primary-600 text-white'
        : 'bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-300 hover:bg-gray-200 dark:hover:bg-gray-600'}`}>
      {label}
    </button>
  )
}

// cron 数值输入框（带范围钳制）
function CronNum({ value, min, max, onChange }) {
  return (
    <input type="number" value={value} min={min} max={max}
      onChange={(e) => {
        let v = Number(e.target.value)
        if (isNaN(v)) v = min
        onChange(Math.max(min, Math.min(max, v)))
      }}
      className="w-16 px-2 py-1 border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-900 text-center" />
  )
}
