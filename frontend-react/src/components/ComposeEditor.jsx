import React, { useState, useEffect } from 'react'
import { composeAPI } from '../api/client.js'

// Compose 文件编辑弹窗：读取 -> 编辑 -> 校验 -> 保存
export function ComposeEditor({ project, filename, onClose }) {
  const [content, setContent] = useState('')
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [msg, setMsg] = useState('')
  const [warnings, setWarnings] = useState([])

  useEffect(() => {
    (async () => {
      setLoading(true)
      try {
        const r = await composeAPI.readFile(project.id, filename)
        if (r.data?.code === 200) setContent(r.data.data?.content || '')
        else setMsg(r.data?.msg || '读取失败')
      } catch (e) {
        setMsg('读取失败：' + e.message)
      } finally { setLoading(false) }
    })()
  }, [project.id, filename])

  const validate = async () => {
    setMsg(''); setWarnings([])
    try {
      const r = await composeAPI.validate(content)
      const d = r.data?.data
      if (d?.valid) {
        setWarnings(d.warnings || [])
        setMsg(d.warnings?.length ? '语法正确，但有风险提示' : '校验通过')
      } else {
        setMsg('校验失败：' + (d?.error || '未知'))
      }
    } catch (e) {
      setMsg('校验失败：' + e.message)
    }
  }

  const save = async () => {
    setSaving(true); setMsg('')
    try {
      const r = await composeAPI.saveFile(project.id, filename, content)
      if (r.data?.code === 200) {
        setWarnings(r.data.data?.warnings || [])
        setMsg('已保存')
      } else {
        setMsg('保存失败：' + (r.data?.msg || '未知错误'))
      }
    } catch (e) {
      setMsg('保存失败：' + e.message)
    } finally { setSaving(false) }
  }

  return (
    <div className="fixed inset-0 bg-black/50 z-50 flex items-center justify-center p-4">
      <div className="bg-white dark:bg-gray-800 rounded-xl w-full max-w-3xl max-h-[90vh] flex flex-col p-5">
        <div className="flex items-center justify-between mb-3">
          <h3 className="text-lg font-bold text-gray-900 dark:text-white truncate">
            {project.name} / {filename}
          </h3>
          <button onClick={onClose} className="text-gray-400 hover:text-gray-600 text-xl leading-none">×</button>
        </div>

        {loading ? (
          <div className="text-gray-500 text-sm py-8 text-center">加载中...</div>
        ) : (
          <textarea value={content} onChange={(e) => setContent(e.target.value)}
            className="flex-1 min-h-[300px] font-mono text-sm p-3 border border-gray-300 dark:border-gray-600 rounded-lg bg-gray-50 dark:bg-gray-900 text-gray-900 dark:text-gray-100 resize-none"
            spellCheck={false} />
        )}

        {warnings.length > 0 && (
          <div className="mt-2 p-2 bg-amber-50 text-amber-700 rounded-lg text-xs space-y-0.5">
            {warnings.map((w, i) => <div key={i}>⚠ {w}</div>)}
          </div>
        )}

        <div className="flex items-center justify-between mt-3">
          <span className="text-sm text-gray-500">{msg}</span>
          <div className="flex gap-2">
            <button onClick={validate} className="px-4 py-2 bg-gray-100 dark:bg-gray-700 rounded-lg text-sm hover:bg-gray-200">校验</button>
            <button onClick={save} disabled={saving || loading}
              className="px-4 py-2 bg-primary-600 text-white rounded-lg text-sm hover:bg-primary-700 disabled:opacity-60">
              {saving ? '保存中...' : '保存'}
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
