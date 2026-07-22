-- +goose Up
-- +goose StatementBegin

CREATE TABLE traffic_usage_daily (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  usage_date DATE NOT NULL,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  subscription_id UUID REFERENCES user_plan_subscriptions(id) ON DELETE SET NULL,
  node_id UUID REFERENCES nodes(id) ON DELETE SET NULL,
  upload_bytes BIGINT NOT NULL DEFAULT 0,
  download_bytes BIGINT NOT NULL DEFAULT 0,
  total_bytes BIGINT GENERATED ALWAYS AS (upload_bytes + download_bytes) STORED,
  unique_devices INTEGER NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (usage_date, user_id, node_id)
);
CREATE INDEX idx_traffic_usage_daily_user_date ON traffic_usage_daily(user_id, usage_date DESC);

CREATE TABLE online_sessions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  credential_id UUID REFERENCES user_credentials(id) ON DELETE SET NULL,
  node_id UUID REFERENCES nodes(id) ON DELETE SET NULL,
  runtime_id UUID REFERENCES runtimes(id) ON DELETE SET NULL,
  client_ip INET,
  client_type VARCHAR(64),
  connected_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  disconnected_at TIMESTAMPTZ,
  status VARCHAR(32) NOT NULL DEFAULT 'online'
);
CREATE INDEX idx_online_sessions_user_status ON online_sessions(user_id, status);
CREATE INDEX idx_online_sessions_node_status ON online_sessions(node_id, status);

CREATE TABLE subscription_templates (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  code VARCHAR(64) NOT NULL UNIQUE,
  name VARCHAR(128) NOT NULL,
  target_client VARCHAR(64) NOT NULL,
  template_type VARCHAR(32) NOT NULL,
  schema_version VARCHAR(32) NOT NULL DEFAULT 'v1',
  content TEXT NOT NULL,
  status VARCHAR(32) NOT NULL DEFAULT 'active',
  created_by_admin_id UUID REFERENCES admins(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE subscription_access_logs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  token_id UUID REFERENCES subscription_tokens(id) ON DELETE SET NULL,
  user_id UUID REFERENCES users(id) ON DELETE SET NULL,
  client_type VARCHAR(64),
  request_ip INET,
  user_agent TEXT,
  response_status INTEGER NOT NULL,
  template_code VARCHAR(64),
  generated_node_count INTEGER NOT NULL DEFAULT 0,
  cache_hit BOOLEAN NOT NULL DEFAULT false,
  requested_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_sub_access_logs_user_at ON subscription_access_logs(user_id, requested_at DESC);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS subscription_access_logs;
DROP TABLE IF EXISTS subscription_templates;
DROP TABLE IF EXISTS online_sessions;
DROP TABLE IF EXISTS traffic_usage_daily;
-- +goose StatementEnd
