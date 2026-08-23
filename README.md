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

一个主打便捷的docker容器管理工具，现在已经支持所有平台。
已经实现：
1. 一键更新容器
2. 指定镜像和tag更新
3. 启动、停止、重启容器
4. 重命名容器
5. 删除无TAG镜像
6. 删除未使用镜像
7. 更新进度查看
8. 备份容器设置
9. 恢复容器设置

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
    image: 0nlylty/dockercopilot:latest
```

## 新增功能说明（改造版）

以下功能在原版基础上新增，均以当前 Go 后端为主线实现。

### 镜像拉取与更新异步化
- 启动时镜像检查改为后台执行，不再阻塞服务启动和其他页面。
- 容器更新、镜像拉取进入统一任务系统，支持并发上限、进度、失败原因和取消。
- 取消任务：`POST /api/progress/:taskid/cancel`。
- 相关配置见 `etc/dockerCopilot.yaml` 的 `Task` 段（`MaxConcurrent`、`PullTimeoutSec`）。

### 定时更新特定容器
- 通过 `/api/schedules` 管理定时规则，支持 cron、容器选择、仅有更新才更新、跳过无 tag/digest 镜像、保留旧容器和通知开关。
- Docker Hub 等私有仓库凭据通过 `/api/registries` 管理，接口和日志均脱敏，不回显明文。
- 配置持久化到 `/data/config/config.json`，可用 `DOCKERCOPILOT_BOT_CONFIG` 覆盖路径。

### Telegram 机器人
- 通过 `/api/bot/telegram` 配置 Token、白名单 Chat ID、代理和通知开关。
- 支持容器列表查询、启动/停止/重启（inline 二次确认），仅白名单会话可操作。
- 定时更新结果可推送到 Telegram。

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
- 参数编辑通过"停旧→建新→启动→校验→可选删旧"任务化重建，失败自动回滚。

## 开发环境

go版本：1.21+

