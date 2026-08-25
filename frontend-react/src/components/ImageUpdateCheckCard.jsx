import React, { useState, useEffect, useCallback } from 'react'
import { BellOff, Save, Loader2, X, Plus, ChevronDown, Search } from 'lucide-react'
import { containerAPI } from '../api/client.js'

// 镜像更新检查卡片：第一行=更新检查周期(分钟)，第二行=屏蔽黑名单(标签式增删+从容器列表勾选)。
// 受控组件：值与变更回调由父级 Settings 统一管理，onSave 提交完整配置避免多卡片互相覆盖。
export function ImageUpdateCheckCard({ intervalMinutes, mutedContainers, onChangeInterval, onChangeMuted, onSave }) {
  const [saving, setSaving] = useState(false)
  const [msg, setMsg] = useState(null) // { ok, text }
  const [names, setNames] = useState([]) // 现有容器名列表，供勾选
  const [pickerOpen, setPickerOpen] = useState(false)
  const [searchText, setSearchText] = useState('') // 下拉菜单内的搜索关键词

  // 拉取现有容器名，供黑名单勾选
  const loadNames = useCallback(async () => {
    try {
      const r = await containerAPI.getContainers()
      const list = r.data?.data || r.data || []
      const ns = list.map((c) => c.name || (c.Names?.[0] || '').replace(/^\//, '')).filter(Boolean)
      setNames([...new Set(ns)].sort())
    } catch (e) {
      // 容器列表拉取失败不阻断，仍可手动输入
    }
  }, [])

  useEffect(() => { loadNames() }, [loadNames])

  const muted = mutedContainers || []
  const addMuted = (name) => {
    const n = (name || '').trim()
    if (!n || muted.includes(n)) return
    onChangeMuted([...muted, n])
  }
  const removeMuted = (name) => onChangeMuted(muted.filter((m) => m !== name))

  // 未被屏蔽的候选容器，根据搜索关键词过滤
  const candidates = names
    .filter((n) => !muted.includes(n))
    .filter((n) => searchText.trim() === '' || n.toLowerCase().includes(searchText.toLowerCase()))

  const save = async () => {
    setSaving(true); setMsg(null)
    try {
      // 提交前把周期规整为整数，<=0 交给后端回退默认 30
      await onSave({
        updateCheckIntervalMinutes: Number(intervalMinutes) || 0,
        mutedContainers: muted,
      })
      setMsg({ ok: true, text: '已保存并生效' })
    } catch (e) {
      setMsg({ ok: false, text: '保存失败：' + (e.response?.data?.msg || e.message || '未知错误') })
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="card space-y-4">
      {/* 卡片标题 */}
      <div className="flex items-center gap-2 text-gray-900 dark:text-white font-semibold">
        <BellOff className="h-4 w-4" /> 镜像更新检查
      </div>

      {/* 第一行：更新检查周期 */}
      <div>
        <label className="block text-sm font-medium mb-1 text-gray-700 dark:text-gray-300">更新检查周期(分钟)</label>
        <input type="number" min="0" value={intervalMinutes}
          onChange={(e) => onChangeInterval(Number(e.target.value))}
          className="input" placeholder="30" />
        <p className="text-xs text-gray-500 mt-1">内置更新检测推送周期，留空或 0 使用默认 30 分钟</p>
      </div>

      {/* 第二行：屏蔽黑名单 */}
      <div>
        <label className="block text-sm font-medium mb-1 text-gray-700 dark:text-gray-300">更新检查屏蔽黑名单</label>
        <p className="text-xs text-gray-500 mb-2">黑名单中的容器不会推送"有更新"通知</p>

        {/* 已选标签 */}
        <div className="flex flex-wrap gap-2 mb-2">
          {muted.length === 0 && <span className="text-xs text-gray-400">暂无屏蔽容器</span>}
          {muted.map((name) => (
            <span key={name}
              className="inline-flex items-center gap-1 px-2 py-1 rounded-lg text-xs bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-200">
              {name}
              <button onClick={() => removeMuted(name)} className="text-gray-400 hover:text-red-500" title="移除">
                <X className="h-3 w-3" />
              </button>
            </span>
          ))}
        </div>

        {/* 从容器列表勾选（输入框样式的下拉选择器） */}
        <div className="relative">
          <div
            onClick={() => setPickerOpen((v) => !v)}
            className="flex items-center gap-2 px-3 py-2 text-sm border border-gray-300 dark:border-gray-600 rounded-lg cursor-pointer hover:border-primary-500 dark:hover:border-primary-400 bg-white dark:bg-gray-800 transition-colors"
          >
            <Plus className="h-4 w-4 text-gray-400" />
            <span className="flex-1 text-gray-500 dark:text-gray-400">从容器列表添加...</span>
            <ChevronDown className="h-4 w-4 text-gray-400" />
          </div>
          {pickerOpen && (
            <>
              {/* 点击外部关闭 */}
              <div className="fixed inset-0 z-10" onClick={() => setPickerOpen(false)} />
              <div className="absolute z-20 mt-1 w-64 max-h-60 bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg shadow-lg">
                {/* 搜索框 */}
                <div className="sticky top-0 p-2 border-b border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800">
                  <div className="relative">
                    <Search className="absolute left-2 top-1/2 -translate-y-1/2 h-4 w-4 text-gray-400" />
                    <input
                      type="text"
                      value={searchText}
                      onChange={(e) => setSearchText(e.target.value)}
                      placeholder="搜索容器名称..."
                      className="w-full pl-8 pr-3 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-700 text-gray-900 dark:text-white placeholder-gray-400 focus:outline-none focus:ring-2 focus:ring-primary-500"
                      onClick={(e) => e.stopPropagation()}
                    />
                  </div>
                </div>
                {/* 容器列表 */}
                <div className="max-h-48 overflow-auto">
                  {candidates.length === 0 ? (
                    <div className="px-3 py-2 text-xs text-gray-400">
                      {searchText.trim() ? '没有匹配的容器' : '没有可添加的容器'}
                    </div>
                  ) : (
                    candidates.map((name) => (
                      <button key={name} type="button"
                        onClick={() => addMuted(name)}
                        className="flex items-center gap-2 w-full text-left px-3 py-2 text-sm hover:bg-gray-100 dark:hover:bg-gray-700">
                        <Plus className="h-3.5 w-3.5 text-gray-400 flex-shrink-0" />
                        <span className="truncate">{name}</span>
                      </button>
                    ))
                  )}
                </div>
              </div>
            </>
          )}
        </div>
      </div>

      {/* 保存 */}
      <div className="flex flex-wrap items-center gap-3 pt-2">
        <button onClick={save} disabled={saving}
          className="flex items-center gap-1 px-4 py-2 bg-primary-600 text-white rounded-lg hover:bg-primary-700 disabled:opacity-60">
          {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}
          {saving ? '保存中...' : '保存'}
        </button>
        {msg && (
          <span className={`text-sm ${msg.ok ? 'text-emerald-600 dark:text-emerald-400' : 'text-red-600 dark:text-red-400'}`}>
            {msg.text}
          </span>
        )}
      </div>
    </div>
  )
}
