-- +goose Up
-- 为 tls_profiles 添加 ECH 私钥列（服务端使用，对应 xray 的 echConfig.key）
-- ech_config_encrypted 已有，存储 ECH config PEM（公开，分发到客户端）
-- ech_key_encrypted 新增，存储 ECH key PEM（私有，仅服务端使用）
ALTER TABLE tls_profiles ADD COLUMN IF NOT EXISTS ech_key_encrypted TEXT;

-- +goose Down
ALTER TABLE tls_profiles DROP COLUMN IF EXISTS ech_key_encrypted;
