-- +goose Up
-- +goose StatementBegin

-- ============================================================
-- 协议预设表（一键选择常用配置组合）
-- ============================================================
CREATE TABLE protocol_presets (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  code VARCHAR(64) NOT NULL UNIQUE,
  name VARCHAR(128) NOT NULL,
  description TEXT,
  protocol_type VARCHAR(32) NOT NULL,
  transport_type VARCHAR(32) NOT NULL,
  security_type VARCHAR(32) NOT NULL,
  default_config JSONB NOT NULL DEFAULT '{}'::jsonb,
  recommended_port INTEGER NOT NULL DEFAULT 443,
  icon VARCHAR(32),
  sort_order INTEGER NOT NULL DEFAULT 0,
  is_recommended BOOLEAN NOT NULL DEFAULT false,
  is_enabled BOOLEAN NOT NULL DEFAULT true,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_protocol_presets_enabled ON protocol_presets(is_enabled) WHERE is_enabled = true;
CREATE INDEX idx_protocol_presets_sort ON protocol_presets(sort_order);

-- ============================================================
-- 补充缺失的协议注册（14种，加上已有4种共18种）
-- ============================================================

-- P02: vless + xhttp + tls
INSERT INTO protocol_registry (protocol_type, transport_type, security_type, schema_version, config_schema, description) VALUES
('vless', 'xhttp', 'tls', 'v1', '{
  "type": "object",
  "properties": {
    "uuid": {"type": "string", "format": "uuid"},
    "xhttp_mode": {"type": "string", "enum": ["auto", "packet-up", "stream-up", "stream-one"], "default": "auto"},
    "xhttp_host": {"type": "array", "items": {"type": "string"}, "default": []},
    "xhttp_path": {"type": "string", "default": ""},
    "xhttp_extra": {"type": "object", "properties": {
      "sc_max_early_data": {"type": "integer", "default": 1400},
      "sc_min_early_data": {"type": "integer", "default": 100},
      "no_grpc_header": {"type": "boolean", "default": false}
    }},
    "tls": {
      "type": "object",
      "properties": {
        "server_name": {"type": "string"},
        "alpn": {"type": "array", "items": {"type": "string", "enum": ["h3", "h2", "http/1.1"]}, "default": ["h2", "http/1.1"]},
        "min_version": {"type": "string", "default": "1.3"},
        "fingerprint": {"type": "string", "enum": ["chrome", "firefox", "safari", "ios", "edge", "random"], "default": "chrome"},
        "allow_insecure": {"type": "boolean", "default": false},
        "certificate_mode": {"type": "string", "enum": ["acme", "upload", "self_signed"], "default": "acme"}
      },
      "required": ["server_name"]
    }
  },
  "required": ["uuid"]
}'::jsonb, 'VLESS + XHTTP + TLS，新一代HTTP传输，推荐用于CDN中转场景')
ON CONFLICT (protocol_type, transport_type, security_type, schema_version) DO NOTHING;

-- P03: vless + xhttp + reality (主推)
INSERT INTO protocol_registry (protocol_type, transport_type, security_type, schema_version, config_schema, description) VALUES
('vless', 'xhttp', 'reality', 'v1', '{
  "type": "object",
  "properties": {
    "uuid": {"type": "string", "format": "uuid"},
    "flow": {"type": "string", "enum": ["", "xtls-rprx-vision"], "default": "xtls-rprx-vision"},
    "xhttp_mode": {"type": "string", "enum": ["auto", "packet-up", "stream-up", "stream-one"], "default": "auto"},
    "xhttp_host": {"type": "array", "items": {"type": "string"}, "default": []},
    "xhttp_path": {"type": "string", "default": "/"},
    "xhttp_extra": {"type": "object", "properties": {
      "sc_max_early_data": {"type": "integer", "default": 1400},
      "sc_min_early_data": {"type": "integer", "default": 100},
      "no_grpc_header": {"type": "boolean", "default": false}
    }},
    "reality": {
      "type": "object",
      "properties": {
        "dest": {"type": "string", "default": "www.microsoft.com:443"},
        "server_names": {"type": "array", "items": {"type": "string"}, "default": ["www.microsoft.com"]},
        "private_key": {"type": "string"},
        "public_key": {"type": "string"},
        "short_ids": {"type": "array", "items": {"type": "string", "pattern": "^[0-9a-f]{0,16}$"}, "default": [""]},
        "spider_x": {"type": "string", "default": "/"},
        "fingerprint": {"type": "string", "enum": ["chrome", "firefox", "safari", "ios", "edge", "random"], "default": "chrome"},
        "max_time_diff": {"type": "integer", "default": 60000}
      },
      "required": ["dest", "server_names", "private_key"]
    }
  },
  "required": ["uuid"]
}'::jsonb, 'VLESS + XHTTP + REALITY ⭐主推方案，抗封锁能力最强，2026推荐首选')
ON CONFLICT (protocol_type, transport_type, security_type, schema_version) DO NOTHING;

-- P05: vless + ws + tls
INSERT INTO protocol_registry (protocol_type, transport_type, security_type, schema_version, config_schema, description) VALUES
('vless', 'ws', 'tls', 'v1', '{
  "type": "object",
  "properties": {
    "uuid": {"type": "string", "format": "uuid"},
    "ws_path": {"type": "string", "default": "/"},
    "ws_host": {"type": "string", "default": ""},
    "ws_early_data": {"type": "integer", "default": 2048},
    "ws_browser_forwarding": {"type": "boolean", "default": false},
    "tls": {
      "type": "object",
      "properties": {
        "server_name": {"type": "string"},
        "alpn": {"type": "array", "items": {"type": "string"}, "default": ["h2", "http/1.1"]},
        "fingerprint": {"type": "string", "enum": ["chrome", "firefox", "safari", "ios", "edge", "random"], "default": "chrome"},
        "allow_insecure": {"type": "boolean", "default": false},
        "certificate_mode": {"type": "string", "enum": ["acme", "upload", "self_signed"], "default": "acme"}
      },
      "required": ["server_name"]
    }
  },
  "required": ["uuid"]
}'::jsonb, 'VLESS + WebSocket + TLS，CDN中转最常用方案')
ON CONFLICT (protocol_type, transport_type, security_type, schema_version) DO NOTHING;

-- P06: vless + grpc + tls
INSERT INTO protocol_registry (protocol_type, transport_type, security_type, schema_version, config_schema, description) VALUES
('vless', 'grpc', 'tls', 'v1', '{
  "type": "object",
  "properties": {
    "uuid": {"type": "string", "format": "uuid"},
    "grpc_service_name": {"type": "string", "default": ""},
    "grpc_multi_mode": {"type": "boolean", "default": true},
    "grpc_idle_timeout": {"type": "integer", "default": 60},
    "grpc_health_check": {"type": "boolean", "default": false},
    "tls": {
      "type": "object",
      "properties": {
        "server_name": {"type": "string"},
        "alpn": {"type": "array", "items": {"type": "string"}, "default": ["h2", "http/1.1"]},
        "fingerprint": {"type": "string", "enum": ["chrome", "firefox", "safari", "ios", "edge", "random"], "default": "chrome"},
        "allow_insecure": {"type": "boolean", "default": false},
        "certificate_mode": {"type": "string", "enum": ["acme", "upload", "self_signed"], "default": "acme"}
      },
      "required": ["server_name"]
    }
  },
  "required": ["uuid"]
}'::jsonb, 'VLESS + gRPC + TLS，多复用高效传输，移动端表现优秀')
ON CONFLICT (protocol_type, transport_type, security_type, schema_version) DO NOTHING;

-- P07: vless + h2 + tls
INSERT INTO protocol_registry (protocol_type, transport_type, security_type, schema_version, config_schema, description) VALUES
('vless', 'h2', 'tls', 'v1', '{
  "type": "object",
  "properties": {
    "uuid": {"type": "string", "format": "uuid"},
    "h2_path": {"type": "string", "default": "/"},
    "h2_host": {"type": "array", "items": {"type": "string"}, "default": []},
    "tls": {
      "type": "object",
      "properties": {
        "server_name": {"type": "string"},
        "alpn": {"type": "array", "items": {"type": "string"}, "default": ["h2", "http/1.1"]},
        "fingerprint": {"type": "string", "enum": ["chrome", "firefox", "safari", "ios", "edge", "random"], "default": "chrome"},
        "allow_insecure": {"type": "boolean", "default": false},
        "certificate_mode": {"type": "string", "enum": ["acme", "upload", "self_signed"], "default": "acme"}
      },
      "required": ["server_name"]
    }
  },
  "required": ["uuid"]
}'::jsonb, 'VLESS + HTTP/2 + TLS，多路复用传输')
ON CONFLICT (protocol_type, transport_type, security_type, schema_version) DO NOTHING;

-- P08: trojan + tcp + tls
INSERT INTO protocol_registry (protocol_type, transport_type, security_type, schema_version, config_schema, description) VALUES
('trojan', 'tcp', 'tls', 'v1', '{
  "type": "object",
  "properties": {
    "password": {"type": "string"},
    "tcp_fast_open": {"type": "boolean", "default": false},
    "tcp_no_fingerprint": {"type": "boolean", "default": false},
    "tls": {
      "type": "object",
      "properties": {
        "server_name": {"type": "string"},
        "alpn": {"type": "array", "items": {"type": "string", "enum": ["h2", "http/1.1"]}, "default": ["h2", "http/1.1"]},
        "fingerprint": {"type": "string", "enum": ["chrome", "firefox", "safari", "ios", "edge", "random"], "default": "chrome"},
        "allow_insecure": {"type": "boolean", "default": false},
        "certificate_mode": {"type": "string", "enum": ["acme", "upload", "self_signed"], "default": "acme"}
      },
      "required": ["server_name"]
    }
  },
  "required": ["password"]
}'::jsonb, 'Trojan + TCP + TLS，经典协议，广泛兼容')
ON CONFLICT (protocol_type, transport_type, security_type, schema_version) DO NOTHING;

-- P09: trojan + ws + tls
INSERT INTO protocol_registry (protocol_type, transport_type, security_type, schema_version, config_schema, description) VALUES
('trojan', 'ws', 'tls', 'v1', '{
  "type": "object",
  "properties": {
    "password": {"type": "string"},
    "ws_path": {"type": "string", "default": "/"},
    "ws_host": {"type": "string", "default": ""},
    "ws_early_data": {"type": "integer", "default": 2048},
    "tls": {
      "type": "object",
      "properties": {
        "server_name": {"type": "string"},
        "alpn": {"type": "array", "items": {"type": "string"}, "default": ["h2", "http/1.1"]},
        "fingerprint": {"type": "string", "enum": ["chrome", "firefox", "safari", "ios", "edge", "random"], "default": "chrome"},
        "allow_insecure": {"type": "boolean", "default": false},
        "certificate_mode": {"type": "string", "enum": ["acme", "upload", "self_signed"], "default": "acme"}
      },
      "required": ["server_name"]
    }
  },
  "required": ["password"]
}'::jsonb, 'Trojan + WebSocket + TLS，CDN中转方案')
ON CONFLICT (protocol_type, transport_type, security_type, schema_version) DO NOTHING;

-- P10: trojan + grpc + tls
INSERT INTO protocol_registry (protocol_type, transport_type, security_type, schema_version, config_schema, description) VALUES
('trojan', 'grpc', 'tls', 'v1', '{
  "type": "object",
  "properties": {
    "password": {"type": "string"},
    "grpc_service_name": {"type": "string", "default": ""},
    "grpc_multi_mode": {"type": "boolean", "default": true},
    "grpc_idle_timeout": {"type": "integer", "default": 60},
    "tls": {
      "type": "object",
      "properties": {
        "server_name": {"type": "string"},
        "alpn": {"type": "array", "items": {"type": "string"}, "default": ["h2", "http/1.1"]},
        "fingerprint": {"type": "string", "enum": ["chrome", "firefox", "safari", "ios", "edge", "random"], "default": "chrome"},
        "allow_insecure": {"type": "boolean", "default": false},
        "certificate_mode": {"type": "string", "enum": ["acme", "upload", "self_signed"], "default": "acme"}
      },
      "required": ["server_name"]
    }
  },
  "required": ["password"]
}'::jsonb, 'Trojan + gRPC + TLS')
ON CONFLICT (protocol_type, transport_type, security_type, schema_version) DO NOTHING;

-- P11: vmess + ws + tls (兼容旧客户端)
INSERT INTO protocol_registry (protocol_type, transport_type, security_type, schema_version, config_schema, description) VALUES
('vmess', 'ws', 'tls', 'v1', '{
  "type": "object",
  "properties": {
    "uuid": {"type": "string", "format": "uuid"},
    "alter_id": {"type": "integer", "default": 0},
    "ws_path": {"type": "string", "default": "/"},
    "ws_host": {"type": "string", "default": ""},
    "tls": {
      "type": "object",
      "properties": {
        "server_name": {"type": "string"},
        "alpn": {"type": "array", "items": {"type": "string"}, "default": ["h2", "http/1.1"]},
        "fingerprint": {"type": "string", "enum": ["chrome", "firefox", "safari", "ios", "edge", "random"], "default": "chrome"},
        "allow_insecure": {"type": "boolean", "default": false},
        "certificate_mode": {"type": "string", "enum": ["acme", "upload", "self_signed"], "default": "acme"}
      },
      "required": ["server_name"]
    }
  },
  "required": ["uuid"]
}'::jsonb, 'VMess + WebSocket + TLS，兼容旧版客户端')
ON CONFLICT (protocol_type, transport_type, security_type, schema_version) DO NOTHING;

-- 更新已有hysteria2 schema到v2（xray v26+原生入站，增加obfs选项和masquerade）
UPDATE protocol_registry SET config_schema = '{
  "type": "object",
  "properties": {
    "password": {"type": "string"},
    "up_mbps": {"type": "integer", "minimum": 1, "default": 1000},
    "down_mbps": {"type": "integer", "minimum": 1, "default": 1000},
    "obfs": {
      "type": "object",
      "properties": {
        "type": {"type": "string", "enum": ["", "salamander", "finalmask-header"], "default": ""},
        "password": {"type": "string", "default": ""}
      }
    },
    "masquerade": {
      "type": "object",
      "properties": {
        "type": {"type": "string", "enum": ["proxy", "file", "string"], "default": "proxy"},
        "proxy_url": {"type": "string", "default": "https://www.microsoft.com/"}
      }
    },
    "tls": {
      "type": "object",
      "properties": {
        "server_name": {"type": "string"},
        "alpn": {"type": "array", "items": {"type": "string"}, "default": ["h3"]},
        "fingerprint": {"type": "string", "enum": ["chrome", "firefox", "safari", "ios", "edge", "random"], "default": "chrome"},
        "allow_insecure": {"type": "boolean", "default": false},
        "certificate_mode": {"type": "string", "enum": ["acme", "upload", "self_signed"], "default": "acme"}
      },
      "required": ["server_name"]
    }
  },
  "required": ["password"]
}'::jsonb, schema_version = 'v2',
  description = 'Hysteria2 基于QUIC的高速协议，xray-core v26.3+原生支持入站，UDP高速推荐'
WHERE protocol_type = 'hysteria2' AND transport_type = 'udp' AND security_type = 'tls';

-- 更新已有tuic schema到v2
UPDATE protocol_registry SET config_schema = '{
  "type": "object",
  "properties": {
    "uuid": {"type": "string", "format": "uuid"},
    "password": {"type": "string"},
    "congestion_control": {"type": "string", "enum": ["bbr", "cubic", "new_reno", "brutal"], "default": "bbr"},
    "udp_relay_mode": {"type": "string", "enum": ["native", "quic"], "default": "native"},
    "auth_timeout": {"type": "string", "default": "3s"},
    "zero_rtt_handshake": {"type": "boolean", "default": true},
    "max_udp_relay_packet_size": {"type": "integer", "default": 1500},
    "alpn": {"type": "array", "items": {"type": "string"}, "default": ["h3"]},
    "tls": {
      "type": "object",
      "properties": {
        "server_name": {"type": "string"},
        "fingerprint": {"type": "string", "enum": ["chrome", "firefox", "safari", "ios", "edge", "random"], "default": "chrome"},
        "allow_insecure": {"type": "boolean", "default": false},
        "certificate_mode": {"type": "string", "enum": ["acme", "upload", "self_signed"], "default": "acme"}
      },
      "required": ["server_name"]
    }
  },
  "required": ["uuid", "password"]
}'::jsonb, schema_version = 'v2',
  description = 'TUICv5 基于QUIC的高性能协议，支持0-RTT握手和brutal拥塞控制'
WHERE protocol_type = 'tuic' AND transport_type = 'udp' AND security_type = 'tls';

-- P15: shadowsocks + udp + none
INSERT INTO protocol_registry (protocol_type, transport_type, security_type, schema_version, config_schema, description) VALUES
('shadowsocks', 'udp', 'none', 'v1', '{
  "type": "object",
  "properties": {
    "method": {"type": "string", "enum": ["2022-blake3-aes-128-gcm", "2022-blake3-aes-256-gcm", "2022-blake3-chacha20-poly1305"], "default": "2022-blake3-aes-256-gcm"},
    "password": {"type": "string"},
    "multiplex": {
      "type": "object",
      "properties": {
        "enabled": {"type": "boolean", "default": false},
        "protocol": {"type": "string", "enum": ["h2mux", "smux", "yamux"], "default": "smux"},
        "max_connections": {"type": "integer", "default": 4}
      }
    }
  },
  "required": ["method", "password"]
}'::jsonb, 'Shadowsocks 2022 UDP中继')
ON CONFLICT (protocol_type, transport_type, security_type, schema_version) DO NOTHING;

-- ============================================================
-- 协议预设种子数据
-- ============================================================
INSERT INTO protocol_presets (code, name, description, protocol_type, transport_type, security_type, recommended_port, icon, sort_order, is_recommended, default_config) VALUES
('xhttp-reality', '⭐ XHTTP + REALITY（主推）', '2026年抗封锁能力最强方案，基于HTTP/3+Finalmask伪装，推荐所有新节点使用', 'vless', 'xhttp', 'reality', 443, 'star', 1, true,
  '{"uuid":"<auto>","flow":"xtls-rprx-vision","xhttp_mode":"auto","reality":{"dest":"www.microsoft.com:443","server_names":["www.microsoft.com"],"fingerprint":"chrome","short_ids":["<random>"]},"xhttp_path":"/"}'::jsonb),
('tcp-reality', 'REALITY (TCP直连)', '经典REALITY方案，TCP直连无需域名，性能好', 'vless', 'tcp', 'reality', 443, 'zap', 2, false,
  '{"uuid":"<auto>","flow":"xtls-rprx-vision","reality":{"dest":"www.microsoft.com:443","server_names":["www.microsoft.com"],"fingerprint":"chrome","short_ids":["<random>"]}}'::jsonb),
('ws-tls-cdn', 'CDN中转 (WS+TLS)', 'WebSocket+TLS方案，支持Cloudflare/CDN中转，适合被封区域', 'vless', 'ws', 'tls', 443, 'globe', 3, false,
  '{"uuid":"<auto>","ws_path":"/<random>","ws_early_data":2048,"tls":{"server_name":"<your-domain>","alpn":["h2","http/1.1"],"fingerprint":"chrome","certificate_mode":"acme"}}'::jsonb),
('grpc-tls', 'gRPC + TLS', 'gRPC多路复用，移动端表现优秀', 'vless', 'grpc', 'tls', 443, 'zap', 4, false,
  '{"uuid":"<auto>","grpc_service_name":"<random>","grpc_multi_mode":true,"tls":{"server_name":"<your-domain>","alpn":["h2","http/1.1"],"fingerprint":"chrome","certificate_mode":"acme"}}'::jsonb),
('hysteria2-udp', '🚀 Hysteria2 (UDP高速)', 'QUIC协议，晚高峰UDP表现优秀，速度快', 'hysteria2', 'udp', 'tls', 443, 'rocket', 5, true,
  '{"password":"<random>","up_mbps":1000,"down_mbps":1000,"obfs":{"type":""},"masquerade":{"type":"proxy","proxy_url":"https://www.microsoft.com/"},"tls":{"server_name":"<your-domain>","alpn":["h3"],"certificate_mode":"acme"}}'::jsonb),
('tuic-quic', 'TUICv5 (QUIC)', 'QUIC协议，支持0-RTT握手，低延迟', 'tuic', 'udp', 'tls', 443, 'zap', 6, false,
  '{"uuid":"<auto>","password":"<random>","congestion_control":"bbr","zero_rtt_handshake":true,"alpn":["h3"],"tls":{"server_name":"<your-domain>","certificate_mode":"acme"}}'::jsonb),
('trojan-tcp', 'Trojan (TCP+TLS)', 'Trojan经典协议，广泛兼容各种客户端', 'trojan', 'tcp', 'tls', 443, 'shield', 7, false,
  '{"password":"<auto-uuid>","tls":{"server_name":"<your-domain>","alpn":["h2","http/1.1"],"fingerprint":"chrome","certificate_mode":"acme"}}'::jsonb),
('trojan-ws-cdn', 'Trojan + WS (CDN)', 'Trojan+WebSocket+TLS，CDN中转方案', 'trojan', 'ws', 'tls', 443, 'globe', 8, false,
  '{"password":"<auto-uuid>","ws_path":"/<random>","ws_early_data":2048,"tls":{"server_name":"<your-domain>","alpn":["h2","http/1.1"],"fingerprint":"chrome","certificate_mode":"acme"}}'::jsonb),
('ss-2022', 'Shadowsocks 2022', 'SS 2022-blake3协议，简单高效', 'shadowsocks', 'tcp', 'none', 0, 'server', 9, false,
  '{"method":"2022-blake3-aes-256-gcm","password":"<random-b64>"}'::jsonb),
('vmess-ws-cdn', 'VMess + WS (兼容)', 'VMess协议，兼容旧版客户端', 'vmess', 'ws', 'tls', 443, 'globe', 10, false,
  '{"uuid":"<auto>","alter_id":0,"ws_path":"/<random>","tls":{"server_name":"<your-domain>","alpn":["h2","http/1.1"],"fingerprint":"chrome","certificate_mode":"acme"}}'::jsonb);

-- ============================================================
-- 补充xray和sing-box的inbound模板（覆盖新增协议）
-- ============================================================

-- xray vless-xhttp-reality inbound (P03 ⭐主推)
INSERT INTO config_templates (code, name, runtime_type, template_type, content, variables_schema, is_default) VALUES
('xray-vless-xhttp-reality-inbound', 'Xray VLESS XHTTP Reality Inbound', 'xray', 'inbound',
'{
  "tag": "{{.tag}}",
  "listen": "{{.listen}}",
  "port": {{.port}},
  "protocol": "vless",
  "settings": {
    "clients": [
      {
        "id": "{{.uuid}}",
        "flow": "{{.flow}}"
      }
    ],
    "decryption": "none"
  },
  "streamSettings": {
    "network": "xhttp",
    "security": "reality",
    "realitySettings": {
      "show": false,
      "dest": "{{.reality_dest}}",
      "xver": 0,
      "serverNames": [{{range $i, $n := .reality_server_names}}{{if $i}},{{end}}"{{$n}}"{{end}}],
      "privateKey": "{{.reality_private_key}}",
      "shortIds": [{{range $i, $s := .reality_short_ids}}{{if $i}},{{end}}"{{$s}}"{{end}}],
      "fingerprint": "{{.fingerprint}}",
      "spiderX": "{{.spider_x}}"
    },
    "xhttpSettings": {
      "host": [{{range $i, $h := .xhttp_host}}{{if $i}},{{end}}"{{$h}}"{{end}}],
      "path": "{{.xhttp_path}}",
      "mode": "{{.xhttp_mode}}",
      "extra": {{.xhttp_extra_json}}
    }
  },
  "sniffing": {
    "enabled": true,
    "destOverride": ["http","tls","quic","fakedns"]
  }
}',
'{"tag":{"type":"string"},"listen":{"type":"string"},"port":{"type":"integer"},"uuid":{"type":"string"},"flow":{"type":"string"},"reality_dest":{"type":"string"},"reality_server_names":{"type":"array"},"reality_private_key":{"type":"string"},"reality_short_ids":{"type":"array"},"fingerprint":{"type":"string"},"spider_x":{"type":"string"},"xhttp_host":{"type":"array"},"xhttp_path":{"type":"string"},"xhttp_mode":{"type":"string"},"xhttp_extra_json":{"type":"string"}}'::jsonb, false)
ON CONFLICT (code) DO NOTHING;

-- xray vless-xhttp-tls inbound (P02)
INSERT INTO config_templates (code, name, runtime_type, template_type, content, variables_schema, is_default) VALUES
('xray-vless-xhttp-tls-inbound', 'Xray VLESS XHTTP TLS Inbound', 'xray', 'inbound',
'{
  "tag": "{{.tag}}",
  "listen": "{{.listen}}",
  "port": {{.port}},
  "protocol": "vless",
  "settings": {
    "clients": [
      {
        "id": "{{.uuid}}",
        "flow": ""
      }
    ],
    "decryption": "none"
  },
  "streamSettings": {
    "network": "xhttp",
    "security": "tls",
    "tlsSettings": {
      "serverName": "{{.server_name}}",
      "alpn": [{{range $i, $a := .alpn}}{{if $i}},{{end}}"{{$a}}"{{end}}],
      "certificates": [
        {
          "certificateFile": "{{.cert_file}}",
          "keyFile": "{{.key_file}}"
        }
      ],
      "fingerprint": "{{.fingerprint}}",
      "minVersion": "{{.min_version}}",
      "allowInsecure": {{.allow_insecure}}
    },
    "xhttpSettings": {
      "host": [{{range $i, $h := .xhttp_host}}{{if $i}},{{end}}"{{$h}}"{{end}}],
      "path": "{{.xhttp_path}}",
      "mode": "{{.xhttp_mode}}"
    }
  },
  "sniffing": {
    "enabled": true,
    "destOverride": ["http","tls","quic","fakedns"]
  }
}',
'{"tag":{"type":"string"},"listen":{"type":"string"},"port":{"type":"integer"},"uuid":{"type":"string"},"server_name":{"type":"string"},"alpn":{"type":"array"},"cert_file":{"type":"string"},"key_file":{"type":"string"},"fingerprint":{"type":"string"},"min_version":{"type":"string"},"allow_insecure":{"type":"boolean"},"xhttp_host":{"type":"array"},"xhttp_path":{"type":"string"},"xhttp_mode":{"type":"string"}}'::jsonb, false)
ON CONFLICT (code) DO NOTHING;

-- xray vless-ws-tls inbound (P05)
INSERT INTO config_templates (code, name, runtime_type, template_type, content, variables_schema, is_default) VALUES
('xray-vless-ws-tls-inbound', 'Xray VLESS WebSocket TLS Inbound', 'xray', 'inbound',
'{
  "tag": "{{.tag}}",
  "listen": "{{.listen}}",
  "port": {{.port}},
  "protocol": "vless",
  "settings": {
    "clients": [
      {
        "id": "{{.uuid}}",
        "flow": ""
      }
    ],
    "decryption": "none"
  },
  "streamSettings": {
    "network": "ws",
    "security": "tls",
    "tlsSettings": {
      "serverName": "{{.server_name}}",
      "alpn": [{{range $i, $a := .alpn}}{{if $i}},{{end}}"{{$a}}"{{end}}],
      "certificates": [
        {
          "certificateFile": "{{.cert_file}}",
          "keyFile": "{{.key_file}}"
        }
      ],
      "fingerprint": "{{.fingerprint}}",
      "allowInsecure": {{.allow_insecure}}
    },
    "wsSettings": {
      "path": "{{.ws_path}}",
      "headers": {
        "Host": "{{.ws_host}}"
      }
    }
  },
  "sniffing": {
    "enabled": true,
    "destOverride": ["http","tls","quic","fakedns"]
  }
}',
'{"tag":{"type":"string"},"listen":{"type":"string"},"port":{"type":"integer"},"uuid":{"type":"string"},"server_name":{"type":"string"},"alpn":{"type":"array"},"cert_file":{"type":"string"},"key_file":{"type":"string"},"fingerprint":{"type":"string"},"allow_insecure":{"type":"boolean"},"ws_path":{"type":"string"},"ws_host":{"type":"string"}}'::jsonb, false)
ON CONFLICT (code) DO NOTHING;

-- xray vless-grpc-tls inbound (P06)
INSERT INTO config_templates (code, name, runtime_type, template_type, content, variables_schema, is_default) VALUES
('xray-vless-grpc-tls-inbound', 'Xray VLESS gRPC TLS Inbound', 'xray', 'inbound',
'{
  "tag": "{{.tag}}",
  "listen": "{{.listen}}",
  "port": {{.port}},
  "protocol": "vless",
  "settings": {
    "clients": [
      {
        "id": "{{.uuid}}",
        "flow": ""
      }
    ],
    "decryption": "none"
  },
  "streamSettings": {
    "network": "grpc",
    "security": "tls",
    "tlsSettings": {
      "serverName": "{{.server_name}}",
      "alpn": [{{range $i, $a := .alpn}}{{if $i}},{{end}}"{{$a}}"{{end}}],
      "certificates": [
        {
          "certificateFile": "{{.cert_file}}",
          "keyFile": "{{.key_file}}"
        }
      ],
      "fingerprint": "{{.fingerprint}}",
      "allowInsecure": {{.allow_insecure}}
    },
    "grpcSettings": {
      "serviceName": "{{.grpc_service_name}}",
      "multiMode": {{.grpc_multi_mode}},
      "idle_timeout": {{.grpc_idle_timeout}},
      "health_check": {{.grpc_health_check}}
    }
  },
  "sniffing": {
    "enabled": true,
    "destOverride": ["http","tls","quic","fakedns"]
  }
}',
'{"tag":{"type":"string"},"listen":{"type":"string"},"port":{"type":"integer"},"uuid":{"type":"string"},"server_name":{"type":"string"},"alpn":{"type":"array"},"cert_file":{"type":"string"},"key_file":{"type":"string"},"fingerprint":{"type":"string"},"allow_insecure":{"type":"boolean"},"grpc_service_name":{"type":"string"},"grpc_multi_mode":{"type":"boolean"},"grpc_idle_timeout":{"type":"integer"},"grpc_health_check":{"type":"boolean"}}'::jsonb, false)
ON CONFLICT (code) DO NOTHING;

-- xray trojan-tcp-tls inbound (P08)
INSERT INTO config_templates (code, name, runtime_type, template_type, content, variables_schema, is_default) VALUES
('xray-trojan-tcp-tls-inbound', 'Xray Trojan TCP TLS Inbound', 'xray', 'inbound',
'{
  "tag": "{{.tag}}",
  "listen": "{{.listen}}",
  "port": {{.port}},
  "protocol": "trojan",
  "settings": {
    "clients": [
      {
        "password": "{{.password}}"
      }
    ],
    "fallbacks": [
      {
        "dest": 80
      }
    ]
  },
  "streamSettings": {
    "network": "tcp",
    "security": "tls",
    "tlsSettings": {
      "serverName": "{{.server_name}}",
      "alpn": [{{range $i, $a := .alpn}}{{if $i}},{{end}}"{{$a}}"{{end}}],
      "certificates": [
        {
          "certificateFile": "{{.cert_file}}",
          "keyFile": "{{.key_file}}"
        }
      ],
      "fingerprint": "{{.fingerprint}}",
      "allowInsecure": {{.allow_insecure}}
    },
    "tcpSettings": {
      "header": {
        "type": "none"
      }
    }
  },
  "sniffing": {
    "enabled": true,
    "destOverride": ["http","tls","quic","fakedns"]
  }
}',
'{"tag":{"type":"string"},"listen":{"type":"string"},"port":{"type":"integer"},"password":{"type":"string"},"server_name":{"type":"string"},"alpn":{"type":"array"},"cert_file":{"type":"string"},"key_file":{"type":"string"},"fingerprint":{"type":"string"},"allow_insecure":{"type":"boolean"}}'::jsonb, false)
ON CONFLICT (code) DO NOTHING;

-- xray trojan-ws-tls inbound (P09)
INSERT INTO config_templates (code, name, runtime_type, template_type, content, variables_schema, is_default) VALUES
('xray-trojan-ws-tls-inbound', 'Xray Trojan WebSocket TLS Inbound', 'xray', 'inbound',
'{
  "tag": "{{.tag}}",
  "listen": "{{.listen}}",
  "port": {{.port}},
  "protocol": "trojan",
  "settings": {
    "clients": [
      {
        "password": "{{.password}}"
      }
    ]
  },
  "streamSettings": {
    "network": "ws",
    "security": "tls",
    "tlsSettings": {
      "serverName": "{{.server_name}}",
      "alpn": [{{range $i, $a := .alpn}}{{if $i}},{{end}}"{{$a}}"{{end}}],
      "certificates": [
        {
          "certificateFile": "{{.cert_file}}",
          "keyFile": "{{.key_file}}"
        }
      ],
      "fingerprint": "{{.fingerprint}}",
      "allowInsecure": {{.allow_insecure}}
    },
    "wsSettings": {
      "path": "{{.ws_path}}",
      "headers": {
        "Host": "{{.ws_host}}"
      }
    }
  },
  "sniffing": {
    "enabled": true,
    "destOverride": ["http","tls","quic","fakedns"]
  }
}',
'{"tag":{"type":"string"},"listen":{"type":"string"},"port":{"type":"integer"},"password":{"type":"string"},"server_name":{"type":"string"},"alpn":{"type":"array"},"cert_file":{"type":"string"},"key_file":{"type":"string"},"fingerprint":{"type":"string"},"allow_insecure":{"type":"boolean"},"ws_path":{"type":"string"},"ws_host":{"type":"string"}}'::jsonb, false)
ON CONFLICT (code) DO NOTHING;

-- xray vmess-ws-tls inbound (P11)
INSERT INTO config_templates (code, name, runtime_type, template_type, content, variables_schema, is_default) VALUES
('xray-vmess-ws-tls-inbound', 'Xray VMess WebSocket TLS Inbound', 'xray', 'inbound',
'{
  "tag": "{{.tag}}",
  "listen": "{{.listen}}",
  "port": {{.port}},
  "protocol": "vmess",
  "settings": {
    "clients": [
      {
        "id": "{{.uuid}}",
        "alterId": {{.alter_id}}
      }
    ]
  },
  "streamSettings": {
    "network": "ws",
    "security": "tls",
    "tlsSettings": {
      "serverName": "{{.server_name}}",
      "alpn": [{{range $i, $a := .alpn}}{{if $i}},{{end}}"{{$a}}"{{end}}],
      "certificates": [
        {
          "certificateFile": "{{.cert_file}}",
          "keyFile": "{{.key_file}}"
        }
      ],
      "fingerprint": "{{.fingerprint}}",
      "allowInsecure": {{.allow_insecure}}
    },
    "wsSettings": {
      "path": "{{.ws_path}}",
      "headers": {
        "Host": "{{.ws_host}}"
      }
    }
  },
  "sniffing": {
    "enabled": true,
    "destOverride": ["http","tls","quic","fakedns"]
  }
}',
'{"tag":{"type":"string"},"listen":{"type":"string"},"port":{"type":"integer"},"uuid":{"type":"string"},"alter_id":{"type":"integer"},"server_name":{"type":"string"},"alpn":{"type":"array"},"cert_file":{"type":"string"},"key_file":{"type":"string"},"fingerprint":{"type":"string"},"allow_insecure":{"type":"boolean"},"ws_path":{"type":"string"},"ws_host":{"type":"string"}}'::jsonb, false)
ON CONFLICT (code) DO NOTHING;

-- xray hysteria2 inbound (xray v26.3+ 原生支持)
INSERT INTO config_templates (code, name, runtime_type, template_type, content, variables_schema, is_default) VALUES
('xray-hysteria2-inbound', 'Xray Hysteria2 Inbound', 'xray', 'inbound',
'{
  "tag": "{{.tag}}",
  "listen": "{{.listen}}",
  "port": {{.port}},
  "protocol": "hysteria2",
  "settings": {
    "password": "{{.password}}",
    "up_mbps": {{.up_mbps}},
    "down_mbps": {{.down_mbps}}{{if .obfs_type}},
    "obfs": "{{.obfs_type}}",
    "obfs_password": "{{.obfs_password}}"{{end}}
  },
  "streamSettings": {
    "network": "udp",
    "security": "tls",
    "tlsSettings": {
      "serverName": "{{.server_name}}",
      "alpn": [{{range $i, $a := .alpn}}{{if $i}},{{end}}"{{$a}}"{{end}}],
      "certificates": [
        {
          "certificateFile": "{{.cert_file}}",
          "keyFile": "{{.key_file}}"
        }
      ],
      "fingerprint": "{{.fingerprint}}"
    }
  },
  "sniffing": {
    "enabled": true,
    "destOverride": ["http","tls","quic","fakedns"]
  }
}',
'{"tag":{"type":"string"},"listen":{"type":"string"},"port":{"type":"integer"},"password":{"type":"string"},"up_mbps":{"type":"integer"},"down_mbps":{"type":"integer"},"obfs_type":{"type":"string"},"obfs_password":{"type":"string"},"server_name":{"type":"string"},"alpn":{"type":"array"},"cert_file":{"type":"string"},"key_file":{"type":"string"},"fingerprint":{"type":"string"}}'::jsonb, false)
ON CONFLICT (code) DO NOTHING;

-- sing-box vless-reality inbound
INSERT INTO config_templates (code, name, runtime_type, template_type, content, variables_schema, is_default) VALUES
('sing-box-vless-reality-inbound', 'sing-box VLESS Reality Inbound', 'sing-box', 'inbound',
'{
  "type": "vless",
  "tag": "{{.tag}}",
  "listen": "::",
  "listen_port": {{.port}},
  "users": [
    {
      "uuid": "{{.uuid}}",
      "flow": "{{.flow}}"
    }
  ],
  "tls": {
    "enabled": true,
    "server_name": "{{.server_name}}",
    "reality": {
      "enabled": true,
      "handshake": {
        "server": "{{.reality_dest}}",
        "server_port": 443
      },
      "short_id": "{{.short_id}}",
      "private_key": "{{.reality_private_key}}"
    }
  },
  "transport": {
    "type": "tcp"
  }
}',
'{"tag":{"type":"string"},"port":{"type":"integer"},"uuid":{"type":"string"},"flow":{"type":"string"},"server_name":{"type":"string"},"reality_dest":{"type":"string"},"short_id":{"type":"string"},"reality_private_key":{"type":"string"}}'::jsonb, false)
ON CONFLICT (code) DO NOTHING;

-- sing-box vless-ws-tls inbound
INSERT INTO config_templates (code, name, runtime_type, template_type, content, variables_schema, is_default) VALUES
('sing-box-vless-ws-tls-inbound', 'sing-box VLESS WebSocket TLS Inbound', 'sing-box', 'inbound',
'{
  "type": "vless",
  "tag": "{{.tag}}",
  "listen": "::",
  "listen_port": {{.port}},
  "users": [
    {
      "uuid": "{{.uuid}}"
    }
  ],
  "tls": {
    "enabled": true,
    "server_name": "{{.server_name}}",
    "certificate_path": "{{.cert_path}}",
    "key_path": "{{.key_path}}",
    "alpn": [{{range $i, $a := .alpn}}{{if $i}},{{end}}"{{$a}}"{{end}}]
  },
  "transport": {
    "type": "ws",
    "path": "{{.ws_path}}",
    "headers": {
      "Host": "{{.ws_host}}"
    },
    "max_early_data": {{.ws_early_data}},
    "early_data_header_name": "Sec-WebSocket-Protocol"
  }
}',
'{"tag":{"type":"string"},"port":{"type":"integer"},"uuid":{"type":"string"},"server_name":{"type":"string"},"cert_path":{"type":"string"},"key_path":{"type":"string"},"alpn":{"type":"array"},"ws_path":{"type":"string"},"ws_host":{"type":"string"},"ws_early_data":{"type":"integer"}}'::jsonb, false)
ON CONFLICT (code) DO NOTHING;

-- sing-box vless-grpc-tls inbound
INSERT INTO config_templates (code, name, runtime_type, template_type, content, variables_schema, is_default) VALUES
('sing-box-vless-grpc-tls-inbound', 'sing-box VLESS gRPC TLS Inbound', 'sing-box', 'inbound',
'{
  "type": "vless",
  "tag": "{{.tag}}",
  "listen": "::",
  "listen_port": {{.port}},
  "users": [
    {
      "uuid": "{{.uuid}}"
    }
  ],
  "tls": {
    "enabled": true,
    "server_name": "{{.server_name}}",
    "certificate_path": "{{.cert_path}}",
    "key_path": "{{.key_path}}",
    "alpn": [{{range $i, $a := .alpn}}{{if $i}},{{end}}"{{$a}}"{{end}}]
  },
  "transport": {
    "type": "grpc",
    "service_name": "{{.grpc_service_name}}",
    "idle_timeout": "{{.grpc_idle_timeout}}s"
  }
}',
'{"tag":{"type":"string"},"port":{"type":"integer"},"uuid":{"type":"string"},"server_name":{"type":"string"},"cert_path":{"type":"string"},"key_path":{"type":"string"},"alpn":{"type":"array"},"grpc_service_name":{"type":"string"},"grpc_idle_timeout":{"type":"integer"}}'::jsonb, false)
ON CONFLICT (code) DO NOTHING;

-- sing-box trojan-tcp-tls inbound
INSERT INTO config_templates (code, name, runtime_type, template_type, content, variables_schema, is_default) VALUES
('sing-box-trojan-tcp-tls-inbound', 'sing-box Trojan TCP TLS Inbound', 'sing-box', 'inbound',
'{
  "type": "trojan",
  "tag": "{{.tag}}",
  "listen": "::",
  "listen_port": {{.port}},
  "users": [
    {
      "password": "{{.password}}"
    }
  ],
  "tls": {
    "enabled": true,
    "server_name": "{{.server_name}}",
    "certificate_path": "{{.cert_path}}",
    "key_path": "{{.key_path}}",
    "alpn": [{{range $i, $a := .alpn}}{{if $i}},{{end}}"{{$a}}"{{end}}]
  },
  "transport": {
    "type": "tcp"
  }
}',
'{"tag":{"type":"string"},"port":{"type":"integer"},"password":{"type":"string"},"server_name":{"type":"string"},"cert_path":{"type":"string"},"key_path":{"type":"string"},"alpn":{"type":"array"}}'::jsonb, false)
ON CONFLICT (code) DO NOTHING;

-- 更新已有vless-reality模板为新版（修正serverNames/shortIds为数组）
UPDATE config_templates SET content = '{
  "tag": "{{.tag}}",
  "listen": "{{.listen}}",
  "port": {{.port}},
  "protocol": "vless",
  "settings": {
    "clients": [
      {
        "id": "{{.uuid}}",
        "flow": "{{.flow}}"
      }
    ],
    "decryption": "none"
  },
  "streamSettings": {
    "network": "tcp",
    "security": "reality",
    "realitySettings": {
      "show": false,
      "dest": "{{.reality_dest}}",
      "xver": 0,
      "serverNames": [{{range $i, $n := .reality_server_names}}{{if $i}},{{end}}"{{$n}}"{{end}}],
      "privateKey": "{{.reality_private_key}}",
      "shortIds": [{{range $i, $s := .reality_short_ids}}{{if $i}},{{end}}"{{$s}}"{{end}}],
      "fingerprint": "{{.fingerprint}}",
      "spiderX": "{{.spider_x}}"
    }
  },
  "sniffing": {
    "enabled": true,
    "destOverride": ["http","tls","quic","fakedns"]
  }
}',
  variables_schema = '{"tag":{"type":"string"},"listen":{"type":"string"},"port":{"type":"integer"},"uuid":{"type":"string"},"flow":{"type":"string"},"reality_dest":{"type":"string"},"reality_server_names":{"type":"array"},"reality_private_key":{"type":"string"},"reality_short_ids":{"type":"array"},"fingerprint":{"type":"string"},"spider_x":{"type":"string"}}'::jsonb
WHERE code = 'xray-vless-reality-inbound';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DELETE FROM config_templates WHERE code IN (
  'xray-vless-xhttp-reality-inbound',
  'xray-vless-xhttp-tls-inbound',
  'xray-vless-ws-tls-inbound',
  'xray-vless-grpc-tls-inbound',
  'xray-trojan-tcp-tls-inbound',
  'xray-trojan-ws-tls-inbound',
  'xray-vmess-ws-tls-inbound',
  'xray-hysteria2-inbound',
  'sing-box-vless-reality-inbound',
  'sing-box-vless-ws-tls-inbound',
  'sing-box-vless-grpc-tls-inbound',
  'sing-box-trojan-tcp-tls-inbound'
);

DELETE FROM protocol_presets;
DROP TABLE IF EXISTS protocol_presets;

DELETE FROM protocol_registry WHERE (protocol_type, transport_type, security_type, schema_version) IN (
  ('vless','xhttp','tls','v1'),
  ('vless','xhttp','reality','v1'),
  ('vless','ws','tls','v1'),
  ('vless','grpc','tls','v1'),
  ('vless','h2','tls','v1'),
  ('trojan','tcp','tls','v1'),
  ('trojan','ws','tls','v1'),
  ('trojan','grpc','tls','v1'),
  ('vmess','ws','tls','v1'),
  ('shadowsocks','udp','none','v1')
);

-- 恢复hysteria2和tuic到v1 schema（简化版）
UPDATE protocol_registry SET config_schema = '{
  "type": "object",
  "properties": {
    "password": {"type": "string"},
    "up_mbps": {"type": "integer", "minimum": 1, "default": 100},
    "down_mbps": {"type": "integer", "minimum": 1, "default": 100},
    "obfs": {"type": "object", "properties": {"type": {"type": "string", "enum": ["salamander"]}, "password": {"type": "string"}}},
    "tls": {"type": "object", "properties": {"server_name": {"type": "string"}, "insecure": {"type": "boolean", "default": false}}}
  },
  "required": ["password"]
}'::jsonb, schema_version = 'v1', description = 'Hysteria2 基于 QUIC 的高速协议，支持带宽控制和混淆'
WHERE protocol_type = 'hysteria2' AND transport_type = 'udp' AND security_type = 'tls';

UPDATE protocol_registry SET config_schema = '{
  "type": "object",
  "properties": {
    "uuid": {"type": "string", "format": "uuid"},
    "password": {"type": "string"},
    "congestion_control": {"type": "string", "enum": ["bbr", "cubic", "new_reno"], "default": "bbr"},
    "udp_relay_mode": {"type": "string", "enum": ["native", "quic"], "default": "native"},
    "alpn": {"type": "array", "items": {"type": "string"}},
    "tls": {"type": "object", "properties": {"server_name": {"type": "string"}, "insecure": {"type": "boolean", "default": false}}}
  },
  "required": ["uuid", "password"]
}'::jsonb, schema_version = 'v1', description = 'TUIC 基于 QUIC 的高性能协议，支持多种拥塞控制'
WHERE protocol_type = 'tuic' AND transport_type = 'udp' AND security_type = 'tls';

-- 恢复原始vless-reality模板
UPDATE config_templates SET content = '{
  "listen": "0.0.0.0",
  "port": {{.port}},
  "protocol": "vless",
  "settings": {
    "clients": [
      {
        "id": "{{.uuid}}",
        "flow": "{{.flow}}"
      }
    ],
    "decryption": "none"
  },
  "streamSettings": {
    "network": "tcp",
    "security": "reality",
    "realitySettings": {
      "show": false,
      "dest": "{{.reality_dest}}",
      "xver": 0,
      "serverNames": ["{{.server_name}}"],
      "privateKey": "{{.reality_private_key}}",
      "shortIds": ["{{.short_id}}"]
    }
  }
}',
  variables_schema = '{"port": {"type": "integer"}, "uuid": {"type": "string"}, "flow": {"type": "string"}, "reality_dest": {"type": "string"}, "server_name": {"type": "string"}, "reality_private_key": {"type": "string"}, "short_id": {"type": "string"}}'::jsonb
WHERE code = 'xray-vless-reality-inbound';

-- +goose StatementEnd
