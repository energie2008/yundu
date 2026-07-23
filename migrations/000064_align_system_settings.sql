-- 000064: 对齐系统设置 key 名称与分组
-- 问题：
--   1. 前端 Settings.tsx 使用 app_url / try_plan_plan_id 等 key
--   2. 后端读取 frontend_url / default_free_plan_id
--   3. 种子数据使用 site_name / free_plan_id
--   4. GetSettings API 返回扁平数组，前端期望分组结构
-- 本迁移统一所有 key 名称和分组，并清理废弃 key

BEGIN;

-- ============================================================
-- 1. 迁移旧 key → 新 key（仅当新 key 不存在时）
-- ============================================================

-- general/site_name → general/app_name
INSERT INTO system_settings (setting_group, setting_key, value_json, is_secret, description)
SELECT 'general', 'app_name', ss.value_json, ss.is_secret, '站点名称'
FROM system_settings ss
WHERE ss.setting_group = 'general' AND ss.setting_key = 'site_name'
  AND NOT EXISTS (
    SELECT 1 FROM system_settings WHERE setting_group = 'general' AND setting_key = 'app_name'
  );

-- general/free_plan_id → billing/default_free_plan_id
INSERT INTO system_settings (setting_group, setting_key, value_json, is_secret, description)
SELECT 'billing', 'default_free_plan_id', ss.value_json, false, '试用套餐ID（UUID），新用户注册时自动分配'
FROM system_settings ss
WHERE ss.setting_group = 'general' AND ss.setting_key = 'free_plan_id'
  AND NOT EXISTS (
    SELECT 1 FROM system_settings WHERE setting_group = 'billing' AND setting_key = 'default_free_plan_id'
  );

-- ============================================================
-- 2. 清理废弃 key（已迁移或不再使用）
-- ============================================================

DELETE FROM system_settings WHERE setting_group = 'general' AND setting_key = 'site_name';
DELETE FROM system_settings WHERE setting_group = 'general' AND setting_key = 'free_plan_id';
-- register 组已有 email_verify_required（迁移 000063），删除 general 组的旧版本
DELETE FROM system_settings WHERE setting_group = 'general' AND setting_key = 'email_verify_required';

-- ============================================================
-- 3. 种子所有系统设置（仅当不存在时插入，不覆盖已有值）
-- ============================================================

-- ===== general 组 =====
INSERT INTO system_settings (setting_group, setting_key, value_json, is_secret, description)
SELECT 'general', 'app_name', '"YunDu Airport"'::jsonb, false, '站点名称'
WHERE NOT EXISTS (SELECT 1 FROM system_settings WHERE setting_group = 'general' AND setting_key = 'app_name');

INSERT INTO system_settings (setting_group, setting_key, value_json, is_secret, description)
SELECT 'general', 'app_description', '""'::jsonb, false, '站点描述'
WHERE NOT EXISTS (SELECT 1 FROM system_settings WHERE setting_group = 'general' AND setting_key = 'app_description');

INSERT INTO system_settings (setting_group, setting_key, value_json, is_secret, description)
SELECT 'general', 'frontend_url', '"https://example.com"'::jsonb, false, '站点地址（用于邮件模板中的链接，应为 user-web 的完整 URL）'
WHERE NOT EXISTS (SELECT 1 FROM system_settings WHERE setting_group = 'general' AND setting_key = 'frontend_url');

INSERT INTO system_settings (setting_group, setting_key, value_json, is_secret, description)
SELECT 'general', 'subscribe_url', '""'::jsonb, false, '订阅地址（如 https://sub.example.com，留空则使用站点地址）'
WHERE NOT EXISTS (SELECT 1 FROM system_settings WHERE setting_group = 'general' AND setting_key = 'subscribe_url');

INSERT INTO system_settings (setting_group, setting_key, value_json, is_secret, description)
SELECT 'general', 'is_https', 'true'::jsonb, false, '启用 HTTPS（强制使用 HTTPS 协议）'
WHERE NOT EXISTS (SELECT 1 FROM system_settings WHERE setting_group = 'general' AND setting_key = 'is_https');

-- ===== billing 组 =====
INSERT INTO system_settings (setting_group, setting_key, value_json, is_secret, description)
SELECT 'billing', 'default_free_plan_id', '""'::jsonb, false, '试用套餐ID（UUID），新用户注册时自动分配，留空则不分配'
WHERE NOT EXISTS (SELECT 1 FROM system_settings WHERE setting_group = 'billing' AND setting_key = 'default_free_plan_id');

INSERT INTO system_settings (setting_group, setting_key, value_json, is_secret, description)
SELECT 'billing', 'try_plan_time', '0'::jsonb, false, '试用时长（小时），0 表示不过期'
WHERE NOT EXISTS (SELECT 1 FROM system_settings WHERE setting_group = 'billing' AND setting_key = 'try_plan_time');

INSERT INTO system_settings (setting_group, setting_key, value_json, is_secret, description)
SELECT 'billing', 'try_plan_reset_traffic', 'false'::jsonb, false, '试用到期后重置用户流量'
WHERE NOT EXISTS (SELECT 1 FROM system_settings WHERE setting_group = 'billing' AND setting_key = 'try_plan_reset_traffic');

-- ===== subscribe 组 =====
INSERT INTO system_settings (setting_group, setting_key, value_json, is_secret, description)
SELECT 'subscribe', 'subscribe_path', '"/api/v1/client/subscribe"'::jsonb, false, '订阅路径'
WHERE NOT EXISTS (SELECT 1 FROM system_settings WHERE setting_group = 'subscribe' AND setting_key = 'subscribe_path');

INSERT INTO system_settings (setting_group, setting_key, value_json, is_secret, description)
SELECT 'subscribe', 'subscribe_domain', '""'::jsonb, false, '订阅域名（留空则使用请求 Host）'
WHERE NOT EXISTS (SELECT 1 FROM system_settings WHERE setting_group = 'subscribe' AND setting_key = 'subscribe_domain');

INSERT INTO system_settings (setting_group, setting_key, value_json, is_secret, description)
SELECT 'subscribe', 'subscribe_key', '""'::jsonb, false, '订阅密钥（用于生成订阅链接的加密密钥）'
WHERE NOT EXISTS (SELECT 1 FROM system_settings WHERE setting_group = 'subscribe' AND setting_key = 'subscribe_key');

INSERT INTO system_settings (setting_group, setting_key, value_json, is_secret, description)
SELECT 'subscribe', 'show_info_to_server', 'true'::jsonb, false, '在订阅中显示节点信息给客户端'
WHERE NOT EXISTS (SELECT 1 FROM system_settings WHERE setting_group = 'subscribe' AND setting_key = 'show_info_to_server');

INSERT INTO system_settings (setting_group, setting_key, value_json, is_secret, description)
SELECT 'subscribe', 'show_method', '1'::jsonb, false, '订阅显示方式：0=仅显示节点，1=显示节点+剩余流量，2=显示节点+到期时间'
WHERE NOT EXISTS (SELECT 1 FROM system_settings WHERE setting_group = 'subscribe' AND setting_key = 'show_method');

INSERT INTO system_settings (setting_group, setting_key, value_json, is_secret, description)
SELECT 'subscribe', 'is_rand_sub', 'false'::jsonb, false, '随机订阅（随机返回订阅节点）'
WHERE NOT EXISTS (SELECT 1 FROM system_settings WHERE setting_group = 'subscribe' AND setting_key = 'is_rand_sub');

INSERT INTO system_settings (setting_group, setting_key, value_json, is_secret, description)
SELECT 'subscribe', 'rand_sub_start', '0'::jsonb, false, '随机起始位置'
WHERE NOT EXISTS (SELECT 1 FROM system_settings WHERE setting_group = 'subscribe' AND setting_key = 'rand_sub_start');

INSERT INTO system_settings (setting_group, setting_key, value_json, is_secret, description)
SELECT 'subscribe', 'rand_sub_end', '10'::jsonb, false, '随机结束位置'
WHERE NOT EXISTS (SELECT 1 FROM system_settings WHERE setting_group = 'subscribe' AND setting_key = 'rand_sub_end');

COMMIT;
