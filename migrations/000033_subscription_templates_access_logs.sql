-- +goose Up
-- +goose StatementBegin

CREATE TABLE IF NOT EXISTS subscription_templates (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  code VARCHAR(64) NOT NULL UNIQUE,
  name VARCHAR(128) NOT NULL,
  target_client VARCHAR(32) NOT NULL DEFAULT 'clash-meta',
  template_type VARCHAR(32) NOT NULL DEFAULT 'custom',
  schema_version VARCHAR(16) NOT NULL DEFAULT '1.0',
  content TEXT NOT NULL DEFAULT '',
  status VARCHAR(16) NOT NULL DEFAULT 'active',
  is_default BOOLEAN NOT NULL DEFAULT false,
  created_by_admin_id UUID REFERENCES users(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_sub_templates_client ON subscription_templates(target_client);
CREATE INDEX idx_sub_templates_status ON subscription_templates(status);
CREATE INDEX idx_sub_templates_default ON subscription_templates(is_default) WHERE is_default = true;

CREATE TABLE IF NOT EXISTS subscription_access_logs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  token_id UUID REFERENCES subscription_tokens(id) ON DELETE SET NULL,
  user_id UUID REFERENCES users(id) ON DELETE SET NULL,
  client_type VARCHAR(32),
  request_ip VARCHAR(64),
  user_agent TEXT,
  response_status INTEGER NOT NULL DEFAULT 200,
  template_code VARCHAR(64),
  generated_node_count INTEGER NOT NULL DEFAULT 0,
  cache_hit BOOLEAN NOT NULL DEFAULT false,
  requested_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_sub_access_token ON subscription_access_logs(token_id);
CREATE INDEX idx_sub_access_user ON subscription_access_logs(user_id);
CREATE INDEX idx_sub_access_ip ON subscription_access_logs(request_ip);
CREATE INDEX idx_sub_access_client ON subscription_access_logs(client_type);
CREATE INDEX idx_sub_access_time ON subscription_access_logs(requested_at DESC);

INSERT INTO subscription_templates (code, name, target_client, template_type, content, status, is_default)
VALUES
  ('default-clash', '默认Clash配置', 'clash', 'builtin', '# Clash 默认模板\nmixed-port: 7890\nallow-lan: false\nmode: rule\nlog-level: info\nproxies: []\nproxy-groups: []\nrules: []\n', 'active', false),
  ('default-clash-meta', '默认Clash Meta配置', 'clash-meta', 'builtin', '# Clash Meta 默认模板\nmixed-port: 7890\nallow-lan: false\nmode: rule\nlog-level: info\nunified-delay: true\nproxies: []\nproxy-groups: []\nrules: []\n', 'active', true),
  ('default-singbox', '默认Sing-box配置', 'sing-box', 'builtin', '{\n  "log": {"level": "info"},\n  "inbounds": [{"type": "mixed", "listen": "127.0.0.1", "listen_port": 7890}],\n  "outbounds": [{"type": "direct"}],\n  "route": {"rules": []}\n}\n', 'active', false)
ON CONFLICT (code) DO NOTHING;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS subscription_access_logs;
DROP TABLE IF EXISTS subscription_templates;
-- +goose StatementEnd
