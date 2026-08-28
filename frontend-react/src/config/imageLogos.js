// 图标匹配纯函数模块（无副作用、无内部状态）。
//
// 设计要点（重构后）：
// - 不再在模块内部异步加载/缓存图标配置，彻底消除"双数据源 + 首屏时序空窗"问题。
// - 图标数据由调用方（react-query 的 ['icons'] 查询）以「IconItem 数组」形式传入。
// - IconItem 结构与后端严格一致：
//   { target: string, targetType: 'container'|'image', iconUrl: string, priority: number }

// 兼容传入非数组（防御式）：统一归一化为数组。
function normalizeIcons(icons) {
  if (Array.isArray(icons)) return icons
  return []
}

// getImageLogo 按镜像名/容器名从图标数组中匹配 iconUrl。
// 优先级：容器名精确 > 镜像名精确 > 简化名精确 > 大小写不敏感 > 模糊(子串)。
// @param {string} imageName 镜像名（可含 tag）
// @param {IconItem[]} icons  图标配置数组（来自后端 /api/icons）
// @param {string|null} containerName 容器名（可选，优先匹配）
// @returns {string|null} 匹配到的 iconUrl，无则 null
export const getImageLogo = (imageName, icons = [], containerName = null) => {
  const items = normalizeIcons(icons)
  if (!imageName && !containerName) return null

  // 1. 容器名精确匹配（容器级优先）
  if (containerName) {
    const hit = items.find(
      (it) => it.targetType === 'container' && it.target === containerName
    )
    if (hit) return hit.iconUrl
  }

  if (!imageName) return null

  const baseImageName = imageName.split(':')[0] // 去掉 tag
  const simpleName = baseImageName.split('/').pop() // 去掉 registry/namespace

  // 2. 镜像名精确匹配（完整名）
  const exact = items.find(
    (it) => it.targetType === 'image' && it.target === baseImageName
  )
  if (exact) return exact.iconUrl

  // 3. 简化名精确匹配
  const simple = items.find(
    (it) => it.targetType === 'image' && it.target === simpleName
  )
  if (simple) return simple.iconUrl

  // 4. 大小写不敏感匹配
  const lowerBase = baseImageName.toLowerCase()
  const lowerSimple = simpleName.toLowerCase()
  const ci = items.find(
    (it) => it.targetType === 'image' && it.target.toLowerCase() === lowerBase
  )
  if (ci) return ci.iconUrl
  const ciSimple = items.find(
    (it) =>
      it.targetType === 'image' &&
      it.target.split('/').pop().toLowerCase() === lowerSimple
  )
  if (ciSimple) return ciSimple.iconUrl

  // 5. 模糊匹配（子串）
  const fuzzy = items.find((it) => {
    if (it.targetType !== 'image') return false
    try {
      const t = it.target.toLowerCase()
      return lowerBase.includes(t) || lowerSimple.includes(t)
    } catch {
      return false
    }
  })
  if (fuzzy) return fuzzy.iconUrl

  return null
}

// getSupportedImageNames 返回所有镜像级图标的 target 列表。
export const getSupportedImageNames = (icons = []) =>
  normalizeIcons(icons)
    .filter((it) => it.targetType === 'image')
    .map((it) => it.target)

// hasBuiltInLogo 判断某镜像/容器是否已有图标配置。
export const hasBuiltInLogo = (imageName, icons = [], containerName = null) =>
  getImageLogo(imageName, icons, containerName) != null

/**
 * resolveContainerIcon 统一解析容器图标 URL（卡片/列表/详情共用，保证优先级一致）。
 *
 * 优先级：容器自定义 iconUrl > 容器名匹配 > 镜像名匹配 > 实时抓取的 favicon。
 * 说明：持久化结果必须压过实时抓取，否则每次刷新 useFaviconMap 探测回来的地址
 *      会覆盖已固定的图标——多端口容器就表现为「刷新就跳」。实时 favicon 退化为兜底，
 *      仅在该镜像还没有任何持久化图标时生效。
 *
 * @param {object} container 容器对象（需含 id / name / iconUrl / usingImage）
 * @param {object} faviconMap 由 useFaviconMap 生成的 {容器id: url} 映射
 * @param {IconItem[]} icons 图标配置数组（来自后端 /api/icons）
 * @returns {string|null} 解析出的图标地址，无则 null
 */
export const resolveContainerIcon = (container, faviconMap = {}, icons = []) => {
  if (!container) return null
  // 1. 容器自身已设置的自定义图标优先级最高
  if (container.iconUrl) return container.iconUrl
  // 2. 图标配置匹配（容器名优先 > 镜像名）
  if (container.usingImage || container.name) {
    const logo = getImageLogo(container.usingImage, icons, container.name)
    if (logo) return logo
  }
  // 3. 兜底：本次会话实时抓取到的 favicon（尚未持久化时才会走到这里）
  if (faviconMap && faviconMap[container.id]) return faviconMap[container.id]
  return null
}
