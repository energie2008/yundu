-- +goose Up
-- +goose StatementBegin

-- 节点-分组 多对多关联表
-- 一个节点可同时属于多个分组；一个分组可包含多个节点
-- 保留 nodes.group_id 作为"主分组"用于显示与排序，关联表用于订阅可见性过滤
CREATE TABLE IF NOT EXISTS node_group_members (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    node_id UUID NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    group_id UUID NOT NULL REFERENCES node_groups(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- 联合唯一：同一节点在同一分组中只有一条记录
    CONSTRAINT uq_node_group_member UNIQUE (node_id, group_id)
);

-- 索引：按分组查节点（管理后台列出分组下节点）
CREATE INDEX IF NOT EXISTS idx_node_group_members_group_id ON node_group_members(group_id);
-- 索引：按节点查分组（订阅渲染、节点编辑回显）
CREATE INDEX IF NOT EXISTS idx_node_group_members_node_id ON node_group_members(node_id);

-- 数据迁移：将 nodes.group_id 既有数据初始化到关联表
-- ON CONFLICT DO NOTHING 保证幂等（重复执行迁移不会报错）
INSERT INTO node_group_members (node_id, group_id)
SELECT id, group_id FROM nodes
WHERE group_id IS NOT NULL AND deleted_at IS NULL
ON CONFLICT DO NOTHING;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS node_group_members CASCADE;

-- +goose StatementEnd
