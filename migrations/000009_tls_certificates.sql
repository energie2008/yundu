-- +goose Up
-- +goose StatementBegin

CREATE TABLE tls_certificates (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  code VARCHAR(64) NOT NULL UNIQUE,
  name VARCHAR(128) NOT NULL,
  cert_type VARCHAR(32) NOT NULL,
  common_name VARCHAR(255) NOT NULL,
  sans JSONB NOT NULL DEFAULT '[]'::jsonb,
  provider VARCHAR(32) NOT NULL DEFAULT 'custom',
  cert_pem TEXT,
  key_pem_encrypted TEXT,
  ca_pem TEXT,
  fingerprint_sha256 VARCHAR(128),
  issued_at TIMESTAMPTZ,
  expires_at TIMESTAMPTZ,
  auto_renew BOOLEAN NOT NULL DEFAULT true,
  renew_days_before INTEGER NOT NULL DEFAULT 21,
  renew_status VARCHAR(32) NOT NULL DEFAULT 'unknown',
  renew_last_attempt_at TIMESTAMPTZ,
  renew_last_error TEXT,
  deploy_mode VARCHAR(32) NOT NULL DEFAULT 'agent_push',
  acme_account_email VARCHAR(255),
  acme_challenge_type VARCHAR(32),
  acme_dns_provider VARCHAR(64),
  acme_credentials_encrypted TEXT,
  cloudflare_zone_id VARCHAR(64),
  status VARCHAR(32) NOT NULL DEFAULT 'active',
  created_by_admin_id UUID REFERENCES admins(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_tls_certificates_expires_at ON tls_certificates(expires_at);

CREATE TABLE tls_profiles (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  code VARCHAR(64) NOT NULL UNIQUE,
  name VARCHAR(128) NOT NULL,
  tls_mode VARCHAR(32) NOT NULL DEFAULT 'tls',
  server_name VARCHAR(255),
  certificate_id UUID REFERENCES tls_certificates(id) ON DELETE SET NULL,
  allow_insecure BOOLEAN NOT NULL DEFAULT false,
  utls_fingerprint VARCHAR(32) NOT NULL DEFAULT 'chrome',
  alpn JSONB NOT NULL DEFAULT '["h2","http/1.1"]'::jsonb,
  min_version VARCHAR(16) NOT NULL DEFAULT 'tls12',
  max_version VARCHAR(16) NOT NULL DEFAULT 'tls13',
  ech_enabled BOOLEAN NOT NULL DEFAULT false,
  ech_config_encrypted TEXT,
  reality_public_key VARCHAR(128),
  reality_private_key_encrypted VARCHAR(256),
  reality_short_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
  reality_spider_x VARCHAR(255),
  reality_dest VARCHAR(255),
  notes TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE cert_deploy_records (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  certificate_id UUID NOT NULL REFERENCES tls_certificates(id) ON DELETE CASCADE,
  server_id UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
  deploy_status VARCHAR(32) NOT NULL DEFAULT 'pending',
  deploy_path VARCHAR(255),
  deployed_at TIMESTAMPTZ,
  error_message TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (certificate_id, server_id)
);
CREATE INDEX idx_cert_deploy_records_cert_id ON cert_deploy_records(certificate_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS cert_deploy_records;
DROP TABLE IF EXISTS tls_profiles;
DROP TABLE IF EXISTS tls_certificates;
-- +goose StatementEnd
