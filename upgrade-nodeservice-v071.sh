#!/bin/bash
set -e
BIN_DIR=/opt/yundu/bin
SRC_DIR=/tmp/yundu-v0.7.1
TS=$(date +%Y%m%d_%H%M%S)
SVC=node-service

echo "=== VPS190 升级 $SVC v0.7.0 -> v0.7.1 ==="
echo "--- 当前版本 ---"
old_ver=$($BIN_DIR/$SVC --version 2>/dev/null | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -1 || echo "unknown")
echo "  old: $old_ver"

# 解除可能的 immutable
chattr -i $BIN_DIR/$SVC 2>/dev/null || true

# 先停止服务（避免 Text file busy）
echo "--- 停止 yundu-$SVC ---"
systemctl stop yundu-$SVC
sleep 1

# 备份
echo "--- 备份 ---"
cp $BIN_DIR/$SVC $BIN_DIR/$SVC.bak.v070_to_v071_$TS

# 替换
echo "--- 替换二进制 ---"
cp $SRC_DIR/$SVC $BIN_DIR/$SVC
chmod +x $BIN_DIR/$SVC

# 启动
echo "--- 启动 yundu-$SVC ---"
systemctl start yundu-$SVC
sleep 4

# 验证
status=$(systemctl is-active yundu-$SVC)
echo "--- 验证 ---"
echo "  status: $status"
if [ "$status" != "active" ]; then
    echo "  ERROR: $SVC 启动失败，回滚..."
    cp $BIN_DIR/$SVC.bak.v070_to_v071_$TS $BIN_DIR/$SVC
    chmod +x $BIN_DIR/$SVC
    systemctl start yundu-$SVC
    echo "  已回滚到 v0.7.0"
    exit 1
fi

echo ""
echo "=== 升级完成 ==="
systemctl status yundu-$SVC --no-pager | grep -E 'Active:|Main PID:|Memory:'
echo ""
echo "--- 最近 10 行日志 ---"
journalctl -u yundu-$SVC -n 10 --no-pager --since '20 seconds ago' | tail -10
