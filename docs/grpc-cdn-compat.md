# gRPC Cloudflare CDN 兼容性文档

**文档版本**: 1.0  
**更新日期**: 2026-07-01

## 1. 概述

本文档描述了gRPC通信在三种不同部署环境下的兼容性表现、配置建议和故障排查指南。在机场节点与管理面板通信场景中，gRPC协议需要通过不同的网络环境（直连、Cloudflare CDN代理、Cloudflare Tunnel）进行传输，各环境对gRPC特性的支持程度存在差异。

## 2. 测试环境说明

| 环境 | 描述 | 网络拓扑 |
|------|------|----------|
| **直连443** | 节点直接暴露gRPC服务到公网，监听443端口，客户端直接连接源站服务器 | 客户端 → 源站服务器:443 |
| **CF Proxy开gRPC** | Cloudflare橙色云开启，gRPC网络级开关启用，通过CDN边缘节点代理gRPC流量 | 客户端 → CF边缘 → 源站服务器:443 |
| **CF Tunnel** | 通过cloudflared隧道将gRPC服务暴露到Cloudflare网络，无需开放入站端口 | 客户端 → CF边缘 → cloudflared隧道 → 源站gRPC |

## 3. 兼容性矩阵

| 功能项 | 直连443 | CF Proxy (gRPC开启) | CF Tunnel (cloudflared) |
|--------|:-------:|:-------------------:|:-----------------------:|
| 基础gRPC连接 | ✅ | ✅ | ✅ |
| 双向流式通信 (Stream RPC) | ✅ | ✅ (需HTTP/2) | ⚠️ (有缓冲) |
| 25s Keepalive ping | ✅ | ⚠️ (可能被截断，需调整>60s) | ⚠️ |
| HMAC认证头 | ✅ | ✅ | ✅ |
| 大消息 (>64KB) | ✅ | ⚠️ (需调整max-message-size) | ⚠️ |
| 连接时长 (>5min) | ✅ | ✅ (需disable idle timeout) | ✅ |
| 自动降级到WS | ✅ | ✅ | ✅ |

**图例说明**:
- ✅: 完全支持，无需特殊配置
- ⚠️: 部分支持，需调整配置或存在已知限制
- ❌: 不支持

## 4. 推荐配置

### 4.1 直连环境

适用于节点直接暴露公网、不经过CDN的场景。

```go
// gRPC服务端配置
grpcServer := grpc.NewServer(
    grpc.KeepaliveParams(keepalive.ServerParameters{
        MaxConnectionIdle:     0,
        MaxConnectionAge:      0,
        MaxConnectionAgeGrace: 5 * time.Second,
        Time:                  20 * time.Second,
        Timeout:               5 * time.Second,
    }),
    grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
        MinTime:             10 * time.Second,
        PermitWithoutStream: true,
    }),
    grpc.MaxRecvMsgSize(4*1024*1024),
    grpc.MaxSendMsgSize(4*1024*1024),
)
```

```go
// gRPC客户端配置
conn, err := grpc.Dial(
    address,
    grpc.WithTransportCredentials(creds),
    grpc.WithKeepaliveParams(keepalive.ClientParameters{
        Time:                20 * time.Second,
        Timeout:             5 * time.Second,
        PermitWithoutStream: true,
    }),
    grpc.WithDefaultCallOptions(
        grpc.MaxCallRecvMsgSize(4*1024*1024),
        grpc.MaxCallSendMsgSize(4*1024*1024),
    ),
)
```

### 4.2 CF Proxy环境

适用于通过Cloudflare CDN橙色云代理的场景。

**前置要求**:
1. Cloudflare控制面板 → 网络 → 开启「gRPC」开关
2. 源站需配置有效SSL证书（Cloudflare Origin CA证书或可信证书）
3. SSL/TLS加密模式设置为「完全」或「完全（严格）」

```go
// gRPC服务端配置（CF Proxy优化）
grpcServer := grpc.NewServer(
    grpc.KeepaliveParams(keepalive.ServerParameters{
        MaxConnectionIdle:     0,
        MaxConnectionAge:      30 * time.Minute,
        MaxConnectionAgeGrace: 30 * time.Second,
        Time:                  60 * time.Second,
        Timeout:               10 * time.Second,
    }),
    grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
        MinTime:             30 * time.Second,
        PermitWithoutStream: true,
    }),
    grpc.MaxRecvMsgSize(8*1024*1024),
    grpc.MaxSendMsgSize(8*1024*1024),
)
```

**Cloudflare配置要点**:
- 确保域名已开启橙色云（代理状态）
- 在「网络」设置中启用gRPC
- 在「规则」→「配置规则」中禁用gRPC路径的Idle Timeout：
  - 匹配表达式：`http.request.uri.path contains "/agent.v1.AgentService/"`
  - 设置项：「源站空闲超时」设置为「关」或更大值

### 4.3 CF Tunnel环境

适用于通过cloudflared隧道暴露服务的场景，无需开放入站端口。

**启动隧道命令**:
```bash
# 启动支持gRPC的cloudflared隧道
cloudflared tunnel --grpc \
  --url https://localhost:50051 \
  --hostname agent.example.com \
  run <tunnel-id>
```

```go
// gRPC服务端配置（CF Tunnel优化）
grpcServer := grpc.NewServer(
    grpc.KeepaliveParams(keepalive.ServerParameters{
        MaxConnectionIdle:     0,
        MaxConnectionAge:      15 * time.Minute,
        MaxConnectionAgeGrace: 30 * time.Second,
        Time:                  75 * time.Second,
        Timeout:               15 * time.Second,
    }),
    grpc.MaxRecvMsgSize(2*1024*1024),
    grpc.MaxSendMsgSize(2*1024*1024),
)
```

**重要提示**:
- CF Tunnel对双向流式通信存在缓冲，长连接场景建议启用WebSocket自动降级
- 消息大小建议控制在2MB以内，避免缓冲导致的延迟
- 优先使用WebSocket通道进行高频双向通信

## 5. 故障排查

### 5.1 连接被重置

**症状**: 客户端立即收到 `UNAVAILABLE: connection reset by peer` 错误

**排查步骤**:
1. 确认Cloudflare控制面板中gRPC开关已开启
2. 检查SSL/TLS加密模式是否为「完全」或以上
3. 验证源站是否监听在443端口并使用TLS
4. 使用grpcurl测试直连源站是否正常：
   ```bash
   grpcurl -insecure source-server:443 list
   ```

### 5.2 Keepalive断连

**症状**: 空闲连接约60秒后被断开，日志显示`RST_STREAM`或`GOAWAY`帧

**解决方案**:
1. 将keepalive间隔从25s调整为60s以上
2. 客户端配置示例：
   ```go
   grpc.WithKeepaliveParams(keepalive.ClientParameters{
       Time:    70 * time.Second,
       Timeout: 10 * time.Second,
   })
   ```
3. 在Cloudflare配置规则中为gRPC路径延长超时

### 5.3 消息被截断

**症状**: 传输大消息时收到`RESOURCE_EXHAUSTED: Received message larger than max`

**解决方案**:
1. 调整gRPC消息大小限制：
   ```go
   // 服务端
   grpc.MaxRecvMsgSize(8*1024*1024)
   // 客户端调用
   grpc.MaxCallRecvMsgSize(8*1024*1024)
   ```
2. 检查Cloudflare WAF规则是否限制请求体大小
3. 考虑将大消息分块传输

### 5.4 TLS握手失败

**症状**: 客户端报`TLS handshake error`或证书验证失败

**排查步骤**:
1. 确认Cloudflare边缘证书有效且未过期
2. 源站需安装Cloudflare Origin CA证书
3. 避免在源站使用自签名证书配合「严格」SSL模式
4. 验证证书域名匹配：
   ```bash
   openssl s_client -connect agent.example.com:443 -servername agent.example.com
   ```

## 6. 降级策略

为保证在各种网络环境下的连通性，系统采用三通道自动降级机制：

### 6.1 通道优先级

```
gRPC (最高优先级)
  ↓ 连续失败3次
WebSocket
  ↓ 连续失败3次
HTTP/1.1 轮询 (最低优先级)
```

### 6.2 健康检查与降级逻辑

```go
type ChannelState int

const (
    ChannelGRPC ChannelState = iota
    ChannelWebSocket
    ChannelHTTP
)

type ChannelManager struct {
    currentChannel ChannelState
    failureCount   int
    lastUpgrade    time.Time
}

func (cm *ChannelManager) RecordFailure() {
    cm.failureCount++
    if cm.failureCount >= 3 {
        cm.Downgrade()
    }
}

func (cm *ChannelManager) Downgrade() {
    switch cm.currentChannel {
    case ChannelGRPC:
        cm.currentChannel = ChannelWebSocket
    case ChannelWebSocket:
        cm.currentChannel = ChannelHTTP
    }
    cm.failureCount = 0
    cm.lastUpgrade = time.Now()
}

func (cm *ChannelManager) TryUpgrade() {
    if time.Since(cm.lastUpgrade) < 60*time.Second {
        return
    }
    // 尝试升级到更高优先级通道
    switch cm.currentChannel {
    case ChannelHTTP:
        if cm.ProbeWebSocket() {
            cm.currentChannel = ChannelWebSocket
            cm.lastUpgrade = time.Now()
        }
    case ChannelWebSocket:
        if cm.ProbeGRPC() {
            cm.currentChannel = ChannelGRPC
            cm.lastUpgrade = time.Now()
        }
    }
}
```

### 6.3 探测实现要点

- gRPC探测：发起空的健康检查RPC，超时5秒
- WebSocket探测：建立WS连接后发送ping帧，3秒内收到pong为成功
- HTTP探测：发起GET /health请求，200响应为成功
- 每60秒自动尝试升级回高优先级通道
- 升级期间不影响当前通道的正常通信

## 7. 最佳实践总结

1. **配置统一**
   - 所有环境启用HMAC认证，不要依赖网络层安全
   - 消息体大小限制设置为4MB或更小，避免CDN缓冲问题
   - 客户端必须实现自动降级逻辑

2. **CF Proxy首选**
   - 生产环境优先使用CF Proxy + gRPC开启模式
   - 记得配置Origin证书并设置正确的SSL模式
   - 为gRPC路径禁用Idle Timeout

3. **Tunnel适用场景**
   - 节点在NAT后面无法开放端口时使用
   - 长连接、高并发场景建议降级到WebSocket
   - 消息体控制在2MB以内

4. **监控告警**
   - 监控通道降级事件，频繁降级说明网络环境异常
   - 记录gRPC状态码分布，重点关注UNAVAILABLE和RESOURCE_EXHAUSTED
   - 跟踪连接时长分布，异常断连需及时告警

5. **客户端适配**
   - 客户端配置重试机制：指数退避 + 最多3次重试
   - 对idempotent的RPC启用自动重试
   - 流式RPC失败时自动重建连接

---

## 参考链接

- [Cloudflare gRPC 官方文档](https://developers.cloudflare.com/support/network/configure-web3-gateway/grpc/)
- [Cloudflare Tunnel 文档](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/)
- [gRPC Keepalive 配置指南](https://grpc.io/docs/guides/keepalive/)
- [gRPC 错误码参考](https://grpc.io/docs/guides/status-codes/)
