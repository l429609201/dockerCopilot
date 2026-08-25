# syntax=docker/dockerfile:1
# 多阶段构建：前端(node) -> 后端(go, 嵌入前端产物) -> 运行时(alpine)
# 最终镜像仅包含编译好的二进制与运行时依赖，不含前端源码与 node。

# ---------- 阶段1：构建前端 ----------
FROM node:22-alpine AS frontend
WORKDIR /web
# 先装依赖（利用层缓存）。用 npm install 而非 npm ci：
# 允许 package-lock 未完全同步时自动解析补齐（如新增的 @xterm 依赖）。
COPY frontend-react/package.json frontend-react/package-lock.json ./
RUN npm install
# 再拷源码并构建，产物默认输出到 dist
COPY frontend-react/ ./
RUN npm run build

# ---------- 阶段2：编译后端并嵌入前端 ----------
FROM golang:1.25-alpine AS backend
WORKDIR /src
ARG TARGETARCH
ARG VERSION=v1.0.0
ARG BUILD_DATE=unknown
ENV GOOS=linux CGO_ENABLED=0
# 先下载 Go 依赖
COPY go.mod go.sum ./
RUN go mod download
# 拷贝后端源码
COPY . .
# 将前端构建产物放入 embed 目录 front/（go:embed front/*）
RUN rm -rf front && mkdir -p front
COPY --from=frontend /web/dist/ ./front/
# 校验前端产物存在
RUN test -f front/index.html
# 用 swag 从 handler 注解自动生成 API 文档（swagger.json），放入 swagger-ui/ 供 embed 嵌入
RUN go install github.com/swaggo/swag/cmd/swag@latest \
    && swag init --parseDependency --parseInternal -o docs \
    && cp docs/swagger.json swagger-ui/swagger.json \
    && rm -rf docs
# 编译（按目标架构）
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

RUN apk add --no-cache tzdata \
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