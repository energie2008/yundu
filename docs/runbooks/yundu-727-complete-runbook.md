# YunDu-727 完合并执行运维手册

## 概述

本手册涵盖 YunDu-727 项目的完整执行方案，包括统一渲染器、内核主辅翻转、证书加固、WARP 出口接入、端口迁移、订阅适配和运维加固。

## 执行阶段总览

| 阶段 | 内容 | 代码变更 | 迁移文件 |
|------|------|---------|---------|
| Phase 0 | 统一渲染器（删除 exposure 僵尸代码 + renderXHTTPDownload 删除 + mode 校验前移） | ✅ 完成 | — |
| Phase 1 | 证书加固（exposure_mode 双源修复 + DB Trigger + 证书四级回退 + CertSHA256Fingerprint） | ✅ 完成 | 000065 |
| Phase 2 | 内核翻转（sing-box 主内核 + xray 辅内核懒加载 + 配置注入方向翻转） | ✅ 完成 | — |
| Phase 2 Agent | Agent 翻转（初始化翻转 + _xray_config 提取 + ConfigFilePath 默认 sing-box） | ✅ 完成 | — |
| Phase 3 | WARP 出口（sing-box 原生 wireguard + outbound 接入 deployment_service + warp-cli 生命周期增强） | ✅ 完成 | 000068 |
| Phase 4 | 订阅适配 + 端口迁移 + 80 端口骨架 | ✅ 完成 | 000066, 000067 |
| Phase 5 | 部署验证（Agent 版本强约束） | ✅ 代码完成 | — |
| Phase 6 | 运维加固（SAN 24h 同步 + 蓝绿 drain 监控 + 运维手册） | ✅ 完成 | — |

## 迁移文件清单

| 编号 | 文件名 | 内容 |
|------|--------|------|
| 000065 | exposure_mode_sync_trigger.sql | exposure_mode 双源同步 trigger（先修复存量+NULL防御） |
| 000066 | migrate_direct_ports.sql | 端口迁移 9450-9600 → 30000-30200 |
| 000067 | kernel_flip_singbox_primary.sql | 内核翻转 runtime_id 迁移 + sing-box runtime 创建 |
| 000068 | warp_profile_wireguard_fields.sql | warp_profile 字段标准化（private_key/public_key/local_address/mtu） |

## 部署步骤

### 1. VPS190 (node-service 部署)

```bash
# 1. 拉取最新代码
cd /opt/yundu-src && git pull origin main

# 2. 编译 node-service
cd apps/node-service && go build -o yundu-node-service ./cmd/server

# 3. 执行数据库迁移
goose -dir migrations postgres "$DATABASE_URL" up

# 4. 重启 node-service
systemctl restart yundu-node-service

# 5. 验证迁移成功
psql $DATABASE_URL -c "SELECT trigger_name FROM information_schema.triggers WHERE event_object_table = 'nodes';"
# 应看到: nodes_exposure_mode_sync, nodes_downstream_exposure_mode_sync
```

### 2. VPS206 (灰度部署 Agent)

```bash
# 1. 拉取最新 Agent 代码
cd /opt/yundu-agent && git pull origin main

# 2. 编译 Agent
cd apps/node-agent && go build -o yundu-agent ./cmd/agent

# 3. 停止旧 Agent
systemctl stop yundu-agent

# 4. 部署新 Agent
cp yundu-agent /usr/local/bin/yundu-agent
systemctl start yundu-agent

# 5. 灰度验证（连续 30 分钟）
# 检查连接成功率
journalctl -u yundu-agent -f --since "30 min ago" | grep -E "connection|error|panic"

# 检查 sing-box 是否为主内核
journalctl -u yundu-agent --since "5 min ago" | grep "sing-box started successfully (primary)"

# 检查 WARP 状态
journalctl -u yundu-agent --since "5 min ago" | grep "warp"
```

### 3. VPS81 (全量部署)

```bash
# 重复 VPS206 的步骤 1-4
# 部署后触发配置重下发：
curl -X POST http://vps190:8080/api/v1/admin/deploy/runtime/all \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

### 4. 防火墙规则

```bash
# 开放新端口范围 30000-30200（替代旧的 9450-9600）
iptables -A INPUT -p tcp --dport 30000:30200 -j ACCEPT
iptables -A INPUT -p udp --dport 30000:30200 -j ACCEPT
ip6tables -A INPUT -p tcp --dport 30000:30200 -j ACCEPT
ip6tables -A INPUT -p udp --dport 30000:30200 -j ACCEPT

# 保留旧端口 7 天过渡期后关闭
# iptables -D INPUT -p tcp --dport 9450:9600 -j ACCEPT
```

## 灰度判定标准

VPS206 灰度部署后，连续 **30 分钟** 监控以下指标：

| 指标 | 通过标准 | 检查方法 |
|------|---------|---------|
| 连接成功率 | > 95% | `journalctl -u yundu-agent \| grep "connection success"` |
| sing-box crash | 0 次 | `journalctl -u yundu-agent \| grep "sing-box.*crash\|panic"` |
| 流量统计正常 | 上行/下行流量 > 0 | `journalctl -u yundu-agent \| grep "traffic"` |
| xray 懒加载正常 | XHTTP 节点可用 | `curl -x socks5://127.0.0.1:1080 https://httpbin.org/ip` |
| WARP 出口可用 | warp_ip 非空 | `journalctl -u yundu-agent \| grep "warp_ip"` |
| Agent 版本检查 | ≥ 727.0.0 | `journalctl -u yundu-agent \| grep "agent_version"` |

**如任一指标不达标，立即回滚：**
```bash
systemctl stop yundu-agent
cp /opt/yundu-agent-backup/yundu-agent-old /usr/local/bin/yundu-agent
systemctl start yundu-agent
```

## 回滚步骤

### 代码回滚

```bash
# 1. 回滚 Agent
systemctl stop yundu-agent
cp /opt/yundu-agent-backup/yundu-agent-old /usr/local/bin/yundu-agent
systemctl start yundu-agent

# 2. 回滚 node-service
systemctl stop yundu-node-service
cp /opt/yundu-node-service-backup/yundu-node-service-old /usr/local/bin/yundu-node-service
systemctl start yundu-node-service
```

### 数据库回滚

```bash
# 按逆序回滚迁移
goose -dir migrations postgres "$DATABASE_URL" down
# 重复执行直到回滚到 000064
```

## 关键配置变更

### 内核翻转 (P2)

- **主内核**: sing-box (始终运行)
- **辅内核**: xray (仅 XHTTP 节点时懒加载)
- **配置注入方向**: sing-box 配置中嵌入 `_xray_config`（旧版为 xray 嵌入 `_singbox_config`）
- **Agent 配置文件**: 默认 `config/sing-box.json`（旧版为 `config.json`）

### WARP 出口 (P3)

- **sing-box**: 原生 wireguard outbound（有 private_key 时）
- **xray**: socks5 代理（通过 warp-cli 本地 SOCKS5）
- **WARP 状态上报**: 每 5 分钟自动采集并上报到面板

### Agent 版本强约束 (P5)

- **最低版本**: 727.0.0
- **低于此版本的 Agent**: 配置推送被拒绝（不会静默失败）
- **升级路径**: 必须先升级 Agent 到 727.0.0+，再执行内核翻转迁移

## 监控告警

### SAN 同步 (P6)

- **频率**: 每 24 小时自动批量同步
- **并发控制**: sync.Mutex 防止并发冲突
- **日志关键字**: `P6: batch SAN sync job started/completed`

### 蓝绿热转 drain 监控 (P6)

- **drain 超时**: 30s（默认）
- **告警阈值**: drain 耗时 > 10s
- **日志关键字**: `P6: blue-green drain took longer than 10s`

### 证书四级回退 (P1)

1. 节点 `cert_bundle_id` 绑定的证书
2. `cert_bundles` 表 SAN 匹配
3. `tls_certificates` 表 SNI 匹配（P1-C 新增）
4. 自签名 ECDSA P-256 兜底（含 SHA256 指纹告警）

## 已知限制

1. **chain 渲染迁移**: `exposure/chain_xray.go` 和 `exposure/chain_singbox.go` 仍在使用，迁移到 `kernelrender` 标记为 P6 后续任务
2. **DualKernelValidator**: 已集成但 dry-run 需要设置 `XRAY_BINARY`/`SINGBOX_BINARY` 环境变量
3. **WARP 原生 wireguard**: 需要 warp_profile 表中有 `private_key` 字段，否则回退到 socks5
4. **端口迁移**: 旧端口 9450-9600 需保留 7 天过渡期
