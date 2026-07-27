#!/bin/bash
set -e
BIN_DIR=/opt/yundu/bin
SVC=node-agent
TS=$(date +%Y%m%d_%H%M%S)
TMP=/tmp/node-agent-v071

echo "=== 升级 $SVC 到 v0.7.1 ==="

# 下载
echo "--- 下载 node-agent-amd64 ---"
mkdir -p $TMP
curl -fsSL -o $TMP/$SVC https://github.com/energie2008/yundu/releases/download/v0.7.1/$SVC-amd64
chmod +x $TMP/$SVC
sz=$(stat -c%s $TMP/$SVC)
echo "  downloaded: $sz bytes"

# 解除 immutable
chattr -i $BIN_DIR/$SVC 2>/dev/null || true

# 停止
echo "--- 停止 yundu-$SVC ---"
systemctl stop yundu-$SVC
sleep 1

# 备份
echo "--- 备份 ---"
cp $BIN_DIR/$SVC $BIN_DIR/$SVC.bak.to_v071_$TS

# 替换
echo "--- 替换二进制 ---"
cp $TMP/$SVC $BIN_DIR/$SVC
chmod +x $BIN_DIR/$SVC

# 启动
echo "--- 启动 yundu-$SVC ---"
systemctl start yundu-$SVC
sleep 5

# 验证
status=$(systemctl is-active yundu-$SVC)
echo "--- 验证 ---"
echo "  status: $status"
if [ "$status" != "active" ]; then
    echo "  ERROR: 启动失败，回滚..."
    cp $BIN_DIR/$SVC.bak.to_v071_$TS $BIN_DIR/$SVC
    chmod +x $BIN_DIR/$SVC
    systemctl start yundu-$SVC
    echo "  已回滚"
    exit 1
fi

echo ""
echo "=== 升级完成 ==="
systemctl status yundu-$SVC --no-pager | grep -E 'Active:|Main PID:|Memory:'
echo ""
echo "--- 最近 15 行日志 ---"
journalctl -u yundu-$SVC -n 15 --no-pager --since '15 seconds ago' | tail -15
rm -rf $TMP
