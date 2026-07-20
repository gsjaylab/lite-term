#!/usr/bin/env bash
set -euo pipefail

# 安装前端依赖、构建生产资源，并复制到 Go embed 目录。
# 用法：./scripts/build-web.sh
# 输出：web/dist/ 和 internal/web/dist/

root=$(cd "$(dirname "$0")/.." && pwd)

# 使用锁文件安装依赖并执行 TypeScript/Vite 生产构建。
npm ci --prefix "${root}/web"
npm run build --prefix "${root}/web"

# 刷新 Go 后端需要嵌入的前端资源。
find "${root}/internal/web/dist" -mindepth 1 ! -name .gitkeep -delete
cp -R "${root}/web/dist/." "${root}/internal/web/dist/"

# 确保生产资源使用 fnOS 应用网关路径。
grep -Eq '/app/liteterm/assets/index-[^"[:space:]]+\.js' \
  "${root}/internal/web/dist/index.html"
