#!/usr/bin/env bash
# YunDu 面板服务自升级脚本（替代人工 SCP 热修复的标准路径）
#
# 用法:
#   sudo bash scripts/panel-upgrade.sh --service=node-service --version=v0.7.17
#   sudo bash scripts/panel-upgrade.sh --all --version=latest
#   sudo bash scripts/panel-upgrade.sh --migrate            # 仅执行数据库迁移
#
# 行为:
#   - 从 GitHub Release 下载指定服务二进制（校验大小后原子替换）
#   - 升级前自动备份当前二进制（/opt/yundu/bin/<name>.bak.<timestamp>）
#   - 替换后 systemctl restart 对应服务
#   - 支持 --migrate 调用 /opt/yundu/bin/migrate up
#
# 与 Agent selfUpgrader 同思路：下载 -> 校验 -> 备份 -> 替换 -> 重启，
# 面板侧零 SSH 升级的最后一公里（本脚本只需在面板机执行一次）。

set -euo pipefail

GITHUB_REPO="${GITHUB_REPO:-energie2008/yundu}"
GITHUB_RELEASES="https://github.com/${GITHUB_REPO}/releases/download"
INSTALL_DIR="${INSTALL_DIR:-/opt/yundu/bin}"

SERVICES=(
    "api-gateway:yundu-api-gateway"
    "identity-service:yundu-identity-service"
    "node-service:yundu-node-service"
    "subscription-service:yundu-subscription-service"
    "traffic-service:yundu-traffic-service"
)

ARCH=""
case "$(uname -m)" in
    x86_64|amd64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) echo "不支持的架构: $(uname -m)" >&2; exit 1 ;;
esac

get_latest_tag() {
    curl -fsSL "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" | grep -o '"tag_name": *"[^"]*"' | head -1 | sed 's/.*"\(.*\)"/\1/'
}

download_binary() {
    local name="$1" version="$2" dest="$3"
    local url="${GITHUB_RELEASES}/${version}/${name}-${ARCH}"
    echo "[INFO] 下载 ${name}-${ARCH} (${version})"
    curl -fSL --connect-timeout 30 --max-time 600 -o "$dest" "$url"
    [ -s "$dest" ] || { echo "[ERROR] 下载内容为空" >&2; rm -f "$dest"; exit 1; }
    chmod +x "$dest"
}

upgrade_one() {
    local bin="$1" svc="$2" version="$3"
    local target="${INSTALL_DIR}/${bin}"
    local new="${target}.new"
    download_binary "$bin" "$version" "$new"
    if [ -f "$target" ]; then
        cp -a "$target" "${target}.bak.$(date +%Y%m%d%H%M%S)"
    fi
    mv "$new" "$target"
    echo "[INFO] ${bin} 已替换，重启 ${svc}"
    systemctl restart "$svc" 2>/dev/null || echo "[WARN] ${svc} 重启失败，请检查 systemd"
}

run_migrate() {
    if [ -x "${INSTALL_DIR}/migrate" ]; then
        echo "[INFO] 执行数据库迁移"
        # 加载面板数据库环境（POSTGRES_DSN 指向 5433 等），并指定迁移目录
        if [ -f "/opt/yundu/config/.env" ]; then
            set -a
            # shellcheck disable=SC1091
            . "/opt/yundu/config/.env"
            set +a
        fi
        export MIGRATIONS_DIR="${MIGRATIONS_DIR:-/opt/yundu/migrations}"
        "${INSTALL_DIR}/migrate" up
    else
        echo "[WARN] 未找到 ${INSTALL_DIR}/migrate，跳过迁移"
    fi
}

main() {
    local version="latest"
    local target=""
    local do_migrate="0"
    for arg in "$@"; do
        case "$arg" in
            --service=*) target="${arg#*=}" ;;
            --version=*) version="${arg#*=}" ;;
            --all)       target="all" ;;
            --migrate)   do_migrate="1" ;;
            *) echo "[ERROR] 未知参数: $arg" >&2; exit 1 ;;
        esac
    done

    [ "$version" = "latest" ] && version=$(get_latest_tag)
    echo "[INFO] 版本: ${version} | 架构: ${ARCH}"

    if [ "$do_migrate" = "1" ]; then
        run_migrate
        exit 0
    fi

    case "$target" in
        all)
            for entry in "${SERVICES[@]}"; do
                upgrade_one "${entry%%:*}" "${entry#*:}" "$version"
            done
            run_migrate
            ;;
        "")
            echo "[ERROR] 请指定 --service=xxx 或 --all" >&2
            exit 1
            ;;
        *)
            local matched=""
            for entry in "${SERVICES[@]}"; do
                if [ "${entry%%:*}" = "$target" ]; then
                    matched="${entry#*:}"
                    break
                fi
            done
            if [ -z "$matched" ]; then
                echo "[ERROR] 未知服务: $target (支持: $(printf '%s ' "${SERVICES[@]%%:*}"))" >&2
                exit 1
            fi
            upgrade_one "$target" "$matched" "$version"
            ;;
    esac
    echo "[OK] 面板升级完成"
}

main "$@"
