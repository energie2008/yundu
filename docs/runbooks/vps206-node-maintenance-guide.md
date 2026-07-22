# VPS206 节点维护指南

> 服务器: Oracle ARM64 (158.101.13.206) | 位置: 美国
> 内核: Xray 26.3.27 + sing-box (双核架构) | nginx stream SNI分流
> 最后更新: 2026-07-20

---

## 一、节点清单

### 直连节点 (nginx stream SNI分流)

| 节点 | 协议 | 端口 | SNI/伪装 | 状态 |
|------|------|------|---------|------|
| P24 | Trojan+TCP+TLS | xray:9460 | cn-hnzz-cm-01-01.bilivideo.com | ✅ 17.76MB/s |
| P25 | AnyTLS | sing-box:9750 | yun1.dannelblog.na.am | ✅ 19.72MB/s |
| P20 | Hysteria2 | sing-box:40020(UDP) | — | ✅ 9.03MB/s |
| P15 | VLESS+XHTTP+REALITY | xray:9452 | sub3.dannelblog.na.am | ✅ |
| P17 | VLESS+XHTTP+REALITY | xray:9453 | sub4.dannelblog.na.am | ✅ |
| P06 | VLESS+XHTTP+REALITY+CDN | xray:9451 | sub6.dannelblog.na.am | ✅ |
| P07 | VLESS+XHTTP+REALITY+CDN | xray:9455 | sub5.dannelblog.na.am | ✅ |
| P09 | VLESS+XHTTP+REALITY | xray:8449 | sub2.dannelblog.na.am | ✅ |

### WS/gRPC/XHTTP节点 (nginx HTTP反代+CDN)

| 节点 | 协议 | 域名 | Path | 本地端口 | 状态 |
|------|------|------|------|---------|------|
| usvps206trojws | Trojan+WS+TLS | y3.dannelblog.na.am | /trojan9ad966 | 127.0.0.1:8454 | ✅ |
| usvps206vlessws | VLESS+WS+TLS | y3.dannelblog.na.am | /vless9ad966 | 127.0.0.1:9445 | ✅ |
| usvps206vmessws | VMess+WS+TLS | y3.dannelblog.na.am | /vmess9ad96602 | 127.0.0.1:9449 | ✅ |
| usvps206trojgrpc | Trojan+gRPC+TLS | y3.dannelblog.na.am | /grpc-path | 127.0.0.1:8453 | ✅ |
| usvps206vlessxhttp | VLESS+XHTTP | y3.dannelblog.na.am | /xhttp-path | 127.0.0.1:8451 | ✅ |
| vps206p20 | VLESS+XHTTP | y4.dannelblog.na.am | /xh-titok2026 | 127.0.0.1:8447 | ✅ |
| usvps206p06 | VLESS+XHTTP+REALITY+CDN | y5.dannelblog.na.am | /77cdddddddddd222d | 127.0.0.1:8600 | ✅ |

---

## 二、架构原理

### 2.1 双核分流架构

```
用户 → nginx:443 (ssl_preread读取SNI, 不终止TLS)
         │
         ├─ SNI=cn-hnzz-cm-01-01.bilivideo.com → 127.0.0.1:9460 (xray P24)
         ├─ SNI=yun1.dannelblog.na.am           → 127.0.0.1:9750 (sing-box P25)
         ├─ SNI=sub3.dannelblog.na.am           → 127.0.0.1:9452 (xray P15)
         └─ ...
```

### 2.2 关键设计原则

- **nginx stream ssl_preread**: 只"偷看"SNI，不终止TCP转发
- **TLS终止在xray/sing-box**: 证书由代理内核管理
- **双流合并**: xray处理VLESS/Trojan，sing-box处理AnyTLS/Hysteria2

---

## 三、问题与解决方案

### 问题1: TLS握手失败 `tlsv1 unrecognized name`

**现象**: 客户端报错 `SSL alert 112 (unrecognized_name)`，所有含SNI的TLS测试均失败。

**根因**: 自签名证书 **缺少 SAN (Subject Alternative Name)**。xray基于Go crypto/tls，只匹配SAN不匹配CN。

**诊断**:
```bash
openssl x509 -in /tmp/cert.pem -text -noout | grep -A1 "Subject Alternative Name"
# 无输出 = 证书缺少SAN
```

**修复**: 用openssl配置文件方式生成（**不可用-addext**，会导致BasicConstraints重复）:
```bash
cat > /tmp/san.cnf << 'EOF'
[req]
distinguished_name = req_distinguished_name
x509_extensions = v3_req
prompt = no

[req_distinguished_name]
C = CN
O = YunDu
CN = cn-hnzz-cm-01-01.bilivideo.com

[v3_req]
subjectAltName = @alt_names
basicConstraints = CA:TRUE

[alt_names]
DNS.1 = cn-hnzz-cm-01-01.bilivideo.com
EOF

openssl req -x509 -newkey rsa:2048 -nodes \
  -keyout /etc/yundu/config/certs/p24_key.pem \
  -out /etc/yundu/config/certs/p24_cert.pem \
  -days 3650 -config /tmp/san.cnf -extensions v3_req
```

**避坑**: `-addext` 参数会导致 `duplicate extension with OID "2.5.29.19"` 错误。

---

### 问题2: 客户端TLS验证失败 `certificate signed by unknown authority`

**现象**: 服务器端TLS已通，但v2rayN仍报错。

**根因**: xray 26.x 废弃了 `allowInsecure` 字段，需用 `insecure: true`。

**各客户端配置**:

**v2rayN**: 勾选 `AllowInsecure` + 地址填IP/入口域名 + SNI填bilivideo域名（分开填）

**sing-box 客户端**:
```json
{
  "type": "trojan",
  "server": "789.douyincdn.yundu.online",
  "server_port": 443,
  "password": "...",
  "tls": {
    "server_name": "cn-hnzz-cm-01-01.bilivideo.com",
    "insecure": true
  }
}
```

**避坑**: `server` 填入口域名/IP，`server_name` 填SNI，两者不能混用。

---

### 问题3: P20 双重TLS

**现象**: P20 VLESS+XHTTP 无法连接。

**根因**: nginx stream只做TCP转发（ssl_preread），不终止TLS。xray P20 inbound 配了 `security: tls`，造成双重加密。

**修复**: xray P20 inbound `security` 改为 `none`。

---

### 问题4: nginx stream SNI路由指向错误端口

**现象**: P25 连接失败。

**根因**: `yun1.dannelblog.na.am` 原来指向 Hysteria2 UDP 端口 40020，TCP流量被转到UDP端口。

**修复**:
```nginx
upstream upstream_tls_vpsp20 { server 127.0.0.1:9750; }  # sing-box AnyTCP TCP端口
# 不是 127.0.0.1:40020 (Hysteria2 UDP端口)
```

---

### 问题5: 域名填错导致连接失败

**现象**: 用 `789.yundu.online` 填到v2rayN没反应。

**根因**: 正确入口域名是 `789.douyincdn.yundu.online`（含douyincdn子域）。

---

### 问题6: VLESS+WS 与 Trojan+gRPC 端口冲突

**现象**: 面板API报错 `usvps206vlessws与usvps206trojgrpc在443冲突`

**根因**: CDN/Tunnel/SaaS节点的 `server_port` (面板侧接入端口) 不应设为443。443是nginx stream的端口，用于直连节点。CDN节点应使用 `local_port` (8446-8600范围)。

**修复**:
```bash
# usvps206vlessws: local_port 设为 8446-8600 范围 (如 9445)
# usvps206trojgrpc: local_port 设为 8446-8600 范围 (如 8453)
# 不要设为 443
```

**防火墙规则**:
```bash
# CDN节点端口仅本地访问
iptables -A INPUT -p tcp --dport 8446:8460 -s 127.0.0.1 -j ACCEPT
iptables -A INPUT -p tcp --dport 8446:8460 -j DROP
```

---

## 四、搭建教程: Trojan+TLS + Bilibili SNI

### 4.1 适用场景

- 直连节点，无CDN中转
- 利用国内视频网站SNI做伪装（bilivideo.com是国内CDN，GFW默认信任国内域名）
- 适合小流量、快速部署

### 4.2 架构

```
客户端 → 443 → nginx stream (ssl_preread)
                └→ SNI=bilivideo → xray:9460 (Trojan+TLS, 返回bilivideo证书)
```

### 4.3 步骤

**Step1: 生成带SAN的证书**:
```bash
DOMAIN="cn-hnzz-cm-01-01.bilivideo.com"
cat > /tmp/san.cnf << EOF
[req]
distinguished_name = req_distinguished_name
x509_extensions = v3_req
prompt = no
[req_distinguished_name]
C = CN
O = YunDu
CN = $DOMAIN
[v3_req]
subjectAltName = DNS:$DOMAIN
basicConstraints = CA:TRUE
EOF
openssl req -x509 -newkey rsa:2048 -nodes \
  -keyout /etc/ssl/bili_key.pem -out /etc/ssl/bili_cert.pem \
  -days 3650 -config /tmp/san.cnf -extensions v3_req
```

**Step2: xray inbound配置**:
```json
{
  "tag": "trojan-bili",
  "listen": "127.0.0.1",
  "port": 9460,
  "protocol": "trojan",
  "settings": { "clients": [{"password": "YOUR_PWD"}] },
  "streamSettings": {
    "network": "tcp",
    "security": "tls",
    "tlsSettings": {
      "serverName": "cn-hnzz-cm-01-01.bilivideo.com",
      "certificates": [{
        "certificateFile": "/etc/ssl/bili_cert.pem",
        "keyFile": "/etc/ssl/bili_key.pem"
      }]
    }
  }
}
```

**Step3: nginx stream路由**:
```nginx
map $ssl_preread_server_name $target {
  ~^cn-hnzz-cm-01-01\.bilivideo\.com$ 127.0.0.1:9460;
}
server {
  listen 443 reuseport;
  proxy_pass $target;
  ssl_preread on;
}
```

**Step4: 客户端 (v2rayN)**:
```
协议: Trojan
地址: 服务器IP (不要填SNI域名!)
端口: 443
SNI: cn-hnzz-cm-01-01.bilivideo.com
AllowInsecure: ✅
```

---

## 五、搭建教程: 中转站(Relay)

### 5.1 为什么用中转

| 直连 | 中转 |
|------|------|
| 用户直连Oracle IP | 用户 → 中转IP → Oracle IP |
| 商业IP段易被注意 | 源IP隐藏 |
| 线路不可控 | 线路CN2 GIA/992.98可选 |

### 5.2 架构

```
用户 → 入口域名 (华为DNS GeoDNS)
         ├─ 电信 → CN2 GIA IP ─┐
         ├─ 联通 → 992.98 IP   ─┼→ DNAT → Oracle IP:443
         └─ 境外 → BGP IP      ─┘
```

### 5.3 步骤

**Step1: 购买中转** (按端口计费约20元/月或按流量约50元/TB)

**Step2: 华为云GeoDNS配置**:
```
entry.yundu.online:
  电信 → A → CN2_GIA_IP
  联通 → A → 992.98_IP
  移动 → A → CMI_IP
  默认 → A → BGP_IP
```

**Step3: 中转服务器配置 (iptables DNAT)**:
```bash
# 在中转服务器
echo 1 > /proc/sys/net/ipv4/ip_forward
iptables -t nat -A PREROUTING -p tcp --dport 443 -j DNAT --to-destination 你的Oracle_IP:443
iptables -t NAT -A POSTROUTING -j MASQUERADE
```

或 **nginx stream方式**:
```nginx
stream {
  server { listen 443; proxy_pass 你的Oracle_IP:443; }
}
```

**Step4: 源站防火墙** (只允许中转IP访问):
```bash
iptables -A INPUT -p tcp --dport 443 ! -s 中转IP -j DROP
```

---

## 六、WS/gRPC/XHTTP 节点维护

### 6.1 架构差异

| 类型 | 直连节点 (P24/P25/REALITY) | WS/gRPC/XHTTP 节点 |
|------|--------------------------|-------------------|
| nginx层 | stream (TCP) | HTTP (第7层) |
| TLS终止 | xray/sing-box | nginx (边缘) |
| xray security | `tls` / `reality` | `none` |
| 反代方式 | ssl_preread SNI路由 | Host + Path 路由 |

### 6.2 WS 节点清单

| 节点 | 域名 | Path | 目标端口 | 协议 |
|------|------|------|---------|------|
| Trojan+WS | y3.dannelblog.na.am | /trojan9ad966 | 127.0.0.1:8454 | Trojan+WS |
| VLESS+WS | y3.dannelblog.na.am | /vless9ad966 | 127.0.0.1:9445 | VLESS+WS |
| VMess+WS | y3.dannelblog.na.am | /vmess9ad96602 | 127.0.0.1:9449 | VMess+WS |
| Trojan+gRPC | y3.dannelblog.na.am | /xhttp4cc53b6 | 127.0.0.1:8453 | Trojan+gRPC |
| VLESS+XHTTP | y3.dannelblog.na.am | /xhttp4cc53b6 | 127.0.0.1:8451 | VLESS+XHTTP |
| VLESS+XHTTP | y4.dannelblog.na.am | /xh-titok2026 | 127.0.0.1:8447 | VLESS+XHTTP |
| VLESS+XHTTP | y5.dannelblog.na.am | /77cdddddddddd222d | 127.0.0.1:8600 | VLESS+XHTTP |

### 6.3 nginx vhost 配置 (WS部分)

```nginx
# /etc/yundu/nginx/vhosts/yundu_autogen.conf
server {
    listen 8445 ssl http2;
    server_name y3.dannelblog.na.am;

    ssl_certificate /etc/ssl/xxx.pem;
    ssl_certificate_key /etc/ssl/xxx.key;

    # Trojan+WS
    location /trojan9ad966 {
        proxy_pass http://127.0.0.1:8454;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_read_timeout 300s;
    }

    # VLESS+WS
    location /vless9ad966 {
        proxy_pass http://127.0.0.1:9445;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }

    # VMess+WS
    location /vmess9ad96602 {
        proxy_pass http://127.0.0.1:9449;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }

    # Trojan+gRPC (HTTP/2)
    location /xhttp4cc53b6 {
        proxy_pass http://127.0.0.1:8453;
        proxy_http_version 1.1;
        grpc_pass grpc://127.0.0.1:8453;
    }

    # VLESS+XHTTP
    location /xhttp4cc53b6 {
        proxy_pass http://127.0.0.1:8451;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
```

### 6.4 xray WS inbound 配置示例 (服务端)

```json
{
  "tag": "in-usvps206trojws",
  "listen": "127.0.0.1",
  "port": 8454,
  "protocol": "trojan",
  "settings": { "clients": [{ "password": "9acd564f-..." }] },
  "streamSettings": {
    "network": "ws",
    "security": "none",
    "wsSettings": {
      "path": "/trojan9ad966",
      "headers": { "Host": "y3.dannelblog.na.am" }
    }
  }
}
```

**关键**: 服务端 `security: "none"` 因为TLS已在nginx反代时终止，nginx以明文HTTP转发给本地xray。

### 6.5 客户端配置示例 (v2rayN / NekoBox)

**客户端到服务器全程需要 TLS，因为 nginx 对外暴露的是 443 HTTPS!**

```
地址: y3.dannelblog.na.am
端口: 443
协议: VLESS / VMess
传输: WS
路径: /vless9ad966  (VLESS) 或 /vmess9ad96602 (VMess)
安全/加密: TLS ✅️ 必须开启!
SNI: y3.dannelblog.na.am
```

**VLESS JSON 订阅**:
```
vless://UUID@y3.dannelblog.na.am:443?encryption=none&security=tls&sni=y3.dannelblog.na.am&type=ws&host=y3.dannelblog.na.am&path=/vless9ad966#US-VLESS-WS
```

**关键避坑**:
- 服务端 `security: none` ≠ 客户端 `security: none`
- 服务端 none = TLS已在nginx终止，xray接收明文
- 客户端 tls = 客户端到nginx需要加密传输
- 配反了 = 连不上!

### 6.5 客户端订阅下发格式

**v2rayN sing-box 通用订阅**:
```
trojan://9acd564f-xxx@y3.dannelblog.na.am:443?type=ws&path=/trojan9ad966&security=tls&sni=y3.dannelblog.na.am#US-Trojan-WS

vless://uuid@y3.dannelblog.na.am:443?type=ws&path=/vless9ad966&security=tls&sni=y3.dannelblog.na.am&encryption=none#US-VLESS-WS

vmess://eyJ2IjoiMiIs...@y3.dannelblog.na.am:443?type=ws&path=/vmess9ad96602&security=tls&sni=y3.dannelblog.na.am#US-VMess-WS
```

### 6.6 WS 节点常见问题

**问题A: WS 连接后立即断开**

- 检查nginx是否支持WebSocket (`proxy_http_version 1.1` + `Upgrade` 头)
- 检查路径是否匹配 (location path == xray ws path)
- 检查CDN是否开启了WebSocket支持

**问题B: CDN 代理后客户端真实IP丢失**

```nginx
# nginx vhost 需加上
set_real_ip_from 103.21.244.0/22;  # CF IP段
set_real_ip_from 104.16.0.0/13;
# ... 更多CF IP段
real_ip_header CF-Connecting-IP;

# location 转发时带上真实IP
proxy_set_header X-Real-IP $remote_addr;
```

**问题C: gRPC 路径匹配**

```
gRPC 需要 HTTP/2 + POST 方法
nginx配置需确保:
  listen 8445 ssl http2;  # 必须http2
  grpc_pass grpc://127.0.0.1:8453;
```

### 6.7 AnyTLS (P25 sing-box)

```
客户端 → nginx:443 → sing-box:9750 (AnyTLS)
```

**sing-box P25 inbound 配置**:
```json
{
  "tag": "in-P25",
  "type": "anytls",
  "listen": "127.0.0.1",
  "listen_port": 9750,
  "users": [{ "password": "..." }],
  "tls": {
    "certificate": "/etc/yundu/config/certs/p25_cert.pem",
    "key": "/etc/yundu/config/certs/p25_key.pem",
    "server_name": "yun1.dannelblog.na.am"
  }
}
```

### 6.8 AnyTLS 维护注意

1. **证书**: 同SAN问题，AnyTLS也需要SAN证书
2. **客户端兼容**: AnyTLS仅支持部分客户端 (sing-box, Hiddify, NekoBox)
3. **与Hysteria2共用**: sing-box同时跑AnyTLS(9750)+Hysteria2(40020)，不冲突

### 6.9 WS路径安全

```
- 路径应随机且足够长 (如 /trojan9ad966, /xhttp4cc53b6)
- 定期更换路径可防止被识别
- 路径不应被搜索引擎收录 (robots.txt 或 随机字符串)
```

---

## 七、避坑指南

### 证书相关

1. **必须生成SAN**: Go的crypto/tls只匹配SAN不匹配CN
2. **不要-addext**: 用openssl配置文件方式，避免BasicConstraints重复
3. **客户端必须跳过验证**: 自签名证书需用 `insecure: true` 或勾AllowInsecure

### nginx stream相关

4. **TCP/UDP端口**: Hysteria2用UDP端口，AnyTLS用TCP端口
5. **ssl_preread不终止TLS**: nginx只做分流，TLS终止在xray/sing-box
6. **修改后reload**: `nginx -s reload`

### 客户端配置

7. **server vs server_name**: server填连接目标，server_name填SNI
8. **不要用SNI域名当IP**: bilivideo.com解析到B站服务器
9. **入口域名用完整正确域名**: `789.douyincdn.yundu.online` 不是 `789.yundu.online`

---

## 七、诊断命令速查

```bash
# 检查端口监听
ss -tlnp | grep -E "9460|9750|40020|443"

# 检查nginx stream路由
cat /etc/yundu/nginx/stream/yundu_autogen.conf | grep -E "upstream|map"

# 检查TLS握手
openssl s_client -connect 127.0.0.1:9460 -servername cn-hnzz-cm-01-01.bilivideo.com -brief

# 检查证书SAN
openssl x509 -in /path/cert.pem -text -noout | grep -A1 "Alternative Name"

# 查看xray日志
journalctl -u xray --since "5 minutes ago"

# VPS本地测速 (SOCKS5)
curl -x socks5://127.0.0.1:11094 -o /dev/null -s -w "speed:%{speed_download}" --max-time 10 URL
```

---

## 八、风控与安全建议

### SNI伪装选择原则

| SNI域名 | 类型 | 风险 | 建议 |
|---------|------|------|------|
| bilivideo.com | 国内视频CDN | 🟢 低 | 推荐，GFW默认信任国内域名 |
| .cn 域名 | 国内域名 | 🟢 低 | 可用 |
| microsoft.com | 国外商业 | 🟡 中 | SNI-IP矛盾时需要海外IP |
| google.com | 被封锁 | 🔴 高 | 不可用 |

### 流量策略

- **小流量** (<50GB/月): 当前方案足够
- **中流量**: 加中转服务器
- **大流量**: CDN + 多中转 + 多国落地

### 长期存活

- 单IP日流量控制在10-50GB以内
- SNI-IP尽量匹配（国内域名配国内IP效果最佳）
- 定期轮换IP（Oracle支持更换reserved IP）
- 同一SNI不要承载过高流量

---

## 九、相关文件速查

| 文件 | 作用 |
|------|------|
| `/etc/yundu/config/xray.json` | xray主配置 |
| `/etc/yundu/config/singbox.json` | sing-box配置 |
| `/etc/yundu/nginx/stream/yundu_autogen.conf` | nginx stream SNI分流 |
| `/etc/yundu/config/certs/` | 证书目录 |
| `/var/log/xray-yundu/xray-start.log` | xray启动日志 |
| `/var/log/nginx/error.log` | nginx错误日志 |
