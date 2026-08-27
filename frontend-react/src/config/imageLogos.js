// 动态图标加载模块
// 优先从后端 API 获取，后端没有时可自动从容器页面抓取 favicon
import { iconAPI } from '../api/client.js'

// 运行时图标缓存（从后端加载）
let dynamicIconCache = {}
let cacheLoaded = false
let loadingPromise = null

// 从后端加载图标配置
async function loadIconsFromBackend() {
  if (cacheLoaded) return dynamicIconCache
  if (loadingPromise) return loadingPromise

  loadingPromise = (async () => {
    try {
      const response = await iconAPI.getIcons()
      if (response.data?.code === 200 && response.data?.data) {
        dynamicIconCache = response.data.data
        cacheLoaded = true
        console.log('✅ 已加载图标配置，共', Object.keys(dynamicIconCache).length, '个镜像')
      }
    } catch (error) {
      console.warn('⚠️ 加载图标配置失败:', error.message)
    } finally {
      loadingPromise = null
    }
    return dynamicIconCache
  })()

  return loadingPromise
}

// 初始化时自动加载
loadIconsFromBackend()

// 导出空对象（向后兼容，实际不再使用）
export const builtInImageLogos = {}

// 获取镜像的logo（同步函数，使用已加载的缓存）
// 优先级: 动态加载的图标 > 用户自定义 > 默认图标
export const getImageLogo = (imageName, customLogos = {}) => {
  // 直接使用已加载的缓存（不等待，避免异步问题）
  const baseImageName = imageName.split(':')[0] // 去掉tag部分
  const simpleName = baseImageName.split('/').pop()

  // 优先匹配动态加载的图标（精确匹配）
  if (dynamicIconCache[baseImageName]) {
    return dynamicIconCache[baseImageName]
  }
  if (dynamicIconCache[simpleName]) {
    return dynamicIconCache[simpleName]
  }

  // 大小写不敏感匹配（动态图标）
  const lowerBaseImageName = baseImageName.toLowerCase()
  const lowerSimpleName = simpleName.toLowerCase()

  for (const [key, url] of Object.entries(dynamicIconCache)) {
    if (key.toLowerCase() === lowerBaseImageName) {
      return url
    }
  }

  for (const [key, url] of Object.entries(dynamicIconCache)) {
    const keySimple = key.split('/').pop().toLowerCase()
    if (keySimple === lowerSimpleName) {
      return url
    }
  }

  // 模糊匹配（动态图标）
  for (const [key, url] of Object.entries(dynamicIconCache)) {
    if (!key) continue
    try {
      const lowerKey = key.toLowerCase()
      if (lowerBaseImageName.includes(lowerKey) || lowerSimpleName.includes(lowerKey)) {
        return url
      }
    } catch (e) {
      // 忽略异常
    }
  }

  // 再检查用户自定义的logo
  if (customLogos[imageName]) {
    return customLogos[imageName]
  }
  if (customLogos[baseImageName]) {
    return customLogos[baseImageName]
  }
  if (customLogos[simpleName]) {
    return customLogos[simpleName]
  }

  // 尝试自定义图标的模糊匹配
  for (const [key, url] of Object.entries(customLogos)) {
    if (!key) continue
    try {
      if (baseImageName === key || baseImageName.startsWith(key + ':') || baseImageName.startsWith(key + '/')) {
        return url
      }
      if (key.split(':')[0] === baseImageName) {
        return url
      }
    } catch (e) {
      // 忽略异常
    }
  }

  // 没有找到logo，返回null
  return null
}

// 获取所有支持的镜像名称列表
export const getSupportedImageNames = () => {
  return Object.keys(dynamicIconCache)
}

// 检查镜像是否有内置logo（同步函数）
export const hasBuiltInLogo = (imageName) => {
  const baseImageName = imageName.split(':')[0]
  if (dynamicIconCache[baseImageName]) return true
  const simpleName = baseImageName.split('/').pop()
  if (dynamicIconCache[simpleName]) return true

  // 关键字（子串）匹配
  for (const key of Object.keys(dynamicIconCache)) {
    if (!key) continue
    try {
      if (baseImageName.includes(key) || simpleName.includes(key)) return true
    } catch (e) {
      // 忽略并继续
    }
  }
  return false
}

/**
 * 统一解析容器图标 URL（卡片/列表/详情共用，保证优先级一致）。
 * 优先级：容器自定义 iconUrl > 已持久化的 logo（内置/自定义/抓取后落盘）> 实时抓取的 favicon。
 * 说明：持久化结果必须压过实时抓取，否则每次刷新 useFaviconMap 探测回来的地址
 *      会覆盖已固定的图标——多端口容器就表现为「刷新就跳」。
 *      实时 favicon 退化为兜底，仅在该镜像还没有任何图标时生效。
 *      getImageLogo 第二参 customLogos 内部已包含"内置 + 自定义 + 模糊匹配"逻辑。
 * @param {object} container 容器对象（需含 id / iconUrl / usingImage）
 * @param {object} faviconMap 由 useFaviconMap 生成的 {容器id: url} 映射
 * @param {object} customIcons 用户自定义图标配置 {镜像名: url}
 * @returns {string|null} 解析出的图标地址，无则返回 null
 */
export const resolveContainerIcon = (container, faviconMap = {}, customIcons = {}) => {
  if (!container) return null;
  // 1. 容器自身已设置的自定义图标优先级最高
  if (container.iconUrl) return container.iconUrl;
  // 2. 内置logo / 用户自定义logo / 抓取后已持久化的图标（getImageLogo 内部已做模糊匹配）
  if (container.usingImage) {
    const logo = getImageLogo(container.usingImage, customIcons || {});
    if (logo) return logo;
  }
  // 3. 兜底：本次会话实时抓取到的 favicon（尚未持久化时才会走到这里）
  if (faviconMap && faviconMap[container.id]) return faviconMap[container.id];
  return null;
};