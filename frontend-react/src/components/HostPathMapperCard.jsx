import React, { useState, useEffect } from 'react'
import { hostPathAPI } from '../api/client.js'
import { AlertCircle, RefreshCw, Plus, Trash2, Save, FolderTree } from 'lucide-react'

/**
 * 宿主机路径映射配置卡片（嵌入「项目」页）
 * 支持两种模式：
 *  - auto：从 dockerCopilot 自身容器的挂载信息自动推导（只读预览）
 *  - custom：手动维护「容器内路径 → 宿主机路径」映射
 */
export function HostPathMapperCard() {
  const [enabled, setEnabled] = useState(false)
  const [mode, setMode] = useState('auto')
  const [mappings, setMappings] = useState([])
  const [autoInfo, setAutoInfo] = useState({ available: false, mappings: [], reason: '' })
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)
  const [saving, setSaving] = useState(false)
  const [saveMsg, setSaveMsg] = useState(null)

  // 加载映射配置（含自动推导预览）
  const loadConfig = async () => {
    try {
      setLoading(true)
      setError(null)
      const response = await hostPathAPI.getConfig()
      if (response.data.code === 200) {
        const data = response.data.data || {}
        setEnabled(!!data.enabled)
        setMode(data.mode || 'auto')
        setMappings(Array.isArray(data.mappings) ? data.mappings : [])
        setAutoInfo({
          available: !!data.autoAvailable,
          mappings: Array.isArray(data.autoMappings) ? data.autoMappings : [],
          reason: data.autoReason || '',
        })
      } else {
        setError(response.data.msg || '加载映射配置失败')
      }
    } catch (err) {
      console.error('Failed to load host path config:', err)
      setError('加载映射配置失败: ' + (err.message || '未知错误'))
    } finally {
      setLoading(false)
    }
  }

  // 保存配置
  const handleSave = async () => {
    try {
      setSaving(true)
      setSaveMsg(null)
      const payload = {
        enabled,
        mode,
        mappings: mappings
          .map(m => ({ containerPath: (m.containerPath || '').trim(), hostPath: (m.hostPath || '').trim() }))
          .filter(m => m.containerPath && m.hostPath),
      }
      const response = await hostPathAPI.saveConfig(payload)
      if (response.data.code === 200) {
        setSaveMsg({ type: 'success', text: response.data.msg || '配置已保存并生效' })
        await loadConfig()
      } else {
        setSaveMsg({ type: 'error', text: response.data.msg || '保存失败' })
      }
    } catch (err) {
      console.error('Failed to save host path config:', err)
      setSaveMsg({ type: 'error', text: '保存失败: ' + (err.message || '未知错误') })
    } finally {
      setSaving(false)
    }
  }

  const addMapping = () => setMappings([...mappings, { containerPath: '', hostPath: '' }])
  const removeMapping = (index) => setMappings(mappings.filter((_, i) => i !== index))
  const updateMapping = (index, field, value) => {
    const next = mappings.slice()
    next[index] = { ...next[index], [field]: value }
    setMappings(next)
  }

  useEffect(() => {
    loadConfig()
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <div className="bg-white dark:bg-gray-800 rounded-xl p-4 shadow-sm border border-gray-200 dark:border-gray-700 space-y-4">
      {/* 卡片标题 */}
      <div className="flex items-center gap-2">
        <FolderTree className="w-5 h-5 text-blue-500" />
        <h3 className="text-base font-semibold text-gray-800 dark:text-gray-100">宿主机路径映射</h3>
      </div>
      <p className="text-sm text-gray-500 dark:text-gray-400">
        用于把容器内路径转换为宿主机真实路径，供挂载配置引用。自动推导读取本容器挂载信息；不可用时可切换为自定义。
      </p>

      {loading ? (
        <div className="flex items-center gap-2 py-6 text-gray-500 dark:text-gray-400">
          <RefreshCw className="w-5 h-5 animate-spin" /> 加载中...
        </div>
      ) : error ? (
        <div className="flex flex-col items-start gap-3 py-4">
          <div className="flex items-center gap-2 text-red-600 dark:text-red-400">
            <AlertCircle className="w-5 h-5" /> {error}
          </div>
          <button onClick={loadConfig}
            className="px-3 py-1.5 text-sm bg-blue-500 text-white rounded-lg hover:bg-blue-600 transition-colors">
            重新加载
          </button>
        </div>
      ) : (
        <HostPathMapperBody
          enabled={enabled} setEnabled={setEnabled}
          mode={mode} setMode={setMode}
          mappings={mappings} autoInfo={autoInfo}
          addMapping={addMapping} removeMapping={removeMapping} updateMapping={updateMapping}
          saving={saving} saveMsg={saveMsg} handleSave={handleSave}
        />
      )}
    </div>
  )
}

// 配置表单主体：启用开关 + 模式选择 + 自动预览/自定义编辑 + 保存
function HostPathMapperBody({
  enabled, setEnabled, mode, setMode, mappings, autoInfo,
  addMapping, removeMapping, updateMapping, saving, saveMsg, handleSave,
}) {
  return (
    <div className="space-y-4">
      {/* 启用开关 */}
      <label className="flex items-center gap-3 cursor-pointer">
        <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)}
          className="w-4 h-4 accent-blue-500" />
        <span className="text-sm font-medium text-gray-700 dark:text-gray-300">启用宿主机路径映射</span>
      </label>

      {/* 模式选择 */}
      <div>
        <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">映射来源</label>
        <div className="flex flex-wrap gap-4">
          <label className="flex items-center gap-2 cursor-pointer">
            <input type="radio" name="hostpath-mode" value="auto"
              checked={mode === 'auto'} onChange={() => setMode('auto')} className="accent-blue-500" />
            <span className="text-sm text-gray-700 dark:text-gray-300">自动推导（读取本容器挂载）</span>
          </label>
          <label className="flex items-center gap-2 cursor-pointer">
            <input type="radio" name="hostpath-mode" value="custom"
              checked={mode === 'custom'} onChange={() => setMode('custom')} className="accent-blue-500" />
            <span className="text-sm text-gray-700 dark:text-gray-300">自定义映射</span>
          </label>
        </div>
      </div>

      {/* 自动模式：只读预览 */}
      {mode === 'auto' && (
        <div className="rounded-lg border border-gray-200 dark:border-gray-700 p-3">
          <p className="text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">自动推导的映射（只读预览）</p>
          {autoInfo.available && autoInfo.mappings.length > 0 ? (
            <div className="space-y-1">
              {autoInfo.mappings.map((m, i) => (
                <div key={i} className="text-sm text-gray-600 dark:text-gray-400 font-mono break-all">
                  {m.containerPath} <span className="text-gray-400">→</span> {m.hostPath}
                </div>
              ))}
            </div>
          ) : (
            <p className="text-sm text-amber-600 dark:text-amber-400">
              {autoInfo.reason || '暂时无法自动获取挂载信息，请改用自定义映射'}
            </p>
          )}
        </div>
      )}

      {/* 自定义模式：映射编辑 */}
      {mode === 'custom' && (
        <div className="space-y-3">
          <div className="flex items-center justify-between">
            <p className="text-sm font-medium text-gray-700 dark:text-gray-300">自定义映射规则</p>
            <button onClick={addMapping}
              className="flex items-center gap-1 px-3 py-1.5 text-sm bg-blue-500 text-white rounded-lg hover:bg-blue-600 transition-colors">
              <Plus className="w-4 h-4" /> 新增
            </button>
          </div>
          {mappings.length === 0 && (
            <p className="text-sm text-gray-500 dark:text-gray-400 py-1">暂无映射，点击「新增」添加。</p>
          )}
          {mappings.map((m, i) => (
            <div key={i} className="flex items-center gap-2">
              <input type="text" value={m.containerPath || ''}
                onChange={(e) => updateMapping(i, 'containerPath', e.target.value)}
                placeholder="容器内路径，如 /data"
                className="flex-1 px-3 py-2 bg-gray-50 dark:bg-gray-900 border border-gray-300 dark:border-gray-600 rounded-lg text-sm text-gray-900 dark:text-gray-100 focus:outline-none focus:ring-2 focus:ring-blue-500" />
              <span className="text-gray-400">→</span>
              <input type="text" value={m.hostPath || ''}
                onChange={(e) => updateMapping(i, 'hostPath', e.target.value)}
                placeholder="宿主机路径，如 /opt/app/data"
                className="flex-1 px-3 py-2 bg-gray-50 dark:bg-gray-900 border border-gray-300 dark:border-gray-600 rounded-lg text-sm text-gray-900 dark:text-gray-100 focus:outline-none focus:ring-2 focus:ring-blue-500" />
              <button onClick={() => removeMapping(i)}
                className="p-2 text-red-500 hover:bg-red-50 dark:hover:bg-red-900/20 rounded-lg transition-colors" title="删除">
                <Trash2 className="w-4 h-4" />
              </button>
            </div>
          ))}
        </div>
      )}

      {/* 保存栏 */}
      <div className="flex items-center gap-3 pt-1">
        <button onClick={handleSave} disabled={saving}
          className="flex items-center gap-2 px-4 py-2 bg-blue-500 text-white rounded-lg hover:bg-blue-600 disabled:opacity-60 transition-colors">
          {saving ? <RefreshCw className="w-4 h-4 animate-spin" /> : <Save className="w-4 h-4" />}
          保存配置
        </button>
        {saveMsg && (
          <span className={saveMsg.type === 'success'
            ? 'text-sm text-green-600 dark:text-green-400'
            : 'text-sm text-red-600 dark:text-red-400'}>
            {saveMsg.text}
          </span>
        )}
      </div>
    </div>
  )
}
