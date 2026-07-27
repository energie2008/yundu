#!/bin/bash
echo '===Latest 10 goose migrations==='
sudo docker exec yundu-postgres psql -U app -d airport -c "SELECT version_id, is_applied, tstamp FROM goose_db_version ORDER BY version_id DESC LIMIT 10;"
echo ''
echo '===Check 000066/000067 applied?==='
sudo docker exec yundu-postgres psql -U app -d airport -t -c "SELECT version_id, is_applied FROM goose_db_version WHERE version_id IN (66, 67) ORDER BY version_id;"
echo ''
echo '===Total migration count==='
sudo docker exec yundu-postgres psql -U app -d airport -t -c "SELECT COUNT(*) FROM goose_db_version;"
