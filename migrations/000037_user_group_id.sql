-- +goose Up
-- +goose StatementBegin

-- 为 users 表新增 group_id 字段，关联 node_groups 表
-- 用户购买套餐时自动赋值 plan.group_id，决定订阅可见节点范围
ALTER TABLE users ADD COLUMN IF NOT EXISTS group_id UUID REFERENCES node_groups(id) ON DELETE SET NULL;

-- 为查询优化添加索引
CREATE INDEX IF NOT EXISTS idx_users_group_id ON users(group_id) WHERE deleted_at IS NULL;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE users DROP COLUMN IF EXISTS group_id;

-- +goose StatementEnd
