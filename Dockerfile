# YunDu 多阶段 Dockerfile
# Stage 1: builder  — 编译 node-service 与 node-agent（含 sing-box / xray-core 全功能标签）
# Stage 2: runtime  — alpine + ca-certificates + iptables + nginx + supervisor

# ============================================================
# Stage 1: builder
# ============================================================
FROM golang:1.22-alpine AS builder

# git: go mod 私有依赖 / 版本信息抓取需要
RUN apk add --no-cache git

WORKDIR /src

# 先复制 go.work / go.mod / go.sum，利用层缓存加速依赖下载
COPY go.work* ./
COPY packages/ ./packages/
COPY apps/node-service/go.mod apps/node-service/go.sum ./apps/node-service/
COPY apps/node-agent/go.mod apps/node-agent/go.sum ./apps/node-agent/

# 拉取依赖（monorepo workspace 模式）
RUN go mod download

# 复制全部源码
COPY . .

# 编译 node-service（控制面，无特殊标签）
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/node-service ./apps/node-service/cmd/server/

# 编译 node-agent（节点端，需 sing-box / xray-core 全功能标签）
#   with_utls       = REALITY
#   with_wireguard  = WireGuard
#   with_gvisor     = WireGuard 协议栈
#   with_quic       = Hysteria2 / Tuic
#   with_grpc       = gRPC 传输
#   with_clash_api  = 流量统计
RUN CGO_ENABLED=0 GOOS=linux go build \
      -tags "with_utls,with_wireguard,with_gvisor,with_quic,with_grpc,with_clash_api" \
      -ldflags "-s -w" \
      -o /out/node-agent ./apps/node-agent/cmd/agent/

# ============================================================
# Stage 2: runtime
# ============================================================
FROM alpine:3.19

# ca-certificates: HTTPS 出站；iptables: 透明代理 / 流量重定向；
# nginx: 前端反代 / TLS 终止；supervisor: 进程托管
RUN apk add --no-cache \
      ca-certificates \
      iptables \
      nginx \
      supervisor

# 复制编译产物
COPY --from=builder /out/node-service /usr/local/bin/node-service
COPY --from=builder /out/node-agent /usr/local/bin/node-agent

# supervisor 配置：托管 node-agent
RUN mkdir -p /etc/supervisor.d && \
    printf '[program:node-agent]\n\
command=/usr/local/bin/node-agent\n\
autostart=true\n\
autorestart=true\n\
startsecs=3\n\
startretries=10\n\
stdout_logfile=/dev/stdout\n\
stdout_logfile_maxbytes=0\n\
stderr_logfile=/dev/stderr\n\
stderr_logfile_maxbytes=0\n' > /etc/supervisor.d/node-agent.ini

# 443  = 节点入站（TLS/REALITY/Hysteria2 等）
# 8081 = node-service API
# 9090 = clash-api / 监控指标
EXPOSE 443 8081 9090

ENTRYPOINT ["/usr/bin/supervisord", "-n", "-c", "/etc/supervisord.conf"]
