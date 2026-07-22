-- +goose Up
-- +goose StatementBegin

-- 删除 user_node_credentials 关联表
-- 改造背景：per-user UUID 模型已落地（对齐 XBoard），每用户一个 UUID 全节点共享，
-- users.uuid 字段取代了 per-user per-node 凭证关联表。
-- - VLESS/VMess/TUIC: 直接用 user.uuid
-- - Trojan/SS(普通)/Hysteria2/AnyTLS: 直接用 user.uuid
-- - SS2022: 通过 {serverKey}:{userKey} 派生（订阅渲染层处理）
-- - 流量统计直接按 user.uuid 反查 users 表
-- 节点端 xray/sing-box 配置由 node-service 查询 users.uuid 生成多用户 inbound，
-- 不再依赖此关联表。

-- 备份表数据（以防回滚需要），保留 7 天后由 DBA 清理
CREATE TABLE IF NOT EXISTS user_node_credentials_backup_20260706 AS
SELECT * FROM user_node_credentials;

DROP TABLE IF EXISTS user_node_credentials CASCADE;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- 回滚：从备份恢复 user_node_credentials 表
CREATE TABLE IF NOT EXISTS user_node_credentials (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    node_id UUID NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    credential_type VARCHAR(16) NOT NULL,
    credential_value VARCHAR(128) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_user_node UNIQUE (user_id, node_id)
);

CREATE INDEX IF NOT EXISTS idx_user_node_credentials_node_id ON user_node_credentials(node_id);
CREATE INDEX IF NOT EXISTS idx_user_node_credentials_user_id ON user_node_credentials(user_id);
CREATE INDEX IF NOT EXISTS idx_user_node_credentials_value ON user_node_credentials(credential_value);

INSERT INTO user_node_credentials
SELECT * FROM user_node_credentials_backup_20260706
ON CONFLICT (id) DO NOTHING;

-- +goose StatementEnd
