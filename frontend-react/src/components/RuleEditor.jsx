import React, { useState, useEffect, useMemo } from 'react'
import { Search, Check } from 'lucide-react'
import { containerAPI } from '../api/client.js'

// 任务类型选项：自动更新 / 自动清理镜像 / 自动备份
const TASK_TYPES = [
  { value: 'update', label: '自动更新', desc: '按策略更新选中容器的镜像' },
  { value: 'prune', label: '自动清理镜像', desc: '定时清理无tag或未使用的镜像' },
  { value: 'backup', label: '自动备份', desc: '定时备份所有容器配置为JSON' },
]

// 定时规则编辑弹窗
export function RuleEditor({ rule, registries, onCancel, onSave }) {
  const [form, setForm] = useState({
    ...rule,
    // 任务类型，默认 update（兼容历史数据无 type 的情况）
    type: rule.type || 'update',
    // 镜像清理范围，默认 dangling
    pruneMode: rule.pruneMode || 'dangling',
    // 已选容器名集合（数组），从规则初始化
    containerNames: rule.containerNames || [],
  })
  // 从后端拉取的可选容器名列表
  const [allContainers, setAllContainers] = useState([])
  const [loadErr, setLoadErr] = useState('')
  const [keyword, setKeyword] = useState('')

  const set = (key, val) => setForm((f) => ({ ...f, [key]: val }))

  // 拉取容器列表供勾选
  useEffect(() => {
    (async () => {
      try {
        const resp = await containerAPI.getContainers()
        const code = resp.data?.code
        if (code === 200 || code === 0) {
          const names = (resp.data?.data || [])
            .map((c) => c.name)
            .filter(Boolean)
          setAllContainers(Array.from(new Set(names)).sort())
        } else {
          setLoadErr(resp.data?.msg || '获取容器列表失败')
        }
      } catch (e) {
        setLoadErr('获取容器列表失败：' + (e.message || '未知错误'))
      }
    })()
  }, [])

  // 按关键词过滤可选容器
  const filtered = useMemo(() => {
    const kw = keyword.trim().toLowerCase()
    if (!kw) return allContainers
    return allContainers.filter((n) => n.toLowerCase().includes(kw))
  }, [allContainers, keyword])

  // 切换某个容器的选中状态
  const toggle = (name) => {
    setForm((f) => {
      const has = f.containerNames.includes(name)
      return {
        ...f,
        containerNames: has
          ? f.containerNames.filter((n) => n !== name)
          : [...f.containerNames, name],
      }
    })
  }

  // 移除已选项
  const removeSelected = (name) =>
    setForm((f) => ({ ...f, containerNames: f.containerNames.filter((n) => n !== name) }))

  const submit = () => {
    if (!form.name) {
      alert('规则名称必填')
      return
    }
    // 仅"自动更新"类型需要选择容器；清理/备份无需选容器
    if (form.type === 'update' && form.containerNames.length === 0) {
      alert('请至少选择一个容器')
      return
    }
    onSave({ ...form })
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

        <div className="text-xs text-gray-400 -mt-1">执行时间由「全局定时设置」统一控制，到点依次执行所有启用的规则。</div>

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

        {/* 自动备份：说明 */}
        {form.type === 'backup' && (
          <div className="p-3 rounded-lg bg-blue-50 dark:bg-blue-900/20 text-sm text-blue-700 dark:text-blue-300">
            💾 备份将导出<b>所有容器</b>的配置为 JSON 文件，保存到备份目录，无需选择容器。
          </div>
        )}

        {/* 自动更新：容器选择 + 拉取凭据 */}
        {form.type === 'update' && (
        <>
        <Field label={`选择容器（已选 ${form.containerNames.length} 个）`}>
          {/* 已选标签 */}
          {form.containerNames.length > 0 && (
            <div className="flex flex-wrap gap-1.5 mb-2">
              {form.containerNames.map((n) => (
                <span key={n} className="inline-flex items-center gap-1 pl-2 pr-1 py-0.5 text-xs rounded-full bg-primary-100 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300">
                  {n}
                  <button type="button" onClick={() => removeSelected(n)}
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

          {/* 可勾选列表 */}
          <div className="max-h-44 overflow-y-auto border border-gray-200 dark:border-gray-700 rounded-lg divide-y divide-gray-100 dark:divide-gray-700">
            {filtered.length === 0 && (
              <div className="px-3 py-4 text-center text-xs text-gray-400">
                {allContainers.length === 0 ? '暂无容器或加载中...' : '无匹配容器'}
              </div>
            )}
            {filtered.map((n) => {
              const checked = form.containerNames.includes(n)
              return (
                <button type="button" key={n} onClick={() => toggle(n)}
                  className="w-full flex items-center gap-2 px-3 py-2 text-sm text-left hover:bg-gray-50 dark:hover:bg-gray-700/50">
                  <span className={`flex items-center justify-center h-4 w-4 rounded border flex-shrink-0 ${
                    checked ? 'bg-primary-600 border-primary-600 text-white' : 'border-gray-300 dark:border-gray-600'}`}>
                    {checked && <Check className="h-3 w-3" />}
                  </span>
                  <span className="text-gray-800 dark:text-gray-200 truncate">{n}</span>
                </button>
              )
            })}
          </div>
          <p className="text-xs text-gray-400 mt-1.5">仅可选择当前存在的容器；已删除的容器会在下次定时执行时自动从规则中移除。</p>
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
