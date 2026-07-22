-- +goose Up
-- +goose StatementBegin

-- P3-1: 加密 Payload Manifest 存储表
-- 控制面在创建 ConfigVersion 时，同步构建 AES-GCM 加密的 PayloadManifest
-- 并写入此表。Agent 通过 HTTP 拉取加密 Payload，使用共享密钥本地解密，
-- 实现 TLS 证书等敏感材料的端到端加密下发。
CREATE TABLE IF NOT EXISTS config_payloads (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    config_version_id UUID NOT NULL REFERENCES config_versions(id) ON DELETE CASCADE,
    version_no BIGINT NOT NULL,
    sha256 VARCHAR(64) NOT NULL,
    kernel VARCHAR(20) NOT NULL DEFAULT 'xray',
    rollback_strategy VARCHAR(20) NOT NULL DEFAULT 'lkg',
    payload_encrypted BOOLEAN NOT NULL DEFAULT true,
    content JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_config_payloads_version ON config_payloads(version_no);
CREATE INDEX IF NOT EXISTS idx_config_payloads_config_version ON config_payloads(config_version_id);

-- P3-1: 部署结果上报表
-- Agent 在 precheck / activate / healthcheck 各阶段完成后通过 HTTP 上报 ACK/NACK。
-- 控制面据此推进部署 phase 或触发回滚，替代原有依赖心跳的隐式结果上报。
CREATE TABLE IF NOT EXISTS deployment_results (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_target_id UUID REFERENCES deployment_targets(id) ON DELETE SET NULL,
    server_code VARCHAR(100) NOT NULL,
    version_no BIGINT NOT NULL,
    status VARCHAR(10) NOT NULL DEFAULT 'ack',
    phase VARCHAR(20) NOT NULL DEFAULT 'activate',
    error TEXT DEFAULT '',
    apply_duration_ms BIGINT DEFAULT 0,
    reported_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_deployment_results_server ON deployment_results(server_code);
CREATE INDEX IF NOT EXISTS idx_deployment_results_version ON deployment_results(version_no);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS deployment_results;
DROP TABLE IF EXISTS config_payloads;
-- +goose StatementEnd
