-- +goose Up
-- +goose StatementBegin

-- 将用户通知偏好字段正式纳入 migration 版本控制
-- 补齐 VPS190 等仅跑了 goose migration 的环境缺失的字段
ALTER TABLE users ADD COLUMN IF NOT EXISTS notify_expiry         BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE users ADD COLUMN IF NOT EXISTS notify_traffic        BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE users ADD COLUMN IF NOT EXISTS notify_ticket_reply   BOOLEAN NOT NULL DEFAULT true;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE users DROP COLUMN IF EXISTS notify_expiry;
ALTER TABLE users DROP COLUMN IF EXISTS notify_traffic;
ALTER TABLE users DROP COLUMN IF EXISTS notify_ticket_reply;
-- +goose StatementEnd
