import axios from 'axios'

// 动态获取 API 基础地址
// 优先级：环境变量 > window.__API_BASE_URL > localStorage > 当前主机 > 默认值
function getAPIBaseURL() {
  // 1. 最高优先级：环境变量（构建时注入）
  if (import.meta.env.VITE_API_BASE_URL) {
    console.log('Using build-time API URL:', import.meta.env.VITE_API_BASE_URL)
    return import.meta.env.VITE_API_BASE_URL
  }

  // 2. 检查全局变量（注入的配置）
  if (typeof window !== 'undefined' && window.__API_BASE_URL) {
    console.log('Using injected API URL:', window.__API_BASE_URL)
    return window.__API_BASE_URL
  }

  // 3. 检查 localStorage（用户保存的地址）
  const savedURL = localStorage.getItem('api_base_url')
  if (savedURL) {
    console.log('Using localStorage API URL:', savedURL)
    return savedURL
  }

  // 4. 使用当前主机
  if (typeof window !== 'undefined' && window.location.host) {
    const currentHostURL = `${window.location.protocol}//${window.location.host}`
    console.log('Using current host API URL:', currentHostURL)
    return currentHostURL
  }

  // 5. 最后的默认值
  const fallbackURL = 'http://localhost'
  console.log('Using fallback API URL:', fallbackURL)
  return fallbackURL
}

const API_BASE_URL = getAPIBaseURL()

// 创建axios实例
const apiClient = axios.create({
  baseURL: API_BASE_URL,
  timeout: 10000,
  headers: {
    'Content-Type': 'application/json',
  },
})

// 请求拦截器 - 添加认证token
apiClient.interceptors.request.use(
  (config) => {
    const token = localStorage.getItem('docker_copilot_token')
    if (token) {
      config.headers.Authorization = `Bearer ${token}`
    }
    return config
  },
  (error) => {
    return Promise.reject(error)
  }
)

// 响应拦截器 - 处理认证过期
apiClient.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response?.status === 401) {
      // 只在有token的情况下移除它
      if (localStorage.getItem('docker_copilot_token')) {
        localStorage.removeItem('docker_copilot_token')
        // 触发自定义事件通知应用认证状态变化
        window.dispatchEvent(new CustomEvent('authChange', { detail: { authenticated: false } }))
      }
    }
    return Promise.reject(error)
  }
)

// 认证相关API
export const authAPI = {
  login: (secretKey) => {
    const formData = new FormData()
    formData.append('secretKey', secretKey)
    return apiClient.post('/api/auth', formData, {
      headers: { 'Content-Type': 'multipart/form-data' }
    })
  },
}

// 版本相关API
export const versionAPI = {
  getVersion: (type) => {
    // 如果type参数为空，则不添加查询参数
    if (!type) {
      return apiClient.get('/api/version')
    }
    return apiClient.get(`/api/version?type=${type}`)
  },
  updateProgram: () => apiClient.put('/api/program'),
}

// 多 Docker 管理：把 hostId 拼到 query（空则省略，保持对本地的向后兼容）
function hostQ(hostId, prefix = '?') {
  return hostId ? `${prefix}hostId=${encodeURIComponent(hostId)}` : ''
}

// 容器相关API（操作方法均支持可选 hostId，用于定位容器所属 Docker 主机）
export const containerAPI = {
  getContainers: () => apiClient.get('/api/containers'),
  // 从零创建新容器（任务化，返回 taskID 供轮询进度）。spec 见后端 CreateContainerReq。
  createContainer: (spec) => apiClient.post('/api/container/create', spec),
  // 解析 docker run 命令为创建参数（仅解析预览，不创建）
  parseRunCommand: (command) => apiClient.post('/api/container/parseRunCommand', { command }),
  startContainer: (id, hostId) => apiClient.post(`/api/container/${id}/start${hostQ(hostId)}`),
  stopContainer: (id, hostId) => apiClient.post(`/api/container/${id}/stop${hostQ(hostId)}`),
  restartContainer: (id, hostId) => apiClient.post(`/api/container/${id}/restart${hostQ(hostId)}`),
  renameContainer: (id, newName, hostId) => {
    return apiClient.post(`/api/container/${id}/rename?newName=${encodeURIComponent(newName)}${hostQ(hostId, '&')}`)
  },
  updateContainer: (id, containerName, imageNameAndTag, delOldContainer, hostId) => {
    const formData = new FormData()
    formData.append('containerName', containerName)
    formData.append('imageNameAndTag', imageNameAndTag)
    formData.append('delOldContainer', delOldContainer ? 'true' : 'false')
    if (hostId) formData.append('hostId', hostId)
    return apiClient.post(`/api/container/${id}/update`, formData, {
      headers: { 'Content-Type': 'multipart/form-data' }
    })
  },
  backupContainer: () => apiClient.get('/api/container/backup'),
  listBackups: () => apiClient.get('/api/container/listBackups'),
  restoreContainer: (filename) => {
    return apiClient.post(`/api/container/backups/${filename}/restore`)
  },
  deleteBackup: (filename) => apiClient.delete(`/api/container/backups?filename=${encodeURIComponent(filename)}`),
  backupToCompose: () => apiClient.get('/api/container/backup2compose'),
  // 阶段7：Portainer 风格容器运维
  pauseContainer: (id, hostId) => apiClient.post(`/api/container/${id}/pause${hostQ(hostId)}`),
  unpauseContainer: (id, hostId) => apiClient.post(`/api/container/${id}/unpause${hostQ(hostId)}`),
  killContainer: (id, hostId) => apiClient.post(`/api/container/${id}/kill${hostQ(hostId)}`),
  removeContainer: (id, force = false, removeVolumes = false, hostId) =>
    apiClient.delete(`/api/container/${id}?force=${force}&removeVolumes=${removeVolumes}${hostQ(hostId, '&')}`),
  inspectContainer: (id, hostId) => apiClient.get(`/api/container/${id}/inspect${hostQ(hostId)}`),
  getContainerLogs: (id, { tail = 200, timestamps = false, since = '' } = {}, hostId) =>
    apiClient.get(`/api/container/${id}/logs?tail=${tail}&timestamps=${timestamps}&since=${encodeURIComponent(since)}${hostQ(hostId, '&')}`),
  execContainer: (id, cmd, workDir = '', user = '', hostId) =>
    apiClient.post(`/api/container/${id}/exec`, { cmd, workDir, user, hostId }),
  topContainer: (id, hostId) => apiClient.get(`/api/container/${id}/top${hostQ(hostId)}`),
  editContainer: (id, spec, hostId) => apiClient.put(`/api/container/${id}/edit`, { ...spec, hostId }),
}

// 容器文件管理 API（后端已统一做防路径穿越校验）
export const filesAPI = {
  // 列目录
  list: (id, path = '/', hostId) =>
    apiClient.get(`/api/container/${id}/files?path=${encodeURIComponent(path)}${hostQ(hostId, '&')}`),
  // 读取文本内容（预览/编辑）
  read: (id, path, hostId) =>
    apiClient.get(`/api/container/${id}/files/read?path=${encodeURIComponent(path)}${hostQ(hostId, '&')}`),
  // 保存文本内容
  write: (id, path, content, hostId) =>
    apiClient.post(`/api/container/${id}/files/write`, { path, content, hostId }),
  // 新建目录
  mkdir: (id, path, name, hostId) =>
    apiClient.post(`/api/container/${id}/files/mkdir`, { path, name, hostId }),
  // 删除文件/目录
  remove: (id, path, hostId) =>
    apiClient.post(`/api/container/${id}/files/delete`, { path, hostId }),
  // 重命名/移动
  rename: (id, src, dst, hostId) =>
    apiClient.post(`/api/container/${id}/files/rename`, { src, dst, hostId }),
  // 下载文件（返回 blob）
  download: (id, path, hostId) =>
    apiClient.get(`/api/container/${id}/files/download?path=${encodeURIComponent(path)}${hostQ(hostId, '&')}`, {
      responseType: 'blob',
    }),
  // 上传文件到目录
  upload: (id, dir, file, onUploadProgress, hostId) => {
    const fd = new FormData()
    fd.append('path', dir)
    fd.append('file', file)
    if (hostId) fd.append('hostId', hostId)
    return apiClient.post(`/api/container/${id}/files/upload${hostQ(hostId)}`, fd, {
      headers: { 'Content-Type': 'multipart/form-data' },
      timeout: 300000, // 上传大文件放宽超时
      onUploadProgress,
    })
  },
}

// 镜像相关API
export const imageAPI = {
  getImages: () => apiClient.get('/api/images'),
  getIcons: () => apiClient.get('/api/icons'),
  // 删除镜像：按 hostId 路由到对应主机（空表示本地）
  deleteImage: (id, force = false, hostId = '') =>
    apiClient.delete(`/api/image/${id}?force=${force}${hostId ? `&hostId=${encodeURIComponent(hostId)}` : ''}`),
  // 异步批量清理镜像：提交 ids 列表，返回 taskID；hostId 指定目标主机
  pruneImages: (ids, force = false, hostId = '') => apiClient.post('/api/images/prune', { ids, force, hostId }),
  // 手动触发检测所有镜像更新（异步执行，立即返回）
  checkUpdate: () => apiClient.post('/api/images/check-update'),
  // 通过 URL 绑定图标到镜像名（无需上传图片）
  setIconUrl: (imageName, url) => apiClient.post('/api/icons/url', { imageName, url }),
  // 自动抓取站点 favicon 并下载持久化到 /data/images，url 为容器访问地址
  fetchIcon: (imageName, url) => apiClient.post('/api/icons/fetch', { imageName, url }),
  uploadIcon: (file, imageName, containerName) => {
    const formData = new FormData()
    formData.append('file', file)
    formData.append('imageName', imageName)
    if (containerName) {
      formData.append('containerName', containerName)
    }
    return apiClient.post('/api/icons', formData, {
      headers: { 'Content-Type': 'multipart/form-data' }
    })
  },
}

// 进度查询API
export const progressAPI = {
  getProgress: (taskid) => apiClient.get(`/api/progress/${taskid}`),
  // 阶段1：取消正在执行的任务
  cancelProgress: (taskid) => apiClient.post(`/api/progress/${taskid}/cancel`),
}

// 阶段2：定时更新规则API（每条规则拥有独立的 cron 定时）
export const scheduleAPI = {
  list: () => apiClient.get('/api/schedules'),
  save: (rule) => apiClient.post('/api/schedules', rule),
  remove: (id) => apiClient.delete(`/api/schedules/${id}`),
  runNow: (id) => apiClient.post(`/api/schedules/${id}/run`),
}

// 阶段2：Registry 凭据API（密码脱敏返回）
export const registryAPI = {
  list: () => apiClient.get('/api/registries'),
  save: (cred) => apiClient.post('/api/registries', cred),
  remove: (id) => apiClient.delete(`/api/registries/${id}`),
  // 查询该凭据在 Docker Hub 的剩余拉取次数（仅 Docker Hub 类型有效）
  rateLimit: (id) => apiClient.get(`/api/registries/${id}/ratelimit`),
}

// 阶段5：Telegram Bot 配置API
export const botAPI = {
  getConfig: () => apiClient.get('/api/bot/telegram'),
  saveConfig: (cfg) => apiClient.post('/api/bot/telegram', cfg),
  // 发送测试消息：验证 Token/代理/白名单是否可达
  testConfig: (cfg) => apiClient.post('/api/bot/telegram/test', cfg),
}

// 阶段3：Compose 项目管理API
export const composeAPI = {
  listProjects: () => apiClient.get('/api/compose/projects'),
  readFile: (id, filename) => apiClient.get(`/api/compose/projects/${id}/files/${encodeURIComponent(filename)}`),
  saveFile: (id, filename, content) =>
    apiClient.put(`/api/compose/projects/${id}/files/${encodeURIComponent(filename)}`, { content }),
  validate: (content) => apiClient.post('/api/compose/validate', { content }),
  // 从内容创建并部署一个新 Compose 项目（写入工作目录后 up），返回 taskID
  create: (payload) => apiClient.post('/api/compose/create', payload),
  action: (id, action, confirmWarnings = false) =>
    apiClient.post(`/api/compose/projects/${id}/action`, { action, confirmWarnings }),
  // 读取/保存 Compose 扫描配置（扫描目录、深度等）
  getConfig: () => apiClient.get('/api/compose/config'),
  saveConfig: (cfg) => apiClient.put('/api/compose/config', cfg),
  // 浏览 DC 自身文件系统目录（目录选择器用）；path 为空返回起始目录
  browse: (path = '') => apiClient.get('/api/compose/browse', { params: { path } }),
  // 创建文件夹
  createFolder: (parentPath, folderName) =>
    apiClient.post('/api/compose/folder', { parentPath, folderName }),
  // 创建 Compose 配置文件
  createFile: (parentPath, fileName) =>
    apiClient.post('/api/compose/file', { parentPath, fileName }),
}

// 阶段8：favicon 抓取API（按容器暴露端口解析站点图标）
export const faviconAPI = {
  resolve: (url) => apiClient.get(`/api/favicon/resolve?url=${encodeURIComponent(url)}`),
}

// 宿主机路径映射API（仅配置管理 + 路径解析，与后端 /api/hostpath 接口对齐）
export const hostPathAPI = {
  // 获取映射配置（含自动推导预览）
  getConfig: () => apiClient.get('/api/hostpath/config'),
  // 保存映射配置（enabled / mode / mappings）
  saveConfig: (config) => apiClient.post('/api/hostpath/config', config),
  // 将容器内路径解析为宿主机路径并校验可访问性
  resolve: (containerPath) => apiClient.post('/api/hostpath/resolve', { containerPath }),
}

// 多 Docker 主机管理 API（本地主机 id 恒为 "local"，不可删除、地址不可改）
export const dockerHostAPI = {
  // 列出全部主机及在线状态
  list: () => apiClient.get('/api/docker/hosts'),
  // 新建或更新主机（本地仅可改名）
  save: (host) => apiClient.post('/api/docker/hosts', host),
  // 删除远程主机
  remove: (id) => apiClient.delete(`/api/docker/hosts/${id}`),
  // 测试指定主机连通性
  ping: (id) => apiClient.post(`/api/docker/hosts/${id}/ping`),
}

// GitHub API - 用于检查前端更新
export const githubAPI = {
  /**
   * 获取 GitHub 仓库的最新 Release
   * @param {string} owner - 仓库所有者
   * @param {string} repo - 仓库名称
   * @returns {Promise} 返回最新 Release 信息
   */
  getLatestRelease: async (owner, repo) => {
    try {
      const response = await axios.get(`https://api.github.com/repos/${owner}/${repo}/releases/latest`, {
        timeout: 5000,
      })
      return response.data
    } catch (error) {
      console.warn('获取 GitHub 最新版本失败:', error.message)
      throw error
    }
  },

  /**
   * 获取 GitHub 仓库的所有 Releases
   * @param {string} owner - 仓库所有者
   * @param {string} repo - 仓库名称
   * @param {number} perPage - 每页返回数量
   * @returns {Promise} 返回 Release 列表
   */
  getReleases: async (owner, repo, perPage = 5) => {
    try {
      const response = await axios.get(`https://api.github.com/repos/${owner}/${repo}/releases`, {
        params: { per_page: perPage },
        timeout: 5000,
      })
      return response.data
    } catch (error) {
      console.warn('获取 GitHub Releases 列表失败:', error.message)
      throw error
    }
  },

  /**
   * 获取 GitHub 仓库信息
   * @param {string} owner - 仓库所有者
   * @param {string} repo - 仓库名称
   * @returns {Promise} 返回仓库信息
   */
  getRepoInfo: async (owner, repo) => {
    try {
      const response = await axios.get(`https://api.github.com/repos/${owner}/${repo}`, {
        timeout: 5000,
      })
      return response.data
    } catch (error) {
      console.warn('获取 GitHub 仓库信息失败:', error.message)
      throw error
    }
  },
}

// 图标管理 API
export const iconAPI = {
  // 获取所有图标配置
  getIcons: () => apiClient.get('/api/icons'),

  // 上传图标文件
  uploadIcon: (imageName, file) => {
    const formData = new FormData()
    formData.append('imageName', imageName)
    formData.append('icon', file)
    return apiClient.post('/api/icons', formData, {
      headers: { 'Content-Type': 'multipart/form-data' }
    })
  },

  // 通过 URL 绑定图标
  setIconURL: (imageName, iconURL) =>
    apiClient.post('/api/icons/url', { imageName, iconURL }),

  // 自动抓取站点 favicon 并持久化
  fetchIcon: (imageName, url) =>
    apiClient.post('/api/icons/fetch', { imageName, url }),
}

// 导出 apiClient 供其他组件使用
export { apiClient }
export default apiClient
