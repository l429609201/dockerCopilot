import { useState, useEffect, useRef } from 'react'

// 通过 SSE 订阅所有运行中容器的资源监控数据。
// 返回 { statsMap, connected }，statsMap 以容器短ID为 key。
// 后端：GET /api/container/stats/stream（EventSource 用 query token 鉴权）。
export function useContainerStats(enabled = true) {
  const [statsMap, setStatsMap] = useState({})
  const [connected, setConnected] = useState(false)
  const esRef = useRef(null)

  useEffect(() => {
    if (!enabled) return

    const token = localStorage.getItem('docker_copilot_token') || ''
    // 复用 client.js 的 baseURL 选择逻辑：优先注入/存储，退回当前主机
    const base =
      (typeof window !== 'undefined' && window.__API_BASE_URL) ||
      localStorage.getItem('api_base_url') ||
      (typeof window !== 'undefined' ? `${window.location.protocol}//${window.location.host}` : '')

    const url = `${base}/api/container/stats/stream?token=${encodeURIComponent(token)}`
    const es = new EventSource(url)
    esRef.current = es

    es.onopen = () => setConnected(true)
    es.onmessage = (ev) => {
      try {
        const arr = JSON.parse(ev.data) // ContainerStat[]
        const map = {}
        for (const s of arr) {
          if (s && s.id) map[s.id] = s
        }
        setStatsMap(map)
      } catch (e) {
        console.error('解析容器监控数据失败:', e)
      }
    }
    es.onerror = () => {
      // EventSource 会自动重连，这里只更新连接状态
      setConnected(false)
    }

    return () => {
      es.close()
      esRef.current = null
      setConnected(false)
    }
  }, [enabled])

  return { statsMap, connected }
}
