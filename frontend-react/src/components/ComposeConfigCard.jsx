import React, { useState, useEffect, useCallback } from 'react'
import { Settings2, Plus, Trash2, Save, FolderOpen, FolderSearch, SlidersHorizontal, Loader2 } from 'lucide-react'
import { composeAPI } from '../api/client.js'
import { DirectoryPicker } from './DirectoryPicker.jsx'

// Compose 目录配置卡片：可视化配置 compose 项目目录及扫描参数，保存后即时生效。
// onSaved 保存成功后回调（父组件据此刷新项目列表）。
export function ComposeConfigCard({ onSaved }) {
  const [paths, setPaths] = useState([]) // 配置目录列表
  const [maxDepth, setMaxDepth] = useState(3)
  const [maxFileSizeMB, setMaxFileSizeMB] = useState(10) // 以 MB 为单位展示
  const [cmdTimeout, setCmdTimeout] = useState(300)
  const [allowHighRisk, setAllowHighRisk] = useState(false)
  const [configured, setConfigured] = useState(false)
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [msg, setMsg] = useState(null) // { ok, text }
  const [pickerIndex, setPickerIndex] = useState(-1) // 正在用目录选择器的路径索引，-1 表示未打开

  // 加载当前生效配置
  const load = useCallback(async () => {
    setLoading(true)
    try {
      const r = await composeAPI.getConfig()
      if (r.data?.code === 200) {
        const d = r.data.data || {}
        setPaths(d.scanPaths?.length ? d.scanPaths : [''])
        setMaxDepth(d.maxDepth || 3)
        setMaxFileSizeMB(Math.round((d.maxFileSize || 10485760) / 1048576))
        setCmdTimeout(d.commandTimeoutSec || 300)
        setAllowHighRisk(!!d.allowHighRisk)
        setConfigured(!!d.configured)
      }
    } catch (e) {
      setMsg({ ok: false, text: '加载配置失败：' + (e.message || '未知错误') })
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { load() }, [load])

  const setPath = (i, v) => setPaths((arr) => arr.map((p, idx) => (idx === i ? v : p)))
  const addPath = () => setPaths((arr) => [...arr, ''])
  const removePath = (i) => setPaths((arr) => arr.filter((_, idx) => idx !== i))

  // 保存配置：过滤空路径，MB 转字节
  const save = async () => {
    const cleaned = paths.map((p) => p.trim()).filter(Boolean)
    setSaving(true); setMsg(null)
    try {
      const r = await composeAPI.saveConfig({
        scanPaths: cleaned,
        maxDepth: Number(maxDepth) || 3,
        maxFileSize: (Number(maxFileSizeMB) || 10) * 1048576,
        commandTimeoutSec: Number(cmdTimeout) || 300,
        allowHighRisk,
      })
      if (r.data?.code === 200) {
        setMsg({ ok: true, text: '配置已保存并生效' })
        setConfigured(true)
        onSaved?.()
      } else {
        setMsg({ ok: false, text: r.data?.msg || '保存失败' })
      }
    } catch (e) {
      setMsg({ ok: false, text: '保存失败：' + (e.response?.data?.msg || e.message || '未知错误') })
    } finally {
      setSaving(false)
    }
  }

  const hasValidPath = paths.some((p) => p.trim())

  return (
    <div className="card h-full flex flex-col">
      {/* 卡片标题 */}
      <div className="flex items-center gap-2 mb-3">
        <Settings2 className="h-5 w-5 text-primary-600" />
        <h3 className="font-semibold text-gray-900 dark:text-white">Compose 目录配置</h3>
      </div>

      {/* 未配置提示 */}
      {!configured && !hasValidPath && (
        <div className="mb-3 p-3 rounded-lg bg-amber-50 dark:bg-amber-900/20 text-sm text-amber-700 dark:text-amber-300">
          ⚠️ 尚未配置 Compose 目录。请在下方填写宿主机上已挂载到容器内的 compose 项目目录（绝对路径）。
        </div>
      )}

      {/* 配置目录列表 */}
      <label className="block text-sm font-medium mb-1.5 text-gray-700 dark:text-gray-300">
        配置目录（绝对路径，需已挂载进容器）
      </label>
      <div className="space-y-2">
        {paths.map((p, i) => (
          <div key={i} className="flex items-center gap-2">
            <div className="relative flex-1">
              <FolderOpen className="absolute left-2.5 top-1/2 -translate-y-1/2 h-4 w-4 text-gray-400" />
              <input value={p} onChange={(e) => setPath(i, e.target.value)}
                className="input pl-8" placeholder="/data/compose 或 /opt/stacks" />
            </div>
            {/* 浏览按钮：打开目录选择器可视化选择 */}
            <button onClick={() => setPickerIndex(i)}
              className="p-2 text-primary-600 hover:bg-primary-50 dark:hover:bg-primary-900/20 rounded-lg"
              title="浏览目录">
              <FolderSearch className="h-4 w-4" />
            </button>
            <button onClick={() => removePath(i)} disabled={paths.length <= 1}
              className="p-2 text-red-500 hover:bg-red-50 dark:hover:bg-red-900/20 rounded-lg disabled:opacity-40"
              title="移除">
              <Trash2 className="h-4 w-4" />
            </button>
          </div>
        ))}
      </div>

      {/* 目录选择器弹窗：选中后写回对应索引的路径 */}
      {pickerIndex >= 0 && (
        <DirectoryPicker
          initialPath={paths[pickerIndex] || ''}
          onSelect={(path) => setPath(pickerIndex, path)}
          onClose={() => setPickerIndex(-1)}
        />
      )}
      <button onClick={addPath}
        className="mt-2 flex items-center gap-1 text-sm text-primary-600 hover:text-primary-700">
        <Plus className="h-4 w-4" /> 添加目录
      </button>

      {/* 高级参数：常态化展示，不折叠 */}
      <div className="mt-4 flex items-center gap-1.5 text-sm font-medium text-gray-600 dark:text-gray-400">
        <SlidersHorizontal className="h-4 w-4" />
        高级参数
      </div>
      <div className="mt-3 grid grid-cols-1 sm:grid-cols-3 gap-3">
        <div>
          <label className="block text-xs font-medium mb-1 text-gray-600 dark:text-gray-400">扫描深度</label>
          <input type="number" min="1" value={maxDepth}
            onChange={(e) => setMaxDepth(e.target.value)} className="input" />
        </div>
        <div>
          <label className="block text-xs font-medium mb-1 text-gray-600 dark:text-gray-400">文件大小上限(MB)</label>
          <input type="number" min="1" value={maxFileSizeMB}
            onChange={(e) => setMaxFileSizeMB(e.target.value)} className="input" />
        </div>
        <div>
          <label className="block text-xs font-medium mb-1 text-gray-600 dark:text-gray-400">命令超时(秒)</label>
          <input type="number" min="1" value={cmdTimeout}
            onChange={(e) => setCmdTimeout(e.target.value)} className="input" />
        </div>
        <label className="sm:col-span-3 flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
          <input type="checkbox" checked={allowHighRisk}
            onChange={(e) => setAllowHighRisk(e.target.checked)} className="rounded" />
          允许部署高风险配置（privileged 等），启用后 up 操作不再拦截高风险警告
        </label>
      </div>

      {/* 操作区：mt-auto 顶到卡片底部，保证等高时保存按钮贴底对齐 */}
      <div className="flex flex-wrap items-center gap-3 pt-4 mt-auto border-t border-gray-100 dark:border-gray-700">
        <button onClick={save} disabled={saving || loading}
          className="flex items-center gap-1 px-4 py-2 bg-primary-600 text-white rounded-lg hover:bg-primary-700 disabled:opacity-60">
          {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}
          {saving ? '保存中...' : '保存配置'}
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
