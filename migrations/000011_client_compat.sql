-- +goose Up
-- +goose StatementBegin

CREATE TABLE client_profiles (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  code VARCHAR(64) NOT NULL UNIQUE,
  name VARCHAR(128) NOT NULL,
  platform VARCHAR(32) NOT NULL,
  min_version VARCHAR(32),
  max_version VARCHAR(32),
  status VARCHAR(32) NOT NULL DEFAULT 'active',
  notes TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE client_compat_matrix (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  client_code VARCHAR(64) NOT NULL,
  feature_code VARCHAR(64) NOT NULL,
  supported BOOLEAN NOT NULL DEFAULT false,
  supported_since_version VARCHAR(32),
  notes TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (client_code, feature_code)
);
CREATE INDEX idx_client_compat_matrix_client_code ON client_compat_matrix(client_code);

CREATE TABLE advanced_patch_profiles (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  node_id UUID NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  runtime_type VARCHAR(32) NOT NULL DEFAULT 'xray',
  patch_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  patch_target VARCHAR(32) NOT NULL DEFAULT 'inbound',
  allowed_keys JSONB NOT NULL DEFAULT '[]'::jsonb,
  schema_version VARCHAR(32) NOT NULL DEFAULT 'v1',
  is_enabled BOOLEAN NOT NULL DEFAULT true,
  last_validated_at TIMESTAMPTZ,
  last_validation_result JSONB,
  notes TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (node_id, runtime_type, patch_target)
);
CREATE INDEX idx_advanced_patch_profiles_node_id ON advanced_patch_profiles(node_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS advanced_patch_profiles;
DROP TABLE IF EXISTS client_compat_matrix;
DROP TABLE IF EXISTS client_profiles;
-- +goose StatementEnd
