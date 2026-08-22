import React, { useState } from 'react'

// 定时规则编辑弹窗
export function RuleEditor({ rule, registries, onCancel, onSave }) {
  const [form, setForm] = useState({
    ...rule,
    containerNamesText: (rule.containerNames || []).join(', '),
  })

  const set = (key, val) => setForm((f) => ({ ...f, [key]: val }))

  const submit = () => {
    const containerNames = form.containerNamesText
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean)
    if (!form.name || !form.cron) {
      alert('名称和 cron 表达式必填')
      return
    }
    const { containerNamesText, ...rest } = form
    onSave({ ...rest, containerNames })
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

        <Field label="Cron 表达式（分 时 日 月 周）">
          <input value={form.cron} onChange={(e) => set('cron', e.target.value)}
            className="input" placeholder="30 4 * * *" />
        </Field>

        <Field label="容器名（逗号分隔）">
          <input value={form.containerNamesText} onChange={(e) => set('containerNamesText', e.target.value)}
            className="input" placeholder="nginx, redis" />
        </Field>

        <Field label="拉取凭据">
          <select value={form.registryId} onChange={(e) => set('registryId', e.target.value)} className="input">
            <option value="">匿名拉取</option>
            {registries.map((r) => <option key={r.id} value={r.id}>{r.name}</option>)}
          </select>
        </Field>

        <div className="grid grid-cols-2 gap-2">
          <Check label="启用" checked={form.enabled} onChange={(v) => set('enabled', v)} />
          <Check label="仅有更新才更新" checked={form.onlyWhenUpdate} onChange={(v) => set('onlyWhenUpdate', v)} />
          <Check label="跳过无tag/digest镜像" checked={form.skipInvalidTag} onChange={(v) => set('skipInvalidTag', v)} />
          <Check label="保留旧容器" checked={form.keepOldContainer} onChange={(v) => set('keepOldContainer', v)} />
          <Check label="开始通知" checked={form.notifyOnStart} onChange={(v) => set('notifyOnStart', v)} />
          <Check label="完成通知" checked={form.notifyOnDone} onChange={(v) => set('notifyOnDone', v)} />
          <Check label="失败通知" checked={form.notifyOnError} onChange={(v) => set('notifyOnError', v)} />
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

function Check({ label, checked, onChange }) {
  return (
    <label className="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300 cursor-pointer">
      <input type="checkbox" checked={!!checked} onChange={(e) => onChange(e.target.checked)}
        className="rounded border-gray-300" />
      {label}
    </label>
  )
}
