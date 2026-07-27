#!/bin/bash
set -e
mkdir -p /tmp/yundu-v0.7.1
cd /tmp/yundu-v0.7.1
BASE='https://github.com/energie2008/yundu/releases/download/v0.7.1'
for svc in node-agent node-service api-gateway identity-service subscription-service traffic-service migrate; do
    echo "--- Downloading $svc-amd64 ---"
    curl -sL -o $svc $BASE/$svc-amd64
    chmod +x $svc
    sz=$(stat -c%s $svc)
    echo "  $svc: $sz bytes"
done
echo '===All downloaded==='
ls -la /tmp/yundu-v0.7.1/
