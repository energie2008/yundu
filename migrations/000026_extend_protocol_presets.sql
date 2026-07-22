-- +goose Up
-- +goose StatementBegin

ALTER TABLE protocol_presets
  ADD COLUMN IF NOT EXISTS badge VARCHAR(32),
  ADD COLUMN IF NOT EXISTS min_xray_version VARCHAR(32),
  ADD COLUMN IF NOT EXISTS min_singbox_version VARCHAR(32),
  ADD COLUMN IF NOT EXISTS client_support JSONB NOT NULL DEFAULT '[]'::jsonb,
  ADD COLUMN IF NOT EXISTS kernel_compat VARCHAR(32) NOT NULL DEFAULT 'both',
  ADD COLUMN IF NOT EXISTS base_spec JSONB NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN IF NOT EXISTS recommendations JSONB NOT NULL DEFAULT '[]'::jsonb,
  ADD COLUMN IF NOT EXISTS warnings JSONB NOT NULL DEFAULT '[]'::jsonb,
  ADD COLUMN IF NOT EXISTS is_builtin BOOLEAN NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS updated_from_upstream TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS deprecated_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_protocol_presets_builtin ON protocol_presets(is_builtin) WHERE is_builtin = true;
CREATE INDEX IF NOT EXISTS idx_protocol_presets_kernel_compat ON protocol_presets(kernel_compat);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE protocol_presets
  DROP COLUMN IF EXISTS badge,
  DROP COLUMN IF EXISTS min_xray_version,
  DROP COLUMN IF EXISTS min_singbox_version,
  DROP COLUMN IF EXISTS client_support,
  DROP COLUMN IF EXISTS kernel_compat,
  DROP COLUMN IF EXISTS base_spec,
  DROP COLUMN IF EXISTS recommendations,
  DROP COLUMN IF EXISTS warnings,
  DROP COLUMN IF EXISTS is_builtin,
  DROP COLUMN IF EXISTS updated_from_upstream,
  DROP COLUMN IF EXISTS deprecated_at;

DROP INDEX IF EXISTS idx_protocol_presets_builtin;
DROP INDEX IF EXISTS idx_protocol_presets_kernel_compat;

-- +goose StatementEnd
