import { useState, useEffect, useRef } from 'react'
import { progressAPI } from '../api/client.js'

/**
 * 进度查询 Hook：轮询 /api/progress/:taskid，返回结构化进度。
 * 返回 { progress, isPolling }，progress 含 percentage/message/detailMsg/isDone/failed。
 */
export function useProgress(taskId, onComplete, onError) {
  const [progress, setProgress] = useState(null)
  const [isPolling, setIsPolling] = useState(false)
  const timerRef = useRef(null)
  const cbRef = useRef({ onComplete, onError })
  cbRef.current = { onComplete, onError }

  useEffect(() => {
    if (!taskId) return
    setIsPolling(true)

    const stop = () => {
      if (timerRef.current) { clearInterval(timerRef.current); timerRef.current = null }
      setIsPolling(false)
    }

    const poll = async () => {
      try {
        const resp = await progressAPI.getProgress(taskId)
        const body = resp.data
        const d = body?.data || {}
        setProgress({
          code: body?.code,
          percentage: d.percentage ?? 0,
          message: d.message || body?.msg || '',
          detailMsg: d.detailMsg || '',
          isDone: !!d.isDone,
          failed: !!d.failed,
          canceled: !!d.canceled,
          raw: d,
        })
        if (body?.code === 400) { stop(); cbRef.current.onError?.(body); return }
        if (d.isDone) {
          stop()
          if (d.failed) cbRef.current.onError?.(d)
          else cbRef.current.onComplete?.(d)
        }
      } catch (err) {
        console.error('查询进度失败:', err)
        stop(); cbRef.current.onError?.(err)
      }
    }

    poll()
    timerRef.current = setInterval(poll, 1500)
    return stop
  }, [taskId])

  return { progress, isPolling }
}