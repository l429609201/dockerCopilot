import { useState, useEffect } from 'react'
import { faviconAPI, imageAPI } from '../api/client.js'

// favicon 结果本地缓存（key: 容器id，value: {url, ts}）
const CACHE_KEY = 'dc_favicon_cache'
const TTL = 7 * 24 * 60 * 60 * 1000 // 7天

function readCache() {
  try { return JSON.parse(localStorage.getItem(CACHE_KEY) || '{}') } catch { return {} }
}
function writeCache(cache) {
  try { localStorage.setItem(CACHE_KEY, JSON.stringify(cache)) } catch { /* 忽略配额错误 */ }
}

// useFavicon：根据容器暴露端口，探测其站点 favicon。
// 优先读缓存；未命中时用当前访问 host + 端口，调后端代理抓取。
// 抓取成功后自动持久化到服务器 /data/images/ 目录。
// 返回 { faviconUrl, loading }。仅对运行中且有端口的容器尝试。
export function useFavicon(container) {
  const [faviconUrl, setFaviconUrl] = useState('')
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    if (!container?.id) return
    const running = (container.status || '').toLowerCase() === 'running'
    // host 网络模式无端口映射，改用容器暴露端口（即宿主机端口）
    const ports = pickProbePorts(container)
    if (!running || ports.length === 0) return

    // 读缓存
    const cache = readCache()
    const hit = cache[container.id]
    if (hit && hit.url && Date.now() - hit.ts < TTL) {
      setFaviconUrl(hit.url)
      return
    }

    let cancelled = false
    const host = window.location.hostname || 'localhost'

    const tryPorts = async () => {
      setLoading(true)
      // 优先尝试常见 Web 端口，其次遍历全部
      const ordered = [...ports].sort((a, b) => scorePort(b) - scorePort(a))
      for (const port of ordered) {
        if (cancelled) return
        try {
          const r = await faviconAPI.resolve(`http://${host}:${port}`)
          const url = r.data?.data?.url
          if (url) {
            if (cancelled) return
            setFaviconUrl(url)
            const next = readCache()
            next[container.id] = { url, ts: Date.now() }
            writeCache(next)
            setLoading(false)

            // 成功抓取后，自动持久化到服务器 /data/images/ 目录
            // 后台异步执行，不阻塞 UI
            persistIconToServer(container, url, `http://${host}:${port}`)

            return
          }
        } catch { /* 忽略单端口失败，继续下一个 */ }
      }
      if (!cancelled) setLoading(false)
    }
    tryPorts()
    return () => { cancelled = true }
  }, [container?.id, container?.status, (container?.ports || []).join(',')])

  return { faviconUrl, loading }
}

// pickProbePorts 选择用于探测 favicon 的端口列表。
// 优先用宿主机映射端口（ports）；若为空且是 host 网络模式，则用容器暴露端口（exposedPorts）。
function pickProbePorts(container) {
  const mapped = container?.ports || []
  if (mapped.length > 0) return mapped
  const isHost = (container?.networkMode || '') === 'host'
  if (isHost) return container?.exposedPorts || []
  return []
}

// scorePort 给常见 Web 端口更高优先级，减少无效探测
function scorePort(port) {
  if (port === 80 || port === 443) return 100
  if (port === 8080 || port === 8443 || port === 3000 || port === 5000) return 80
  if (port >= 8000 && port <= 9999) return 60
  return 10
}

// useFaviconMap：对一批容器批量解析 favicon，返回 { [containerId]: iconUrl }。
// 供列表渲染时按容器 id 取图标，避免在 .map 循环里调用 hook。
export function useFaviconMap(containers) {
  const [map, setMap] = useState({})

  useEffect(() => {
    const list = (containers || []).filter(
      (c) => (c.status || '').toLowerCase() === 'running' && pickProbePorts(c).length > 0
    )
    if (list.length === 0) return
    let cancelled = false
    const host = window.location.hostname || 'localhost'

    const run = async () => {
      const cache = readCache()
      const result = {}
      let cacheDirty = false
      for (const c of list) {
        if (cancelled) return
        const hit = cache[c.id]
        if (hit && hit.url && Date.now() - hit.ts < TTL) {
          result[c.id] = hit.url
          continue
        }
        const ordered = [...pickProbePorts(c)].sort((a, b) => scorePort(b) - scorePort(a))
        for (const port of ordered) {
          if (cancelled) return
          try {
            const r = await faviconAPI.resolve(`http://${host}:${port}`)
            const url = r.data?.data?.url
            if (url) {
              result[c.id] = url
              cache[c.id] = { url, ts: Date.now() }
              cacheDirty = true

              // 批量抓取时也自动持久化到服务器
              persistIconToServer(c, url, `http://${host}:${port}`)

              break
            }
          } catch { /* 忽略，继续 */ }
        }
      }
      if (cancelled) return
      if (cacheDirty) writeCache(cache)
      setMap((prev) => ({ ...prev, ...result }))
    }
    run()
    return () => { cancelled = true }
  }, [(containers || []).map((c) => c.id).join(',')])

  return map
}

// persistIconToServer 将抓取到的 favicon 持久化到服务器 /data/images/ 目录。
// 后台异步执行，失败时静默忽略（不影响前端显示）。
async function persistIconToServer(container, iconUrl, containerUrl) {
  try {
    // 提取镜像名称（去掉 tag）
    const imageName = container.image.split(':')[0]

    // 调用后端接口下载并保存
    await imageAPI.fetchIcon({
      imageName: imageName,
      url: containerUrl
    })

    console.log(`✅ 图标已持久化: ${imageName} -> ${iconUrl}`)
  } catch (error) {
    // 静默失败，不影响用户体验
    console.debug(`图标持久化失败 (${container.name}):`, error.message)
  }
}
