package handler

import (
	"github.com/gin-gonic/gin"
)

// InstallHandler 处理一键安装脚本的 HTTP 请求。
//
// 端点 GET /api/v1/install.sh?endpoint=<panel_url>&token=<agent_token>
//
// 返回一个 bash 安装脚本，客户端可通过如下方式使用：
//
//	curl -fsSL https://panel.example.com/api/v1/install.sh | bash -s -- --token <token> --endpoint https://panel.example.com
//
// 脚本完成以下工作：
//  1. 解析 --token / --endpoint 参数
//  2. 检测操作系统（仅支持 linux）与 CPU 架构（amd64/arm64）
//  3. 从配置的 CDN 或 GitHub releases 下载 node-agent 二进制
//  4. 调用 Bootstrap API 获取 runtime_type（xray / sing-box）
//  5. 下载对应内核二进制（xray-core / sing-box）
//  6. 生成 systemd unit 文件
//  7. 启动服务
type InstallHandler struct {
	// PanelURL 是面板对外可访问的 URL，当请求未携带 endpoint 查询参数时使用。
	PanelURL string
	// AgentCDNURL 是 node-agent 二进制下载的基础 URL。
	// 为空时脚本回退到 GitHub releases。
	AgentCDNURL string
}

// NewInstallHandler 构造 InstallHandler。
func NewInstallHandler(panelURL, agentCDNURL string) *InstallHandler {
	return &InstallHandler{
		PanelURL:    panelURL,
		AgentCDNURL: agentCDNURL,
	}
}

// ServeInstallScript 处理 GET /install.sh 请求，返回 bash 安装脚本。
func (h *InstallHandler) ServeInstallScript(c *gin.Context) {
	// 从查询参数获取 endpoint，回退到配置的 PanelURL
	endpoint := c.Query("endpoint")
	if endpoint == "" {
		endpoint = h.PanelURL
	}

	c.Header("Content-Type", "text/x-shellscript; charset=utf-8")
	c.Header("Content-Disposition", `inline; filename="install.sh"`)
	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.String(200, installScriptTemplate(endpoint, h.AgentCDNURL))
}

// installScriptTemplate 生成一键安装 bash 脚本。
// endpoint  是面板 API 地址（如 https://panel.example.com）。
// agentCDN  是 node-agent 二进制下载基础 URL；为空时回退到 GitHub releases。
func installScriptTemplate(endpoint, agentCDN string) string {
	return `#!/usr/bin/env bash
set -euo pipefail

# ==============================================================================
# node-agent 一键安装脚本
#
# 用法:
#   curl -fsSL ` + endpoint + `/api/v1/install.sh | bash -s -- --token <token> [--endpoint <url>]
#   curl -fsSL ` + endpoint + `/api/v1/install.sh | bash -s -- --token <token> --mode machine
#
# 参数:
#   --token       Agent 认证 token（必填）
#   --endpoint    面板 API 地址（可选，默认使用脚本内置地址）
#   --version     node-agent 版本（可选，默认 latest）
#   --config-dir  配置目录（可选，默认 /etc/yundu）
#   --mode        运行模式: node(默认) | machine(单进程多节点)
#
# 该脚本会：
#   1. 检测系统架构与操作系统
#   2. 下载 node-agent 二进制
#   3. 调用 Bootstrap API 获取 runtime_type（xray / sing-box）
#   4. 下载对应内核二进制
#   5. 生成 systemd unit 并启动服务
# ==============================================================================

PANEL_URL="` + endpoint + `"
AGENT_CDN_URL="` + agentCDN + `"
AGENT_TOKEN=""
AGENT_VERSION="latest"
CONFIG_DIR="/etc/yundu"
LOG_DIR="/var/log/yundu"
INSTALL_DIR="/usr/local/bin"
SERVICE_NAME="node-agent"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"
ENV_FILE="${CONFIG_DIR}/config.env"
BIN_PATH="${INSTALL_DIR}/node-agent"
AGENT_MODE="node"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m'

info()    { echo -e "${GREEN}[INFO]${NC}  $*"; }
warn()    { echo -e "${YELLOW}[WARN]${NC}  $*"; }
error()   { echo -e "${RED}[ERROR]${NC} $*" >&2; }
success() { echo -e "${GREEN}${BOLD}[OK]${NC}    $*"; }
header()  { echo -e "${BLUE}${BOLD}==>${NC}    $*"; }

die() { error "$*"; exit 1; }

# ------------------------------------------------------------------------------
# 参数解析
# ------------------------------------------------------------------------------
parse_args() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --token)
                AGENT_TOKEN="$2"
                shift 2
                ;;
            --endpoint)
                PANEL_URL="$2"
                shift 2
                ;;
            --version)
                AGENT_VERSION="$2"
                shift 2
                ;;
            --config-dir)
                CONFIG_DIR="$2"
                shift 2
                ;;
            --mode)
                AGENT_MODE="$2"
                shift 2
                ;;
            --help|-h)
                sed -n '2,20p' "$0" 2>/dev/null || true
                exit 0
                ;;
            *)
                error "未知参数: $1"
                exit 1
                ;;
        esac
    done

    if [[ -z "$AGENT_TOKEN" ]]; then
        die "缺少 --token 参数，请通过 --token <token> 指定 Agent 认证 token"
    fi

    case "$AGENT_MODE" in
        node|machine)
            info "运行模式: ${BOLD}${AGENT_MODE}${NC}"
            ;;
        *)
            die "不支持的 --mode 值: ${AGENT_MODE}（仅支持 node | machine）"
            ;;
    esac

    # 去掉尾部斜杠
    PANEL_URL="${PANEL_URL%/}"
}

# ------------------------------------------------------------------------------
# 操作系统与架构检测
# ------------------------------------------------------------------------------
detect_os_arch() {
    header "检测操作系统与架构..."

    local os_name
    os_name="$(uname -s | tr '[:upper:]' '[:lower:]')"
    if [[ "$os_name" != "linux" ]]; then
        die "仅支持 Linux 系统，当前系统: $os_name"
    fi
    info "操作系统: Linux"

    local arch
    arch="$(uname -m)"
    case "$arch" in
        x86_64|amd64)
            ARCH="amd64"
            ;;
        aarch64|arm64|armv8*)
            ARCH="arm64"
            ;;
        *)
            die "不支持的 CPU 架构: $arch（仅支持 amd64 / arm64）"
            ;;
    esac
    info "CPU 架构: ${BOLD}${ARCH}${NC}"

    # 检测 init 系统
    if command -v systemctl >/dev/null 2>&1 && [[ -d /run/systemd/system ]]; then
        info "init 系统: systemd"
    else
        die "未检测到 systemd，本脚本仅支持 systemd 系统"
    fi
}

# ------------------------------------------------------------------------------
# 安装依赖
# ------------------------------------------------------------------------------
install_deps() {
    header "检查依赖..."

    local need=()
    for cmd in curl tar; do
        if ! command -v "$cmd" >/dev/null 2>&1; then
            need+=("$cmd")
        fi
    done

    if [[ ${#need[@]} -eq 0 ]]; then
        success "依赖已就绪 (curl, tar)"
        return
    fi

    info "安装缺失依赖: ${need[*]}"
    if [[ -f /etc/os-release ]]; then
        . /etc/os-release
        case "$ID" in
            ubuntu|debian|raspbian)
                export DEBIAN_FRONTEND=noninteractive
                apt-get update -qq && apt-get install -y -qq "${need[@]}" ca-certificates
                ;;
            centos|rhel|fedora|rocky|alma)
                if command -v dnf >/dev/null 2>&1; then
                    dnf install -y -q "${need[@]}" ca-certificates
                else
                    yum install -y -q "${need[@]}" ca-certificates
                fi
                ;;
            alpine)
                apk add --no-cache "${need[@]}" ca-certificates
                ;;
            *)
                die "无法自动安装依赖，请手动安装: ${need[*]}"
                ;;
        esac
    else
        die "无法识别发行版，请手动安装: ${need[*]}"
    fi
    success "依赖安装完成"
}

# ------------------------------------------------------------------------------
# 下载 node-agent 二进制
# ------------------------------------------------------------------------------
download_agent() {
    header "下载 node-agent 二进制..."

    local tmp_dir
    tmp_dir="$(mktemp -d)"
    trap 'rm -rf "$tmp_dir"' EXIT

    local bin_name="node-agent-linux-${ARCH}"
    local download_url=""

    # 优先从配置的 CDN 下载，回退到 GitHub releases
    if [[ -n "$AGENT_CDN_URL" ]]; then
        if [[ "$AGENT_VERSION" == "latest" ]]; then
            download_url="${AGENT_CDN_URL}/node-agent/latest/${bin_name}"
        else
            download_url="${AGENT_CDN_URL}/node-agent/${AGENT_VERSION}/${bin_name}"
        fi
    else
        # GitHub releases 回退
        if [[ "$AGENT_VERSION" == "latest" ]]; then
            download_url="https://github.com/airport-panel/node-agent/releases/latest/download/${bin_name}"
        else
            download_url="https://github.com/airport-panel/node-agent/releases/download/${AGENT_VERSION}/${bin_name}"
        fi
    fi

    info "下载地址: ${download_url}"
    local bin_tmp="${tmp_dir}/node-agent"

    if ! curl -fsSL --connect-timeout 30 --max-time 300 "$download_url" -o "$bin_tmp"; then
        die "下载 node-agent 失败，请检查网络或手动指定 AGENT_CDN_URL"
    fi

    if [[ ! -s "$bin_tmp" ]]; then
        die "下载的 node-agent 文件为空"
    fi

    chmod +x "$bin_tmp"
    install -m 755 "$bin_tmp" "$BIN_PATH"
    success "node-agent 安装成功: ${BIN_PATH}"
}

# ------------------------------------------------------------------------------
# 调用 Bootstrap API 获取 runtime_type
# ------------------------------------------------------------------------------
fetch_runtime_type() {
    header "调用 Bootstrap API 获取运行时类型..."

    local bootstrap_url="${PANEL_URL}/api/v1/agent/bootstrap?token=${AGENT_TOKEN}"
    info "Bootstrap URL: ${bootstrap_url}"

    local resp
    if ! resp="$(curl -fsSL --connect-timeout 15 --max-time 30 "$bootstrap_url" 2>/dev/null)"; then
        die "调用 Bootstrap API 失败，请检查 token 和 endpoint 是否正确"
    fi

    # 解析 runtime_type（使用 grep/sed 避免依赖 jq）
    RUNTIME_TYPE="$(echo "$resp" | grep -o '"runtime_type"[[:space:]]*:[[:space:]]*"[^"]*"' | head -1 | sed 's/.*"runtime_type"[[:space:]]*:[[:space:]]*"//;s/"//')"
    if [[ -z "$RUNTIME_TYPE" ]]; then
        warn "Bootstrap 响应中未找到 runtime_type，默认使用 xray"
        RUNTIME_TYPE="xray"
    fi

    info "运行时类型: ${BOLD}${RUNTIME_TYPE}${NC}"

    # 解析 runtime_bin（可选）
    RUNTIME_BIN="$(echo "$resp" | grep -o '"runtime_bin"[[:space:]]*:[[:space:]]*"[^"]*"' | head -1 | sed 's/.*"runtime_bin"[[:space:]]*:[[:space:]]*"//;s/"//')"
    if [[ -z "$RUNTIME_BIN" ]]; then
        case "$RUNTIME_TYPE" in
            sing-box) RUNTIME_BIN="/usr/local/bin/sing-box" ;;
            *)        RUNTIME_BIN="/usr/local/bin/xray" ;;
        esac
    fi
    info "内核路径: ${RUNTIME_BIN}"
}

# ------------------------------------------------------------------------------
# 下载内核二进制（xray / sing-box）
# ------------------------------------------------------------------------------
download_kernel() {
    header "检查内核二进制 (${RUNTIME_TYPE})..."

    # 如果内核已存在且可执行，跳过下载
    if [[ -x "$RUNTIME_BIN" ]]; then
        success "内核已存在: ${RUNTIME_BIN}"
        return
    fi

    case "$RUNTIME_TYPE" in
        xray|xray-core)
            download_xray
            ;;
        sing-box|singbox)
            download_singbox
            ;;
        *)
            warn "未知运行时类型: ${RUNTIME_TYPE}，跳过内核下载"
            ;;
    esac
}

download_xray() {
    info "下载 Xray-core..."
    local xray_version="v26.3.27"
    local tmp_dir
    tmp_dir="$(mktemp -d)"
    trap 'rm -rf "$tmp_dir"' EXIT

    local url="https://github.com/XTLS/Xray-core/releases/download/${xray_version}/Xray-linux-${ARCH}.zip"
    info "下载地址: ${url}"

    if ! curl -fsSL --connect-timeout 30 --max-time 300 "$url" -o "${tmp_dir}/xray.zip"; then
        die "下载 Xray-core 失败，请手动安装"
    fi

    if ! command -v unzip >/dev/null 2>&1; then
        warn "unzip 未安装，尝试安装..."
        if command -v apt-get >/dev/null 2>&1; then
            apt-get install -y -qq unzip
        elif command -v dnf >/dev/null 2>&1; then
            dnf install -y -q unzip
        elif command -v yum >/dev/null 2>&1; then
            yum install -y -q unzip
        elif command -v apk >/dev/null 2>&1; then
            apk add --no-cache unzip
        fi
    fi

    (cd "$tmp_dir" && unzip -o xray.zip xray)
    install -m 755 "${tmp_dir}/xray" /usr/local/bin/xray
    success "Xray-core 安装完成: /usr/local/bin/xray"
}

download_singbox() {
    info "下载 sing-box..."
    local sb_version="1.13.14"
    local tmp_dir
    tmp_dir="$(mktemp -d)"
    trap 'rm -rf "$tmp_dir"' EXIT

    local url="https://github.com/SagerNet/sing-box/releases/download/v${sb_version}/sing-box-${sb_version}-linux-${ARCH}.tar.gz"
    info "下载地址: ${url}"

    if ! curl -fsSL --connect-timeout 30 --max-time 300 "$url" -o "${tmp_dir}/singbox.tar.gz"; then
        die "下载 sing-box 失败，请手动安装"
    fi

    tar xzf "${tmp_dir}/singbox.tar.gz" -C "$tmp_dir"
    install -m 755 "${tmp_dir}"/sing-box-*/sing-box /usr/local/bin/sing-box
    success "sing-box 安装完成: /usr/local/bin/sing-box"
}

# ------------------------------------------------------------------------------
# 生成配置文件
# ------------------------------------------------------------------------------
create_config() {
    header "创建配置文件..."

    mkdir -p "$CONFIG_DIR" "$LOG_DIR"
    mkdir -p "${CONFIG_DIR}/configs" "${CONFIG_DIR}/certs"
    chmod 700 "$CONFIG_DIR"

    if [[ "$AGENT_MODE" == "machine" ]]; then
        # Machine 模式：使用 YUNDU_MODE + YUNDU_SERVER_TOKEN
        # Agent 通过 --mode machine 启动，通过 server_token 发现该机器上所有节点
        cat > "$ENV_FILE" << EOF
PANEL_URL=${PANEL_URL}
YUNDU_MODE=machine
YUNDU_SERVER_TOKEN=${AGENT_TOKEN}
RUNTIME_TYPE=${RUNTIME_TYPE}
RUNTIME_BIN=${RUNTIME_BIN}
CONFIG_DIR=${CONFIG_DIR}
LOG_DIR=${LOG_DIR}
LOG_LEVEL=info
EOF
    else
        # Node 模式（默认）：使用 AGENT_TOKEN
        cat > "$ENV_FILE" << EOF
PANEL_URL=${PANEL_URL}
AGENT_TOKEN=${AGENT_TOKEN}
RUNTIME_TYPE=${RUNTIME_TYPE}
RUNTIME_BIN=${RUNTIME_BIN}
CONFIG_DIR=${CONFIG_DIR}
LOG_DIR=${LOG_DIR}
LOG_LEVEL=info
EOF
    fi
    chmod 600 "$ENV_FILE"
    success "配置文件已创建: ${ENV_FILE} (mode=${AGENT_MODE})"
}

# ------------------------------------------------------------------------------
# 生成 systemd unit 并启动服务
# ------------------------------------------------------------------------------
create_systemd_service() {
    header "创建 systemd 服务..."

    cat > "$SERVICE_FILE" << EOF
[Unit]
Description=node-agent - Airport Node Agent
After=network-online.target nss-lookup.target
Wants=network-online.target

[Service]
Type=simple
EnvironmentFile=${ENV_FILE}
ExecStart=${BIN_PATH}
Restart=always
RestartSec=5
LimitNOFILE=65536
LimitNPROC=4096
StandardOutput=journal
StandardError=journal
SyslogIdentifier=${SERVICE_NAME}
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=${LOG_DIR} ${CONFIG_DIR}
PrivateTmp=true

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload
    systemctl enable "$SERVICE_NAME"
    info "启动服务..."
    systemctl restart "$SERVICE_NAME" || die "服务启动失败，请检查: journalctl -u ${SERVICE_NAME} -n 50 --no-pager"

    sleep 2
    if systemctl is-active --quiet "$SERVICE_NAME"; then
        success "node-agent 服务已启动"
    else
        warn "服务可能未正常运行，请检查: systemctl status ${SERVICE_NAME}"
    fi
}

# ------------------------------------------------------------------------------
# 输出安装结果
# ------------------------------------------------------------------------------
print_result() {
    echo ""
    echo -e "${GREEN}${BOLD}============================================================${NC}"
    echo -e "${GREEN}${BOLD}           node-agent 安装完成！${NC}"
    echo -e "${GREEN}${BOLD}============================================================${NC}"
    echo ""
    echo -e "  ${BOLD}服务名称:${NC}   ${SERVICE_NAME}"
    echo -e "  ${BOLD}运行模式:${NC}   ${AGENT_MODE}"
    echo -e "  ${BOLD}二进制路径:${NC} ${BIN_PATH}"
    echo -e "  ${BOLD}配置文件:${NC}   ${ENV_FILE}"
    echo -e "  ${BOLD}运行时:${NC}     ${RUNTIME_TYPE} (${RUNTIME_BIN})"
    echo -e "  ${BOLD}面板地址:${NC}   ${PANEL_URL}"
    echo ""
    if [[ "$AGENT_MODE" == "machine" ]]; then
        echo -e "  ${BOLD}Machine 模式提示:${NC}"
        echo -e "    Agent 将自动发现该服务器上的所有节点"
        echo -e "    通过 yunductl machine list 查看托管节点"
        echo ""
    fi
    echo -e "  ${BOLD}常用命令:${NC}"
    echo -e "    systemctl status ${SERVICE_NAME}"
    echo -e "    journalctl -u ${SERVICE_NAME} -f"
    echo -e "    systemctl restart ${SERVICE_NAME}"
    echo -e "    yunductl status   (需在节点本地执行)"
    echo ""
}

# ------------------------------------------------------------------------------
# 主流程
# ------------------------------------------------------------------------------
main() {
    if [[ "$(id -u)" -ne 0 ]]; then
        die "请使用 root 用户运行此脚本 (sudo)"
    fi

    parse_args "$@"
    detect_os_arch
    install_deps
    download_agent
    fetch_runtime_type
    download_kernel
    create_config
    create_systemd_service
    print_result
}

main "$@"
`
}
