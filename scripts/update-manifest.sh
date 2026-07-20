#!/usr/bin/env bash
set -euo pipefail

# 更新待打包 manifest 中的版本号、平台及架构说明。
# 用法：./scripts/update-manifest.sh <版本号> [平台] [manifest 路径]
# 默认平台：x86
# 默认文件：package/manifest

root=$(cd "$(dirname "$0")/.." && pwd)
version=${1:-}
platform=${2:-x86}
manifest=${3:-"${root}/package/manifest"}
version=${version#v}

# 校验版本号、平台和目标文件，避免生成无效安装包。
if [[ ! "${version}" =~ ^[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ]]; then
  printf 'Invalid version: %s\n' "${version}" >&2
  exit 2
fi

case "${platform}" in
  x86) artifact_arch=x86_64 ;;
  arm) artifact_arch=aarch64 ;;
  *)
    printf 'Unsupported platform: %s (expected x86 or arm)\n' "${platform}" >&2
    exit 2
    ;;
esac

if [[ ! -f "${manifest}" ]]; then
  printf 'Manifest not found: %s\n' "${manifest}" >&2
  exit 1
fi

# 保留 manifest 原有对齐格式；架构注释用于人工检查 FPK 内容。
sed -E \
  -e "s/^version[[:space:]]*=.*/version               = ${version}/" \
  -e "s/^platform[[:space:]]*=.*/platform              = ${platform}/" \
	-e "s/^# Package artifact architecture:.*/# Package artifact architecture: arch=\"${artifact_arch}\"/" \
  "${manifest}" > "${manifest}.tmp"
mv "${manifest}.tmp" "${manifest}"

# 确认两个字段都已成功写入。
grep -Eq "^version[[:space:]]*=[[:space:]]*${version}$" "${manifest}"
grep -Eq "^platform[[:space:]]*=[[:space:]]*${platform}$" "${manifest}"
grep -Fqx "# Package artifact architecture: arch=\"${artifact_arch}\"" "${manifest}"
