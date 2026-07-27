#!/bin/bash
echo '===systemd services==='
systemctl is-active yundu-api-gateway yundu-identity-service yundu-node-service yundu-subscription-service yundu-traffic-service
echo ''
echo '===Docker containers==='
sudo docker ps --format '{{.Names}} | {{.Status}}' | grep -iE 'postgres|redis|nats|yundu'
echo ''
echo '===DB connection test==='
sudo docker exec yundu-postgres psql -U app -d airport -t -c "SELECT 1;" 2>&1 | head -3
echo ''
echo '===Check panel.env==='
grep -E 'DB_|POSTGRES|REDIS' /etc/yundu/panel.env 2>/dev/null | head -10
echo ''
echo '===Last 20 lines node-service log==='
journalctl -u yundu-node-service -n 20 --no-pager 2>&1 | tail -20
