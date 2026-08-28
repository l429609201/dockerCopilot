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
// icons 为后端 /api/icons 的图标配置数组：已有图标的镜像不再重复探测，
// 避免多端口容器每次刷新探到不同端口导致图标跳变，同时省掉无谓的网络请求。
export function useFaviconMap(containers, icons = []) {
  const [map, setMap] = useState({})

  useEffect(() => {
    const list = (containers || []).filter((c) => {
      const isRunning = (c.status || '').toLowerCase() === 'running'
      const hasPorts = pickProbePorts(c).length > 0
      // 已有持久化图标（含容器名/镜像名匹配）的容器直接跳过，只对"没图标"的才抓取
      const hasLogo = !!getImageLogo(c.usingImage, icons, c.name)
      return isRunning && hasPorts && !hasLogo
    })

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
          // 解耦"显示缓存"与"后端持久化"：命中 localStorage 仅代表本机显示过，
          // 不代表后端 icons.json 已写入（首次持久化可能失败/被并发覆盖）。
          // 能走到这里说明该容器不在过滤时的 hasLogo 命中集（icons 里没有），
          // 故补写一次持久化，确保图标最终落库。
          // 修复死角：旧缓存无 src 字段时，用容器实时端口重建一个访问地址再补写，
          // 而非直接跳过（否则该容器会永远进抓取队列却永不落库）。
          let src = hit.src
          if (!src) {
            const host = await getHostIP(c.hostId)
            if (cancelled) return
            const ordered = [...pickProbePorts(c)].sort((a, b) => scorePort(b) - scorePort(a))
            if (ordered.length > 0) {
              src = `http://${host}:${ordered[0]}`
              cache[c.id] = { ...hit, src } // 回填 src，下次直接可用
              cacheDirty = true
            }
          }
          if (src) persistIconToServer(c, src)
          continue
        }
        // 远程容器用其所属主机 IP，本地用当前访问 hostname
        const host = await getHostIP(c.hostId)
        if (cancelled) return
        const ordered = [...pickProbePorts(c)].sort((a, b) => scorePort(b) - scorePort(a))
        for (const port of ordered) {
          if (cancelled) return
          try {
            const src = `http://${host}:${port}`
            const r = await faviconAPI.resolve(src)
            const url = r.data?.data?.url
            if (url) {
              result[c.id] = url
              // 缓存额外记录容器访问地址 src：命中缓存补写持久化时，
              // 后端 fetchIcon 需要容器地址去 resolve+落盘，仅有 favicon url 不够。
              cache[c.id] = { url, src, ts: Date.now() }
              cacheDirty = true

              // 批量抓取时也自动持久化到服务器
              persistIconToServer(c, src)

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
    // icons 变化后重新评估：新持久化的图标会让对应容器从待抓取列表中移除
  }, [(containers || []).map((c) => c.id).join(','), (icons || []).length])

  return map
}

// persistIconToServer 将抓取到的 favicon 持久化到服务器 /data/images/ 目录。
// 后台异步执行，失败时静默忽略（不影响前端显示）。
async function persistIconToServer(container, containerUrl) {
  try {
    // 容器对象的镜像字段为 usingImage，去掉 tag 作为绑定 key
    const imageName = (container.usingImage || '').split(':')[0]
    if (!imageName) return

    await imageAPI.fetchIcon({
      imageName,
      url: containerUrl,
      targetType: 'image',
    })
  } catch (error) {
    // 静默失败，不影响用户体验（仅调试级日志）
    console.debug(`图标持久化失败 (${container.name}):`, error.message)
  }
}
