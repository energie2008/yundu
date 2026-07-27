#!/bin/bash
echo '===DB migrate version (latest 15)==='
sudo docker exec yundu-postgres psql -U app -d airport -t -c "SELECT version, is_applied FROM schema_migrations ORDER BY version DESC LIMIT 15;"
echo ''
echo '===Migration files 000065-000071 on disk==='
ls /opt/yundu/migrations/ 2>/dev/null | grep -E '00006[5-9]|00007[0-1]'
echo ''
echo '===Binary versions==='
/opt/yundu/bin/node-service --version 2>&1 | head -3
/opt/yundu/bin/api-gateway --version 2>&1 | head -3
/opt/yundu/bin/identity-service --version 2>&1 | head -3
/opt/yundu/bin/subscription-service --version 2>&1 | head -3
/opt/yundu/bin/traffic-service --version 2>&1 | head -3
