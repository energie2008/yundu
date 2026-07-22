-- +goose Up
-- +goose StatementBegin

CREATE TABLE edge_exposures (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  server_id UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
  code VARCHAR(64) NOT NULL UNIQUE,
  name VARCHAR(128) NOT NULL,
  exposure_mode VARCHAR(32) NOT NULL,
  public_hostname VARCHAR(255),
  public_port INTEGER NOT NULL DEFAULT 443,
  origin_host VARCHAR(255) NOT NULL DEFAULT '127.0.0.1',
  origin_port INTEGER NOT NULL,
  nginx_enabled BOOLEAN NOT NULL DEFAULT false,
  nginx_ws_path VARCHAR(255),
  nginx_host_header VARCHAR(255),
  nginx_extra_conf TEXT,
  tls_profile_id UUID REFERENCES tls_profiles(id) ON DELETE SET NULL,
  cf_tunnel_token_encrypted TEXT,
  cf_tunnel_id VARCHAR(128),
  cf_tunnel_name VARCHAR(128),
  cf_protocol VARCHAR(16) NOT NULL DEFAULT 'auto',
  cf_no_tls_verify BOOLEAN NOT NULL DEFAULT false,
  cf_origin_server_name VARCHAR(255),
  argo_ws_token_encrypted TEXT,
  status VARCHAR(32) NOT NULL DEFAULT 'pending',
  health_check_url TEXT,
  last_health_check_at TIMESTAMPTZ,
  last_health_status VARCHAR(32),
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_edge_exposures_server_id ON edge_exposures(server_id);

CREATE TABLE nginx_generated_configs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  exposure_id UUID NOT NULL REFERENCES edge_exposures(id) ON DELETE CASCADE,
  config_content TEXT NOT NULL,
  config_hash VARCHAR(128) NOT NULL,
  schema_version VARCHAR(32) NOT NULL DEFAULT 'v1',
  generated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  deployed_at TIMESTAMPTZ,
  deploy_status VARCHAR(32) NOT NULL DEFAULT 'pending',
  deploy_error TEXT,
  UNIQUE (exposure_id, config_hash)
);
CREATE INDEX idx_nginx_generated_configs_exposure_id ON nginx_generated_configs(exposure_id);

CREATE TABLE exposure_compat_rules (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  protocol_type VARCHAR(32) NOT NULL,
  transport_type VARCHAR(32),
  security_type VARCHAR(32),
  exposure_mode VARCHAR(32) NOT NULL,
  is_allowed BOOLEAN NOT NULL DEFAULT true,
  reason TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (protocol_type, transport_type, security_type, exposure_mode)
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS exposure_compat_rules;
DROP TABLE IF EXISTS nginx_generated_configs;
DROP TABLE IF EXISTS edge_exposures;
-- +goose StatementEnd
