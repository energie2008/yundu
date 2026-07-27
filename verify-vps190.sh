#!/bin/bash
echo '========================================='
echo '=== VPS190 面板验证 ==='
echo '========================================='
echo '--- 5 个面板服务状态 ---'
systemctl is-active yundu-api-gateway yundu-identity-service yundu-node-service yundu-subscription-service yundu-traffic-service
echo ''
echo '--- node-service 错误检查（最近30秒）---'
err_cnt=$(journalctl -u yundu-node-service --since '30 seconds ago' --no-pager | grep -ciE '"level":"ERROR"|"level":"FATAL"|panic' || echo 0)
echo "  node-service 错误数: $err_cnt"
echo ''
echo '--- node-service 最近日志 ---'
journalctl -u yundu-node-service --since '30 seconds ago' --no-pager | tail -8
echo ''
echo '--- 检查 agent 心跳接入（面板侧）---'
journalctl -u yundu-node-service --since '60 seconds ago' --no-pager | grep -iE 'agent|heartbeat|register|runtime' | tail -10
