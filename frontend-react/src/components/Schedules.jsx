import React, { useState, useEffect, useCallback } from 'react'
import { Clock, Plus, Trash2, Play } from 'lucide-react'
import { scheduleAPI, registryAPI } from '../api/client.js'
import { RuleEditor } from './RuleEditor.jsx'
import { CronSetting } from './CronSetting.jsx'

// 定时更新与 Registry 凭据管理页面
export function Schedules() {
  const [rules, setRules] = useState([])
  const [registries, setRegistries] = useState([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [editing, setEditing] = useState(null) // 正在编辑的规则
  const [cron, setCron] = useState('') // 全局定时 cron

  const load = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const [r1, r2, r3] = await Promise.all([scheduleAPI.list(), registryAPI.list(), scheduleAPI.getCron()])
      setRules(r1.data?.data || [])
      setRegistries(r2.data?.data || [])
      setCron(r3.data?.data?.cron || '30 4 * * *')
    } catch (e) {
      setError('加载失败：' + (e.message || '未知错误'))
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { load() }, [load])

  const emptyRule = () => ({
    id: '', name: '', enabled: true,
    containerNames: [], onlyWhenUpdate: true, skipInvalidTag: true,
    registryId: '', keepOldContainer: false,
    notifyOnStart: false, notifyOnDone: true, notifyOnError: true,
  })

  const saveRule = async (rule) => {
    try {
      await scheduleAPI.save(rule)
      setEditing(null)
      await load()
    } catch (e) {
      alert('保存失败：' + (e.message || '未知错误'))
    }
  }

  const removeRule = async (id) => {
    if (!confirm('确认删除该定时规则？')) return
    try { await scheduleAPI.remove(id); await load() } catch (e) { alert('删除失败：' + e.message) }
  }

  const runRule = async (id) => {
    try { await scheduleAPI.runNow(id); alert('已触发执行') } catch (e) { alert('执行失败：' + e.message) }
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-bold text-gray-900 dark:text-white flex items-center gap-2">
          <Clock className="h-5 w-5" /> 定时更新
        </h2>
        <button onClick={() => setEditing(emptyRule())}
          className="flex items-center gap-1 px-3 py-2 bg-primary-600 text-white rounded-lg hover:bg-primary-700 text-sm">
          <Plus className="h-4 w-4" /> 新建规则
        </button>
      </div>

      {error && <div className="p-3 bg-red-50 text-red-600 rounded-lg text-sm">{error}</div>}
      {loading && <div className="text-gray-500 text-sm">加载中...</div>}

      {/* 全局定时设置：所有规则共用同一执行时间 */}
      <CronSetting cron={cron} onSaved={setCron} />

      <div className="grid gap-3">
        {rules.map((r) => (
          <div key={r.id} className="p-4 bg-white dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700 flex items-center justify-between">
            <div className="min-w-0">
              <div className="flex items-center gap-2">
                <span className="font-semibold text-gray-900 dark:text-white truncate">{r.name}</span>
                <span className={`text-xs px-2 py-0.5 rounded-full ${r.enabled ? 'bg-emerald-100 text-emerald-700' : 'bg-gray-100 text-gray-500'}`}>
                  {r.enabled ? '已启用' : '已禁用'}
                </span>
              </div>
              <div className="text-xs text-gray-500 mt-1">
                容器 {r.containerNames?.length || 0} 个 · 使用全局定时时间
                {r.registryId === 'auto' && ' · 凭证自适应'}
              </div>
              {/* 展示该规则选中的容器名 */}
              {r.containerNames?.length > 0 && (
                <div className="flex flex-wrap gap-1 mt-1.5">
                  {r.containerNames.slice(0, 8).map((n) => (
                    <span key={n} className="inline-flex items-center px-2 py-0.5 text-xs rounded-md bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-300">
                      {n}
                    </span>
                  ))}
                  {r.containerNames.length > 8 && (
                    <span className="inline-flex items-center px-2 py-0.5 text-xs rounded-md bg-gray-50 dark:bg-gray-800 text-gray-400">
                      +{r.containerNames.length - 8}
                    </span>
                  )}
                </div>
              )}
              {r.lastResult && <div className="text-xs text-gray-400 mt-1">上次：{r.lastResult}</div>}
            </div>
            <div className="flex items-center gap-2 flex-shrink-0">
              <button onClick={() => runRule(r.id)} title="立即执行" className="p-2 text-emerald-600 hover:bg-emerald-50 rounded-lg"><Play className="h-4 w-4" /></button>
              <button onClick={() => setEditing(r)} className="px-2 py-1 text-sm text-primary-600 hover:bg-primary-50 rounded-lg">编辑</button>
              <button onClick={() => removeRule(r.id)} title="删除" className="p-2 text-red-600 hover:bg-red-50 rounded-lg"><Trash2 className="h-4 w-4" /></button>
            </div>
          </div>
        ))}
        {!loading && rules.length === 0 && <div className="text-gray-400 text-sm">暂无定时规则</div>}
      </div>

      {editing && (
        <RuleEditor rule={editing} registries={registries}
          onCancel={() => setEditing(null)} onSave={saveRule} />
      )}
    </div>
  )
}
