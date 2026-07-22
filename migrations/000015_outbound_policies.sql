-- +goose Up
-- +goose StatementBegin

CREATE TABLE outbound_policies (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  node_id UUID NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  policy_type VARCHAR(32) NOT NULL,
  priority INTEGER NOT NULL DEFAULT 100,
  config_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  routing_rules JSONB NOT NULL DEFAULT '[]'::jsonb,
  is_enabled BOOLEAN NOT NULL DEFAULT true,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_outbound_policies_node ON outbound_policies(node_id, priority);
CREATE INDEX idx_outbound_policies_enabled ON outbound_policies(is_enabled) WHERE is_enabled = true;

CREATE TABLE warp_profiles (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  code VARCHAR(64) NOT NULL UNIQUE,
  name VARCHAR(128) NOT NULL,
  warp_mode VARCHAR(16) NOT NULL DEFAULT 'warp',
  endpoint VARCHAR(255),
  license_key VARCHAR(255),
  config_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  is_default BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_warp_profiles_default ON warp_profiles(is_default) WHERE is_default = true;

-- 种子数据：一个默认 WARP profile
INSERT INTO warp_profiles (code, name, warp_mode, endpoint, license_key, config_json, is_default) VALUES
('default-warp', 'Default WARP Profile', 'warp', 'engage.cloudflareclient.com:2408', '',
 '{"mtu": 1280, "enabled": true}'::jsonb, true);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS warp_profiles;
DROP TABLE IF EXISTS outbound_policies;
-- +goose StatementEnd
