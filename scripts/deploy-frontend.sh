#!/usr/bin/env bash
# YunDu 前端自动部署脚本（零 SSH 收尾：前端随 Release 部署到 nginx 真实站点目录）
#
# 用法:
#   sudo bash scripts/deploy-frontend.sh [--version=v0.7.21] [--admin-root=/path] [--user-root=/path]
#
# 行为:
#   - 从 GitHub Release 下载 admin-web-dist.tar.gz / user-web-dist.tar.gz
#   - 自动探测 nginx 站点根目录（server_name 6.* / 7.*），可用参数覆盖
#   - 部署前自动备份（<root>.bak.<timestamp>），替换 assets/index.html
#   - 回滚：恢复最近的 .bak 目录
set -euo pipefail

GITHUB_REPO="${GITHUB_REPO:-energie2008/yundu}"
GITHUB_RELEASES="https://github.com/${GITHUB_REPO}/releases/download"
VERSION="latest"
ADMIN_ROOT="${YUNDU_ADMIN_WEB_ROOT:-}"
USER_ROOT="${YUNDU_USER_WEB_ROOT:-}"

get_latest_tag() {
    curl -fsSL "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" | grep -o '"tag_name": *"[^"]*"' | head -1 | sed 's/.*"\(.*\)"/\1/'
}

detect_root() {
    local pattern="$1"
    nginx -T 2>/dev/null | awk -v p="$pattern" '$0 ~ ("server_name[[:space:]]+" p){found=1} found && /root[[:space:]]/{print $2; exit}' | tr -d ';'
}

deploy_one() {
    local name="$1" root="$2"
    [ -z "$root" ] && { echo "[WARN] 未指定 ${name} 站点根目录，跳过"; return; }
    local tmp="/tmp/yundu-${name}-dist.tar.gz"
    local url="${GITHUB_RELEASES}/${VERSION}/${name}-dist.tar.gz"
    echo "[INFO] 下载 ${name}-dist.tar.gz (${VERSION})"
    curl -fsSL --connect-timeout 30 --max-time 600 -o "$tmp" "$url"
    if [ -d "$root" ]; then
        cp -a "$root" "${root}.bak.$(date +%Y%m%d%H%M%S)"
    fi
    mkdir -p "$root"
    rm -rf "$root/assets" "$root/index.html" 2>/dev/null || true
    tar -xzf "$tmp" -C "$root"
    chown -R www:www "$root" 2>/dev/null || true
    echo "[OK] ${name} 已部署到 ${root}"
}

for arg in "$@"; do
    case "$arg" in
        --version=*) VERSION="${arg#*=}" ;;
        --admin-root=*) ADMIN_ROOT="${arg#*=}" ;;
        --user-root=*) USER_ROOT="${arg#*=}" ;;
        *) echo "[ERROR] 未知参数: $arg" >&2; exit 1 ;;
    esac
done

[ "$VERSION" = "latest" ] && VERSION=$(get_latest_tag)

if [ -z "$ADMIN_ROOT" ]; then ADMIN_ROOT=$(detect_root '6[.]tiktokplay'); fi
if [ -z "$USER_ROOT" ]; then USER_ROOT=$(detect_root '7[.]tiktokplay'); fi

deploy_one "admin-web" "$ADMIN_ROOT"
deploy_one "user-web" "$USER_ROOT"
echo "[OK] 前端部署完成（版本 ${VERSION}）"
