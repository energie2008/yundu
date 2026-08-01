-- +goose Up
-- +goose StatementBegin

-- 1. protocol_registry 增加 status 列（active/draft/deprecated）
-- 修复前端"协议都是草稿"：此前表无 status 列，前端 fallback 为 draft。
ALTER TABLE protocol_registry ADD COLUMN IF NOT EXISTS status VARCHAR(16) NOT NULL DEFAULT 'active';
UPDATE protocol_registry SET status = 'active' WHERE is_enabled = true;
UPDATE protocol_registry SET status = 'draft'  WHERE is_enabled = false;

-- 2. 完善/补齐协议 schema（字段对齐 Xray 官方配置与仓库 config_templates/presets 真实字段）

-- VLESS + TCP + REALITY
INSERT INTO protocol_registry (protocol_type, transport_type, security_type, schema_version, config_schema, description, status) VALUES
('vless', 'tcp', 'reality', 'v1', '{
  "type": "object",
  "properties": {
    "uuid": {"type": "string", "format": "uuid", "description": "VLESS UUID"},
    "flow": {"type": "string", "enum": ["", "xtls-rprx-vision", "xtls-rprx-vision-udp443"], "default": "xtls-rprx-vision"},
    "reality": {
      "type": "object",
      "properties": {
        "dest": {"type": "string", "description": "回落目标，如 www.microsoft.com:443"},
        "server_names": {"type": "array", "items": {"type": "string"}},
        "private_key": {"type": "string", "description": "xray x25519 生成的私钥"},
        "short_ids": {"type": "array", "items": {"type": "string"}},
        "fingerprint": {"type": "string", "enum": ["chrome", "firefox", "safari", "random"], "default": "chrome"},
        "spider_x": {"type": "string", "default": "/"}
      },
      "required": ["dest", "server_names", "private_key", "short_ids"]
    }
  },
  "required": ["uuid", "reality"]
}'::jsonb, 'VLESS + REALITY（TCP 直连），抗主动探测，推荐生产环境', 'active')
ON CONFLICT (protocol_type, transport_type, security_type, schema_version) DO UPDATE SET config_schema = EXCLUDED.config_schema, description = EXCLUDED.description, status = 'active';

-- VLESS + TCP + TLS
INSERT INTO protocol_registry (protocol_type, transport_type, security_type, schema_version, config_schema, description, status) VALUES
('vless', 'tcp', 'tls', 'v1', '{
  "type": "object",
  "properties": {
    "uuid": {"type": "string", "format": "uuid"},
    "flow": {"type": "string", "enum": ["", "xtls-rprx-vision"], "default": "xtls-rprx-vision"},
    "tls": {
      "type": "object",
      "properties": {
        "server_name": {"type": "string"},
        "alpn": {"type": "array", "items": {"type": "string"}, "default": ["h2", "http/1.1"]},
        "fingerprint": {"type": "string", "default": "chrome"},
        "certificate_mode": {"type": "string", "enum": ["acme", "file", "content", "self"], "default": "acme"}
      },
      "required": ["server_name"]
    }
  },
  "required": ["uuid", "tls"]
}'::jsonb, 'VLESS + TCP + TLS 直连', 'active')
ON CONFLICT (protocol_type, transport_type, security_type, schema_version) DO UPDATE SET config_schema = EXCLUDED.config_schema, description = EXCLUDED.description, status = 'active';

-- VLESS + WS + TLS（CDN）
INSERT INTO protocol_registry (protocol_type, transport_type, security_type, schema_version, config_schema, description, status) VALUES
('vless', 'ws', 'tls', 'v1', '{
  "type": "object",
  "properties": {
    "uuid": {"type": "string", "format": "uuid"},
    "ws_path": {"type": "string", "default": "/"},
    "ws_host": {"type": "string", "description": "HTTP Host 头，CDN 场景填域名"},
    "ws_early_data": {"type": "integer", "default": 2048},
    "tls": {
      "type": "object",
      "properties": {
        "server_name": {"type": "string"},
        "alpn": {"type": "array", "items": {"type": "string"}, "default": ["h2", "http/1.1"]},
        "fingerprint": {"type": "string", "default": "chrome"},
        "certificate_mode": {"type": "string", "enum": ["acme", "file", "content", "self"], "default": "acme"}
      },
      "required": ["server_name"]
    }
  },
  "required": ["uuid", "ws_path", "tls"]
}'::jsonb, 'VLESS + WebSocket + TLS，支持 Cloudflare/CDN 中转', 'active')
ON CONFLICT (protocol_type, transport_type, security_type, schema_version) DO UPDATE SET config_schema = EXCLUDED.config_schema, description = EXCLUDED.description, status = 'active';

-- VLESS + gRPC + TLS
INSERT INTO protocol_registry (protocol_type, transport_type, security_type, schema_version, config_schema, description, status) VALUES
('vless', 'grpc', 'tls', 'v1', '{
  "type": "object",
  "properties": {
    "uuid": {"type": "string", "format": "uuid"},
    "grpc_service_name": {"type": "string"},
    "grpc_multi_mode": {"type": "boolean", "default": true},
    "tls": {
      "type": "object",
      "properties": {
        "server_name": {"type": "string"},
        "alpn": {"type": "array", "items": {"type": "string"}, "default": ["h2", "http/1.1"]},
        "fingerprint": {"type": "string", "default": "chrome"},
        "certificate_mode": {"type": "string", "enum": ["acme", "file", "content", "self"], "default": "acme"}
      },
      "required": ["server_name"]
    }
  },
  "required": ["uuid", "grpc_service_name", "tls"]
}'::jsonb, 'VLESS + gRPC + TLS，多路复用表现优秀', 'active')
ON CONFLICT (protocol_type, transport_type, security_type, schema_version) DO UPDATE SET config_schema = EXCLUDED.config_schema, description = EXCLUDED.description, status = 'active';

-- VLESS + XHTTP + REALITY（主推）
INSERT INTO protocol_registry (protocol_type, transport_type, security_type, schema_version, config_schema, description, status) VALUES
('vless', 'xhttp', 'reality', 'v1', '{
  "type": "object",
  "properties": {
    "uuid": {"type": "string", "format": "uuid"},
    "flow": {"type": "string", "enum": ["", "xtls-rprx-vision"], "default": "xtls-rprx-vision"},
    "xhttp_mode": {"type": "string", "enum": ["auto", "packet-up", "stream-up", "stream-down"], "default": "auto"},
    "xhttp_path": {"type": "string", "default": "/"},
    "xhttp_host": {"type": "array", "items": {"type": "string"}},
    "reality": {
      "type": "object",
      "properties": {
        "dest": {"type": "string"},
        "server_names": {"type": "array", "items": {"type": "string"}},
        "private_key": {"type": "string"},
        "short_ids": {"type": "array", "items": {"type": "string"}},
        "fingerprint": {"type": "string", "default": "chrome"},
        "spider_x": {"type": "string", "default": "/"}
      },
      "required": ["dest", "server_names", "private_key", "short_ids"]
    }
  },
  "required": ["uuid", "xhttp_path", "reality"]
}'::jsonb, 'VLESS + XHTTP + REALITY，2026 抗封锁能力最强方案（主推）', 'active')
ON CONFLICT (protocol_type, transport_type, security_type, schema_version) DO UPDATE SET config_schema = EXCLUDED.config_schema, description = EXCLUDED.description, status = 'active';

-- VLESS + XHTTP + TLS（CDN）
INSERT INTO protocol_registry (protocol_type, transport_type, security_type, schema_version, config_schema, description, status) VALUES
('vless', 'xhttp', 'tls', 'v1', '{
  "type": "object",
  "properties": {
    "uuid": {"type": "string", "format": "uuid"},
    "xhttp_mode": {"type": "string", "enum": ["auto", "packet-up", "stream-up", "stream-down"], "default": "auto"},
    "xhttp_path": {"type": "string", "default": "/"},
    "xhttp_host": {"type": "array", "items": {"type": "string"}},
    "tls": {
      "type": "object",
      "properties": {
        "server_name": {"type": "string"},
        "alpn": {"type": "array", "items": {"type": "string"}, "default": ["h2", "http/1.1"]},
        "fingerprint": {"type": "string", "default": "chrome"},
        "certificate_mode": {"type": "string", "enum": ["acme", "file", "content", "self"], "default": "acme"}
      },
      "required": ["server_name"]
    }
  },
  "required": ["uuid", "xhttp_path", "tls"]
}'::jsonb, 'VLESS + XHTTP + TLS，CDN 中转方案', 'active')
ON CONFLICT (protocol_type, transport_type, security_type, schema_version) DO UPDATE SET config_schema = EXCLUDED.config_schema, description = EXCLUDED.description, status = 'active';

-- VLESS + HTTPUpgrade + TLS
INSERT INTO protocol_registry (protocol_type, transport_type, security_type, schema_version, config_schema, description, status) VALUES
('vless', 'httpupgrade', 'tls', 'v1', '{
  "type": "object",
  "properties": {
    "uuid": {"type": "string", "format": "uuid"},
    "httpupgrade_path": {"type": "string", "default": "/"},
    "httpupgrade_host": {"type": "string"},
    "tls": {
      "type": "object",
      "properties": {
        "server_name": {"type": "string"},
        "alpn": {"type": "array", "items": {"type": "string"}, "default": ["h2", "http/1.1"]},
        "fingerprint": {"type": "string", "default": "chrome"},
        "certificate_mode": {"type": "string", "enum": ["acme", "file", "content", "self"], "default": "acme"}
      },
      "required": ["server_name"]
    }
  },
  "required": ["uuid", "httpupgrade_path", "tls"]
}'::jsonb, 'VLESS + HTTPUpgrade + TLS，兼容 HTTP/1.1 代理路径', 'active')
ON CONFLICT (protocol_type, transport_type, security_type, schema_version) DO UPDATE SET config_schema = EXCLUDED.config_schema, description = EXCLUDED.description, status = 'active';

-- Trojan + TCP + TLS
INSERT INTO protocol_registry (protocol_type, transport_type, security_type, schema_version, config_schema, description, status) VALUES
('trojan', 'tcp', 'tls', 'v1', '{
  "type": "object",
  "properties": {
    "password": {"type": "string"},
    "tls": {
      "type": "object",
      "properties": {
        "server_name": {"type": "string"},
        "alpn": {"type": "array", "items": {"type": "string"}, "default": ["h2", "http/1.1"]},
        "fingerprint": {"type": "string", "default": "chrome"},
        "certificate_mode": {"type": "string", "enum": ["acme", "file", "content", "self"], "default": "acme"}
      },
      "required": ["server_name"]
    }
  },
  "required": ["password", "tls"]
}'::jsonb, 'Trojan + TCP + TLS，客户端兼容性最广', 'active')
ON CONFLICT (protocol_type, transport_type, security_type, schema_version) DO UPDATE SET config_schema = EXCLUDED.config_schema, description = EXCLUDED.description, status = 'active';

-- Trojan + WS + TLS（CDN）
INSERT INTO protocol_registry (protocol_type, transport_type, security_type, schema_version, config_schema, description, status) VALUES
('trojan', 'ws', 'tls', 'v1', '{
  "type": "object",
  "properties": {
    "password": {"type": "string"},
    "ws_path": {"type": "string", "default": "/"},
    "ws_host": {"type": "string"},
    "tls": {
      "type": "object",
      "properties": {
        "server_name": {"type": "string"},
        "alpn": {"type": "array", "items": {"type": "string"}, "default": ["h2", "http/1.1"]},
        "fingerprint": {"type": "string", "default": "chrome"},
        "certificate_mode": {"type": "string", "enum": ["acme", "file", "content", "self"], "default": "acme"}
      },
      "required": ["server_name"]
    }
  },
  "required": ["password", "ws_path", "tls"]
}'::jsonb, 'Trojan + WebSocket + TLS，CDN 中转方案', 'active')
ON CONFLICT (protocol_type, transport_type, security_type, schema_version) DO UPDATE SET config_schema = EXCLUDED.config_schema, description = EXCLUDED.description, status = 'active';

-- Trojan + gRPC + TLS
INSERT INTO protocol_registry (protocol_type, transport_type, security_type, schema_version, config_schema, description, status) VALUES
('trojan', 'grpc', 'tls', 'v1', '{
  "type": "object",
  "properties": {
    "password": {"type": "string"},
    "grpc_service_name": {"type": "string"},
    "grpc_multi_mode": {"type": "boolean", "default": true},
    "tls": {
      "type": "object",
      "properties": {
        "server_name": {"type": "string"},
        "alpn": {"type": "array", "items": {"type": "string"}, "default": ["h2", "http/1.1"]},
        "fingerprint": {"type": "string", "default": "chrome"},
        "certificate_mode": {"type": "string", "enum": ["acme", "file", "content", "self"], "default": "acme"}
      },
      "required": ["server_name"]
    }
  },
  "required": ["password", "grpc_service_name", "tls"]
}'::jsonb, 'Trojan + gRPC + TLS', 'active')
ON CONFLICT (protocol_type, transport_type, security_type, schema_version) DO UPDATE SET config_schema = EXCLUDED.config_schema, description = EXCLUDED.description, status = 'active';

-- VMess + TCP + none
INSERT INTO protocol_registry (protocol_type, transport_type, security_type, schema_version, config_schema, description, status) VALUES
('vmess', 'tcp', 'none', 'v1', '{
  "type": "object",
  "properties": {
    "uuid": {"type": "string", "format": "uuid"},
    "alter_id": {"type": "integer", "default": 0, "minimum": 0},
    "security": {"type": "string", "enum": ["auto", "none", "aes-128-gcm", "chacha20-poly1305"], "default": "auto"}
  },
  "required": ["uuid"]
}'::jsonb, 'VMess + TCP，兼容旧版客户端', 'active')
ON CONFLICT (protocol_type, transport_type, security_type, schema_version) DO UPDATE SET config_schema = EXCLUDED.config_schema, description = EXCLUDED.description, status = 'active';

-- VMess + WS + TLS（CDN）
INSERT INTO protocol_registry (protocol_type, transport_type, security_type, schema_version, config_schema, description, status) VALUES
('vmess', 'ws', 'tls', 'v1', '{
  "type": "object",
  "properties": {
    "uuid": {"type": "string", "format": "uuid"},
    "alter_id": {"type": "integer", "default": 0, "minimum": 0},
    "ws_path": {"type": "string", "default": "/"},
    "ws_host": {"type": "string"},
    "tls": {
      "type": "object",
      "properties": {
        "server_name": {"type": "string"},
        "alpn": {"type": "array", "items": {"type": "string"}, "default": ["h2", "http/1.1"]},
        "fingerprint": {"type": "string", "default": "chrome"},
        "certificate_mode": {"type": "string", "enum": ["acme", "file", "content", "self"], "default": "acme"}
      },
      "required": ["server_name"]
    }
  },
  "required": ["uuid", "ws_path", "tls"]
}'::jsonb, 'VMess + WebSocket + TLS，CDN 中转方案', 'active')
ON CONFLICT (protocol_type, transport_type, security_type, schema_version) DO UPDATE SET config_schema = EXCLUDED.config_schema, description = EXCLUDED.description, status = 'active';

-- Shadowsocks 2022
INSERT INTO protocol_registry (protocol_type, transport_type, security_type, schema_version, config_schema, description, status) VALUES
('shadowsocks', 'tcp', 'none', 'v1', '{
  "type": "object",
  "properties": {
    "method": {"type": "string", "enum": ["2022-blake3-aes-128-gcm", "2022-blake3-aes-256-gcm", "2022-blake3-chacha20-poly1305"], "default": "2022-blake3-aes-256-gcm"},
    "password": {"type": "string", "description": "SS2022 使用 base64 密钥，需符合长度要求"},
    "network": {"type": "string", "enum": ["tcp", "tcp,udp", "udp"], "default": "tcp"},
    "multiplex": {
      "type": "object",
      "properties": {
        "enabled": {"type": "boolean", "default": false},
        "protocol": {"type": "string", "enum": ["h2mux", "smux", "yamux"], "default": "smux"}
      }
    }
  },
  "required": ["method", "password"]
}'::jsonb, 'Shadowsocks 2022（AEAD 2022）', 'active')
ON CONFLICT (protocol_type, transport_type, security_type, schema_version) DO UPDATE SET config_schema = EXCLUDED.config_schema, description = EXCLUDED.description, status = 'active';

-- Hysteria2（QUIC）
INSERT INTO protocol_registry (protocol_type, transport_type, security_type, schema_version, config_schema, description, status) VALUES
('hysteria2', 'udp', 'tls', 'v1', '{
  "type": "object",
  "properties": {
    "password": {"type": "string"},
    "up_mbps": {"type": "integer", "minimum": 1, "default": 100},
    "down_mbps": {"type": "integer", "minimum": 1, "default": 100},
    "obfs": {
      "type": "object",
      "properties": {
        "type": {"type": "string", "enum": ["salamander"], "default": "salamander"},
        "password": {"type": "string"}
      }
    },
    "masquerade": {
      "type": "object",
      "properties": {
        "type": {"type": "string", "enum": ["proxy", "file", "404"], "default": "proxy"},
        "proxy_url": {"type": "string", "default": "https://www.microsoft.com/"}
      }
    },
    "tls": {
      "type": "object",
      "properties": {
        "server_name": {"type": "string"},
        "alpn": {"type": "array", "items": {"type": "string"}, "default": ["h3"]},
        "insecure": {"type": "boolean", "default": false},
        "certificate_mode": {"type": "string", "enum": ["acme", "file", "content", "self"], "default": "acme"}
      },
      "required": ["server_name"]
    }
  },
  "required": ["password", "tls"]
}'::jsonb, 'Hysteria2（QUIC 高速，支持混淆与伪装）', 'active')
ON CONFLICT (protocol_type, transport_type, security_type, schema_version) DO UPDATE SET config_schema = EXCLUDED.config_schema, description = EXCLUDED.description, status = 'active';

-- TUIC v5（QUIC）
INSERT INTO protocol_registry (protocol_type, transport_type, security_type, schema_version, config_schema, description, status) VALUES
('tuic', 'udp', 'tls', 'v1', '{
  "type": "object",
  "properties": {
    "uuid": {"type": "string", "format": "uuid"},
    "password": {"type": "string"},
    "congestion_control": {"type": "string", "enum": ["bbr", "cubic", "new_reno"], "default": "bbr"},
    "udp_relay_mode": {"type": "string", "enum": ["native", "quic"], "default": "native"},
    "zero_rtt_handshake": {"type": "boolean", "default": false},
    "alpn": {"type": "array", "items": {"type": "string"}, "default": ["h3"]},
    "tls": {
      "type": "object",
      "properties": {
        "server_name": {"type": "string"},
        "insecure": {"type": "boolean", "default": false},
        "certificate_mode": {"type": "string", "enum": ["acme", "file", "content", "self"], "default": "acme"}
      },
      "required": ["server_name"]
    }
  },
  "required": ["uuid", "password", "tls"]
}'::jsonb, 'TUIC v5（QUIC，支持 0-RTT）', 'active')
ON CONFLICT (protocol_type, transport_type, security_type, schema_version) DO UPDATE SET config_schema = EXCLUDED.config_schema, description = EXCLUDED.description, status = 'active';

-- AnyTLS + TLS
INSERT INTO protocol_registry (protocol_type, transport_type, security_type, schema_version, config_schema, description, status) VALUES
('anytls', 'tcp', 'tls', 'v1', '{
  "type": "object",
  "properties": {
    "password": {"type": "string"},
    "padding_scheme": {"type": "string", "enum": ["", "max-0", "max-1024", "mixed"], "default": "max-0", "description": "填充方案，空为内核默认"},
    "tls": {
      "type": "object",
      "properties": {
        "server_name": {"type": "string"},
        "alpn": {"type": "array", "items": {"type": "string"}, "default": ["http/1.1"]},
        "fingerprint": {"type": "string", "default": "chrome"},
        "certificate_mode": {"type": "string", "enum": ["acme", "file", "content", "self"], "default": "acme"}
      },
      "required": ["server_name"]
    }
  },
  "required": ["password", "tls"]
}'::jsonb, 'AnyTLS（新协议，填充混淆 + TLS）', 'active')
ON CONFLICT (protocol_type, transport_type, security_type, schema_version) DO UPDATE SET config_schema = EXCLUDED.config_schema, description = EXCLUDED.description, status = 'active';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE protocol_registry DROP COLUMN IF EXISTS status;
-- +goose StatementEnd
