-- +goose Up
-- +goose StatementBegin

-- ============================================================
-- 1. 默认管理员账号
-- ============================================================

-- 插入admin用户（password_hash为空，需要通过CLI seed命令设置初始密码）
INSERT INTO users (id, email, status, email_verified_at, locale, timezone, created_at, updated_at)
VALUES (
  'a0000000-0000-0000-0000-000000000001'::uuid,
  'a***@yundu.local',
  'active',
  now(),
  'zh-CN',
  'Asia/Shanghai',
  now(),
  now()
)
ON CONFLICT (email) DO NOTHING;

-- 插入admins表记录（超级管理员）
INSERT INTO admins (id, user_id, display_name, status, is_super_admin, created_at, updated_at)
VALUES (
  'a0000000-0000-0000-0000-000000000002'::uuid,
  'a0000000-0000-0000-0000-000000000001'::uuid,
  '系统管理员',
  'active',
  true,
  now(),
  now()
)
ON CONFLICT (user_id) DO NOTHING;

-- 确保super_admin角色存在（000006已插入，这里只是幂等保障）
INSERT INTO roles (code, name, description)
VALUES ('super_admin', 'Super Admin', '全量权限角色')
ON CONFLICT (code) DO NOTHING;

-- 绑定admin到super_admin角色
INSERT INTO admin_roles (admin_id, role_id, created_at)
SELECT
  'a0000000-0000-0000-0000-000000000002'::uuid,
  r.id,
  now()
FROM roles r
WHERE r.code = 'super_admin'
  AND NOT EXISTS (
    SELECT 1 FROM admin_roles ar
    WHERE ar.admin_id = 'a0000000-0000-0000-0000-000000000002'::uuid
      AND ar.role_id = r.id
  );

-- ============================================================
-- 2. 默认套餐数据
-- ============================================================

-- 免费体验套餐
INSERT INTO plans (
  id, code, name, status, billing_type,
  traffic_bytes, speed_limit_mbps, device_limit,
  duration_days, can_renew, sort_order, tags, feature_flags,
  created_at, updated_at
) VALUES (
  'd0000000-0000-0000-0000-000000000001'::uuid,
  'free',
  '免费体验',
  'active',
  'one_time',
  1073741824,
  1,
  2,
  7,
  false,
  0,
  ARRAY['free', 'new-user'],
  '{"auto_grant_on_verify": true}'::jsonb,
  now(),
  now()
)
ON CONFLICT (code) DO NOTHING;

-- 基础版套餐
INSERT INTO plans (
  id, code, name, status, billing_type,
  traffic_bytes, speed_limit_mbps, device_limit,
  duration_days, can_renew, sort_order, tags, feature_flags,
  created_at, updated_at
) VALUES (
  'd0000000-0000-0000-0000-000000000002'::uuid,
  'basic',
  '基础版',
  'active',
  'periodic',
  1099511627776,
  50,
  5,
  30,
  true,
  1,
  ARRAY['basic', 'popular'],
  '{}'::jsonb,
  now(),
  now()
)
ON CONFLICT (code) DO NOTHING;

-- Pro版套餐
INSERT INTO plans (
  id, code, name, status, billing_type,
  traffic_bytes, speed_limit_mbps, device_limit,
  duration_days, can_renew, sort_order, tags, feature_flags,
  created_at, updated_at
) VALUES (
  'd0000000-0000-0000-0000-000000000003'::uuid,
  'pro',
  'Pro专业版',
  'active',
  'periodic',
  10995116277760,
  NULL,
  10,
  30,
  true,
  2,
  ARRAY['pro', 'premium'],
  '{"unlimited_speed": true}'::jsonb,
  now(),
  now()
)
ON CONFLICT (code) DO NOTHING;

-- ============================================================
-- 3. 套餐定价（USDT-TRC20，4档×4周期）
-- ============================================================

-- 基础版定价
INSERT INTO plan_prices (plan_id, period_code, currency_code, amount_minor, is_active, created_at) VALUES
('d0000000-0000-0000-0000-000000000002'::uuid, 'monthly',     'USDT-TRC20',   999, true, now()),
('d0000000-0000-0000-0000-000000000002'::uuid, 'quarterly',   'USDT-TRC20',  2797, true, now()),
('d0000000-0000-0000-0000-000000000002'::uuid, 'half_yearly', 'USDT-TRC20',  5295, true, now()),
('d0000000-0000-0000-0000-000000000002'::uuid, 'yearly',      'USDT-TRC20',  9993, true, now())
ON CONFLICT (plan_id, period_code, currency_code) DO UPDATE
  SET amount_minor = EXCLUDED.amount_minor, is_active = EXCLUDED.is_active;

-- Pro版定价
INSERT INTO plan_prices (plan_id, period_code, currency_code, amount_minor, is_active, created_at) VALUES
('d0000000-0000-0000-0000-000000000003'::uuid, 'monthly',     'USDT-TRC20',  2991, true, now()),
('d0000000-0000-0000-0000-000000000003'::uuid, 'quarterly',   'USDT-TRC20',  8289, true, now()),
('d0000000-0000-0000-0000-000000000003'::uuid, 'half_yearly', 'USDT-TRC20', 15987, true, now()),
('d0000000-0000-0000-0000-000000000003'::uuid, 'yearly',      'USDT-TRC20', 29985, true, now())
ON CONFLICT (plan_id, period_code, currency_code) DO UPDATE
  SET amount_minor = EXCLUDED.amount_minor, is_active = EXCLUDED.is_active;

-- ============================================================
-- 4. 更新protocol_presets推荐标记（000022已插入数据，这里更新推荐状态）
-- ============================================================

-- 更新已有presets的推荐状态和端口
UPDATE protocol_presets SET
  is_recommended = true,
  sort_order = CASE code
    WHEN 'xhttp-reality' THEN 1
    WHEN 'tcp-reality' THEN 2
    WHEN 'ws-tls-cdn' THEN 3
    WHEN 'grpc-tls' THEN 4
    WHEN 'hysteria2-udp' THEN 5
    WHEN 'tuic-quic' THEN 6
    WHEN 'trojan-tcp' THEN 7
    WHEN 'ss-2022' THEN 8
    WHEN 'vmess-ws-cdn' THEN 9
    ELSE sort_order
  END,
  recommended_port = CASE code
    WHEN 'ss-2022' THEN 30080
    ELSE recommended_port
  END,
  updated_at = now()
WHERE code IN (
  'xhttp-reality',
  'tcp-reality',
  'ws-tls-cdn',
  'grpc-tls',
  'hysteria2-udp',
  'tuic-quic',
  'trojan-tcp',
  'ss-2022',
  'vmess-ws-cdn'
);

-- 更新presets名称，增加推荐标记emoji
UPDATE protocol_presets SET
  name = CASE code
    WHEN 'xhttp-reality' THEN '⭐ XHTTP + REALITY（主推）'
    WHEN 'tcp-reality'   THEN '🟢 REALITY直连'
    WHEN 'ws-tls-cdn'    THEN '🟢 CDN中转WS'
    WHEN 'grpc-tls'      THEN '🟢 gRPC多复用'
    WHEN 'hysteria2-udp' THEN '🟢 UDP高速Hysteria2'
    WHEN 'tuic-quic'     THEN '🟢 TUICv5 QUIC'
    WHEN 'trojan-tcp'    THEN '🟢 Trojan+TLS'
    WHEN 'ss-2022'       THEN '🟢 Shadowsocks 2022'
    WHEN 'vmess-ws-cdn'  THEN '🟠 VMess+WS（兼容）'
    ELSE name
  END,
  updated_at = now()
WHERE code IN (
  'xhttp-reality',
  'tcp-reality',
  'ws-tls-cdn',
  'grpc-tls',
  'hysteria2-udp',
  'tuic-quic',
  'trojan-tcp',
  'ss-2022',
  'vmess-ws-cdn'
);

-- ============================================================
-- 5. 系统设置补充
-- ============================================================

-- SMTP邮件配置
INSERT INTO system_settings (setting_group, setting_key, value_json, description, updated_at)
VALUES (
  'smtp',
  'config',
  jsonb_build_object(
    'enabled', false,
    'host', '',
    'port', 587,
    'username', '',
    'password', '',
    'from_email', 'n***@yundu.local',
    'from_name', '云渡Yundu',
    'encryption', 'tls'
  ),
  'SMTP邮件发送配置',
  now()
)
ON CONFLICT (setting_group, setting_key) DO UPDATE
  SET value_json = EXCLUDED.value_json, description = EXCLUDED.description, updated_at = now();

-- TRC20支付配置（确保enabled=false）
INSERT INTO system_settings (setting_group, setting_key, value_json, description, updated_at)
VALUES (
  'payment',
  'trc20',
  jsonb_build_object(
    'enabled', false,
    'address', '',
    'usdt_contract', 'TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t',
    'trongrid_api', 'https://api.trongrid.io',
    'min_confirmations', 1,
    'poll_interval_seconds', 15,
    'amount_tolerance', '0.01',
    'order_expiry_hours', 24,
    'auto_activate', true
  ),
  'TRC20-USDT支付配置',
  now()
)
ON CONFLICT (setting_group, setting_key) DO UPDATE
  SET value_json = EXCLUDED.value_json, description = EXCLUDED.description, updated_at = now();

-- 注册设置
INSERT INTO system_settings (setting_group, setting_key, value_json, description, updated_at)
VALUES (
  'register',
  'config',
  jsonb_build_object(
    'enabled', true,
    'require_email_verification', true,
    'free_plan_id', 'd0000000-0000-0000-0000-000000000001'::uuid,
    'enable_invite_only', false,
    'default_traffic_reset_cycle', 'monthly'
  ),
  '用户注册配置',
  now()
)
ON CONFLICT (setting_group, setting_key) DO UPDATE
  SET value_json = EXCLUDED.value_json, description = EXCLUDED.description, updated_at = now();

-- 用户封禁策略
INSERT INTO system_settings (setting_group, setting_key, value_json, description, updated_at)
VALUES (
  'user',
  'ban_strategy',
  jsonb_build_object(
    'strategy', 'empty_subscription',
    'return_empty_subscription_instead_of_403', true
  ),
  '用户封禁策略 - 返回空订阅而非403',
  now()
)
ON CONFLICT (setting_group, setting_key) DO UPDATE
  SET value_json = EXCLUDED.value_json, description = EXCLUDED.description, updated_at = now();

-- 订阅设置
INSERT INTO system_settings (setting_group, setting_key, value_json, description, updated_at)
VALUES (
  'subscription',
  'config',
  jsonb_build_object(
    'cache_ttl_seconds', 60,
    'default_client', 'uri',
    'token_length', 32,
    'enable_subscription_log', false
  ),
  '订阅服务配置',
  now()
)
ON CONFLICT (setting_group, setting_key) DO UPDATE
  SET value_json = EXCLUDED.value_json, description = EXCLUDED.description, updated_at = now();

-- 站点信息
INSERT INTO system_settings (setting_group, setting_key, value_json, description, updated_at)
VALUES (
  'site',
  'config',
  jsonb_build_object(
    'name', '云渡Yundu',
    'base_url', 'http://localhost:8080',
    'user_web_url', 'http://localhost:5174',
    'admin_web_url', 'http://localhost:5173',
    'logo_url', '',
    'contact_email', 's***@yundu.local'
  ),
  '站点基础信息配置',
  now()
)
ON CONFLICT (setting_group, setting_key) DO UPDATE
  SET value_json = EXCLUDED.value_json, description = EXCLUDED.description, updated_at = now();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- 删除站点信息
DELETE FROM system_settings WHERE setting_group = 'site' AND setting_key = 'config';

-- 删除订阅设置
DELETE FROM system_settings WHERE setting_group = 'subscription' AND setting_key = 'config';

-- 删除封禁策略
DELETE FROM system_settings WHERE setting_group = 'user' AND setting_key = 'ban_strategy';

-- 删除注册设置
DELETE FROM system_settings WHERE setting_group = 'register' AND setting_key = 'config';

-- 删除SMTP配置
DELETE FROM system_settings WHERE setting_group = 'smtp' AND setting_key = 'config';

-- 删除basic和pro套餐定价
DELETE FROM plan_prices WHERE plan_id IN (
  'd0000000-0000-0000-0000-000000000002'::uuid,
  'd0000000-0000-0000-0000-000000000003'::uuid
) AND currency_code = 'USDT-TRC20';

-- 删除套餐
DELETE FROM plans WHERE id IN (
  'd0000000-0000-0000-0000-000000000001'::uuid,
  'd0000000-0000-0000-0000-000000000002'::uuid,
  'd0000000-0000-0000-0000-000000000003'::uuid
);

-- 删除admin角色关联
DELETE FROM admin_roles WHERE admin_id = 'a0000000-0000-0000-0000-000000000002'::uuid;

-- 删除admin记录
DELETE FROM admins WHERE id = 'a0000000-0000-0000-0000-000000000002'::uuid;

-- 删除admin用户
DELETE FROM users WHERE id = 'a0000000-0000-0000-0000-000000000001'::uuid;

-- 恢复protocol_presets为000022原始状态（推荐标记）
UPDATE protocol_presets SET
  is_recommended = CASE code
    WHEN 'xhttp-reality' THEN true
    WHEN 'hysteria2-udp' THEN true
    ELSE false
  END,
  sort_order = CASE code
    WHEN 'xhttp-reality' THEN 1
    WHEN 'tcp-reality' THEN 2
    WHEN 'ws-tls-cdn' THEN 3
    WHEN 'grpc-tls' THEN 4
    WHEN 'hysteria2-udp' THEN 5
    WHEN 'tuic-quic' THEN 6
    WHEN 'trojan-tcp' THEN 7
    WHEN 'trojan-ws-cdn' THEN 8
    WHEN 'ss-2022' THEN 9
    WHEN 'vmess-ws-cdn' THEN 10
    ELSE sort_order
  END,
  recommended_port = CASE code
    WHEN 'ss-2022' THEN 0
    ELSE recommended_port
  END,
  name = CASE code
    WHEN 'xhttp-reality' THEN '⭐ XHTTP + REALITY（主推）'
    WHEN 'tcp-reality'   THEN 'REALITY (TCP直连)'
    WHEN 'ws-tls-cdn'    THEN 'CDN中转 (WS+TLS)'
    WHEN 'grpc-tls'      THEN 'gRPC + TLS'
    WHEN 'hysteria2-udp' THEN '🚀 Hysteria2 (UDP高速)'
    WHEN 'tuic-quic'     THEN 'TUICv5 (QUIC)'
    WHEN 'trojan-tcp'    THEN 'Trojan (TCP+TLS)'
    WHEN 'trojan-ws-cdn' THEN 'Trojan + WS (CDN)'
    WHEN 'ss-2022'       THEN 'Shadowsocks 2022'
    WHEN 'vmess-ws-cdn'  THEN 'VMess + WS (兼容)'
    ELSE name
  END,
  updated_at = now()
WHERE code IN (
  'xhttp-reality',
  'tcp-reality',
  'ws-tls-cdn',
  'grpc-tls',
  'hysteria2-udp',
  'tuic-quic',
  'trojan-tcp',
  'trojan-ws-cdn',
  'ss-2022',
  'vmess-ws-cdn'
);

-- +goose StatementEnd
