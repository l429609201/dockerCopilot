# dockerCopilot
<a href="https://www.gnu.org/licenses/agpl-3.0.en.html">
    <img alt="License: AGPLv3" src="https://shields.io/badge/License-AGPL%20v3-blue.svg">
  </a>

> **Fork 声明**：本项目 fork 自 [onlyLTY/dockerCopilot](https://github.com/onlyLTY/dockerCopilot)，
> 在其基础上进行了修改与功能新增（如容器文件管理器、任务中心、镜像图标自适应等）。
> 原始版权归原作者 **onlyLTY** 及其贡献者所有，特此致谢。
> 本 Fork 依据 AGPLv3 继续开源，详见 [LICENSE](./LICENSE)。
>
> 衷心感谢原作者 onlyLTY 及社区所有贡献者的付出。

# 致谢

感谢原项目 [onlyLTY/dockerCopilot](https://github.com/onlyLTY/dockerCopilot) 及其所有贡献者，
没有原项目就没有本 Fork。本项目在其基础上持续改进，欢迎提交 Issue 与建议。

# 介绍

一个主打便捷的 docker 容器管理工具，支持多平台（amd64 / arm64）。

**基础能力**
1. 一键更新容器 / 指定镜像和 tag 更新
2. 启动、停止、重启、暂停、恢复、删除、重命名容器
3. 删除无 TAG 镜像 / 未使用镜像
4. 更新进度查看（含分层进度）
5. 备份 / 恢复容器设置

**本 Fork 新增能力**
6. 镜像拉取与更新异步化（统一任务系统：并发、进度、取消）
7. 定时任务：每条规则独立定时，支持自动更新 / 清理镜像 / 备份
8. Telegram 机器人：按钮式菜单、单容器面板、交互式终端、周期更新通知
9. Compose 项目管理：扫描、编辑、校验、部署（up/down/restart/pull）
10. Portainer 风格运维：交互式终端、文件管理器、6 Tab 参数编辑
11. 容器文件管理器、镜像图标自适应、任务中心

## 使用

docker compose 安装

```
services:
  dockercopilot:
    container_name: dockercopilot
    restart: always
    privileged: true
    network_mode: bridge
    ports:
      - 12712:12712
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - ./data:/data
    environment:
      - TZ=Asia/Shanghai
      - DOCKER_HOST=unix:///var/run/docker.sock
      - secretKey=密码，不少于八位且非纯数字
    image: ghcr.io/l429609201/dockercopilot:latest
```

### 镜像来源与架构

本 Fork 的镜像同时发布到 Docker Hub 与 GitHub Container Registry (GHCR)：

- Docker Hub：`<DockerHub用户名>/dockercopilot`
- GHCR：`ghcr.io/l429609201/dockercopilot`

标签说明：

- `:latest` / `:{版本号}` —— `main` 分支产出，**多架构镜像**（`linux/amd64` + `linux/arm64`，拉取时自动匹配当前平台）。
- `:test` —— `test` 分支产出，仅 `linux/amd64`，用于测试验证。

> 升级方式：本项目已移除二进制自更新，统一通过"拉取新镜像并重建容器"完成升级。

## 新增功能说明（改造版）

以下功能在原版基础上新增，均以当前 Go 后端为主线实现。

### 镜像拉取与更新异步化
- 启动时镜像检查改为后台执行，不再阻塞服务启动和其他页面。
- 容器更新、镜像拉取进入统一任务系统，支持并发上限、进度、失败原因和取消。
- 取消任务：`POST /api/progress/:taskid/cancel`。
- 相关配置见 `etc/dockerCopilot.yaml` 的 `Task` 段（`MaxConcurrent`、`PullTimeoutSec`）。

### 定时任务（更新 / 清理 / 备份）
- 通过 `/api/schedules` 管理定时规则，**每条规则拥有独立的执行时间**（cron 表达式，或 `daily:HH:MM`、`hourly:MM`、`interval:Xh` 等简化写法）。
- 规则类型支持：自动更新容器、定时清理镜像（悬空 / 未使用）、定时备份容器配置。
- 更新规则可配置容器选择、仅有更新才更新、跳过无 tag/digest 镜像、保留旧容器和通知开关。
- Docker Hub 等私有仓库凭据通过 `/api/registries` 管理，接口和日志均脱敏，不回显明文。
- 配置持久化到 `/data/config/config.json`，可用 `DOCKERCOPILOT_BOT_CONFIG` 覆盖路径。

### Telegram 机器人
- 通过 `/api/bot/telegram` 配置 Token、白名单 Chat ID、代理和通知开关，仅白名单会话可操作。
- **按钮式主菜单**：概览 / 容器 / 更新 / 镜像 / 备份 / 实例 / 设置 / 帮助，所有操作点按钮完成，交互全程在同一条消息上编辑，避免刷屏。
- **单容器面板**：启动 / 停止 / 重启 / 暂停 / 恢复、更新、切换标签、日志、详情、资源、重命名、删除，以及**交互式终端**（连续执行命令、保持工作目录）。
- **列表分页**：镜像、Compose 项目、更新中心等长列表均带内联翻页（上一页 / 下一页）和返回。
- **周期更新通知**：按可配置周期检测镜像更新，推送带交互式键盘的通知（逐容器更新 / 屏蔽、全部更新 / 全部屏蔽、调整检查间隔）。
- 定时更新与检测结果均可推送到 Telegram。

### Compose 项目管理
- 需将宿主机 compose 目录挂载进容器，并在配置中设置 `Compose.ScanPaths`：

```yaml
volumes:
  - /宿主机/compose目录:/compose
  - ./data:/data
```

- 支持项目扫描、文件查看/编辑、YAML 校验、高风险配置提示和部署操作（up/down/restart/pull）。
- 部署 `up` 遇高风险配置需二次确认；所有部署命令带超时并进入任务系统。

### Portainer 风格容器运维
- 容器暂停、恢复、强制终止、删除、重命名、命令执行、日志查看和参数编辑。
- **交互式终端**：基于 WebSocket + xterm.js 连接容器 `exec`，支持 bash/sh/ash 与自定义命令。
- **容器详情**：多 Tab 展示基本信息、网络、挂载、环境变量、资源限制等。
- **参数编辑（6 Tab）**：常规（重启策略）、网络（网络模式 / 端口映射）、挂载（卷绑定）、环境变量、资源（内存 / CPU）、标签 & 命令（Labels / Cmd / Entrypoint）。
- 编辑通过"停旧→建新→启动→校验→可选删旧"任务化重建，失败自动回滚；挂载等高风险改动带确认提示。
- **文件管理器**：浏览容器内文件系统，支持在线查看 / 编辑 / 上传 / 下载 / 新建 / 删除 / 重命名，兼容 BusyBox 与 GNU 环境。

### 前端界面
- React (Vite + Tailwind) 单页应用，构建产物通过 `//go:embed` 嵌入 Go 二进制，无需单独部署。
- 容器支持卡片 / 列表两种视图，镜像图标自适应（自定义图标 > favicon 探测 > 内置 logo）。
- 任务中心实时展示更新进度，镜像拉取任务可展开查看每层（layer）独立进度。

## 开发环境

- Go 版本：1.23+（Docker 构建镜像使用 `golang:1.23-alpine`）
- 前端：Node 18+（推荐 Node 22，与 CI 一致）

### 本地构建

后端（交叉编译示例，产物为 Linux 二进制）：

```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o dockerCopilot .
```

前端：

```bash
cd frontend-react
npm install
npm run build   # 产物输出到 frontend-react/dist
```

### 镜像构建与发布（CI）

- 工作流：`.github/workflows/docker-build-push.yml`
- 多阶段 `docker/Dockerfile`：Node 构建前端 → Go 编译（嵌入前端）→ Alpine 运行时。
- 分支策略：
  - `test` 分支 → 构建 `linux/amd64`，推送 `:test`。
  - `main` 分支 → 并行构建 `linux/amd64` + `linux/arm64`，用 `docker buildx imagetools` 合并为多架构镜像，推送 `:{版本号}` 与 `:latest`。
- 镜像同时推送 Docker Hub 与 GHCR。需在仓库 Secrets 配置 `DOCKERHUB_USERNAME`、`DOCKERHUB_TOKEN`（GHCR 使用内置的 `GITHUB_TOKEN`，无需额外配置）。

