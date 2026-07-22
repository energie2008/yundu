-- +goose Up
-- +goose StatementBegin

CREATE TABLE protocol_registry (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  protocol_type VARCHAR(32) NOT NULL,
  transport_type VARCHAR(32) NOT NULL DEFAULT 'tcp',
  security_type VARCHAR(32) NOT NULL DEFAULT 'none',
  schema_version VARCHAR(16) NOT NULL DEFAULT 'v1',
  config_schema JSONB NOT NULL,
  description TEXT,
  is_enabled BOOLEAN NOT NULL DEFAULT true,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (protocol_type, transport_type, security_type, schema_version)
);
CREATE INDEX idx_protocol_registry_combo ON protocol_registry(protocol_type, transport_type, security_type);
CREATE INDEX idx_protocol_registry_enabled ON protocol_registry(is_enabled);

CREATE TABLE config_templates (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  code VARCHAR(64) NOT NULL UNIQUE,
  name VARCHAR(128) NOT NULL,
  runtime_type VARCHAR(16) NOT NULL,
  template_type VARCHAR(32) NOT NULL,
  content TEXT NOT NULL,
  variables_schema JSONB NOT NULL DEFAULT '{}'::jsonb,
  is_default BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_config_templates_runtime ON config_templates(runtime_type, template_type);
CREATE INDEX idx_config_templates_default ON config_templates(is_default) WHERE is_default = true;

-- 种子数据：协议 schema（vless+reality / hysteria2 / tuic / shadowsocks2022）

INSERT INTO protocol_registry (protocol_type, transport_type, security_type, schema_version, config_schema, description) VALUES
('vless', 'tcp', 'reality', 'v1', '{
  "type": "object",
  "properties": {
    "uuid": {"type": "string", "format": "uuid"},
    "flow": {"type": "string", "enum": ["", "xtls-rprx-vision"]},
    "reality": {
      "type": "object",
      "properties": {
        "public_key": {"type": "string"},
        "short_id": {"type": "string"},
        "server_name": {"type": "string"},
        "spider_x": {"type": "string"},
        "fingerprint": {"type": "string"}
      },
      "required": ["public_key", "short_id", "server_name"]
    }
  },
  "required": ["uuid"]
}'::jsonb, 'VLESS + REALITY 协议，抗主动探测，推荐用于生产环境');

INSERT INTO protocol_registry (protocol_type, transport_type, security_type, schema_version, config_schema, description) VALUES
('hysteria2', 'udp', 'tls', 'v1', '{
  "type": "object",
  "properties": {
    "password": {"type": "string"},
    "up_mbps": {"type": "integer", "minimum": 1, "default": 100},
    "down_mbps": {"type": "integer", "minimum": 1, "default": 100},
    "obfs": {
      "type": "object",
      "properties": {
        "type": {"type": "string", "enum": ["salamander"]},
        "password": {"type": "string"}
      }
    },
    "tls": {
      "type": "object",
      "properties": {
        "server_name": {"type": "string"},
        "insecure": {"type": "boolean", "default": false}
      }
    }
  },
  "required": ["password"]
}'::jsonb, 'Hysteria2 基于 QUIC 的高速协议，支持带宽控制和混淆');

INSERT INTO protocol_registry (protocol_type, transport_type, security_type, schema_version, config_schema, description) VALUES
('tuic', 'udp', 'tls', 'v1', '{
  "type": "object",
  "properties": {
    "uuid": {"type": "string", "format": "uuid"},
    "password": {"type": "string"},
    "congestion_control": {"type": "string", "enum": ["bbr", "cubic", "new_reno"], "default": "bbr"},
    "udp_relay_mode": {"type": "string", "enum": ["native", "quic"], "default": "native"},
    "alpn": {"type": "array", "items": {"type": "string"}},
    "tls": {
      "type": "object",
      "properties": {
        "server_name": {"type": "string"},
        "insecure": {"type": "boolean", "default": false}
      }
    }
  },
  "required": ["uuid", "password"]
}'::jsonb, 'TUIC 基于 QUIC 的高性能协议，支持多种拥塞控制');

INSERT INTO protocol_registry (protocol_type, transport_type, security_type, schema_version, config_schema, description) VALUES
('shadowsocks', 'tcp', 'none', 'v1', '{
  "type": "object",
  "properties": {
    "method": {"type": "string", "enum": ["2022-blake3-aes-128-gcm", "2022-blake3-aes-256-gcm", "2022-blake3-chacha20-poly1305"]},
    "password": {"type": "string"},
    "multiplex": {
      "type": "object",
      "properties": {
        "enabled": {"type": "boolean", "default": false},
        "protocol": {"type": "string", "enum": ["h2mux", "smux", "yamux"]}
      }
    }
  },
  "required": ["method", "password"]
}'::jsonb, 'Shadowsocks 2022 协议，使用 AEAD 2022 加密');

-- 种子数据：配置模板

INSERT INTO config_templates (code, name, runtime_type, template_type, content, variables_schema, is_default) VALUES
('xray-vless-reality-inbound', 'Xray VLESS Reality Inbound', 'xray', 'inbound',
'{
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
'{"port": {"type": "integer"}, "uuid": {"type": "string"}, "flow": {"type": "string"}, "reality_dest": {"type": "string"}, "server_name": {"type": "string"}, "reality_private_key": {"type": "string"}, "short_id": {"type": "string"}}'::jsonb, true);

INSERT INTO config_templates (code, name, runtime_type, template_type, content, variables_schema, is_default) VALUES
('sing-box-hysteria2-inbound', 'sing-box Hysteria2 Inbound', 'sing-box', 'inbound',
'{
  "type": "hysteria2",
  "tag": "{{.tag}}",
  "listen": "::",
  "listen_port": {{.port}},
  "users": [
    {
      "password": "{{.password}}"
    }
  ],
  "up_mbps": {{.up_mbps}},
  "down_mbps": {{.down_mbps}},
  "tls": {
    "server_name": "{{.server_name}}",
    "alpn": ["h3"],
    "certificate_path": "{{.cert_path}}",
    "key_path": "{{.key_path}}"
  }
}',
'{"tag": {"type": "string"}, "port": {"type": "integer"}, "password": {"type": "string"}, "up_mbps": {"type": "integer"}, "down_mbps": {"type": "integer"}, "server_name": {"type": "string"}, "cert_path": {"type": "string"}, "key_path": {"type": "string"}}'::jsonb, true);

INSERT INTO config_templates (code, name, runtime_type, template_type, content, variables_schema, is_default) VALUES
('sing-box-tuic-inbound', 'sing-box TUIC Inbound', 'sing-box', 'inbound',
'{
  "type": "tuic",
  "tag": "{{.tag}}",
  "listen": "::",
  "listen_port": {{.port}},
  "users": [
    {
      "uuid": "{{.uuid}}",
      "password": "{{.password}}"
    }
  ],
  "congestion_control": "{{.congestion_control}}",
  "udp_relay_mode": "{{.udp_relay_mode}}",
  "tls": {
    "server_name": "{{.server_name}}",
    "certificate_path": "{{.cert_path}}",
    "key_path": "{{.key_path}}"
  }
}',
'{"tag": {"type": "string"}, "port": {"type": "integer"}, "uuid": {"type": "string"}, "password": {"type": "string"}, "congestion_control": {"type": "string"}, "udp_relay_mode": {"type": "string"}, "server_name": {"type": "string"}, "cert_path": {"type": "string"}, "key_path": {"type": "string"}}'::jsonb, true);

INSERT INTO config_templates (code, name, runtime_type, template_type, content, variables_schema, is_default) VALUES
('sing-box-shadowsocks-inbound', 'sing-box Shadowsocks 2022 Inbound', 'sing-box', 'inbound',
'{
  "type": "shadowsocks",
  "tag": "{{.tag}}",
  "listen": "::",
  "listen_port": {{.port}},
  "method": "{{.method}}",
  "password": "{{.password}}"
}',
'{"tag": {"type": "string"}, "port": {"type": "integer"}, "method": {"type": "string"}, "password": {"type": "string"}}'::jsonb, true);

INSERT INTO config_templates (code, name, runtime_type, template_type, content, variables_schema, is_default) VALUES
('xray-routing-general', 'Xray Routing General', 'xray', 'general',
'{
  "domainStrategy": "IPIfNonMatch",
  "rules": [
    {
      "type": "field",
      "outboundTag": "direct",
      "ip": ["geoip:private"]
    }
  ]
}',
'{}'::jsonb, true);

INSERT INTO config_templates (code, name, runtime_type, template_type, content, variables_schema, is_default) VALUES
('sing-box-general', 'sing-box General Config', 'sing-box', 'general',
'{
  "log": {
    "level": "info",
    "timestamp": true
  }
}',
'{}'::jsonb, true);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS config_templates;
DROP TABLE IF EXISTS protocol_registry;
-- +goose StatementEnd
