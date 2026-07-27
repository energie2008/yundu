-- +goose Up
-- +goose StatementBegin

-- P3-B: warp_profile 字段标准化
-- 新增 wireguard 原生字段，支持 sing-box 原生 wireguard outbound（替代 socks5 兜底）。
-- 这些字段可从 ConfigJSON 中查询，标准化为列便于索引和查询。

ALTER TABLE warp_profiles ADD COLUMN IF NOT EXISTS private_key TEXT;
ALTER TABLE warp_profiles ADD COLUMN IF NOT EXISTS public_key TEXT;
ALTER TABLE warp_profiles ADD COLUMN IF NOT EXISTS local_address TEXT;
ALTER TABLE warp_profiles ADD COLUMN IF NOT EXISTS mtu INTEGER DEFAULT 1280;

-- 从 ConfigJSON 中提取已有数据到独立列（如果存在）
UPDATE warp_profiles SET
    private_key = COALESCE(config_json->>'private_key', private_key),
    public_key = COALESCE(config_json->>'public_key', public_key),
    local_address = COALESCE(config_json->>'local_address', local_address),
    mtu = COALESCE((config_json->>'mtu')::integer, mtu)
WHERE deleted_at IS NULL
  AND config_json IS NOT NULL
  AND (
    config_json ? 'private_key' OR
    config_json ? 'public_key' OR
    config_json ? 'local_address' OR
    config_json ? 'mtu'
  );

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE warp_profiles DROP COLUMN IF EXISTS private_key;
ALTER TABLE warp_profiles DROP COLUMN IF EXISTS public_key;
ALTER TABLE warp_profiles DROP COLUMN IF EXISTS local_address;
ALTER TABLE warp_profiles DROP COLUMN IF EXISTS mtu;
-- +goose StatementEnd
