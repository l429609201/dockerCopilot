// 内置常用镜像logo配置
// 格式: { "镜像名称": "logo 文件路径" }
// 支持镜像名称匹配，如 "nginx" 会匹配 "nginx:latest", "nginx:alpine" 等

// 导入图片资源
import MediaSaberLogo from '../assets/logos/media-saber.png';
import MoviepilotLogo from '../assets/logos/moviepilot.png';
import DockerCopilotLogo from '../assets/logos/docker-copilot.png';
import MTPhotos from '../assets/logos/mt-photos.png';
import ITToolsLogo from '../assets/logos/it-tools.webp';
import SubStoreLogo from '../assets/logos/sub-store.webp';
import JellyfinLogo from '../assets/logos/jellyfin.png';
import RedisLogo from '../assets/logos/redis.png';
import PostgresLogo from '../assets/logos/postgres.png';
import SunPanelLogo from '../assets/logos/sun-panel.png';
import QinglongLogo from '../assets/logos/qinglong.svg';
import TransmissionLogo from '../assets/logos/transmission.png';
import QBittorrentLogo from '../assets/logos/qbittorrent.webp';
import FnDeskLogo from '../assets/logos/fndesk.png';
import FNTVLogo from '../assets/logos/fntv.png';
import CookiecloudLogo from '../assets/logos/cookiecloud.png';
import CodeServerLogo from '../assets/logos/code-server.png';
import IYUULogo from '../assets/logos/iyuu.png';
import LuckyLogo from '../assets/logos/lucky.png';
import EmbyserverLogo from '../assets/logos/embyserver.png';
import AudiobookshelfLogo from '../assets/logos/audiobookshelf.png';
import MySQLLogo from '../assets/logos/mysql.png';
import OneApiLogo from '../assets/logos/one-api.png';
import QDLogo from '../assets/logos/qd.png';
import OneHubogo from '../assets/logos/one-hub.png';
import ByteMuseLogo from '../assets/logos/byte-muse.jpg';
import NextChatLogo from '../assets/logos/next-chat.png';
import MdcNgLogo from '../assets/logos/mdc-ng.png';
import RichDogLogo from '../assets/logos/rich-dog.svg';
import MsTmdbLogo from '../assets/logos/ms_tmdb.png';

export const builtInImageLogos = {
  "xylplm/media-saber": MediaSaberLogo,
  "xylplm/bm-simulate-xunlei-api-to-media-saber": MediaSaberLogo,
  "jxxghp/moviepilot-v2": MoviepilotLogo,
  "0nlylty/dockercopilot": DockerCopilotLogo,
  "mtphotos/mt-photos": MTPhotos,
  "kqstone/mt-photos-insightface-unofficial": MTPhotos,
  "mtphotos/mt-photos-ai": MTPhotos,
  "corentinth/it-tools": ITToolsLogo,
  "xream/sub-store": SubStoreLogo,
  "nyanmisaka/jellyfin": JellyfinLogo,
  "redis": RedisLogo,
  "postgres": PostgresLogo,
  "hslr/sun-panel": SunPanelLogo,
  "whyour/qinglong": QinglongLogo,
  "linuxserver/transmission": TransmissionLogo,
  "linuxserver/qbittorrent": QBittorrentLogo,
  "imgzcq/fndesk": FnDeskLogo,
  "qiaokes/fntv-record-view": FNTVLogo,
  "easychen/cookiecloud": CookiecloudLogo,
  "codercom/code-server": CodeServerLogo,
  "iyuucn/iyuuplus": IYUULogo,
  "iyuucn/iyuuplus-dev-nodb": IYUULogo,
  "gdy666/lucky": LuckyLogo,
  "amilys/embyserver": EmbyserverLogo,
  "audiobookshelf": AudiobookshelfLogo,
  "mysql": MySQLLogo,
  "qdtoday/qd": QDLogo,
  "songquanpeng/one-api": OneApiLogo,
  "martialbe/one-api": OneHubogo,
  "envyafish/byte-muse":ByteMuseLogo,
  "yidadaa/chatgpt-next-web":NextChatLogo,
  "mdcng/mdc":MdcNgLogo,
  "zhaoyangguang/rebatedog":RichDogLogo,
  "gatecross/ms_tmdb":MsTmdbLogo,
  "ms_tmdb":MsTmdbLogo,
};

// 获取镜像的logo
// 优先级: 内置logo > 用户自定义 > 默认图标
export const getImageLogo = (imageName, customLogos = {}) => {
  // 先检查内置logo（优先级最高）
  const baseImageName = imageName.split(':')[0]; // 去掉tag部分

  // 优先匹配完整镜像名（包含 registry/namespace）
  if (builtInImageLogos[baseImageName]) {
    return builtInImageLogos[baseImageName];
  }

  // 尝试匹配最后一段镜像名（去掉 registry/namespace）
  const simpleName = baseImageName.split('/').pop();
  if (builtInImageLogos[simpleName]) {
    return builtInImageLogos[simpleName];
  }

  // 如果仍未匹配，使用关键字（子串）匹配
  for (const [key, url] of Object.entries(builtInImageLogos)) {
    if (!key) continue;
    try {
      if (baseImageName.includes(key) || simpleName.includes(key)) {
        return url;
      }
    } catch (e) {
      // 防御性代码：忽略任何异常并继续
    }
  }

  // 再检查用户自定义的logo
  if (customLogos[imageName]) {
    return customLogos[imageName];
  }
  if (customLogos[baseImageName]) {
    return customLogos[baseImageName];
  }
  if (customLogos[simpleName]) {
    return customLogos[simpleName];
  }

  // 尝试自定义图标的模糊匹配
  for (const [key, url] of Object.entries(customLogos)) {
    if (!key) continue;
    try {
      // 检查key是否是imageName的前缀（处理tag不同的情况）
      if (baseImageName === key || baseImageName.startsWith(key + ':') || baseImageName.startsWith(key + '/')) {
        return url;
      }
      // 反向检查：如果自定义图标配置的是 nginx:latest，但当前是 nginx
      if (key.split(':')[0] === baseImageName) {
        return url;
      }
    } catch (e) {
      // 忽略异常
    }
  }

  // 没有找到logo，返回null
  return null;
};

// 获取所有支持的镜像名称列表
export const getSupportedImageNames = () => {
  return Object.keys(builtInImageLogos);
};

// 检查镜像是否有内置logo
export const hasBuiltInLogo = (imageName) => {
  const baseImageName = imageName.split(':')[0];
  if (builtInImageLogos[baseImageName]) return true;
  const simpleName = baseImageName.split('/').pop();
  if (builtInImageLogos[simpleName]) return true;

  // 关键字（子串）匹配
  for (const key of Object.keys(builtInImageLogos)) {
    if (!key) continue;
    try {
      if (baseImageName.includes(key) || simpleName.includes(key)) return true;
    } catch (e) {
      // 忽略并继续
    }
  }
  return false;
};

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