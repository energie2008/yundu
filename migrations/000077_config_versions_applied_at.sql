-- migration 000077: config_versions 记录实际应用时间
-- ReportConfigResult 三写事务需要 applied_at 列；此前版本代码已引用该列但迁移缺失，
-- 导致 ack 回写报 column does not exist，节点 applied 状态无法落库。

-- +goose Up
-- +goose StatementBegin
ALTER TABLE config_versions ADD COLUMN IF NOT EXISTS applied_at TIMESTAMPTZ;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE config_versions DROP COLUMN IF EXISTS applied_at;
-- +goose StatementEnd
