-- +goose Up
-- +goose StatementBegin

-- ============================================================
-- 000025: Phase 1 完整种子数据
-- 1. 更新默认套餐为用户要求的规格
-- 2. 设置管理员密码
-- 3. 补充扁平化系统设置
-- 4. 补充缺失的4种协议配置（达到18种）
-- 5. 确保viewer角色有所有.read权限
-- ============================================================

-- ============================================================
-- 1. 更新默认套餐
-- ============================================================

-- Free套餐: 10GB/30天
UPDATE plans SET
  name = 'Free',
  traffic_bytes = 10737418240,
  duration_days = 30,
  speed_limit_mbps = 10,
  device_limit = 2,
  can_renew = false,
  sort_order = 0,
  tags = ARRAY['free', 'new-user'],
  feature_flags = '{"auto_grant_on_verify": true}'::jsonb,
  updated_at = now()
WHERE code = 'free';

-- Basic套餐: 100GB/30天
UPDATE plans SET
  name = 'Basic',
  traffic_bytes = 107374182400,
  duration_days = 30,
  speed_limit_mbps = 50,
  device_limit = 5,
  can_renew = true,
  sort_order = 1,
  tags = ARRAY['basic', 'popular'],
  feature_flags = '{}'::jsonb,
  updated_at = now()
WHERE code = 'basic';

-- Pro套餐: 500GB/30天
UPDATE plans SET
  name = 'Pro',
  traffic_bytes = 536870912000,
  duration_days = 30,
  speed_limit_mbps = NULL,
  device_limit = 10,
  can_renew = true,
  sort_order = 2,
  tags = ARRAY['pro', 'premium'],
  feature_flags = '{"unlimited_speed": true}'::jsonb,
  updated_at = now()
WHERE code = 'pro';

-- 更新套餐定价
-- Basic: 月付$4.99, 季付$13.99, 半年付$25.99, 年付$47.99
DELETE FROM plan_prices WHERE plan_id = 'd0000000-0000-0000-0000-000000000002'::uuid;
INSERT INTO plan_prices (plan_id, period_code, currency_code, amount_minor, is_active, created_at) VALUES
('d0000000-0000-0000-0000-000000000002'::uuid, 'monthly',     'USDT-TRC20',   499, true, now()),
('d0000000-0000-0000-0000-000000000002'::uuid, 'quarterly',   'USDT-TRC20',  1399, true, now()),
('d0000000-0000-0000-0000-000000000002'::uuid, 'half_yearly', 'USDT-TRC20',  2599, true, now()),
('d0000000-0000-0000-0000-000000000002'::uuid, 'yearly',      'USDT-TRC20',  4799, true, now());

-- Pro: 月付$14.99, 季付$41.99, 半年付$77.99, 年付$143.99
DELETE FROM plan_prices WHERE plan_id = 'd0000000-0000-0000-0000-000000000003'::uuid;
INSERT INTO plan_prices (plan_id, period_code, currency_code, amount_minor, is_active, created_at) VALUES
('d0000000-0000-0000-0000-000000000003'::uuid, 'monthly',     'USDT-TRC20',  1499, true, now()),
('d0000000-0000-0000-0000-000000000003'::uuid, 'quarterly',   'USDT-TRC20',  4199, true, now()),
('d0000000-0000-0000-0000-000000000003'::uuid, 'half_yearly', 'USDT-TRC20',  7799, true, now()),
('d0000000-0000-0000-0000-000000000003'::uuid, 'yearly',      'USDT-TRC20', 14399, true, now());

-- ============================================================
-- 2. 设置管理员密码 (argon2id hash of 'admin123456')
-- ============================================================

UPDATE users SET
  password_hash = '$argon2id$v=19$m=65536,t=3,p=4$AQIDBAUGBwgJCgsMDQ4PEA$DW7r7BI3shrswj8TI6xH8UYN7LzSfoeQRFx8giiSRLM',
  password_algo = 'argon2id',
  status = 'active',
  email_verified_at = COALESCE(email_verified_at, now()),
  updated_at = now()
WHERE email = 'a***@yundu.local';

-- 确保admin的is_super_admin为true
UPDATE admins SET
  is_super_admin = true,
  status = 'active',
  updated_at = now()
WHERE user_id = 'a0000000-0000-0000-0000-000000000001'::uuid;

-- 幂等确保admin绑定super_admin角色
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
-- 3. 插入扁平化系统设置 (general组)
-- ============================================================

INSERT INTO system_settings (setting_group, setting_key, value_json, description, updated_at) VALUES
('general', 'site_name',             '"YunDu Airport"'::jsonb,                '站点名称', now()),
('general', 'smtp_enabled',          'false'::jsonb,                          'SMTP邮件开关', now()),
('general', 'trc20_enabled',         'false'::jsonb,                          'TRC20支付开关', now()),
('general', 'register_enabled',      'true'::jsonb,                           '开放注册', now()),
('general', 'email_verify_required', 'false'::jsonb,                          '注册需要邮箱验证', now()),
('general', 'free_plan_id',          '"d0000000-0000-0000-0000-000000000001"'::jsonb, '默认免费套餐ID', now())
ON CONFLICT (setting_group, setting_key) DO UPDATE
  SET value_json = EXCLUDED.value_json, description = EXCLUDED.description, updated_at = now();

-- 更新现有分组配置以保持一致性
-- 更新站点名称
INSERT INTO system_settings (setting_group, setting_key, value_json, description, updated_at)
VALUES (
  'site', 'config',
  jsonb_build_object(
    'name', 'YunDu Airport',
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

-- 更新SMTP配置 (enabled=false)
INSERT INTO system_settings (setting_group, setting_key, value_json, description, updated_at)
VALUES (
  'smtp', 'config',
  jsonb_build_object(
    'enabled', false,
    'host', '',
    'port', 587,
    'username', '',
    'password', '',
    'from_email', 'n***@yundu.local',
    'from_name', 'YunDu Airport',
    'encryption', 'tls'
  ),
  'SMTP邮件发送配置',
  now()
)
ON CONFLICT (setting_group, setting_key) DO UPDATE
  SET value_json = EXCLUDED.value_json, description = EXCLUDED.description, updated_at = now();

-- 更新TRC20配置 (enabled=false)
INSERT INTO system_settings (setting_group, setting_key, value_json, description, updated_at)
VALUES (
  'payment', 'trc20',
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

-- 更新注册配置 (enabled=true, require_email_verification=false)
INSERT INTO system_settings (setting_group, setting_key, value_json, description, updated_at)
VALUES (
  'register', 'config',
  jsonb_build_object(
    'enabled', true,
    'require_email_verification', false,
    'free_plan_id', 'd0000000-0000-0000-0000-000000000001'::uuid,
    'enable_invite_only', false,
    'default_traffic_reset_cycle', 'monthly'
  ),
  '用户注册配置',
  now()
)
ON CONFLICT (setting_group, setting_key) DO UPDATE
  SET value_json = EXCLUDED.value_json, description = EXCLUDED.description, updated_at = now();

-- ============================================================
-- 4. 补充缺失的4种协议组合（使总数达到18种）
-- ============================================================

-- P12: vless + tcp + tls
INSERT INTO protocol_registry (protocol_type, transport_type, security_type, schema_version, config_schema, description) VALUES
('vless', 'tcp', 'tls', 'v1', '{
  "type": "object",
  "properties": {
    "uuid": {"type": "string", "format": "uuid"},
    "flow": {"type": "string", "enum": ["", "xtls-rprx-vision"], "default": ""},
    "tcp_fast_open": {"type": "boolean", "default": false},
    "tcp_no_fingerprint": {"type": "boolean", "default": false},
    "tls": {
      "type": "object",
      "properties": {
        "server_name": {"type": "string"},
        "alpn": {"type": "array", "items": {"type": "string", "enum": ["h3", "h2", "http/1.1"]}, "default": ["h2", "http/1.1"]},
        "min_version": {"type": "string", "default": "1.3"},
        "fingerprint": {"type": "string", "enum": ["chrome", "firefox", "safari", "ios", "edge", "random"], "default": "chrome"},
        "allow_insecure": {"type": "boolean", "default": false},
        "certificate_mode": {"type": "string", "enum": ["acme", "upload", "self_signed"], "default": "acme"}
      },
      "required": ["server_name"]
    }
  },
  "required": ["uuid"]
}'::jsonb, 'VLESS + TCP + TLS，经典TLS直连方案')
ON CONFLICT (protocol_type, transport_type, security_type, schema_version) DO NOTHING;

-- P13: trojan + h2 + tls
INSERT INTO protocol_registry (protocol_type, transport_type, security_type, schema_version, config_schema, description) VALUES
('trojan', 'h2', 'tls', 'v1', '{
  "type": "object",
  "properties": {
    "password": {"type": "string"},
    "h2_path": {"type": "string", "default": "/"},
    "h2_host": {"type": "array", "items": {"type": "string"}, "default": []},
    "tls": {
      "type": "object",
      "properties": {
        "server_name": {"type": "string"},
        "alpn": {"type": "array", "items": {"type": "string"}, "default": ["h2", "http/1.1"]},
        "fingerprint": {"type": "string", "enum": ["chrome", "firefox", "safari", "ios", "edge", "random"], "default": "chrome"},
        "allow_insecure": {"type": "boolean", "default": false},
        "certificate_mode": {"type": "string", "enum": ["acme", "upload", "self_signed"], "default": "acme"}
      },
      "required": ["server_name"]
    }
  },
  "required": ["password"]
}'::jsonb, 'Trojan + HTTP/2 + TLS，多路复用传输')
ON CONFLICT (protocol_type, transport_type, security_type, schema_version) DO NOTHING;

-- P14: vmess + tcp + tls
INSERT INTO protocol_registry (protocol_type, transport_type, security_type, schema_version, config_schema, description) VALUES
('vmess', 'tcp', 'tls', 'v1', '{
  "type": "object",
  "properties": {
    "uuid": {"type": "string", "format": "uuid"},
    "alter_id": {"type": "integer", "default": 0},
    "tcp_fast_open": {"type": "boolean", "default": false},
    "tls": {
      "type": "object",
      "properties": {
        "server_name": {"type": "string"},
        "alpn": {"type": "array", "items": {"type": "string"}, "default": ["h2", "http/1.1"]},
        "fingerprint": {"type": "string", "enum": ["chrome", "firefox", "safari", "ios", "edge", "random"], "default": "chrome"},
        "allow_insecure": {"type": "boolean", "default": false},
        "certificate_mode": {"type": "string", "enum": ["acme", "upload", "self_signed"], "default": "acme"}
      },
      "required": ["server_name"]
    }
  },
  "required": ["uuid"]
}'::jsonb, 'VMess + TCP + TLS，兼容旧版客户端')
ON CONFLICT (protocol_type, transport_type, security_type, schema_version) DO NOTHING;

-- P15: vmess + h2 + tls
INSERT INTO protocol_registry (protocol_type, transport_type, security_type, schema_version, config_schema, description) VALUES
('vmess', 'h2', 'tls', 'v1', '{
  "type": "object",
  "properties": {
    "uuid": {"type": "string", "format": "uuid"},
    "alter_id": {"type": "integer", "default": 0},
    "h2_path": {"type": "string", "default": "/"},
    "h2_host": {"type": "array", "items": {"type": "string"}, "default": []},
    "tls": {
      "type": "object",
      "properties": {
        "server_name": {"type": "string"},
        "alpn": {"type": "array", "items": {"type": "string"}, "default": ["h2", "http/1.1"]},
        "fingerprint": {"type": "string", "enum": ["chrome", "firefox", "safari", "ios", "edge", "random"], "default": "chrome"},
        "allow_insecure": {"type": "boolean", "default": false},
        "certificate_mode": {"type": "string", "enum": ["acme", "upload", "self_signed"], "default": "acme"}
      },
      "required": ["server_name"]
    }
  },
  "required": ["uuid"]
}'::jsonb, 'VMess + HTTP/2 + TLS，兼容旧版客户端的多路复用方案')
ON CONFLICT (protocol_type, transport_type, security_type, schema_version) DO NOTHING;

-- ============================================================
-- 5. 确保viewer角色有所有.read权限（幂等补充）
-- ============================================================

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code LIKE '%.read'
WHERE r.code = 'viewer'
  AND NOT EXISTS (
    SELECT 1 FROM role_permissions rp
    WHERE rp.role_id = r.id AND rp.permission_id = p.id
  );

-- 同时确保super_admin拥有所有权限
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE r.code = 'super_admin'
  AND NOT EXISTS (
    SELECT 1 FROM role_permissions rp
    WHERE rp.role_id = r.id AND rp.permission_id = p.id
  );

-- 确保operator拥有运维需要的.read权限
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code IN (
  'tickets.read', 'announcements.read', 'notifications.read'
)
WHERE r.code = 'operator'
  AND NOT EXISTS (
    SELECT 1 FROM role_permissions rp
    WHERE rp.role_id = r.id AND rp.permission_id = p.id
  );

-- 确保support拥有客服需要的.read权限
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code IN (
  'audit.read', 'nodes.read', 'system.read'
)
WHERE r.code = 'support'
  AND NOT EXISTS (
    SELECT 1 FROM role_permissions rp
    WHERE rp.role_id = r.id AND rp.permission_id = p.id
  );

-- ============================================================
-- 验证：统计协议数量（确保18种）
-- ============================================================
-- 此查询仅用于验证，不影响数据：
-- SELECT COUNT(*) as protocol_count FROM protocol_registry;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- 删除general组设置
DELETE FROM system_settings WHERE setting_group = 'general';

-- 删除新增的4种协议
DELETE FROM protocol_registry WHERE (protocol_type, transport_type, security_type, schema_version) IN (
  ('vless', 'tcp', 'tls', 'v1'),
  ('trojan', 'h2', 'tls', 'v1'),
  ('vmess', 'tcp', 'tls', 'v1'),
  ('vmess', 'h2', 'tls', 'v1')
);

-- 恢复套餐原始数据（与000024一致）
UPDATE plans SET
  name = '免费体验',
  traffic_bytes = 1073741824,
  duration_days = 7,
  speed_limit_mbps = 1,
  device_limit = 2,
  can_renew = false,
  sort_order = 0,
  tags = ARRAY['free', 'new-user'],
  feature_flags = '{"auto_grant_on_verify": true}'::jsonb,
  updated_at = now()
WHERE code = 'free';

UPDATE plans SET
  name = '基础版',
  traffic_bytes = 1099511627776,
  duration_days = 30,
  speed_limit_mbps = 50,
  device_limit = 5,
  can_renew = true,
  sort_order = 1,
  tags = ARRAY['basic', 'popular'],
  feature_flags = '{}'::jsonb,
  updated_at = now()
WHERE code = 'basic';

UPDATE plans SET
  name = 'Pro专业版',
  traffic_bytes = 10995116277760,
  duration_days = 30,
  speed_limit_mbps = NULL,
  device_limit = 10,
  can_renew = true,
  sort_order = 2,
  tags = ARRAY['pro', 'premium'],
  feature_flags = '{"unlimited_speed": true}'::jsonb,
  updated_at = now()
WHERE code = 'pro';

-- 恢复套餐定价
DELETE FROM plan_prices WHERE plan_id IN (
  'd0000000-0000-0000-0000-000000000002'::uuid,
  'd0000000-0000-0000-0000-000000000003'::uuid
);
INSERT INTO plan_prices (plan_id, period_code, currency_code, amount_minor, is_active, created_at) VALUES
('d0000000-0000-0000-0000-000000000002'::uuid, 'monthly',     'USDT-TRC20',   999, true, now()),
('d0000000-0000-0000-0000-000000000002'::uuid, 'quarterly',   'USDT-TRC20',  2797, true, now()),
('d0000000-0000-0000-0000-000000000002'::uuid, 'half_yearly', 'USDT-TRC20',  5295, true, now()),
('d0000000-0000-0000-0000-000000000002'::uuid, 'yearly',      'USDT-TRC20',  9993, true, now()),
('d0000000-0000-0000-0000-000000000003'::uuid, 'monthly',     'USDT-TRC20',  2991, true, now()),
('d0000000-0000-0000-0000-000000000003'::uuid, 'quarterly',   'USDT-TRC20',  8289, true, now()),
('d0000000-0000-0000-0000-000000000003'::uuid, 'half_yearly', 'USDT-TRC20', 15987, true, now()),
('d0000000-0000-0000-0000-000000000003'::uuid, 'yearly',      'USDT-TRC20', 29985, true, now());

-- 清除管理员密码（恢复到000024状态）
UPDATE users SET
  password_hash = NULL,
  password_algo = 'argon2id',
  status = 'active',
  updated_at = now()
WHERE email = 'a***@yundu.local';

-- 恢复站点配置为000024状态
INSERT INTO system_settings (setting_group, setting_key, value_json, description, updated_at)
VALUES (
  'site', 'config',
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

INSERT INTO system_settings (setting_group, setting_key, value_json, description, updated_at)
VALUES (
  'smtp', 'config',
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

INSERT INTO system_settings (setting_group, setting_key, value_json, description, updated_at)
VALUES (
  'register', 'config',
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

-- +goose StatementEnd
