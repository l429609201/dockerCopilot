// 动态图标加载模块
// 从后端 API 获取，支持容器名称和镜像名称匹配
import { iconAPI } from '../api/client.js'

// 图标配置项结构
// {
//   target: "容器名或镜像名",
//   targetType: "container" | "image",
//   iconUrl: "/images/xxx.png",
//   priority: 1 (容器级) | 2 (镜像级)
// }

// 运行时图标缓存（从后端加载）
let iconItems = [] // 扩展数组格式
let cacheLoaded = false
let loadingPromise = null

// 从后端加载图标配置
async function loadIconsFromBackend() {
  if (cacheLoaded) return iconItems
  if (loadingPromise) return loadingPromise

  loadingPromise = (async () => {
    try {
      const response = await iconAPI.getIcons()
      if (response.data?.code === 200 && Array.isArray(response.data?.data)) {
        iconItems = response.data.data
        cacheLoaded = true
        console.log('✅ 已加载图标配置，共', iconItems.length, '项')
      }
    } catch (error) {
      console.warn('⚠️ 加载图标配置失败:', error.message)
    } finally {
      loadingPromise = null
    }
    return iconItems
  })()

  return loadingPromise
}

// 初始化时自动加载
loadIconsFromBackend()

// 导出空对象（向后兼容，实际不再使用）
export const builtInImageLogos = {}

// 获取镜像的logo（同步函数，使用已加载的缓存）
// 优先级: 容器名称匹配 > 镜像名称匹配 > 用户自定义 > 默认图标
// containerName: 容器名称（可选，如果提供则优先匹配）
// imageName: 镜像名称
// customLogos: 用户自定义图标（向后兼容）
export const getImageLogo = (imageName, customLogos = {}, containerName = null) => {
  // 直接使用已加载的缓存（不等待，避免异步问题）

  // 1. 优先匹配容器名称（如果提供）
  if (containerName) {
    const containerMatch = iconItems.find(item =>
      item.targetType === 'container' && item.target === containerName
    )
    if (containerMatch) {
      return containerMatch.iconUrl
    }
  }

  // 2. 匹配镜像名称（多种匹配策略）
  const baseImageName = imageName.split(':')[0] // 去掉tag部分
  const simpleName = baseImageName.split('/').pop() // 去掉 registry/namespace

  // 精确匹配（完整镜像名）
  const exactMatch = iconItems.find(item =>
    item.targetType === 'image' && item.target === baseImageName
  )
  if (exactMatch) return exactMatch.iconUrl

  // 精确匹配（简化镜像名）
  const simpleMatch = iconItems.find(item =>
    item.targetType === 'image' && item.target === simpleName
  )
  if (simpleMatch) return simpleMatch.iconUrl

  // 大小写不敏感匹配
  const lowerBaseImageName = baseImageName.toLowerCase()
  const lowerSimpleName = simpleName.toLowerCase()

  const caseInsensitiveMatch = iconItems.find(item =>
    item.targetType === 'image' && item.target.toLowerCase() === lowerBaseImageName
  )
  if (caseInsensitiveMatch) return caseInsensitiveMatch.iconUrl

  const caseInsensitiveSimpleMatch = iconItems.find(item =>
    item.targetType === 'image' && item.target.split('/').pop().toLowerCase() === lowerSimpleName
  )
  if (caseInsensitiveSimpleMatch) return caseInsensitiveSimpleMatch.iconUrl

  // 模糊匹配（子串匹配）
  const fuzzyMatch = iconItems.find(item => {
    if (item.targetType !== 'image') return false
    try {
      const lowerTarget = item.target.toLowerCase()
      return lowerBaseImageName.includes(lowerTarget) || lowerSimpleName.includes(lowerTarget)
    } catch (e) {
      return false
    }
  })
  if (fuzzyMatch) return fuzzyMatch.iconUrl

  // 3. 检查用户自定义的logo（向后兼容）
  if (customLogos[imageName]) return customLogos[imageName]
  if (customLogos[baseImageName]) return customLogos[baseImageName]
  if (customLogos[simpleName]) return customLogos[simpleName]

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
  return iconItems.filter(item => item.targetType === 'image').map(item => item.target)
}

// 检查镜像是否有内置logo（同步函数）
export const hasBuiltInLogo = (imageName, containerName = null) => {
  if (containerName) {
    const hasContainer = iconItems.some(item =>
      item.targetType === 'container' && item.target === containerName
    )
    if (hasContainer) return true
  }

  const baseImageName = imageName.split(':')[0]
  const simpleName = baseImageName.split('/').pop()

  return iconItems.some(item => {
    if (item.targetType !== 'image') return false
    try {
      return item.target === baseImageName ||
             item.target === simpleName ||
             baseImageName.includes(item.target) ||
             simpleName.includes(item.target)
    } catch (e) {
      return false
    }
  })
}

/**
 * 统一解析容器图标 URL（卡片/列表/详情共用，保证优先级一致）。
 * 优先级：容器自定义 iconUrl > 容器名称匹配 > 镜像名称匹配 > 用户自定义 > 实时抓取的 favicon。
 * 说明：持久化结果必须压过实时抓取，否则每次刷新 useFaviconMap 探测回来的地址
 *      会覆盖已固定的图标——多端口容器就表现为「刷新就跳」。
 *      实时 favicon 退化为兜底，仅在该镜像还没有任何图标时生效。
 *      getImageLogo 已支持容器名称优先匹配。
 * @param {object} container 容器对象（需含 id / name / iconUrl / usingImage）
 * @param {object} faviconMap 由 useFaviconMap 生成的 {容器id: url} 映射
 * @param {object} customIcons 用户自定义图标配置 {镜像名: url}
 * @returns {string|null} 解析出的图标地址，无则返回 null
 */
export const resolveContainerIcon = (container, faviconMap = {}, customIcons = {}) => {
  if (!container) return null;
  // 1. 容器自身已设置的自定义图标优先级最高
  if (container.iconUrl) return container.iconUrl;
  // 2. 内置logo（容器名称优先 > 镜像名称）/ 用户自定义logo / 抓取后已持久化的图标
  if (container.usingImage) {
    const logo = getImageLogo(container.usingImage, customIcons || {}, container.name);
    if (logo) return logo;
  }
  // 3. 兜底：本次会话实时抓取到的 favicon（尚未持久化时才会走到这里）
  if (faviconMap && faviconMap[container.id]) return faviconMap[container.id];
  return null;
};