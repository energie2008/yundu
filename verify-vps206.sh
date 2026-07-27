#!/bin/bash
echo '========================================='
echo '=== VPS206 节点验证 ==='
echo '========================================='
sleep 12
echo '--- 服务状态 ---'
systemctl is-active yundu-node-agent
echo ''
echo '--- 版本与 runtime ---'
journalctl -u yundu-node-agent --since '30 seconds ago' --no-pager | grep -oE '"version":"[0-9.]+"|"runtime_type":"[a-z\-]+"|NATIVE MODE ENABLED' | head -5
echo ''
echo '--- 心跳与配置拉取 ---'
journalctl -u yundu-node-agent --since '30 seconds ago' --no-pager | grep -iE 'heartbeat|config|reconnect|registered|runtime' | tail -10
echo ''
echo '--- 错误检查 ---'
err_cnt=$(journalctl -u yundu-node-agent --since '30 seconds ago' --no-pager | grep -ciE '"level":"ERROR"|"level":"FATAL"|panic' || echo 0)
echo "  错误数: $err_cnt"
