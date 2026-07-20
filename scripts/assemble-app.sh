#!/usr/bin/env bash
set -euo pipefail

# 组装 fnpack 所需的应用目录。
# 用法：./scripts/assemble-app.sh [输出目录]
# 默认输出：package/

root=$(cd "$(dirname "$0")/.." && pwd)
output=${1:-"${root}/package"}

# 允许从任意工作目录传入相对路径。
if [[ "${output}" != /* ]]; then
  output="${root}/${output}"
fi

# 前端已经通过 go:embed 写入后端二进制，组装阶段只需要检查最终二进制。
test -x "${root}/build/liteterm"

# 复制 fnOS 包基础结构，并创建应用产物目录。
rm -rf "${output}"
mkdir -p "${output}"
cp -R "${root}/packaging/fnos/." "${output}/"
mkdir -p "${output}/app/bin" "${output}/app/ui/images"

# 写入包含前端资源的后端二进制和桌面图标。
cp "${root}/build/liteterm" "${output}/app/bin/liteterm"
chmod 755 "${output}/app/bin/liteterm"
cp "${root}/packaging/fnos/ICON.PNG" "${output}/app/ui/images/liteterm_icon_64.png"
cp "${root}/packaging/fnos/ICON_256.PNG" "${output}/app/ui/images/liteterm_icon_256.png"
