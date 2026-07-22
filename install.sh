#!/bin/bash
# YunDu 一键安装脚本
#
# 用法:
#   1. 安装 node-agent（节点端，部署在 VPS 节点上）:
#      curl -fsSL https://github.com/energie2008/yundu/raw/main/install.sh | bash -s -- agent --token=AGENT_TOKEN --endpoint=https://panel.example.com
#
#   2. 安装面板（控制端，部署在面板服务器上）:
#      curl -fsSL https://github.com/energie2008/yundu/raw/main/install.sh | bash -s -- panel
#
#   3. 仅下载二进制:
#      curl -fsSL https://github.com/energie2008/yundu/raw/main/install.sh | bash -s -- download --version=v0.3.0 --bin=node-agent --dest=/tmp
#
#   4. 升级 node-agent 到最新版:
#      curl -fsSL https://github.com/energie2008/yundu/raw/main/install.sh | bash -s -- upgrade agent
#
#   5. 升级面板组件到最新版:
#      curl -fsSL https://github.com/energie2008/yundu/raw/main/install.sh | bash -s -- upgrade panel
#
# 子命令:
#   agent     安装节点代理（node-agent + xray/sing-box 内核）
#   panel     安装面板服务（node-service/api-gateway/identity/subscription/traffic + migrate）
#   upgrade   升级已安装组件（agent|panel|all）
#   download  仅下载二进制到指定目录

set -euo pipefail

# ===== 配置 =====
GITHUB_REPO="energie2008/yundu"
GITHUB_API="https://api.github.com/repos/${GITHUB_REPO}"
GITHUB_RELEASES="https://github.com/${GITHUB_REPO}/releases"
INSTALL_DIR="/opt/yundu/bin"
CONFIG_DIR="/etc/yundu"
LOG_DIR="/var/log/yundu"
SERVICE_DIR="/etc/systemd/system"

# ===== 颜色输出 =====
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

info()    { echo -e "${GREEN}[INFO]${NC}  $*"; }
warn()    { echo -e "${YELLOW}[WARN]${NC}  $*"; }
error()   { echo -e "${RED}[ERROR]${NC} $*" >&2; }
success() { echo -e "${GREEN}${BOLD}[OK]${NC}    $*"; }
header()  { echo -e "${BLUE}${BOLD}==>${NC}    $*"; }

print_banner() {
    echo -e "${CYAN}"
    echo "========================================"
    echo "     YunDu 一键安装脚本"
    echo "     github.com/energie2008/yundu"
    echo "========================================"
    echo -e "${NC}"
}

# ===== 工具函数 =====

# 检测 root 权限
check_root() {
    if [ "$(id -u)" -ne 0 ]; then
        error "请使用 root 用户运行此脚本 (使用 sudo)"
        exit 1
    fi
}

# 检测操作系统
detect_os() {
    if [ -f /etc/os-release ]; then
        . /etc/os-release
        OS="$ID"
        OS_FAMILY="${ID_LIKE:-}"
    else
        OS=$(uname -s | tr '[:upper:]' '[:lower:]')
        OS_FAMILY="unknown"
    fi
    case "$OS" in
        ubuntu|debian|raspbian) OS_FAMILY="debian" ;;
        centos|rhel|fedora|rocky|alma|ol) OS_FAMILY="rhel" ;;
        alpine) OS_FAMILY="alpine" ;;
    esac
    info "系统: ${OS} (${OS_FAMILY})"
}

# 检测架构
detect_arch() {
    local arch
    arch=$(uname -m)
    case "$arch" in
        x86_64|amd64)   ARCH="amd64" ;;
        aarch64|arm64)  ARCH="arm64" ;;
        *)
            error "不支持的架构: $arch (仅支持 amd64/arm64)"
            exit 1
            ;;
    esac
    info "架构: ${ARCH}"
}

# 安装依赖
install_deps() {
    local deps=()
    for cmd in curl tar jq; do
        command -v "$cmd" >/dev/null 2>&1 || deps+=("$cmd")
    done
    [ ${#deps[@]} -eq 0 ] && return 0

    header "安装依赖: ${deps[*]}"
    case "$OS_FAMILY" in
        debian)
            export DEBIAN_FRONTEND=noninteractive
            apt-get update -qq && apt-get install -y -qq "${deps[@]}" ca-certificates
            ;;
        rhel)
            if command -v dnf >/dev/null 2>&1; then
                dnf install -y -q "${deps[@]}"
            else
                yum install -y -q "${deps[@]}"
            fi
            ;;
        alpine)
            apk add --no-cache "${deps[@]}"
            ;;
        *)
            error "无法自动安装依赖: ${deps[*]}，请手动安装"
            exit 1
            ;;
    esac
    success "依赖安装完成"
}

# 获取最新 release tag
get_latest_tag() {
    local tag
    tag=$(curl -fsSL "${GITHUB_API}/releases/latest" 2>/dev/null | jq -r '.tag_name // empty')
    if [ -z "$tag" ] || [ "$tag" = "null" ]; then
        error "无法获取最新版本（仓库可能还未发布 release）"
        error "请先在 GitHub 上创建 release，或指定 --version=<tag>"
        exit 1
    fi
    echo "$tag"
}

# 下载二进制
# 参数: $1=二进制名(node-agent/node-service/...), $2=版本tag, $3=目标路径
download_binary() {
    local name="$1"
    local version="$2"
    local dest="$3"
    local filename="${name}-${ARCH}"
    local url="${GITHUB_RELEASES}/download/${version}/${filename}"

    header "下载 ${name} ${version} (${ARCH})"
    info "URL: ${url}"

    local tmp="${dest}.tmp"
    if ! curl -fSL --connect-timeout 30 --max-time 300 -o "$tmp" "$url"; then
        error "下载失败: ${url}"
        rm -f "$tmp"
        return 1
    fi
    [ ! -s "$tmp" ] && { error "下载文件为空: ${url}"; rm -f "$tmp"; return 1; }
    chmod +x "$tmp"
    mv "$tmp" "$dest"
    success "已下载: ${dest} ($(du -h "$dest" | cut -f1))"
}

# 安装 systemd 服务
# 参数: $1=服务名, $2=ExecStart, $3=描述
install_systemd() {
    local name="$1"
    local exec_start="$2"
    local desc="$3"
    local unit="${SERVICE_DIR}/${name}.service"

    cat > "$unit" << EOF
[Unit]
Description=${desc}
After=network-online.target nss-lookup.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=${exec_start}
Restart=always
RestartSec=5
LimitNOFILE=65536
LimitNPROC=4096
StandardOutput=journal
StandardError=journal
SyslogIdentifier=${name}
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=${LOG_DIR} ${CONFIG_DIR} ${INSTALL_DIR}
PrivateTmp=true

[Install]
WantedBy=multi-user.target
EOF
    systemctl daemon-reload
    success "systemd 服务: ${unit}"
}

# ===== 子命令: agent =====

cmd_agent() {
    local token=""
    local endpoint=""
    local runtime="xray"
    local version="latest"

    for arg in "$@"; do
        case "$arg" in
            --token=*)     token="${arg#*=}" ;;
            --endpoint=*)  endpoint="${arg#*=}" ;;
            --runtime=*)   runtime="${arg#*=}" ;;
            --version=*)   version="${arg#*=}" ;;
            *) warn "未知参数: $arg" ;;
        esac
    done

    [ -z "$token" ] && { error "缺少 --token 参数"; exit 1; }
    [ -z "$endpoint" ] && { error "缺少 --endpoint 参数"; exit 1; }
    endpoint="${endpoint%/}"

    [ "$version" = "latest" ] && version=$(get_latest_tag)

    header "安装 node-agent ${version}"
    mkdir -p "$INSTALL_DIR" "$CONFIG_DIR" "$LOG_DIR" "${CONFIG_DIR}/config" "${CONFIG_DIR}/certs"
    chmod 700 "$CONFIG_DIR"

    download_binary "node-agent" "$version" "${INSTALL_DIR}/node-agent"

    # 安装 xray 内核（agent 内嵌 sing-box，但 xray 需外部安装或由 agent 拉取）
    if [ "$runtime" = "xray" ] && ! command -v xray >/dev/null 2>&1; then
        header "安装 xray-core"
        if bash -c "$(curl -fsSL https://github.com/XTLS/Xray-install/raw/main/install-release.sh)" @ install; then
            success "xray 安装完成"
        else
            warn "xray 安装失败，agent 启动后会自动拉取"
        fi
    fi

    install_systemd "yundu-node-agent" \
        "${INSTALL_DIR}/node-agent --endpoint=${endpoint} --token=${token} --runtime=${runtime} --config-dir=${CONFIG_DIR}" \
        "YunDu Node Agent - 节点代理服务"

    systemctl enable yundu-node-agent
    systemctl restart yundu-node-agent
    sleep 3

    if systemctl is-active --quiet yundu-node-agent; then
        echo ""
        success "node-agent 安装完成并已启动"
        echo ""
        echo -e "  ${BOLD}版本:${NC}     ${version}"
        echo -e "  ${BOLD}路径:${NC}     ${INSTALL_DIR}/node-agent"
        echo -e "  ${BOLD}配置:${NC}     ${CONFIG_DIR}"
        echo -e "  ${BOLD}面板:${NC}     ${endpoint}"
        echo -e "  ${BOLD}内核:${NC}     ${runtime}"
        echo ""
        echo -e "  ${CYAN}常用命令:${NC}"
        echo -e "    systemctl status yundu-node-agent"
        echo -e "    journalctl -u yundu-node-agent -f"
        echo -e "    systemctl restart yundu-node-agent"
        echo ""
    else
        error "服务启动失败，查看日志: journalctl -u yundu-node-agent -n 50"
        exit 1
    fi
}

# ===== 子命令: panel =====

cmd_panel() {
    local version="latest"
    local db_url=""
    local redis_url=""

    for arg in "$@"; do
        case "$arg" in
            --version=*)     version="${arg#*=}" ;;
            --db-url=*)      db_url="${arg#*=}" ;;
            --redis-url=*)   redis_url="${arg#*=}" ;;
            *) warn "未知参数: $arg" ;;
        esac
    done

    [ "$version" = "latest" ] && version=$(get_latest_tag)

    header "安装 YunDu 面板 ${version}"
    mkdir -p "$INSTALL_DIR" "$CONFIG_DIR" "$LOG_DIR"
    chmod 700 "$CONFIG_DIR"

    local services=(
        "api-gateway:8080:API 网关"
        "identity-service:8081:认证/用户/订单服务"
        "node-service:8082:节点服务"
        "subscription-service:8083:订阅服务"
        "traffic-service:8084:流量统计服务"
    )

    for svc in "${services[@]}"; do
        local name="${svc%%:*}"
        local rest="${svc#*:}"
        local port="${rest%%:*}"
        local desc="${rest#*:}"
        header "安装 ${name} (${desc})"
        download_binary "$name" "$version" "${INSTALL_DIR}/${name}"
        install_systemd "yundu-${name}" \
            "${INSTALL_DIR}/${name}" \
            "YunDu ${desc}"
        systemctl enable "yundu-${name}"
    done

    # 安装 migrate 工具
    header "安装 migrate 工具"
    download_binary "migrate" "$version" "${INSTALL_DIR}/migrate"

    # 写入环境配置（若提供）
    if [ -n "$db_url" ] || [ -n "$redis_url" ]; then
        cat > "${CONFIG_DIR}/panel.env" << EOF
YUNDU_DB_URL=${db_url}
YUNDU_REDIS_URL=${redis_url}
EOF
        chmod 600 "${CONFIG_DIR}/panel.env"
        info "环境配置已写入: ${CONFIG_DIR}/panel.env"
    fi

    # 提示数据库迁移
    echo ""
    header "下一步: 数据库迁移"
    echo -e "  面板服务启动前需先执行数据库迁移:"
    echo -e "    ${GREEN}${INSTALL_DIR}/migrate up${NC}"
    echo ""

    # 启动所有服务
    header "启动面板服务"
    for svc in "${services[@]}"; do
        local name="${svc%%:*}"
        systemctl restart "yundu-${name}" || warn "${name} 启动失败"
    done
    sleep 3

    echo ""
    success "面板安装完成"
    echo ""
    echo -e "  ${BOLD}版本:${NC}     ${version}"
    echo -e "  ${BOLD}安装目录:${NC} ${INSTALL_DIR}"
    echo -e "  ${BOLD}配置目录:${NC} ${CONFIG_DIR}"
    echo ""
    echo -e "  ${CYAN}服务列表:${NC}"
    for svc in "${services[@]}"; do
        local name="${svc%%:*}"
        local rest="${svc#*:}"
        local port="${rest%%:*}"
        echo -e "    yundu-${name}  :${port}"
    done
    echo ""
    echo -e "  ${CYAN}常用命令:${NC}"
    echo -e "    systemctl status 'yundu-*'"
    echo -e "    journalctl -u yundu-node-service -f"
    echo ""
    warn "请先执行数据库迁移: ${INSTALL_DIR}/migrate up"
    warn "并在 ${CONFIG_DIR}/panel.env 配置 DB/Redis 连接"
}

# ===== 子命令: download =====

cmd_download() {
    local version="latest"
    local bin="node-agent"
    local dest="/tmp"

    for arg in "$@"; do
        case "$arg" in
            --version=*) version="${arg#*=}" ;;
            --bin=*)     bin="${arg#*=}" ;;
            --dest=*)    dest="${arg#*=}" ;;
            *) warn "未知参数: $arg" ;;
        esac
    done

    [ "$version" = "latest" ] && version=$(get_latest_tag)
    mkdir -p "$dest"
    download_binary "$bin" "$version" "${dest}/${bin}-${ARCH}"
    success "下载完成: ${dest}/${bin}-${ARCH}"
}

# ===== 子命令: upgrade =====

cmd_upgrade() {
    local target="${1:-all}"
    local version="latest"
    shift 2>/dev/null || true

    for arg in "$@"; do
        case "$arg" in
            --version=*) version="${arg#*=}" ;;
            *) warn "未知参数: $arg" ;;
        esac
    done

    [ "$version" = "latest" ] && version=$(get_latest_tag)

    case "$target" in
        agent)
            header "升级 node-agent 到 ${version}"
            download_binary "node-agent" "$version" "${INSTALL_DIR}/node-agent.new"
            mv "${INSTALL_DIR}/node-agent.new" "${INSTALL_DIR}/node-agent"
            chmod +x "${INSTALL_DIR}/node-agent"
            systemctl restart yundu-node-agent 2>/dev/null || true
            success "node-agent 升级完成并已重启"
            ;;
        panel)
            header "升级面板组件到 ${version}"
            local bins=(api-gateway identity-service node-service subscription-service traffic-service migrate)
            for b in "${bins[@]}"; do
                download_binary "$b" "$version" "${INSTALL_DIR}/${b}.new"
                mv "${INSTALL_DIR}/${b}.new" "${INSTALL_DIR}/${b}"
                chmod +x "${INSTALL_DIR}/${b}"
            done
            # 重启所有面板服务
            for b in api-gateway identity-service node-service subscription-service traffic-service; do
                systemctl restart "yundu-${b}" 2>/dev/null || true
            done
            success "面板升级完成，所有服务已重启"
            ;;
        all)
            cmd_upgrade agent "$@"
            cmd_upgrade panel "$@"
            ;;
        *)
            error "未知升级目标: ${target} (支持: agent|panel|all)"
            exit 1
            ;;
    esac
}

# ===== 主入口 =====

main() {
    check_root
    print_banner
    detect_os
    detect_arch
    install_deps

    local subcmd="${1:-help}"
    shift || true

    case "$subcmd" in
        agent)    cmd_agent "$@" ;;
        panel)    cmd_panel "$@" ;;
        upgrade)  cmd_upgrade "$@" ;;
        download) cmd_download "$@" ;;
        help|-h|--help)
            cat << 'EOF'
YunDu 一键安装脚本

用法:
  install.sh <子命令> [参数]

子命令:
  agent      安装节点代理（部署在 VPS 节点上）
  panel      安装面板服务（部署在面板服务器上）
  upgrade    升级已安装组件 (agent|panel|all)
  download   仅下载二进制

示例:
  # 安装 node-agent
  install.sh agent --token=ABC123 --endpoint=https://panel.example.com

  # 安装面板
  install.sh panel --db-url=postgres://... --redis-url=redis://...

  # 升级 node-agent
  install.sh upgrade agent

  # 升级全部
  install.sh upgrade all

  # 仅下载
  install.sh download --bin=node-agent --version=v0.3.0 --dest=/tmp

完整文档: https://github.com/energie2008/yundu
EOF
            ;;
        *)
            error "未知子命令: $subcmd"
            echo "运行 'install.sh help' 查看用法"
            exit 1
            ;;
    esac
}

main "$@"
