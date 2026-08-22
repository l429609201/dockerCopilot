#!/bin/sh

# 统一的前端配置和启动脚本
# 用途1 (Docker): docker-config.sh               -> 容器启动时运行
# 用途2 (本地): ./docker-config.sh http://... ./dist -> 手动配置 dist

set -e

# 解析参数
if [ $# -eq 0 ]; then
  # Docker 容器内运行模式
  API_BASE_URL="${VITE_API_BASE_URL:-http://localhost}"
  DIST_DIR="/app/dist"
  PORT="${PORT:-12713}"
  START_SERVER=true
  CONFIG_DIR="/app/dist/config"
  IMAGE_CONFIG_DIR="/app/dist/config"
else
  # 本地配置模式
  API_BASE_URL="${1}"
  DIST_DIR="${2:-.}/dist"
  PORT="${3:-12713}"
  START_SERVER=false
  CONFIG_DIR="./src/config"
  IMAGE_CONFIG_DIR="./dist/config"
fi

echo "================================================"
echo "Frontend Configuration & Startup"
echo "================================================"
echo "API Base URL: $API_BASE_URL"
echo "Distribution: $DIST_DIR"
echo "Server Port:  $PORT"
echo "Config Dir:   $CONFIG_DIR"
echo "Image Config: $IMAGE_CONFIG_DIR"
echo "================================================"

# 检查 dist 目录
if [ ! -d "$DIST_DIR" ]; then
  echo "❌ Error: $DIST_DIR not found"
  ls -la "$(dirname "$DIST_DIR")" 2>/dev/null || echo "Parent directory not accessible"
  exit 1
fi

# 检查 index.html
if [ ! -f "${DIST_DIR}/index.html" ]; then
  echo "❌ Error: ${DIST_DIR}/index.html not found"
  echo "Files in dist:"
  ls -la "$DIST_DIR"
  exit 1
fi

# 配置文件已内置到镜像中，无需初始化
echo "✓ Config files are built into the image"

# 检查配置目录（现在在dist/config中）
CONFIG_DIR_CHECK="/app/dist/config"
if [ ! -d "$CONFIG_DIR_CHECK" ]; then
  echo "⚠ Warning: Config directory $CONFIG_DIR_CHECK not found"
else
  echo "✓ Config directory found at $CONFIG_DIR_CHECK"
fi

# 在 index.html 中注入 API 配置
echo "Injecting API configuration..."

TEMP_FILE="${DIST_DIR}/index.html.tmp"

awk -v api_url="$API_BASE_URL" '
  BEGIN { injected = 0 }
  /<head>/ && injected == 0 {
    print $0
    print "<script>"
    print "  window.__API_BASE_URL = '"'"'" api_url "'"'"';"
    print "  console.log('"'"'API configured:'"'"', window.__API_BASE_URL);"
    print "</script>"
    injected = 1
    next
  }
  { print }
' "${DIST_DIR}/index.html" > "$TEMP_FILE"

if [ $? -eq 0 ]; then
  mv "$TEMP_FILE" "${DIST_DIR}/index.html"
  echo "✓ Configuration injected successfully"
else
  echo "⚠ Warning: Failed to inject configuration"
  rm -f "$TEMP_FILE"
  exit 1
fi

# 验证注入
if grep -q "window.__API_BASE_URL" "${DIST_DIR}/index.html"; then
  echo "✓ Injection verified"
else
  echo "⚠ Warning: Could not verify injection"
fi

echo ""

# 如果是本地运行模式，不启动服务器
if [ "$START_SERVER" = "false" ]; then
  echo "✓ Configuration completed. Run your server with:"
  echo "  npx serve -s $DIST_DIR -l $PORT"
  exit 0
fi

# Docker 模式：启动服务器
echo "Starting server on http://0.0.0.0:${PORT}..."
echo "================================================"
echo ""

exec serve -s "$(basename "$DIST_DIR")" -l "$PORT"