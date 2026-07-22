-- +goose Up
-- +goose StatementBegin

-- 为 plans 表新增 group_id 字段，关联 node_groups 表
-- 套餐绑定的会员分组，决定用户购买此套餐后能看到哪些节点
ALTER TABLE plans ADD COLUMN IF NOT EXISTS group_id UUID REFERENCES node_groups(id) ON DELETE SET NULL;

-- 为查询优化添加索引
CREATE INDEX IF NOT EXISTS idx_plans_group_id ON plans(group_id) WHERE deleted_at IS NULL;

-- 为已存在的节点表补充 group_id（node-service 已使用 nodes.group_id）
-- 这里仅做幂等补充，避免 nodes 表未同步迁移时出错
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'nodes' AND column_name = 'group_id'
    ) THEN
        -- 已存在，无需操作
        NULL;
    ELSE
        ALTER TABLE nodes ADD COLUMN group_id UUID REFERENCES node_groups(id) ON DELETE SET NULL;
        CREATE INDEX IF NOT EXISTS idx_nodes_group_id ON nodes(group_id) WHERE deleted_at IS NULL;
    END IF;
END $$;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE plans DROP COLUMN IF EXISTS group_id;
-- 注意：不回滚 nodes.group_id，避免影响 node-service 已有逻辑

-- +goose StatementEnd
