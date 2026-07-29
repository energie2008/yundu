-- +goose Up
-- +goose StatementBegin

-- WARP 账户注册元数据扩展
-- 支持 warpreg 模块（移植 3X-UI）自动注册 WARP 账户并集中管理。
-- 核心约束：一 VPS N 账户（WireGuard public key 唯一性），账户池 + load_balance 负载均衡。

-- Cloudflare WARP API 返回的账户元数据
ALTER TABLE warp_profiles ADD COLUMN IF NOT EXISTS device_id TEXT;
-- Cloudflare 返回的 device_id（注册响应的 id 字段），用于查询/删除/轮换

ALTER TABLE warp_profiles ADD COLUMN IF NOT EXISTS access_token TEXT;
-- Cloudflare 返回的 access_token（注册响应的 token 字段），Bearer 认证用

ALTER TABLE warp_profiles ADD COLUMN IF NOT EXISTS client_id TEXT;
-- Cloudflare 返回的 config.client_id（base64 字符串）
-- xray wireguard outbound 需要 reserved 字段（解码为 3 字节 int 数组）
-- sing-box wireguard outbound 不需要 reserved，但保留用于 xray 兼容

-- 双栈地址拆分（便于查询和渲染）
ALTER TABLE warp_profiles ADD COLUMN IF NOT EXISTS ipv4_address TEXT;
-- 从 local_address 拆分的 IPv4 地址，如 172.16.0.2/32

ALTER TABLE warp_profiles ADD COLUMN IF NOT EXISTS ipv6_address TEXT;
-- 从 local_address 拆分的 IPv6 地址，如 2606:4700:xxx/128

-- 账户状态管理
ALTER TABLE warp_profiles ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'active';
-- status: active / banned / rate_limited / expired / unassigned
--   active        - 正常使用中
--   banned        - 被 Cloudflare 封禁（健康检查 403）
--   rate_limited  - 注册/查询被限流（429）
--   expired       - 账户过期
--   unassigned    - 预注册但未绑定到 VPS

-- VPS 绑定关系（一账户一 VPS 约束）
ALTER TABLE warp_profiles ADD COLUMN IF NOT EXISTS node_id UUID REFERENCES servers(id);
-- 绑定的节点 ID，NULL 表示未绑定（预注册账户池）

ALTER TABLE warp_profiles ADD COLUMN IF NOT EXISTS outbound_tag TEXT;
-- 在 sing-box/xray config 中的 outbound tag，如 warp-1 / warp-2 / warp-3
-- 用于 load_balance outbound 引用

-- 运维时间戳
ALTER TABLE warp_profiles ADD COLUMN IF NOT EXISTS last_rotated_at TIMESTAMPTZ;
-- 最后 IP 轮换时间（warpreg.RotateIP 更新）

ALTER TABLE warp_profiles ADD COLUMN IF NOT EXISTS last_health_check_at TIMESTAMPTZ;
-- 最后健康检查时间（warpreg.HealthCheck 更新）

-- 索引：按 node_id 查询某 VPS 的所有 WARP 账户
CREATE INDEX IF NOT EXISTS idx_warp_profiles_node_id ON warp_profiles(node_id);

-- 索引：按 status 查询活跃/未绑定账户
CREATE INDEX IF NOT EXISTS idx_warp_profiles_status ON warp_profiles(status);

-- 索引：按 outbound_tag 查询（渲染时快速定位）
CREATE INDEX IF NOT EXISTS idx_warp_profiles_outbound_tag ON warp_profiles(outbound_tag) WHERE outbound_tag IS NOT NULL;

-- 从现有 local_address 拆分 ipv4_address 和 ipv6_address（数据回填）
UPDATE warp_profiles SET
    ipv4_address = split_part(local_address, ',', 1),
    ipv6_address = CASE
        WHEN local_address LIKE '%,%' THEN trim(split_part(local_address, ',', 2))
        ELSE NULL
    END
WHERE local_address IS NOT NULL
  AND local_address != '';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE warp_profiles DROP COLUMN IF EXISTS device_id;
ALTER TABLE warp_profiles DROP COLUMN IF EXISTS access_token;
ALTER TABLE warp_profiles DROP COLUMN IF EXISTS client_id;
ALTER TABLE warp_profiles DROP COLUMN IF EXISTS ipv4_address;
ALTER TABLE warp_profiles DROP COLUMN IF EXISTS ipv6_address;
ALTER TABLE warp_profiles DROP COLUMN IF EXISTS status;
ALTER TABLE warp_profiles DROP COLUMN IF EXISTS node_id;
ALTER TABLE warp_profiles DROP COLUMN IF EXISTS outbound_tag;
ALTER TABLE warp_profiles DROP COLUMN IF EXISTS last_rotated_at;
ALTER TABLE warp_profiles DROP COLUMN IF EXISTS last_health_check_at;
DROP INDEX IF EXISTS idx_warp_profiles_node_id;
DROP INDEX IF EXISTS idx_warp_profiles_status;
DROP INDEX IF EXISTS idx_warp_profiles_outbound_tag;
-- +goose StatementEnd
