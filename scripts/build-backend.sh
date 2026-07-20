#!/usr/bin/env bash
set -euo pipefail

# 将 Go 后端交叉编译为 fnOS 使用的 Linux 可执行文件。
# 用法：./scripts/build-backend.sh [x86|arm] [输出文件]
# 默认：x86，输出 build/liteterm

root=$(cd "$(dirname "$0")/.." && pwd)
platform=${1:-x86}
output=${2:-"${root}/build/liteterm"}

case "${platform}" in
  x86) goarch=amd64 ;;
  arm) goarch=arm64 ;;
  *)
    printf 'Unsupported platform: %s (expected x86 or arm)\n' "${platform}" >&2
    exit 2
    ;;
esac

# 允许从任意工作目录传入相对路径。
if [[ "${output}" != /* ]]; then
  output="${root}/${output}"
fi

# 禁用 CGO，生成无需额外动态库的精简二进制。
mkdir -p "$(dirname "${output}")"
CGO_ENABLED=0 GOOS=linux GOARCH="${goarch}" \
  go build \
    -trimpath \
    -ldflags='-s -w' \
    -o "${output}" \
    "${root}/cmd/liteterm"

# 确保构建产物存在且可以执行。
test -x "${output}"
