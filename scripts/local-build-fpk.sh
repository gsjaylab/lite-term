#!/usr/bin/env bash
set -euo pipefail

# 本地完整构建并使用已安装的 fnpack 生成 FPK。
# 用法：./scripts/local-build-fpk.sh <版本号> [x86|arm]
# 示例：./scripts/local-build-fpk.sh 0.0.1 arm
# 输出：dist/liteterm-<版本号>-<平台>.fpk

root=$(cd "$(dirname "$0")/.." && pwd)
version=${1:-}
platform=${2:-x86}
version=${version#v}
artifact="${root}/dist/liteterm-${version}-${platform}.fpk"

# 版本号必须符合项目使用的语义化版本格式。
if [[ ! "${version}" =~ ^[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ]]; then
  printf 'Usage: %s <version> [x86|arm]\nExample: %s 0.0.1 arm\n' "$0" "$0" >&2
  exit 2
fi

if [[ "${platform}" != x86 && "${platform}" != arm ]]; then
  printf 'Unsupported platform: %s (expected x86 or arm)\n' "${platform}" >&2
  exit 2
fi

# 本地打包依赖用户预先安装 fnpack。
if ! command -v fnpack >/dev/null 2>&1; then
  printf '%s\n' 'fnpack is required and must be available in PATH.' >&2
  exit 1
fi

# 使用与 GitHub Actions 相同的脚本构建并组装应用。
"${root}/scripts/build-web.sh"
"${root}/scripts/build-backend.sh" "${platform}"
"${root}/scripts/assemble-app.sh"
"${root}/scripts/update-manifest.sh" "${version}" "${platform}"

# fnpack 在当前目录生成 FPK，随后统一移动并重命名。
cd "${root}"
rm -f ./*.fpk "${artifact}"
fnpack build --directory package

fpk_file=$(find . -maxdepth 1 -type f -name '*.fpk' -print -quit)
if [[ -z "${fpk_file}" ]]; then
  printf '%s\n' 'fnpack completed without producing an FPK file.' >&2
  exit 1
fi

# 将最终安装包保存到 dist 目录。
mkdir -p "${root}/dist"
mv "${fpk_file}" "${artifact}"
ls -lh "${artifact}"
