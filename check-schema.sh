#!/bin/bash
echo '===All tables in airport DB==='
sudo docker exec yundu-postgres psql -U app -d airport -t -c "SELECT tablename FROM pg_tables WHERE schemaname='public' ORDER BY tablename;" | head -50
echo ''
echo '===Tables related to migrations/schema==='
sudo docker exec yundu-postgres psql -U app -d airport -t -c "SELECT tablename FROM pg_tables WHERE schemaname='public' AND (tablename LIKE '%migrat%' OR tablename LIKE '%schema%' OR tablename LIKE '%version%') ORDER BY tablename;"
echo ''
echo '===Check schema_migrations or similar==='
for tbl in schema_migrations migrations schema_versions goose_db_version; do
    cnt=$(sudo docker exec yundu-postgres psql -U app -d airport -t -c "SELECT COUNT(*) FROM $tbl;" 2>&1 | head -1)
    echo "$tbl: $cnt"
done
