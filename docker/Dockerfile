# syntax=docker/dockerfile:1
# 多阶段构建：前端(node) -> 后端(go+swag, 嵌入前端+swagger) -> 运行时(alpine)
# 前端和后端编译阶段强制在构建机原生架构(amd64)上运行（--platform=$BUILDPLATFORM），
# Go 通过 GOARCH=$TARGETARCH 交叉编译输出目标架构二进制。
# 这样 ARM 构建完全避免 QEMU 模拟 node/swag/go，大幅降低构建时间。

# ---------- 阶段1：构建前端（始终在构建机原生架构运行）----------
FROM --platform=$BUILDPLATFORM node:22-alpine AS frontend
WORKDIR /web
# 先装依赖（利用层缓存）。用 npm install 而非 npm ci：
# 允许 package-lock 未完全同步时自动解析补齐（如新增的 @xterm 依赖）。
COPY frontend-react/package.json frontend-react/package-lock.json ./
RUN npm install
# 再拷源码并构建，产物默认输出到 dist（纯静态资源，架构无关）
COPY frontend-react/ ./
RUN npm run build

# ---------- 阶段2：编译后端并嵌入前端（始终在构建机原生架构运行）----------
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS backend
WORKDIR /src
ARG TARGETARCH
ARG VERSION=v1.0.0
ARG BUILD_DATE=unknown
ENV GOOS=linux CGO_ENABLED=0
# 先下载 Go 依赖（架构无关，只下载模块源码）
COPY go.mod go.sum ./
RUN go mod download
# 拷贝后端源码
COPY . .
# 将前端构建产物放入 embed 目录 front/（go:embed front/*）
RUN rm -rf front && mkdir -p front
COPY --from=frontend /web/dist/ ./front/
# 校验前端产物存在
RUN test -f front/index.html
# 用 swag 从 handler 注解自动生成 API 文档（swagger.json），放入 swagger-ui/ 供 embed 嵌入。
# swag 安装和运行都在构建机原生架构上，不走 QEMU。
RUN go install github.com/swaggo/swag/cmd/swag@latest \
    && swag init -g dockercopilot.go --parseDependency --parseInternal -o docs \
    && cp docs/swagger.json swagger-ui/swagger.json \
    && rm -rf docs
# 交叉编译：GOARCH 由 TARGETARCH 驱动，在原生 amd64 上编译出 arm64 二进制（Go 原生支持，无需 QEMU）
RUN GOARCH=${TARGETARCH} go build -a --trimpath \
    -ldflags="-w -s -X 'github.com/l429609201/dockerCopilot/internal/config.Version=${VERSION}' -X 'github.com/l429609201/dockerCopilot/internal/config.BuildDate=${BUILD_DATE}'" \
    -o /out/dockerCopilot .

# ---------- 阶段3：运行时镜像 ----------
FROM alpine
LABEL authors="l429609201 (fork of onlyLTY/dockerCopilot)"
WORKDIR /app

COPY --from=backend /out/dockerCopilot ./dockerCopilot
COPY etc/. ./etc
COPY start.sh ./start.sh

# 安装时区数据、Docker CLI 和 Compose 插件（用于执行 docker compose 命令）
RUN apk add --no-cache tzdata docker-cli docker-cli-compose \
    && chmod +x start.sh \
    && chmod +x dockerCopilot

# 设置环境变量
ENV secretKey="" \
    DOCKER_HOST="unix:///var/run/docker.sock" \
    BACKUP_DIR="/data/backups" \
    TZ="Asia/Shanghai"

VOLUME ["/data"]
# 设置启动命令
CMD ["./start.sh"]