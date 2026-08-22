# 构建阶段 - Build Stage
FROM node:18-alpine AS builder

# 设置工作目录
WORKDIR /app

# 复制 package 文件
COPY package*.json ./

# 安装依赖
RUN npm install

# 复制项目文件
COPY . .

# 构建应用
RUN npm run build

# ============================================
# 运行阶段 - Production Stage
FROM node:18-alpine

# 安装轻量级HTTP服务器和必要工具（使用 sh 而非 bash）
RUN npm install -g serve && apk add --no-cache gawk

# 设置工作目录
WORKDIR /app

# 从构建阶段复制构建结果
COPY --from=builder /app/dist ./dist

# 从构建阶段复制配置文件到 dist 目录中，使其可以通过 Web 服务器访问
COPY --from=builder /app/src/config ./dist/config

# 复制配置脚本
COPY docker-config.sh /app/docker-config.sh
RUN chmod +x /app/docker-config.sh

# 暴露端口
EXPOSE 12713

# 健康检查
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --quiet --tries=1 --spider http://localhost:12713/ || exit 1

# 使用配置脚本启动应用
CMD ["/app/docker-config.sh"]