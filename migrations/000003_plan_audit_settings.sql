-- +goose Up
-- +goose StatementBegin

CREATE TABLE plans (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  code VARCHAR(64) NOT NULL UNIQUE,
  name VARCHAR(128) NOT NULL,
  status VARCHAR(32) NOT NULL DEFAULT 'draft',
  billing_type VARCHAR(32) NOT NULL DEFAULT 'periodic',
  traffic_bytes BIGINT NOT NULL DEFAULT 0,
  speed_limit_mbps INTEGER,
  device_limit INTEGER,
  ip_limit INTEGER,
  reset_cycle VARCHAR(32),
  duration_days INTEGER,
  can_renew BOOLEAN NOT NULL DEFAULT true,
  sort_order INTEGER NOT NULL DEFAULT 0,
  tags TEXT[] NOT NULL DEFAULT '{}',
  feature_flags JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at TIMESTAMPTZ
);
CREATE INDEX idx_plans_status ON plans(status);

CREATE TABLE plan_prices (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  plan_id UUID NOT NULL REFERENCES plans(id) ON DELETE CASCADE,
  period_code VARCHAR(32) NOT NULL,
  currency_code VARCHAR(8) NOT NULL DEFAULT 'CNY',
  amount_minor BIGINT NOT NULL,
  is_active BOOLEAN NOT NULL DEFAULT true,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (plan_id, period_code, currency_code)
);

CREATE TABLE user_plan_subscriptions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  plan_id UUID NOT NULL REFERENCES plans(id),
  status VARCHAR(32) NOT NULL DEFAULT 'inactive',
  started_at TIMESTAMPTZ,
  expires_at TIMESTAMPTZ,
  renewal_mode VARCHAR(32) NOT NULL DEFAULT 'manual',
  traffic_quota_bytes BIGINT NOT NULL DEFAULT 0,
  traffic_used_bytes BIGINT NOT NULL DEFAULT 0,
  reset_at TIMESTAMPTZ,
  speed_limit_mbps INTEGER,
  device_limit INTEGER,
  ip_limit INTEGER,
  source VARCHAR(32) NOT NULL DEFAULT 'manual',
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at TIMESTAMPTZ
);
CREATE INDEX idx_user_plan_subscriptions_user_id ON user_plan_subscriptions(user_id);
CREATE INDEX idx_user_plan_subscriptions_status ON user_plan_subscriptions(status);
CREATE INDEX idx_user_plan_subscriptions_expires_at ON user_plan_subscriptions(expires_at);

CREATE TABLE system_settings (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  setting_group VARCHAR(64) NOT NULL,
  setting_key VARCHAR(128) NOT NULL,
  value_json JSONB NOT NULL,
  is_secret BOOLEAN NOT NULL DEFAULT false,
  description TEXT,
  updated_by_admin_id UUID REFERENCES admins(id),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (setting_group, setting_key)
);

CREATE TABLE audit_logs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  actor_type VARCHAR(32) NOT NULL,
  actor_id UUID,
  actor_display VARCHAR(128),
  action VARCHAR(128) NOT NULL,
  resource_type VARCHAR(64) NOT NULL,
  resource_id UUID,
  request_id VARCHAR(128),
  ip_address INET,
  user_agent TEXT,
  before_json JSONB,
  after_json JSONB,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_audit_logs_resource ON audit_logs(resource_type, resource_id, created_at DESC);
CREATE INDEX idx_audit_logs_actor ON audit_logs(actor_type, actor_id, created_at DESC);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS system_settings;
DROP TABLE IF EXISTS user_plan_subscriptions;
DROP TABLE IF EXISTS plan_prices;
DROP TABLE IF EXISTS plans;
-- +goose StatementEnd
