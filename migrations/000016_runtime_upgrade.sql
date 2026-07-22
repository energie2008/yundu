-- +goose Up
-- +goose StatementBegin

CREATE TABLE runtime_upgrade_tasks (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  server_id UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
  runtime_id UUID NOT NULL REFERENCES runtimes(id) ON DELETE CASCADE,
  from_version TEXT,
  to_version TEXT NOT NULL,
  status VARCHAR(32) NOT NULL DEFAULT 'pending',
  scope VARCHAR(32) NOT NULL DEFAULT 'single',
  batch_id UUID,
  canary_percent INTEGER,
  download_url TEXT,
  expected_sha256 TEXT,
  started_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  error_message TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_runtime_upgrade_tasks_server_id ON runtime_upgrade_tasks(server_id, created_at DESC);
CREATE INDEX idx_runtime_upgrade_tasks_batch_id ON runtime_upgrade_tasks(batch_id);
CREATE INDEX idx_runtime_upgrade_tasks_status ON runtime_upgrade_tasks(status);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS runtime_upgrade_tasks;
-- +goose StatementEnd
