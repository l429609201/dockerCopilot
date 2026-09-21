import React, { useState, useEffect, useMemo, useRef } from 'react'
import { composeAPI } from '../api/client.js'

// Compose 文件编辑弹窗：读取 -> 编辑 -> 校验 -> 保存
export function ComposeEditor({ project, filename, onClose }) {
  const [content, setContent] = useState('')
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [msg, setMsg] = useState('')
  const [warnings, setWarnings] = useState([])
  const [validation, setValidation] = useState(null)
  const [activeLine, setActiveLine] = useState(1)
  const editorRef = useRef(null)
  const codeRef = useRef(null)
  const gutterRef = useRef(null)
  const lines = useMemo(() => content.split('\n'), [content])
  const errorLine = validation?.line || 0

  const updateActiveLine = () => {
    const value = editorRef.current?.value || ''
    const cursor = editorRef.current?.selectionStart || 0
    setActiveLine(value.slice(0, cursor).split('\n').length)
  }

  const syncScroll = (e) => {
    const { scrollTop, scrollLeft } = e.currentTarget
    if (codeRef.current) {
      codeRef.current.scrollTop = scrollTop
      codeRef.current.scrollLeft = scrollLeft
    }
    if (gutterRef.current) {
      gutterRef.current.scrollTop = scrollTop
    }
  }

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
        setValidation(d)
        setMsg(d.warnings?.length ? '语法正确，但有风险提示' : '校验通过')
      } else {
        setValidation(d || null)
        const location = d?.line ? `（第 ${d.line} 行${d.column ? `，第 ${d.column} 列` : ''}）` : ''
        setMsg('校验失败' + location + '：' + (d?.error || r.data?.msg || '未知'))
      }
    } catch (e) {
      const result = e.response?.data?.data || null
      setValidation(result)
      const location = result?.line ? `（第 ${result.line} 行${result.column ? `，第 ${result.column} 列` : ''}）` : ''
      setMsg('校验失败' + location + '：' + (result?.error || e.response?.data?.msg || e.message))
      setWarnings([])
    }
  }

  const save = async () => {
    setSaving(true); setMsg('')
    try {
      const r = await composeAPI.saveFile(project.id, filename, content)
      if (r.data?.code === 200) {
        const result = r.data.data || {}
        setValidation(result)
        setWarnings(result.warnings || [])
        setMsg('已保存')
      } else {
        const result = r.data?.data || null
        setValidation(result)
        const location = result?.line ? `（第 ${result.line} 行${result.column ? `，第 ${result.column} 列` : ''}）` : ''
        setMsg('保存失败' + location + '：' + (result?.error || r.data?.msg || '未知错误'))
      }
    } catch (e) {
      const result = e.response?.data?.data || null
      setValidation(result)
      setWarnings(result?.warnings || [])
      const location = result?.line ? `（第 ${result.line} 行${result.column ? `，第 ${result.column} 列` : ''}）` : ''
      setMsg('保存失败' + location + '：' + (result?.error || e.response?.data?.msg || e.message))
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
          <div className="flex-1 min-h-[300px] border border-gray-300 dark:border-gray-600 rounded-lg overflow-hidden bg-gray-50 dark:bg-gray-900">
            <div className="flex h-full min-h-[300px] max-h-[55vh] font-mono text-sm leading-6">
              <div ref={gutterRef} className="w-12 flex-shrink-0 overflow-auto border-r border-gray-200 dark:border-gray-700 bg-gray-100 dark:bg-gray-800 text-right text-gray-400 select-none [scrollbar-width:none] [&::-webkit-scrollbar]:hidden" aria-hidden="true">
                <div className="py-3 pr-3">
                  {lines.map((_, index) => (
                    <div key={index} className={`h-6 ${errorLine === index + 1 ? 'bg-red-100 dark:bg-red-900/40 text-red-600 dark:text-red-300 font-semibold' : activeLine === index + 1 ? 'bg-blue-100/70 dark:bg-blue-900/30 text-blue-600 dark:text-blue-300' : ''}`}>
                      {index + 1}
                    </div>
                  ))}
                </div>
              </div>
              <div className="relative flex-1 min-w-0 overflow-hidden">
                <pre ref={codeRef} className="absolute inset-0 m-0 overflow-auto p-3 pointer-events-none text-gray-800 dark:text-gray-100" aria-hidden="true">
                  {lines.map((line, index) => (
                    <div key={index} className={`h-6 whitespace-pre ${errorLine === index + 1 ? 'bg-red-100/80 dark:bg-red-900/40' : activeLine === index + 1 ? 'bg-blue-100/40 dark:bg-blue-900/20' : ''}`}>
                      {line || ' '}
                    </div>
                  ))}
                </pre>
                <textarea ref={editorRef} value={content}
                  onChange={(e) => { setContent(e.target.value); setValidation(null); setWarnings([]); updateActiveLine() }}
                  onClick={updateActiveLine} onKeyUp={updateActiveLine} onScroll={syncScroll}
                  className="absolute inset-0 w-full h-full resize-none overflow-auto p-3 bg-transparent text-transparent caret-gray-900 dark:caret-white outline-none border-0 whitespace-pre leading-6"
                  spellCheck={false} aria-label="Compose YAML 编辑器" />
              </div>
            </div>
          </div>
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
