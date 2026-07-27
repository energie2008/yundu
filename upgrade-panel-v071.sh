#!/bin/bash
set -e
BIN_DIR=/opt/yundu/bin
SRC_DIR=/tmp/yundu-v0.7.1
TS=$(date +%Y%m%d_%H%M%S)

# 面板服务（VPS190 不跑 node-agent）
SERVICES="node-service api-gateway identity-service subscription-service traffic-service"

echo "=== VPS190 面板服务升级 v0.7.0 -> v0.7.1 ==="
echo ""

for svc in $SERVICES; do
    echo "--- Upgrading $svc ---"
    # 解除可能的 immutable
    chattr -i $BIN_DIR/$svc 2>/dev/null || true
    # 先停止服务（避免 Text file busy）
    systemctl stop yundu-$svc
    sleep 1
    # 备份
    cp $BIN_DIR/$svc $BIN_DIR/$svc.bak.v070_to_v071_$TS
    # 替换
    cp $SRC_DIR/$svc $BIN_DIR/$svc
    chmod +x $BIN_DIR/$svc
    # 启动
    systemctl start yundu-$svc
    sleep 3
    # 验证
    status=$(systemctl is-active yundu-$svc)
    echo "  $svc: $status"
    if [ "$status" != "active" ]; then
        echo "  ERROR: $svc failed to start, rolling back..."
        cp $BIN_DIR/$svc.bak.v070_to_v071_$TS $BIN_DIR/$svc
        chmod +x $BIN_DIR/$svc
        systemctl restart yundu-$svc
        echo "  Rolled back to v0.7.0"
        exit 1
    fi
done

echo ""
echo "=== All panel services upgraded to v0.7.1 ==="
systemctl status yundu-node-service yundu-api-gateway yundu-identity-service yundu-subscription-service yundu-traffic-service --no-pager | grep -E 'Active:|Main PID:'
