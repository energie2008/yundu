-- +goose Up
-- +goose StatementBegin

-- 用户节点凭证表（per-user per-node UUID/密码）
-- 借鉴 XBoard 方案：每个用户对每个节点有独立的 UUID/密码，
-- 节点端 xray/sing-box 配置包含所有用户的凭证，
-- 流量统计按 UUID/密码关联到具体用户
CREATE TABLE IF NOT EXISTS user_node_credentials (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    node_id UUID NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    -- credential_type: 'uuid' (VLESS/VMess/TUIC) 或 'password' (Trojan/SS/Hysteria2/AnyTLS)
    credential_type VARCHAR(16) NOT NULL,
    -- credential_value: 实际的 UUID 或密码字符串
    credential_value VARCHAR(128) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- 每个用户对每个节点只有一条凭证记录
    CONSTRAINT uq_user_node UNIQUE (user_id, node_id)
);

-- 索引：按节点查询所有用户凭证（节点配置生成时使用）
CREATE INDEX IF NOT EXISTS idx_user_node_credentials_node_id ON user_node_credentials(node_id);
-- 索引：按用户查询所有节点凭证（订阅渲染时使用）
CREATE INDEX IF NOT EXISTS idx_user_node_credentials_user_id ON user_node_credentials(user_id);
-- 索引：按凭证值查询（流量上报时按 UUID/密码反查用户）
CREATE INDEX IF NOT EXISTS idx_user_node_credentials_value ON user_node_credentials(credential_value);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS user_node_credentials CASCADE;

-- +goose StatementEnd
