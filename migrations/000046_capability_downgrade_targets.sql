-- +goose Up
-- +goose StatementBegin

-- P2-2: 为 sing-box 不支持但可降级的协议组合补充 downgrade_to 目标
-- 策略：当 capability_degrade_strategy = "downgrade" 时，preflight/render 自动应用降级
--       并记录 CapabilityLost 事件；strategy = "deny" 时仍拒绝发布

-- sing-box 不支持 XHTTP，可降级为 HTTPUpgrade（保留 path/host）
UPDATE kernel_capabilities
SET downgrade_to = '{"transport": "httpupgrade"}'::jsonb,
    message = 'sing-box 不支持 XHTTP，可降级为 HTTPUpgrade（P2-2）'
WHERE kernel = 'sing-box' AND protocol = 'vless' AND transport = 'xhttp' AND feature = '*';

-- sing-box 不支持 XMUX（XHTTP 特性），降级为去除 XMUX
UPDATE kernel_capabilities
SET downgrade_to = '{"feature": "mux", "action": "drop"}'::jsonb,
    message = 'sing-box 不支持 XMUX，降级为去除 XMUX（P2-2）'
WHERE kernel = 'sing-box' AND transport = 'xhttp' AND feature = 'xmux';

-- sing-box 不支持 XHTTP split，降级为去除 split
UPDATE kernel_capabilities
SET downgrade_to = '{"feature": "split", "action": "drop"}'::jsonb,
    message = 'sing-box 不支持 XHTTP split，降级为去除 split（P2-2）'
WHERE kernel = 'sing-box' AND transport = 'xhttp' AND feature = 'split';

-- P2-2: CapabilityLost 事件审计表
-- 记录每次发布时发生的能力降级，用于可观测性和故障排查
CREATE TABLE IF NOT EXISTS capability_lost_events (
    id BIGSERIAL PRIMARY KEY,
    runtime_id UUID,
    node_id UUID,
    node_code VARCHAR(128),
    kernel VARCHAR(16) NOT NULL,           -- xray / sing-box
    protocol VARCHAR(32) NOT NULL,
    transport VARCHAR(32) NOT NULL,
    security VARCHAR(32) NOT NULL,
    feature VARCHAR(64) NOT NULL DEFAULT '*',
    original_support VARCHAR(16) NOT NULL, -- blocked / degradable
    degrade_strategy VARCHAR(16) NOT NULL, -- deny / downgrade / force_kernel
    downgrade_to JSONB,                    -- 实际应用的降级目标
    message TEXT,
    config_version_no VARCHAR(64),         -- 关联的 config_versions 版本号
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_caplost_runtime ON capability_lost_events(runtime_id);
CREATE INDEX idx_caplost_node ON capability_lost_events(node_id);
CREATE INDEX idx_caplost_created ON capability_lost_events(created_at DESC);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS capability_lost_events;

UPDATE kernel_capabilities
SET downgrade_to = NULL,
    message = 'sing-box 不支持 XHTTP'
WHERE kernel = 'sing-box' AND protocol = 'vless' AND transport = 'xhttp' AND feature = '*';

UPDATE kernel_capabilities
SET downgrade_to = NULL,
    message = 'sing-box 不支持 XMUX'
WHERE kernel = 'sing-box' AND transport = 'xhttp' AND feature = 'xmux';

UPDATE kernel_capabilities
SET downgrade_to = NULL,
    message = 'sing-box 不支持 XHTTP split'
WHERE kernel = 'sing-box' AND transport = 'xhttp' AND feature = 'split';
-- +goose StatementEnd
