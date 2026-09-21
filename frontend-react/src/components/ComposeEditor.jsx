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
  const errorLine = Number(validation?.line ?? validation?.Line) || 0

  useEffect(() => {
    // 校验失败后自动滚动到错误行，避免用户还要手动寻找报错位置。
    if (errorLine > 0 && editorRef.current) {
      const top = Math.max(0, (errorLine - 1) * 24)
      editorRef.current.scrollTop = top
      if (codeRef.current) codeRef.current.scrollTop = top
      if (gutterRef.current) gutterRef.current.scrollTop = top
    }
  }, [errorLine])

  const updateActiveLine = () => {
    const value = editorRef.current?.value || ''
    const cursor = editorRef.current?.selectionStart || 0
    setActiveLine(value.slice(0, cursor).split('\n').length)
  }

  const jumpToLine = (lineNumber) => {
    if (!editorRef.current || lineNumber < 1) return
    const target = lines.slice(0, lineNumber - 1).reduce((offset, line) => offset + line.length + 1, 0)
    editorRef.current.focus()
    editorRef.current.setSelectionRange(target, target)
    updateActiveLine()
    const top = Math.max(0, (lineNumber - 1) * 24)
    editorRef.current.scrollTop = top
    if (codeRef.current) codeRef.current.scrollTop = top
    if (gutterRef.current) gutterRef.current.scrollTop = top
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

  // 兼容不同网关/代理对响应 data 的包装方式，避免把 success 当成错误信息。
  const getValidationResult = (response) => {
    const root = response?.data
    const isValidation = (value) => value && typeof value === 'object' && (
      Object.prototype.hasOwnProperty.call(value, 'valid') ||
      Object.prototype.hasOwnProperty.call(value, 'Valid') ||
      Object.prototype.hasOwnProperty.call(value, 'error') ||
      Object.prototype.hasOwnProperty.call(value, 'Error') ||
      Object.prototype.hasOwnProperty.call(value, 'warnings') ||
      Object.prototype.hasOwnProperty.call(value, 'Warnings')
    )
    const queue = [root]
    for (let depth = 0; queue.length > 0 && depth < 3; depth += 1) {
      const currentLevel = [...queue]
      queue.length = 0
      for (const value of currentLevel) {
        if (isValidation(value)) return value
        if (value && typeof value === 'object') {
          Object.values(value).forEach((child) => {
            if (child && typeof child === 'object') queue.push(child)
          })
        }
      }
    }
    return null
  }

  const getValidationField = (result, field, fallbackField = field) => result?.[field] ?? result?.[fallbackField]

  const getResponseMessage = (response, fallback) => {
    const message = response?.data?.msg
    return message && message.toLowerCase() !== 'success' ? message : fallback
  }

  const formatValidationMessage = (result, fallback = '未知错误') => {
    const line = getValidationField(result, 'line', 'Line')
    const column = getValidationField(result, 'column', 'Column')
    const error = getValidationField(result, 'error', 'Error')
    const location = line ? `（第 ${line} 行${column ? `，第 ${column} 列` : ''}）` : ''
    return `${location}${error || fallback}`
  }

  const validate = async () => {
    setMsg(''); setWarnings([])
    try {
      const r = await composeAPI.validate(content)
      const result = getValidationResult(r)
      const valid = getValidationField(result, 'valid', 'Valid')
      const warnings = getValidationField(result, 'warnings', 'Warnings') || []
      setValidation(result)
      if (valid === true) {
        setWarnings(warnings)
        setMsg(warnings.length ? '语法正确，但有风险提示' : '校验通过')
      } else if (valid === false) {
        // 已返回明确的失败结果，不应误报为响应缺失，也不能默认视为通过。
        setWarnings(warnings)
        setMsg('校验失败：' + formatValidationMessage(result, getResponseMessage(r, '后端返回校验未通过，但未提供原因')))
      } else {
        // 缺少布尔型 valid 属于接口响应异常，与 YAML 内容错误分开提示。
        setValidation(null)
        setMsg('校验接口异常：' + getResponseMessage(r, '响应缺少有效的 valid 字段，无法判断校验结果'))
      }
    } catch (e) {
      const result = getValidationResult(e.response)
      setValidation(result)
      setMsg('校验失败：' + formatValidationMessage(result, getResponseMessage(e.response, e.message)))
      setWarnings([])
    }
  }

  const save = async () => {
    setSaving(true); setMsg('')
    try {
      const r = await composeAPI.saveFile(project.id, filename, content)
      const result = getValidationResult(r)
      if (r.data?.code === 200) {
        setValidation(result)
        setWarnings(getValidationField(result, 'warnings', 'Warnings') || r.data?.data?.warnings || [])
        setMsg('已保存')
      } else {
        setValidation(result)
        setWarnings(getValidationField(result, 'warnings', 'Warnings') || [])
        setMsg('保存失败：' + formatValidationMessage(result, getResponseMessage(r, '未知错误')))
      }
    } catch (e) {
      const result = getValidationResult(e.response)
      setValidation(result)
      setWarnings(getValidationField(result, 'warnings', 'Warnings') || [])
      setMsg('保存失败：' + formatValidationMessage(result, getResponseMessage(e.response, e.message)))
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
                    <div key={index} role="button" tabIndex={0} onClick={() => jumpToLine(index + 1)} onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') jumpToLine(index + 1) }} title={`跳转到第 ${index + 1} 行`} aria-label={`跳转到第 ${index + 1} 行`} className={`h-6 cursor-pointer ${errorLine === index + 1 ? 'bg-red-100 dark:bg-red-900/40 text-red-600 dark:text-red-300 font-semibold' : activeLine === index + 1 ? 'bg-blue-100/70 dark:bg-blue-900/30 text-blue-600 dark:text-blue-300' : 'hover:bg-gray-200 dark:hover:bg-gray-700'}`}>
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
