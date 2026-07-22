#!/bin/bash
# YunDu Node Agent 一键安装脚本（零配置部署版）
#
# 用法:
#   curl -fsSL https://panel.example.com/install.sh | bash -s -- --token=AGENT_TOKEN --endpoint=https://panel.example.com
#
# 参数:
#   --token=TOKEN        Agent 认证 token（必填）
#   --endpoint=URL       面板 URL（必填）
#   --runtime=TYPE       运行时内核: xray(默认) / sing-box
#   --config-dir=DIR     配置目录，默认 /etc/yundu
#   --install-dir=DIR    二进制安装目录，默认 /usr/local/bin
#   --version=VER        Agent 版本号，默认 latest
#   --uninstall          卸载 node-agent
#
# Agent 启动后通过 Bootstrap API（GET /api/v1/agent/bootstrap?token=xxx）
# 自动从面板拉取 runtime_bin、config_dir、节点列表等完整配置，
# 无需手动配置 SERVER_CODE / RUNTIME_PATH 等环境变量。

set -euo pipefail

# ===== 默认参数 =====
TOKEN=""
ENDPOINT=""
RUNTIME="xray"
CONFIG_DIR="/etc/yundu"
INSTALL_DIR="/usr/local/bin"
VERSION="latest"
UNINSTALL_MODE=false

SERVICE_NAME="node-agent"
BIN_PATH="${INSTALL_DIR}/node-agent"
LOG_DIR="/var/log/yundu"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"

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
    echo "     YunDu Node Agent 一键安装脚本"
    echo "     (零配置部署版 - Bootstrap API)"
    echo "========================================"
    echo -e "${NC}"
}

print_usage() {
    cat << 'EOF'
用法:
  install-agent.sh [选项]

必填参数:
  --token=TOKEN        Agent 认证 token
  --endpoint=URL       面板 URL，例如 https://panel.example.com

可选参数:
  --runtime=TYPE       运行时内核: xray(默认) / sing-box
  --config-dir=DIR     配置目录，默认 /etc/yundu
  --install-dir=DIR    二进制安装目录，默认 /usr/local/bin
  --version=VER        Agent 版本号，默认 latest
  --uninstall          卸载 node-agent
  -h, --help           显示帮助信息

示例:
  # 零配置安装（3 个参数即可启动）
  curl -fsSL https://panel.example.com/install.sh | bash -s -- \
    --token=AGENT_TOKEN --endpoint=https://panel.example.com

  # 指定 sing-box 内核
  curl -fsSL https://panel.example.com/install.sh | bash -s -- \
    --token=AGENT_TOKEN --endpoint=https://panel.example.com --runtime=sing-box

  # 卸载
  curl -fsSL https://panel.example.com/install.sh | bash -s -- --uninstall
EOF
}

# ===== 参数解析 =====
parse_args() {
    for arg in "$@"; do
        case "$arg" in
            --token=*)
                TOKEN="${arg#*=}"
                ;;
            --endpoint=*)
                ENDPOINT="${arg#*=}"
                ;;
            --runtime=*)
                RUNTIME="${arg#*=}"
                ;;
            --config-dir=*)
                CONFIG_DIR="${arg#*=}"
                ;;
            --install-dir=*)
                INSTALL_DIR="${arg#*=}"
                BIN_PATH="${INSTALL_DIR}/node-agent"
                ;;
            --version=*)
                VERSION="${arg#*=}"
                ;;
            --uninstall)
                UNINSTALL_MODE=true
                ;;
            -h|--help)
                print_usage
                exit 0
                ;;
            *)
                warn "未知参数: $arg"
                ;;
        esac
    done

    # 规范化 endpoint（去掉尾部斜杠）
    ENDPOINT="${ENDPOINT%/}"
}

# ===== 系统检测 =====
detect_os() {
    header "检测操作系统..."

    OS=""
    OS_FAMILY=""

    if [ -f /etc/os-release ]; then
        . /etc/os-release
        OS="$ID"
        OS_FAMILY="$ID_LIKE"
    elif [ -f /etc/alpine-release ]; then
        OS="alpine"
        OS_FAMILY="alpine"
    else
        OS=$(uname -s | tr '[:upper:]' '[:lower:]')
        OS_FAMILY="unknown"
    fi

    case "$OS" in
        ubuntu|debian|raspbian)
            OS_FAMILY="debian"
            info "检测到系统: ${BOLD}${OS}${NC} (Debian系)"
            ;;
        centos|rhel|fedora|rocky|alma|ol)
            OS_FAMILY="rhel"
            info "检测到系统: ${BOLD}${OS}${NC} (RHEL系)"
            ;;
        alpine)
            OS_FAMILY="alpine"
            info "检测到系统: ${BOLD}Alpine Linux${NC}"
            ;;
        *)
            warn "未知系统: ${OS}，将尝试通用方式继续..."
            ;;
    esac

    if command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
        info "检测到 ${BOLD}systemd${NC} 服务管理器"
    else
        warn "未检测到 systemd，请手动配置服务启动"
    fi
}

detect_arch() {
    header "检测系统架构..."
    local arch
    arch=$(uname -m)
    case "$arch" in
        x86_64|amd64)
            ARCH="amd64"
            ;;
        aarch64|arm64|armv8*|armv9*)
            ARCH="arm64"
            ;;
        *)
            error "不支持的架构: $arch"
            error "目前仅支持 amd64 (x86_64) 和 arm64 (aarch64)"
            exit 1
            ;;
    esac
    info "检测到架构: ${BOLD}${ARCH}${NC}"
}

# ===== 依赖安装 =====
install_deps() {
    header "检查依赖..."

    local need_install=false
    local deps=()

    for cmd in curl tar; do
        if ! command -v "$cmd" >/dev/null 2>&1; then
            deps+=("$cmd")
            need_install=true
        fi
    done

    if [ "$need_install" = false ]; then
        success "所有依赖已安装 (curl, tar)"
        return
    fi

    info "需要安装: ${deps[*]}"

    case "$OS_FAMILY" in
        debian)
            export DEBIAN_FRONTEND=noninteractive
            apt-get update -qq
            apt-get install -y -qq "${deps[@]}" ca-certificates
            ;;
        rhel)
            if command -v dnf >/dev/null 2>&1; then
                dnf install -y -q "${deps[@]}" ca-certificates
            else
                yum install -y -q "${deps[@]}" ca-certificates
            fi
            ;;
        alpine)
            apk add --no-cache "${deps[@]}" ca-certificates
            ;;
        *)
            error "无法自动安装依赖，请手动安装: ${deps[*]}"
            exit 1
            ;;
    esac
    success "依赖安装完成"
}

# ===== 下载 Agent 二进制 =====
download_agent() {
    header "下载 node-agent 二进制文件..."

    local tmp_dir
    tmp_dir=$(mktemp -d)
    trap 'rm -rf "$tmp_dir"' EXIT

    local download_url=""
    local bin_name="node-agent-linux-${ARCH}"

    # 优先从面板下载，回退到 GitHub releases
    if [ "$VERSION" != "latest" ]; then
        download_url="${ENDPOINT}/downloads/node-agent/${VERSION}/${bin_name}"
    else
        download_url="${ENDPOINT}/install/${bin_name}"
    fi

    info "下载地址: ${download_url}"

    local bin_tmp="${tmp_dir}/node-agent"
    local dl_ok=false

    if curl -fsSL --connect-timeout 30 --max-time 300 "$download_url" -o "$bin_tmp"; then
        dl_ok=true
    else
        # 回退到 GitHub releases
        local gh_url="https://github.com/airport-panel/yundu/releases/download/v0.1.0/${bin_name}"
        warn "面板下载失败，尝试 GitHub releases: ${gh_url}"
        if curl -fsSL --connect-timeout 30 --max-time 300 "$gh_url" -o "$bin_tmp"; then
            dl_ok=true
        fi
    fi

    if [ "$dl_ok" = false ]; then
        error "下载失败！请检查："
        error "  1. 网络连接是否正常"
        error "  2. 面板地址是否正确: ${ENDPOINT}"
        error "  3. 或手动下载二进制文件放置到 ${BIN_PATH}"
        exit 1
    fi

    if [ ! -s "$bin_tmp" ]; then
        error "下载的文件为空"
        exit 1
    fi

    chmod +x "$bin_tmp"

    # 确保安装目录存在
    mkdir -p "$INSTALL_DIR"

    info "安装到 ${BIN_PATH}..."
    install -m 755 "$bin_tmp" "$BIN_PATH"

    if [ ! -x "$BIN_PATH" ]; then
        error "二进制安装失败或不可执行"
        exit 1
    fi

    success "node-agent 安装成功: ${BIN_PATH}"
}

# ===== 下载内核二进制 =====
download_runtime() {
    header "安装运行时内核 (${RUNTIME})..."

    case "$RUNTIME" in
        xray)
            install_xray
            ;;
        sing-box)
            install_singbox
            ;;
        *)
            warn "未知运行时类型: ${RUNTIME}，跳过内核安装"
            ;;
    esac
}

install_xray() {
    if command -v xray >/dev/null 2>&1; then
        success "Xray 已安装: $(xray version 2>&1 | head -1)"
        return
    fi

    info "正在安装 Xray-core..."
    if ! bash -c "$(curl -fsSL https://github.com/XTLS/Xray-install/raw/main/install-release.sh)" @ install; then
        warn "Xray 安装失败，请手动安装"
        warn "Agent 启动后可通过 BinaryReconciler 自动拉取"
        return
    fi
    success "Xray 安装完成"
}

install_singbox() {
    if command -v sing-box >/dev/null 2>&1; then
        success "sing-box 已安装: $(sing-box version 2>&1 | head -1)"
        return
    fi

    info "正在安装 sing-box..."
    local SB_VERSION="1.13.14"
    local SB_ARCH=""
    case "$ARCH" in
        amd64) SB_ARCH="amd64" ;;
        arm64) SB_ARCH="arm64" ;;
    esac

    local tmp_dir
    tmp_dir=$(mktemp -d)

    local sb_url="https://github.com/SagerNet/sing-box/releases/download/v${SB_VERSION}/sing-box-${SB_VERSION}-linux-${SB_ARCH}.tar.gz"

    info "下载 sing-box v${SB_VERSION}..."
    if ! curl -fsSL "$sb_url" | tar xz -C "$tmp_dir"; then
        warn "sing-box 下载失败，请手动安装"
        rm -rf "$tmp_dir"
        return
    fi

    install -m 755 "$tmp_dir"/sing-box-*/sing-box /usr/local/bin/sing-box
    rm -rf "$tmp_dir"
    success "sing-box 安装完成: $(sing-box version 2>&1 | head -1)"
}

# ===== 创建配置目录 =====
create_dirs() {
    header "创建配置目录..."

    mkdir -p "$CONFIG_DIR" "$LOG_DIR"
    mkdir -p "${CONFIG_DIR}/certs" "${CONFIG_DIR}/config"
    chmod 700 "$CONFIG_DIR"

    success "配置目录: ${CONFIG_DIR}"
    success "日志目录: ${LOG_DIR}"
}

# ===== 生成 systemd unit =====
create_systemd_service() {
    header "创建 systemd 服务..."

    # 零配置部署：systemd unit 直接使用 CLI flags 启动 agent
    # Agent 启动后通过 Bootstrap API 自动拉取完整配置
    cat > "$SERVICE_FILE" << EOF
[Unit]
Description=YunDu Node Agent - 机场节点代理服务（零配置部署版）
Documentation=https://github.com/airport-panel/yundu
After=network-online.target nss-lookup.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=${BIN_PATH} --endpoint=${ENDPOINT} --token=${TOKEN} --runtime=${RUNTIME} --config-dir=${CONFIG_DIR}
Restart=always
RestartSec=5
LimitNOFILE=65536
LimitNPROC=4096
StandardOutput=journal
StandardError=journal
SyslogIdentifier=${SERVICE_NAME}

# 安全加固
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=${LOG_DIR} ${CONFIG_DIR}
PrivateTmp=true

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload
    info "启用开机自启..."
    systemctl enable "$SERVICE_NAME"

    info "启动服务..."
    systemctl restart "$SERVICE_NAME" || {
        error "服务启动失败！请检查日志："
        error "  journalctl -u ${SERVICE_NAME} -n 50 --no-pager"
        exit 1
    }

    sleep 2

    if systemctl is-active --quiet "$SERVICE_NAME"; then
        success "服务启动成功"
    else
        warn "服务可能未正常运行，请检查状态"
        warn "  systemctl status ${SERVICE_NAME}"
    fi
}

# ===== 卸载 =====
uninstall() {
    header "卸载 node-agent..."

    if [ -f "$SERVICE_FILE" ]; then
        info "停止并禁用服务..."
        systemctl stop "$SERVICE_NAME" 2>/dev/null || true
        systemctl disable "$SERVICE_NAME" 2>/dev/null || true
        rm -f "$SERVICE_FILE"
        systemctl daemon-reload
    fi

    if [ -f "$BIN_PATH" ]; then
        info "删除二进制文件: ${BIN_PATH}"
        rm -f "$BIN_PATH"
    fi

    if [ -d "$CONFIG_DIR" ]; then
        warn "配置目录保留: ${CONFIG_DIR} (如需删除请手动执行: rm -rf ${CONFIG_DIR})"
    fi
    if [ -d "$LOG_DIR" ]; then
        warn "日志目录保留: ${LOG_DIR} (如需删除请手动执行: rm -rf ${LOG_DIR})"
    fi

    success "node-agent 已卸载"
}

# ===== 安装完成提示 =====
print_success_info() {
    echo ""
    echo -e "${GREEN}${BOLD}============================================================${NC}"
    echo -e "${GREEN}${BOLD}        node-agent 安装完成！（零配置部署）${NC}"
    echo -e "${GREEN}${BOLD}============================================================${NC}"
    echo ""
    echo -e "  ${BOLD}服务名称:${NC}     ${SERVICE_NAME}"
    echo -e "  ${BOLD}安装路径:${NC}     ${BIN_PATH}"
    echo -e "  ${BOLD}配置目录:${NC}     ${CONFIG_DIR}"
    echo -e "  ${BOLD}日志目录:${NC}     ${LOG_DIR}"
    echo -e "  ${BOLD}运行时:${NC}       ${RUNTIME}"
    echo -e "  ${BOLD}面板地址:${NC}     ${ENDPOINT}"
    echo ""
    echo -e "  ${BOLD}启动参数:${NC}"
    echo -e "    ${GREEN}${BIN_PATH} --endpoint=${ENDPOINT} --token=*** --runtime=${RUNTIME}${NC}"
    echo ""
    echo -e "  ${CYAN}${BOLD}常用命令:${NC}"
    echo ""
    echo -e "  查看服务状态:"
    echo -e "    ${GREEN}systemctl status ${SERVICE_NAME}${NC}"
    echo ""
    echo -e "  查看实时日志:"
    echo -e "    ${GREEN}journalctl -u ${SERVICE_NAME} -f${NC}"
    echo ""
    echo -e "  重启服务:"
    echo -e "    ${GREEN}systemctl restart ${SERVICE_NAME}${NC}"
    echo ""
    echo -e "  卸载:"
    echo -e "    ${GREEN}bash $0 --uninstall${NC}"
    echo ""
    echo -e "${YELLOW}Agent 启动后会自动通过 Bootstrap API 拉取完整配置，${NC}"
    echo -e "${YELLOW}包括 runtime_bin、节点列表、心跳间隔等，无需手动配置。${NC}"
    echo ""
}

# ===== 参数校验 =====
validate_params() {
    if [ -z "$TOKEN" ]; then
        error "缺少 --token 参数"
        echo ""
        print_usage
        exit 1
    fi
    if [ -z "$ENDPOINT" ]; then
        error "缺少 --endpoint 参数"
        echo ""
        print_usage
        exit 1
    fi

    case "$RUNTIME" in
        xray|sing-box)
            ;;
        *)
            error "不支持的 runtime 类型: ${RUNTIME}（仅支持 xray / sing-box）"
            exit 1
            ;;
    esac
}

# ===== 主流程 =====
main() {
    parse_args "$@"

    if [ "$(id -u)" -ne 0 ]; then
        error "请使用 root 用户运行此脚本 (使用 sudo)"
        exit 1
    fi

    print_banner

    detect_os
    detect_arch

    if [ "$UNINSTALL_MODE" = true ]; then
        uninstall
        exit 0
    fi

    validate_params
    install_deps
    download_agent
    download_runtime
    create_dirs
    create_systemd_service
    print_success_info
}

main "$@"
