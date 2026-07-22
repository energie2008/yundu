-- +goose Up
-- +goose StatementBegin

CREATE TABLE users (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  email CITEXT NOT NULL UNIQUE,
  username VARCHAR(64) UNIQUE,
  password_hash TEXT,
  password_algo VARCHAR(32) NOT NULL DEFAULT 'argon2id',
  status VARCHAR(32) NOT NULL DEFAULT 'pending',
  email_verified_at TIMESTAMPTZ,
  telegram_chat_id VARCHAR(64),
  inviter_user_id UUID REFERENCES users(id),
  locale VARCHAR(16) NOT NULL DEFAULT 'zh-CN',
  timezone VARCHAR(64) NOT NULL DEFAULT 'Asia/Shanghai',
  last_login_at TIMESTAMPTZ,
  last_login_ip INET,
  last_seen_at TIMESTAMPTZ,
  notes TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at TIMESTAMPTZ
);
CREATE INDEX idx_users_status ON users(status);
CREATE INDEX idx_users_inviter_user_id ON users(inviter_user_id);

CREATE TABLE admins (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL UNIQUE REFERENCES users(id),
  display_name VARCHAR(128) NOT NULL,
  status VARCHAR(32) NOT NULL DEFAULT 'active',
  is_super_admin BOOLEAN NOT NULL DEFAULT false,
  last_login_at TIMESTAMPTZ,
  last_login_ip INET,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at TIMESTAMPTZ
);

CREATE TABLE roles (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  code VARCHAR(64) NOT NULL UNIQUE,
  name VARCHAR(128) NOT NULL,
  description TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE permissions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  code VARCHAR(128) NOT NULL UNIQUE,
  name VARCHAR(128) NOT NULL,
  resource VARCHAR(64) NOT NULL,
  action VARCHAR(64) NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE admin_roles (
  admin_id UUID NOT NULL REFERENCES admins(id) ON DELETE CASCADE,
  role_id UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (admin_id, role_id)
);

CREATE TABLE role_permissions (
  role_id UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
  permission_id UUID NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (role_id, permission_id)
);

CREATE TABLE user_auth_factors (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  factor_type VARCHAR(32) NOT NULL,
  factor_status VARCHAR(32) NOT NULL DEFAULT 'active',
  secret_encrypted TEXT,
  recovery_codes_encrypted TEXT,
  verified_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (user_id, factor_type)
);

CREATE TABLE auth_sessions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  session_type VARCHAR(32) NOT NULL DEFAULT 'web',
  token_id UUID NOT NULL UNIQUE,
  refresh_token_id UUID UNIQUE,
  user_agent TEXT,
  ip_address INET,
  device_fingerprint VARCHAR(256),
  expires_at TIMESTAMPTZ NOT NULL,
  revoked_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_auth_sessions_user_id ON auth_sessions(user_id);
CREATE INDEX idx_auth_sessions_expires_at ON auth_sessions(expires_at);

CREATE TABLE user_profiles (
  user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  avatar_url TEXT,
  contact_email CITEXT,
  phone VARCHAR(32),
  country_code VARCHAR(8),
  tags TEXT[] NOT NULL DEFAULT '{}',
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE user_credentials (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  credential_type VARCHAR(32) NOT NULL,
  uuid_value UUID,
  token_value VARCHAR(255),
  label VARCHAR(64),
  status VARCHAR(32) NOT NULL DEFAULT 'active',
  rotated_from_id UUID REFERENCES user_credentials(id),
  expires_at TIMESTAMPTZ,
  last_used_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_user_credentials_uuid_value
  ON user_credentials(uuid_value) WHERE uuid_value IS NOT NULL;
CREATE UNIQUE INDEX uq_user_credentials_token_value
  ON user_credentials(token_value) WHERE token_value IS NOT NULL;
CREATE INDEX idx_user_credentials_user_id ON user_credentials(user_id);

CREATE TABLE subscription_tokens (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash VARCHAR(255) NOT NULL UNIQUE,
  token_preview VARCHAR(16) NOT NULL,
  status VARCHAR(32) NOT NULL DEFAULT 'active',
  client_hint VARCHAR(64),
  allow_ip_bind BOOLEAN NOT NULL DEFAULT false,
  bound_ip INET,
  last_access_at TIMESTAMPTZ,
  last_access_ip INET,
  expires_at TIMESTAMPTZ,
  revoked_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_subscription_tokens_user_id ON subscription_tokens(user_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS subscription_tokens;
DROP TABLE IF EXISTS user_credentials;
DROP TABLE IF EXISTS user_profiles;
DROP TABLE IF EXISTS auth_sessions;
DROP TABLE IF EXISTS user_auth_factors;
DROP TABLE IF EXISTS role_permissions;
DROP TABLE IF EXISTS admin_roles;
DROP TABLE IF EXISTS permissions;
DROP TABLE IF EXISTS roles;
DROP TABLE IF EXISTS admins;
DROP TABLE IF EXISTS users;
-- +goose StatementEnd
