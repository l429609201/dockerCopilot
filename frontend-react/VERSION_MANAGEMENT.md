# 版本管理指南

本项目实现了完整的前后端版本管理系统，支持版本检查、更新提示等功能。

## 📦 版本配置

所有版本信息集中管理在 `src/config/version.js` 文件中：

```javascript
export const VERSION_CONFIG = {
  FRONTEND_VERSION: '1.0.0',      // 前端版本号
  BUILD_TIME: ISO 8601 format,    // 构建时间
  BUILD_ENV: 'development',       // 构建环境（development/production）
  APP_NAME: 'Docker Copilot Frontend',
  APP_DESC: 'Docker 容器管理前端应用',
}
```

## 🔄 版本号规则

采用 **Semantic Versioning (语义化版本)** 规范：`major.minor.patch`

- **major**：主版本号，功能有重大改变时增加（如 1.0.0 → 2.0.0）
- **minor**：次版本号，添加新功能时增加（如 1.0.0 → 1.1.0）
- **patch**：补丁版本号，修复bug时增加（如 1.0.0 → 1.0.1）

例如：
- `1.0.0` 初始版本
- `1.1.0` 添加新功能
- `1.1.1` 修复bug
- `2.0.0` 大版本更新

## 🎯 版本管理使用指南

### 1. 更新前端版本

修改 `src/config/version.js` 中的 `FRONTEND_VERSION`：

```javascript
export const VERSION_CONFIG = {
  FRONTEND_VERSION: '1.1.0',  // 更新版本号
  // ...
}
```

构建时间会自动生成。

### 2. 后端版本获取

后端版本从 API 接口动态获取：

```bash
# 获取后端本地版本
GET /api/version?type=local
响应：{ code: 200, data: { version: "1.0.0", buildDate: "2024-11-18T10:00:00Z" } }

# 获取远端版本（检查是否有新版本）
GET /api/version?type=remote
响应：{ code: 200, data: { remoteVersion: "1.1.0" } }
```

### 3. 前端版本显示

在应用的侧边栏中显示：

- **前端版本**：从 `VERSION_CONFIG.FRONTEND_VERSION` 显示
- **构建环境**：从 `VERSION_CONFIG.BUILD_ENV` 显示（开发/生产）
- **构建时间**：从 `VERSION_CONFIG.BUILD_TIME` 显示

- **后端版本**：从 API `/api/version?type=local` 获取
- **后端构建时间**：从 API 响应中的 `buildDate` 获取

### 4. 版本更新检查

应用会每分钟自动检查一次是否有新版本：

1. 获取本地后端版本
2. 获取远端后端版本
3. 对比版本号，如果有新版本则显示"更新"提示

## 🚀 使用 Hook

在任何组件中使用 `useVersionCheck` Hook：

```jsx
import { useVersionCheck } from '@/hooks/useVersionCheck'

function MyComponent() {
  const {
    frontendVersion,      // 前端版本
    backendVersion,       // 后端当前版本
    remoteVersion,        // 后端最新版本
    hasBackendUpdate,     // 是否有后端更新
    buildTime,           // 前端构建时间
    buildEnv,            // 构建环境
    refreshPage,         // 刷新页面函数
    updateBackend,       // 更新后端函数
    checkForUpdates,     // 检查更新函数
    formatBuildTime,     // 格式化时间函数
  } = useVersionCheck()

  return (
    <div>
      前端版本: {frontendVersion}
      后端版本: {backendVersion}
      {hasBackendUpdate && <button onClick={updateBackend}>更新</button>}
    </div>
  )
}
```

## 📝 版本比较工具函数

### `shouldUpdate(currentVersion, latestVersion)`

检查是否需要更新：

```javascript
import { shouldUpdate } from '@/config/version'

shouldUpdate('1.0.0', '1.1.0')  // true - 有更新
shouldUpdate('1.0.0', '1.0.0')  // false - 无更新
shouldUpdate('1.1.0', '1.0.0')  // false - 本地版本更新
```

### `parseVersion(version)`

解析版本号字符串：

```javascript
import { parseVersion } from '@/config/version'

parseVersion('1.0.0')
// { major: 1, minor: 0, patch: 0, raw: '1.0.0' }

parseVersion('1.2.3-beta')
// { major: 1, minor: 2, patch: 3, raw: '1.2.3-beta' }
```

### `formatBuildTime(dateString)`

格式化构建时间为北京时间：

```javascript
import { formatBuildTime } from '@/config/version'

formatBuildTime('2024-11-18T10:00:00Z')
// '2024-11-18 18:00:00'
```

## 🔍 版本显示示例

侧边栏版本卡片显示如下信息：

```
┌─────────────────────────────────┐
│ 前端  v1.0.0  [开发]            │
│ 后端  v1.0.0                     │
│ 构建：2024-11-18 18:00:00       │
│ 后端：2024-11-18 16:30:00       │
└─────────────────────────────────┘
```

如果有新版本可用：

```
┌─────────────────────────────────┐
│ 前端  v1.0.0  [开发]            │
│ 后端  v1.0.0       [更新] ⚡     │
│ 构建：2024-11-18 18:00:00       │
│ 后端：2024-11-18 16:30:00       │
└─────────────────────────────────┘
```

点击"更新"按钮会打开更新提示弹窗。

## 📋 CI/CD 集成

### GitHub Actions 版本管理

版本号在 GitHub Actions 工作流中的使用：

1. **dev 分支**：打包为 `dev` tag 的 Docker 镜像
2. **master 分支**：打包为 `latest` 和 `v{version}` tag 的 Docker 镜像

更新 `src/config/version.js` 中的版本号后：

```bash
git add .
git commit -m "chore: bump version to 1.1.0"
git push origin master
```

CI/CD 会自动：
1. 构建新的 Docker 镜像
2. 推送到 GitHub Container Registry
3. 创建 GitHub Release

## ⚙️ 环境变量

版本管理会自动检测构建环境：

- `VITE_DEV=true`：开发环境（侧边栏显示"开发"）
- `VITE_DEV=false`：生产环境（侧边栏显示"生产"）

## 🐛 故障排除

### 版本信息显示为 "unknown"

1. 检查后端 API 是否正常运行
2. 检查 `VITE_API_BASE_URL` 环境变量配置
3. 在浏览器控制台查看 API 请求错误

### 版本更新检查不生效

1. 确保后端 `/api/version?type=remote` 接口返回正确的版本信息
2. 查看浏览器网络标签页检查 API 响应
3. 检查版本号格式是否符合 `major.minor.patch` 规范

## 📚 相关文件

- `src/config/version.js` - 版本配置文件
- `src/hooks/useVersionCheck.js` - 版本检查 Hook
- `src/components/Header.jsx` - 侧边栏版本显示
- `src/components/UpdatePrompt.jsx` - 版本更新提示弹窗
- `src/api/client.js` - API 客户端配置
