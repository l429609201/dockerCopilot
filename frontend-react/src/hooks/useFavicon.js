import { useState, useEffect } from 'react'
import { faviconAPI, imageAPI, dockerHostAPI } from '../api/client.js'
import { getImageLogo } from '../config/imageLogos.js'

// favicon 结果本地缓存（key: 容器id，value: {url, ts}）
const CACHE_KEY = 'dc_favicon_cache'
const TTL = 7 * 24 * 60 * 60 * 1000 // 7天

// 从 Docker 主机连接地址（tcp://ip:port）解析出主机 IP/域名。
function parseHostIP(address) {
  if (!address) return ''
  let a = address.replace(/^tcp:\/\//i, '').replace(/^https?:\/\//i, '').split('/')[0]
  const idx = a.lastIndexOf(':')
  return idx > 0 ? a.slice(0, idx) : a
}

// 模块级 hostId→IP 映射缓存：多个容器卡片共享，避免每张卡都请求主机列表。
let _hostIPMap = null
let _hostIPPromise = null
async function getHostIP(hostId) {
  if (!hostId || hostId === 'local') return window.location.hostname || 'localhost'
  if (!_hostIPMap) {
    if (!_hostIPPromise) {
      _hostIPPromise = dockerHostAPI.list().then((r) => {
        const map = {}
        if (r.data?.code === 200 && Array.isArray(r.data.data)) {
          for (const h of r.data.data) map[h.id] = parseHostIP(h.address)
        }
        _hostIPMap = map
        return map
      }).catch(() => { _hostIPMap = {}; return {} })
    }
    await _hostIPPromise
  }
  return _hostIPMap[hostId] || (window.location.hostname || 'localhost')
}

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

    const tryPorts = async () => {
      setLoading(true)
      // 远程容器用其所属主机 IP，本地用当前访问 hostname
      const host = await getHostIP(container.hostId)
      if (cancelled) return
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
// customIcons 传入已持久化的图标映射：已有图标的镜像不再重复探测，
// 避免多端口容器每次刷新探到不同端口导致图标跳变，同时省掉无谓的网络请求。
export function useFaviconMap(containers, customIcons = {}) {
  const [map, setMap] = useState({})

  useEffect(() => {
    const list = (containers || []).filter(
      (c) =>
        (c.status || '').toLowerCase() === 'running' &&
        pickProbePorts(c).length > 0 &&
        // 已有持久化图标（含内置 logo 与模糊匹配）的镜像直接跳过，只对"没图标"的才抓取
        !(c.usingImage && getImageLogo(c.usingImage, customIcons || {}))
    )
    if (list.length === 0) return
    let cancelled = false

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
        // 远程容器用其所属主机 IP，本地用当前访问 hostname
        const host = await getHostIP(c.hostId)
        if (cancelled) return
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
    // customIcons 变化后重新评估：新持久化的图标会让对应容器从待抓取列表中移除
  }, [(containers || []).map((c) => c.id).join(','), Object.keys(customIcons || {}).join(',')])

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
