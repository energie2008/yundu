-- +goose Up
-- +goose StatementBegin

-- P2-3: 节点 Host 表（多订阅地址）
-- 一个节点可关联多个订阅域名，支持 CDN fallback / 多 CDN 提供商 / 地理路由
-- subscription-service 渲染时按 host_type+priority 展开为多条连接配置
CREATE TABLE IF NOT EXISTS node_hosts (
    id BIGSERIAL PRIMARY KEY,
    node_id UUID NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    host VARCHAR(255) NOT NULL,           -- 订阅域名（如 cdn1.example.com）
    host_type VARCHAR(32) NOT NULL DEFAULT 'cdn',  -- cdn / direct / tunnel
    port INTEGER,                          -- 覆盖节点端口（NULL=使用 node.port）
    path VARCHAR(255),                     -- 覆盖传输路径（NULL=使用 node path）
    sni VARCHAR(255),                      -- 覆盖 TLS SNI（NULL=使用 node.sni）
    host_header VARCHAR(255),              -- 覆盖 HTTP Host 头（NULL=使用 node.host_header）
    priority INTEGER NOT NULL DEFAULT 100, -- 优先级（小=优先）
    is_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    remark VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(node_id, host)
);

CREATE INDEX idx_node_hosts_node ON node_hosts(node_id);
CREATE INDEX idx_node_hosts_enabled ON node_hosts(is_enabled, priority);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS node_hosts;
-- +goose StatementEnd
