#!/bin/bash
# verify-vps-standard.sh — YunDu VPS 标准化检查脚本 (P3-2)
# 在每台 VPS 上执行，检查节点是否符合零 SSH 部署标准。
# 用法: sudo bash verify-vps-standard.sh
# 退出码: 0=全部通过, 1=有警告, 2=有严重问题

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

PASS=0
WARN=0
FAIL=0

pass() { echo -e "${GREEN}[PASS]${NC} $1"; ((PASS++)); }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; ((WARN++)); }
fail() { echo -e "${RED}[FAIL]${NC} $1"; ((FAIL++)); }

echo "========================================"
echo " YunDu VPS 标准化检查"
echo " $(date)"
echo "========================================"
echo ""

# 1. node-agent 服务状态
echo "--- 1. node-agent 服务状态 ---"
if systemctl is-active --quiet yundu-node-agent.service 2>/dev/null; then
    pass "node-agent 服务 active running"
else
    fail "node-agent 服务未运行或不存在"
fi

# 检查 agent 版本
AGENT_BIN="/opt/yundu/bin/node-agent"
if [ -x "$AGENT_BIN" ]; then
    AGENT_VER=$("$AGENT_BIN" --version 2>/dev/null || echo "unknown")
    pass "agent 版本: $AGENT_VER"
else
    fail "node-agent 二进制不存在: $AGENT_BIN"
fi
echo ""

# 2. 遗留 systemd 服务检查
echo "--- 2. 遗留 systemd 服务检查 ---"
LEGACY_SERVICES=$(systemctl list-units --type=service --all 2>/dev/null | \
    grep -iE 'xray|sing-box|singbox|trojan|hysteria|tuic|v2ray' | \
    grep -v 'yundu-node-agent' || true)

if [ -z "$LEGACY_SERVICES" ]; then
    pass "无遗留代理服务"
else
    fail "发现遗留服务:"
    echo "$LEGACY_SERVICES" | while read -r line; do
        echo "       $line"
    done
fi
echo ""

# 3. 独立 sing-box/xray 进程检查
echo "--- 3. 独立内核进程检查 ---"
STRAY_PROCS=$(ps aux 2>/dev/null | \
    grep -iE '\bsing-box\b|\bxray\b' | \
    grep -v 'node-agent' | \
    grep -v grep || true)

if [ -z "$STRAY_PROCS" ]; then
    pass "无独立 sing-box/xray 进程（双内核在 node-agent 进程内运行）"
else
    fail "发现独立内核进程（应由 node-agent 管理）:"
    echo "$STRAY_PROCS" | while read -r line; do
        echo "       $line"
    done
fi
echo ""

# 4. 配置文件规范性检查
echo "--- 4. 配置文件规范性检查 ---"
CONFIG_DIR="/etc/yundu/config"
if [ -d "$CONFIG_DIR" ]; then
    # 检查必要的配置文件
    for f in sing-box.json xray.json; do
        if [ -f "$CONFIG_DIR/$f" ]; then
            SIZE=$(stat -c%s "$CONFIG_DIR/$f" 2>/dev/null || stat -f%z "$CONFIG_DIR/$f" 2>/dev/null || echo "?")
            pass "配置文件存在: $f (${SIZE} bytes)"
        else
            warn "配置文件缺失: $f"
        fi
    done

    # 检查 LKG 文件
    for f in sing-box.lkg.json xray.lkg.json; do
        if [ -f "$CONFIG_DIR/$f" ]; then
            pass "LKG 备份存在: $f"
        else
            warn "LKG 备份缺失: $f"
        fi
    done

    # 检查 .bak 文件堆积
    BAK_COUNT=$(find "$CONFIG_DIR" -name '*.bak*' -o -name '*.old*' 2>/dev/null | wc -l)
    if [ "$BAK_COUNT" -eq 0 ]; then
        pass "无 .bak 备份文件堆积"
    elif [ "$BAK_COUNT" -le 3 ]; then
        warn "有 $BAK_COUNT 个 .bak 文件（可接受范围）"
    else
        fail "有 $BAK_COUNT 个 .bak 文件堆积（超过 3 个，需清理）"
    fi

    # 检查遗留配置文件
    LEGACY_CONFIGS=$(find "$CONFIG_DIR" -name '*transit*' -o -name '*turkey*' -o -name '*bridge*' 2>/dev/null || true)
    if [ -z "$LEGACY_CONFIGS" ]; then
        pass "无遗留配置文件"
    else
        fail "发现遗留配置文件:"
        echo "$LEGACY_CONFIGS" | while read -r f; do
            echo "       $f"
        done
    fi
else
    fail "配置目录不存在: $CONFIG_DIR"
fi
echo ""

# 5. 端口规划检查
echo "--- 5. 端口监听检查 ---"
LISTEN_PORTS=$(ss -tlnp 2>/dev/null | grep -vE '127\.0\.0\.1|::1' | awk '{print $4}' | sed 's/.*://' | sort -n | uniq || true)
if [ -n "$LISTEN_PORTS" ]; then
    pass "监听端口一览:"
    echo "$LISTEN_PORTS" | while read -r port; do
        if [ -n "$port" ]; then
            # 检查是否在规划范围 30000-30200 或标准端口 80/443
            if [ "$port" -ge 30000 ] && [ "$port" -le 30200 ] 2>/dev/null; then
                echo "       :$port (节点端口 - 规划范围内)"
            elif [ "$port" = "80" ] || [ "$port" = "443" ] 2>/dev/null; then
                echo "       :$port (标准端口)"
            elif [ "$port" = "8445" ] 2>/dev/null; then
                echo "       :$port (nginx HTTPS vhost)"
            else
                echo "       :$port (其他)"
            fi
        fi
    done
else
    warn "无法获取端口列表"
fi
echo ""

# 6. WARP 状态检查
echo "--- 6. WARP 状态检查 ---"
WARP_ENV=$(grep -i 'WARP_MODE' /etc/yundu/node-agent.env 2>/dev/null || echo "")
if [ -n "$WARP_ENV" ]; then
    pass "WARP 配置: $WARP_ENV"
    # 检查 wireproxy 进程
    if pgrep -x wireproxy > /dev/null 2>&1; then
        pass "wireproxy 进程运行中"
    else
        warn "wireproxy 进程未检测到（可能未启用或使用其他模式）"
    fi
else
    warn "WARP 未配置（如不需要可忽略）"
fi
echo ""

# 7. systemd service 文件检查
echo "--- 7. systemd service 规范性 ---"
SERVICE_FILE="/etc/systemd/system/yundu-node-agent.service"
if [ -f "$SERVICE_FILE" ]; then
    if grep -q "EnvironmentFile=/etc/yundu/node-agent.env" "$SERVICE_FILE" 2>/dev/null; then
        pass "systemd service 引用了 node-agent.env"
    else
        warn "systemd service 未引用 node-agent.env"
    fi

    if grep -q "Restart=always" "$SERVICE_FILE" 2>/dev/null; then
        pass "systemd service 配置了自动重启"
    else
        warn "systemd service 未配置自动重启"
    fi
else
    fail "systemd service 文件不存在"
fi
echo ""

# 8. 磁盘空间检查
echo "--- 8. 磁盘空间检查 ---"
DISK_USAGE=$(df -h / 2>/dev/null | awk 'NR==2{print $5}' | tr -d '%' || echo "?")
if [ "$DISK_USAGE" != "?" ]; then
    if [ "$DISK_USAGE" -lt 80 ]; then
        pass "磁盘使用率: ${DISK_USAGE}%"
    elif [ "$DISK_USAGE" -lt 90 ]; then
        warn "磁盘使用率: ${DISK_USAGE}% (偏高)"
    else
        fail "磁盘使用率: ${DISK_USAGE}% (严重，需清理)"
    fi
else
    warn "无法获取磁盘使用率"
fi
echo ""

# 总结
echo "========================================"
echo " 检查结果汇总"
echo "========================================"
echo -e " ${GREEN}PASS: $PASS${NC}  ${YELLOW}WARN: $WARN${NC}  ${RED}FAIL: $FAIL${NC}"
echo ""

if [ "$FAIL" -gt 0 ]; then
    echo -e "${RED}有 $FAIL 个严重问题需要修复${NC}"
    exit 2
elif [ "$WARN" -gt 0 ]; then
    echo -e "${YELLOW}有 $WARN 个警告需要关注${NC}"
    exit 1
else
    echo -e "${GREEN}所有检查项通过！VPS 符合零 SSH 部署标准。${NC}"
    exit 0
fi
