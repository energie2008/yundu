-- +goose Up
-- +goose StatementBegin

CREATE TABLE node_doctor_reports (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  node_id UUID NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  report_type VARCHAR(32) NOT NULL DEFAULT 'full',
  trigger_source VARCHAR(32) NOT NULL DEFAULT 'scheduled',
  overall_status VARCHAR(32) NOT NULL DEFAULT 'unknown',
  checks JSONB NOT NULL DEFAULT '[]'::jsonb,
  summary_ok INTEGER NOT NULL DEFAULT 0,
  summary_warn INTEGER NOT NULL DEFAULT 0,
  summary_fail INTEGER NOT NULL DEFAULT 0,
  duration_ms INTEGER,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_node_doctor_reports_node_id ON node_doctor_reports(node_id, created_at DESC);

CREATE TABLE node_doctor_check_defs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  code VARCHAR(64) NOT NULL UNIQUE,
  name VARCHAR(128) NOT NULL,
  description TEXT,
  check_category VARCHAR(32) NOT NULL,
  severity VARCHAR(32) NOT NULL DEFAULT 'warn',
  applicable_exposure_modes JSONB NOT NULL DEFAULT '["*"]'::jsonb,
  applicable_protocol_types JSONB NOT NULL DEFAULT '["*"]'::jsonb,
  auto_fix_available BOOLEAN NOT NULL DEFAULT false,
  auto_fix_action VARCHAR(64),
  sort_order INTEGER NOT NULL DEFAULT 0,
  is_enabled BOOLEAN NOT NULL DEFAULT true,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE config_import_jobs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_type VARCHAR(32) NOT NULL,
  raw_content TEXT NOT NULL,
  parse_result JSONB NOT NULL DEFAULT '{}'::jsonb,
  parse_status VARCHAR(32) NOT NULL DEFAULT 'pending',
  parse_error TEXT,
  preview_node_spec JSONB,
  apply_status VARCHAR(32) NOT NULL DEFAULT 'pending',
  applied_node_id UUID REFERENCES nodes(id) ON DELETE SET NULL,
  created_by_admin_id UUID REFERENCES admins(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_at TIMESTAMPTZ
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS config_import_jobs;
DROP TABLE IF EXISTS node_doctor_check_defs;
DROP TABLE IF EXISTS node_doctor_reports;
-- +goose StatementEnd
