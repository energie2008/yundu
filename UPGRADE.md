# YunDu 升级维护手册

> **宗旨**：只升级内核，不动架构。系统稳定就不重构。
> **适用场景**：sing-box / xray-core 版本升级、紧急 hotfix、常规发版。
> 基于 `编译推送与安装升级维护教程.md` 提炼，去掉了安装和前端部署，聚焦升级。

---

## 目录

- [一、升级概览：两条路径](#一升级概览两条路径)
- [二、路径 A：CI 发版（标准流程）](#二路径-aci-发版标准流程)
- [三、路径 B：本地交叉编译热修复（紧急 hotfix）](#三路径-b本地交叉编译热修复紧急-hotfix)
- [四、sing-box 内核升级](#四sing-box-内核升级)
- [五、xray-core 内核升级](#五xray-core-内核升级)
- [六、面板服务升级（node-service 等）](#六面板服务升级node-service-等)
- [七、数据库迁移](#七数据库迁移)
- [八、回滚](#八回滚)
- [九、部署后验证](#九部署后验证)
- [十、快速参考卡](#十快速参考卡)

---

## 一、升级概览：两条路径

| 维度 | 路径 A：CI 发版 | 路径 B：本地交叉编译 |
|------|----------------|-------------------|
| 适用场景 | 正式发版、内核升级、功能发布 | 紧急修复（生产故障） |
| 耗时 | 5-8 分钟 CI + 1 分钟 VPS 升级 | 5 分钟本地编译 + 部署 |
| 触发方式 | `git tag v* && git push` | 本地 `go build` + SCP |
| 产出 | GitHub Release（14 个产物） | 单个二进制文件 |
| 前端 | 不含（前端需单独构建） | 不含 |

**选择建议**：内核升级（sing-box/xray）用路径 A，紧急 hotfix 用路径 B。

---

## 二、路径 A：CI 发版（标准流程）

### 2.1 提交代码

```bash
cd d:\机场搭建\yundu-src

# 查看改动
git status
git diff

# 提交（每个逻辑改动单独提交）
git add -A
git commit -m "fix(node-agent): 描述改动"

# 推送到 GitHub
git push origin main
```

### 2.2 打 tag 触发 CI

```bash
# 版本号规范：日常修复递增第三位
# v0.3.0 → v0.3.1 → v0.3.2
# 重大改动递增第二位：v0.4.0

git tag -a v0.3.1 -m "YunDu v0.3.1

变更:
- 升级 sing-box v1.13.14 → v1.14.0
- 修复 xxx"

git push origin v0.3.1
```

### 2.3 查看 CI 状态

浏览器打开 https://github.com/energie2008/yundu/actions

CI 正常 5-8 分钟完成，产物清单：

| 二进制 | 大小 | 说明 |
|--------|:----:|------|
| `node-agent-{amd64,arm64}` | ~81MB | 节点代理（含双内核） |
| `node-service-{amd64,arm64}` | ~59MB | 节点服务 |
| `api-gateway-{amd64,arm64}` | ~44MB | API 网关 |
| `identity-service-{amd64,arm64}` | ~49MB | 认证服务 |
| `subscription-service-{amd64,arm64}` | ~50MB | 订阅服务 |
| `traffic-service-{amd64,arm64}` | ~47MB | 流量统计 |
| `migrate-{amd64,arm64}` | ~14MB | 数据库迁移 |

### 2.4 VPS 升级

```bash
# 升级 node-agent（VPS 节点上执行）
curl -fsSL https://github.com/energie2008/yundu/raw/main/install.sh | bash -s -- upgrade agent

# 升级面板服务（vps190 上执行）
curl -fsSL https://github.com/energie2008/yundu/raw/main/install.sh | bash -s -- upgrade panel

# 全部升级
curl -fsSL https://github.com/energie2008/yundu/raw/main/install.sh | bash -s -- upgrade all
```

---

## 三、路径 B：本地交叉编译热修复（紧急 hotfix）

> 适用：生产故障需 5 分钟内修复。密钥路径 `D:\机场搭建\key\`

### 3.1 编译 node-agent（含 sing-box/xray 双内核）

```powershell
# Windows PowerShell
cd D:\机场搭建\yundu-src\apps\node-agent
$env:GOOS="linux"; $env:GOARCH="amd64"; $env:CGO_ENABLED="0"

# ⚠️ 6 个编译标签缺一不可
go build -tags "with_utls,with_wireguard,with_gvisor,with_quic,with_grpc,with_clash_api" `
  -ldflags "-s -w -X main.AgentVersion=v0.8.2" `
  -o D:\机场搭建\tmp\node-agent-new ./cmd/agent/
```

### 3.2 编译面板服务（纯 Go，无需 tags）

```powershell
cd D:\机场搭建\yundu-src\apps\node-service
$env:GOOS="linux"; $env:GOARCH="amd64"; $env:CGO_ENABLED="0"
go build -o D:\机场搭建\tmp\node-service-new ./cmd/api/
```

### 3.3 上传并部署

```powershell
# 上传 node-agent 到 vps206（节点）
scp -i "D:\机场搭建\key\206key" `
  D:\机场搭建\tmp\node-agent-new ubuntu@158.101.13.206:/tmp/

# 上传 node-service 到 vps190（面板）
scp -i "D:\机场搭建\key\190key.pem" `
  D:\机场搭建\tmp\node-service-new root@43.135.147.190:/tmp/
```

**部署 node-agent（节点端）**：

```bash
# SSH 到目标 VPS
ssh -i "D:\机场搭建\key\206key" ubuntu@158.101.13.206

# ⚠️ 先 chmod +x（SCP 上传后是 644，无执行权限）
chmod +x /tmp/node-agent-new

# 必须先 stop 再 cp（运行中文件不可覆盖，报 Text file busy）
sudo systemctl stop yundu-node-agent
sudo cp /tmp/node-agent-new /opt/yundu/bin/node-agent
sudo systemctl start yundu-node-agent

# 验证
sudo systemctl status yundu-node-agent --no-pager | head -5
sudo journalctl -u yundu-node-agent -n 10 --no-pager
```

**部署 node-service（面板端）**：

```bash
ssh -i "D:\机场搭建\key\190key.pem" root@43.135.147.190
chmod +x /tmp/node-service-new
systemctl stop yundu-node-service
cp /tmp/node-service-new /opt/yundu/bin/node-service
systemctl start yundu-node-service
systemctl status yundu-node-service --no-pager | head -5
tail -5 /opt/yundu/logs/node-service.log
```

### 3.4 一键部署组合命令

```bash
# node-agent 一键部署（vps206）
ssh -i "D:\机场搭建\key\206key" ubuntu@158.101.13.206 '
set -e
chmod +x /tmp/node-agent-new
sudo systemctl stop yundu-node-agent
sudo cp /tmp/node-agent-new /opt/yundu/bin/node-agent
sudo systemctl start yundu-node-agent
echo "=== STATUS ==="
sudo systemctl is-active yundu-node-agent
echo "=== LOG ==="
sudo journalctl -u yundu-node-agent -n 5 --no-pager
echo "=== DEPLOY_OK ==="
'

# node-service 一键部署（vps190）
ssh -i "D:\机场搭建\key\190key.pem" root@43.135.147.190 '
set -e
chmod +x /tmp/node-service-new
systemctl stop yundu-node-service
cp /tmp/node-service-new /opt/yundu/bin/node-service
systemctl start yundu-node-service
echo "=== STATUS ==="
systemctl is-active yundu-node-service
echo "=== LOG ==="
tail -5 /opt/yundu/logs/node-service.log
echo "=== DEPLOY_OK ==="
'
```

---

## 四、sing-box 内核升级

### 4.1 当前版本

| 依赖 | 版本 | 用途 |
|------|------|------|
| `github.com/sagernet/sing-box` | **v1.13.14** | 主内核（所有协议） |
| `github.com/sagernet/sing` | **v0.8.11** | sing-box 核心库 |
| `github.com/sagernet/sing-box/option` | — | 配置解析 |
| `github.com/sagernet/sing-box/include` | — | 协议注册表 |

### 4.2 升级步骤

```bash
cd d:\机场搭建\yundu-src\apps\node-agent

# 升级 sing-box 主库
go get github.com/sagernet/sing-box@v1.14.0

# 同步升级 sing 核心库（通常需一起升）
go get github.com/sagernet/sing@latest

# 整理依赖
go mod tidy
```

### 4.3 编译验证

```powershell
cd D:\机场搭建\yundu-src\apps\node-agent
$env:GOOS="linux"; $env:GOARCH="amd64"; $env:CGO_ENABLED="0"

# 编译验证（90% 的问题在编译阶段暴露）
go build -tags "with_utls,with_wireguard,with_gvisor,with_quic,with_grpc,with_clash_api" `
  -o D:\机场搭建\tmp\sb-test ./cmd/agent/
```

**如果编译失败**，通常是以下 API 变更：

| 可能的变更 | 涉及文件 | 修复方式 |
|-----------|---------|---------|
| `box.Options` 结构体字段变化 | `native_singbox.go` | 对照新版本调整字段 |
| `box.Box.Router().AppendTracker()` 签名变化 | `native_singbox.go:159,188` | 调整方法调用 |
| `option.Options.UnmarshalJSONContext()` 签名变化 | `native_singbox.go:125` | 调整方法调用 |
| `include.Context()` 返回值类型变化 | `native_singbox.go:121` | 调整类型断言 |

### 4.4 灰度部署

先部署到 VPS81（非核心节点），观察 24h：

```bash
# 部署到 VPS81
ssh -i "D:\机场搭建\key\81key" ubuntu@132.226.156.81 '
set -e
sudo systemctl stop yundu-node-agent
sudo cp /tmp/node-agent-new /opt/yundu/bin/node-agent
sudo systemctl start yundu-node-agent
sudo journalctl -u yundu-node-agent -n 20 --no-pager
'
```

**观察指标**（24h）：

| 指标 | 检查方式 | 预期 |
|------|---------|------|
| 心跳正常 | 面板 → 节点列表 | 绿色在线 |
| 用户连接正常 | 无断流投诉 | — |
| 流量统计正常 | 面板 → 流量图表 | 数据持续更新 |
| 日志无 panic | `journalctl -u yundu-node-agent \| grep -i panic` | 无输出 |
| 节点下发状态 | SQL 见 §9.1 | 全部 `pushed` |

确认 VPS81 稳定后，再部署到 VPS206（核心节点）。

---

## 五、xray-core 内核升级

### 5.1 当前版本

| 依赖 | 版本 | 用途 |
|------|------|------|
| `github.com/xtls/xray-core` | **v1.260327.0** | 辅内核（XHTTP 协议） |

### 5.2 升级步骤

```bash
cd d:\机场搭建\yundu-src\apps\node-agent

# 升级 xray-core
go get github.com/xtls/xray-core@v1.260328.0

# 整理依赖
go mod tidy
```

### 5.3 特点

**xray 升级风险远低于 sing-box**，因为你的使用面主要是 gRPC 接口：

| API | 用途 | 稳定性 |
|-----|------|--------|
| `core.LoadConfig("json", ...)` | 从字节流加载配置 | 多年未变 |
| `core.New(config)` | 创建内核实例 | 稳定 |
| `proxymanCmd.HandlerServiceClient.AlterInbound()` | 增量用户管理 | gRPC 协议，兼容性好 |
| `statsCmd.StatsServiceClient.QueryStats()` | 流量统计查询 | gRPC 协议，兼容性好 |

编译验证和灰度部署步骤同 sing-box（§4.3-§4.4）。

---

## 六、面板服务升级（node-service 等）

> 内核升级（sing-box/xray）只需编译 `node-agent`，其他面板服务不动。
> 只有改了面板代码（node-service/api-gateway/identity-service 等）才需要升级面板。

### 6.1 哪些情况需要升级面板

| 情况 | 需要编译的服务 |
|------|--------------|
| 只改了 sing-box/xray 版本 | 仅 `node-agent` |
| 改了 node-service 代码 | `node-service` |
| 改了 API 网关 | `api-gateway` |
| 改了认证/订单 | `identity-service` |
| 改了订阅 | `subscription-service` |
| 改了流量统计 | `traffic-service` |
| 有新的数据库迁移 | `migrate` + SQL |

### 6.2 面板编译

```powershell
# 面板服务是纯 Go，无需编译标签
cd D:\机场搭建\yundu-src\apps\node-service
$env:GOOS="linux"; $env:GOARCH="amd64"; $env:CGO_ENABLED="0"
go build -o D:\机场搭建\tmp\node-service-new ./cmd/api/
```

---

## 七、数据库迁移

> 只在有新的 migration SQL 文件时才需要执行。

### 7.1 上传 SQL 并执行

```powershell
scp -i "D:\机场搭建\key\190key.pem" `
  D:\机场搭建\yundu-src\migrations\0000NN_xxx.sql `
  root@43.135.147.190:/opt/yundu/migrations/
```

```bash
ssh -i "D:\机场搭建\key\190key.pem" root@43.135.147.190 '
# 复制到 postgres 容器
docker cp /opt/yundu/migrations/0000NN_xxx.sql yundu-postgres:/tmp/m.sql

# 执行 SQL（用 -f 避免引号问题）
docker exec yundu-postgres psql -U app -d airport -f /tmp/m.sql

# 注册 goose 版本号（NN 替换为实际版本号）
docker exec yundu-postgres psql -U app -d airport -c \
  "INSERT INTO goose_db_version (version_id, is_applied, tstamp) VALUES (NN, true, NOW()) ON CONFLICT DO NOTHING;"

# 验证
docker exec yundu-postgres psql -U app -d airport -c \
  "SELECT version_id, is_applied FROM goose_db_version ORDER BY version_id DESC LIMIT 5;"
'
```

### 7.2 使用 migrate 工具（推荐）

```bash
# 如果面板服务已升级到含新 migrate 的版本
/opt/yundu/bin/migrate up
```

---

## 八、回滚

### 8.1 回滚 node-agent 到指定版本

```bash
# 通过 CI 回滚（指定旧版本 tag）
curl -fsSL https://github.com/energie2008/yundu/raw/main/install.sh | bash -s -- upgrade agent --version=v0.3.0

# 查看所有历史版本
curl -s https://api.github.com/repos/energie2008/yundu/releases | jq '.[] | .tag_name'
```

### 8.2 本地备份回滚（hotfix 场景）

```bash
# 部署前备份当前二进制
cp /opt/yundu/bin/node-agent /opt/yundu/bin/node-agent.bak.$(date +%Y%m%d%H%M)

# 出问题时恢复
sudo systemctl stop yundu-node-agent
sudo cp /opt/yundu/bin/node-agent.bak.202608071200 /opt/yundu/bin/node-agent
sudo systemctl start yundu-node-agent
```

### 8.3 回滚数据库迁移

```bash
# 回滚最近一次迁移
/opt/yundu/bin/migrate down
```

---

## 九、部署后验证

### 9.1 节点下发状态验证

```bash
ssh -i "D:\机场搭建\key\190key.pem" root@43.135.147.190 '
cat > /tmp/check_nodes.sql << "SQLEOF"
SELECT s.code AS server, n.code AS node,
       n.metadata->>'"'"'_dispatch_status'"'"' AS status,
       n.metadata->>'"'"'_dispatch_version'"'"' AS ver
FROM nodes n
JOIN runtimes r ON n.runtime_id=r.id
JOIN servers s ON r.server_id=s.id
WHERE n.deleted_at IS NULL
ORDER BY s.code, n.code;
SQLEOF
docker cp /tmp/check_nodes.sql yundu-postgres:/tmp/check_nodes.sql
docker exec yundu-postgres psql -U app -d airport -f /tmp/check_nodes.sql
'
```

预期：全部节点 `status=pushed`，无 `failed`。

### 9.2 日志检查

```bash
# node-agent（节点端，journald）
journalctl -u yundu-node-agent -n 50 --no-pager | grep -i 'ERROR\|panic\|fatal'
# 预期：无输出（或仅 WARN 级别）

# node-service（面板端，文件日志）
tail -50 /opt/yundu/logs/node-service.log | grep -i 'ERROR\|panic\|fatal'
```

### 9.3 版本确认

```bash
# 查看 node-agent 版本
/opt/yundu/bin/node-agent --version

# 查看面板最新 Release 版本
curl -s https://api.github.com/repos/energie2008/yundu/releases/latest | jq .tag_name
```

---

## 十、快速参考卡

```bash
# ===== CI 发版（标准流程）=====
cd d:\机场搭建\yundu-src
git add -A && git commit -m "描述改动"
git push origin main
git tag -a v0.3.X -m "版本说明"
git push origin v0.3.X
# → CI 自动编译，VPS 执行 install.sh upgrade

# ===== 本地交叉编译 node-agent（内核升级/hotfix）=====
cd D:\机场搭建\yundu-src\apps\node-agent
$env:GOOS="linux"; $env:GOARCH="amd64"; $env:CGO_ENABLED="0"
go build -tags "with_utls,with_wireguard,with_gvisor,with_quic,with_grpc,with_clash_api" `
  -ldflags "-s -w -X main.AgentVersion=v0.8.2" `
  -o D:\机场搭建\tmp\node-agent-new ./cmd/agent/

# ===== 部署到 VPS206（节点）=====
scp -i "D:\机场搭建\key\206key" D:\机场搭建\tmp\node-agent-new ubuntu@158.101.13.206:/tmp/
ssh -i "D:\机场搭建\key\206key" ubuntu@158.101.13.206 '
chmod +x /tmp/node-agent-new
sudo systemctl stop yundu-node-agent
sudo cp /tmp/node-agent-new /opt/yundu/bin/node-agent
sudo systemctl start yundu-node-agent
sudo systemctl is-active yundu-node-agent
'

# ===== 部署到 VPS81（节点）=====
scp -i "D:\机场搭建\key\81key" D:\机场搭建\tmp\node-agent-new ubuntu@132.226.156.81:/tmp/
ssh -i "D:\机场搭建\key\81key" ubuntu@132.226.156.81 '
chmod +x /tmp/node-agent-new
sudo systemctl stop yundu-node-agent
sudo cp /tmp/node-agent-new /opt/yundu/bin/node-agent
sudo systemctl start yundu-node-agent
sudo systemctl is-active yundu-node-agent
'

# ===== 部署到 vps190（面板服务）=====
scp -i "D:\机场搭建\key\190key.pem" D:\机场搭建\tmp\node-service-new root@43.135.147.190:/tmp/
ssh -i "D:\机场搭建\key\190key.pem" root@43.135.147.190 '
chmod +x /tmp/node-service-new
systemctl stop yundu-node-service
cp /tmp/node-service-new /opt/yundu/bin/node-service
systemctl start yundu-node-service
systemctl is-active yundu-node-service
'

# ===== 数据库迁移 =====
scp -i "D:\机场搭建\key\190key.pem" D:\机场搭建\yundu-src\migrations\0000NN.sql root@43.135.147.190:/opt/yundu/migrations/
ssh -i "D:\机场搭建\key\190key.pem" root@43.135.147.190 '
docker cp /opt/yundu/migrations/0000NN.sql yundu-postgres:/tmp/m.sql
docker exec yundu-postgres psql -U app -d airport -f /tmp/m.sql
docker exec yundu-postgres psql -U app -d airport -c \
  "INSERT INTO goose_db_version (version_id, is_applied, tstamp) VALUES (NN, true, NOW()) ON CONFLICT DO NOTHING;"
'

# ===== VPS 上升级（CI 发版后）=====
curl -fsSL https://github.com/energie2008/yundu/raw/main/install.sh | bash -s -- upgrade agent
curl -fsSL https://github.com/energie2008/yundu/raw/main/install.sh | bash -s -- upgrade panel
curl -fsSL https://github.com/energie2008/yundu/raw/main/install.sh | bash -s -- upgrade all

# ===== 回滚 =====
curl -fsSL https://github.com/energie2008/yundu/raw/main/install.sh | bash -s -- upgrade agent --version=v0.3.0

# ===== 查看最新版本 =====
curl -s https://api.github.com/repos/energie2008/yundu/releases/latest | jq .tag_name
```

### 密钥对照

| 密钥文件 | VPS | IP | 用户 | 用途 |
|---------|-----|-----|------|------|
| `190key.pem` | vps190 | 43.135.147.190 | root | 面板服务器 |
| `206key` | vps206 | 158.101.13.206 | ubuntu | 核心节点 |
| `81key` | vps81 | 132.226.156.81 | ubuntu | Argo Tunnel 节点 |

### 数据库连接信息

| 项 | 值 |
|----|-----|
| 用户 | `app`（不是 postgres / yundu） |
| 数据库 | `airport`（不是 yundu） |
| 端口 | `5433`（仅监听 127.0.0.1） |
| 密码 | `YunDuProd2026Secure` |
| 迁移表 | `goose_db_version`（不是 schema_migrations） |
| 查询方式 | `docker exec yundu-postgres psql -U app -d airport -f /tmp/xxx.sql` |