-- +goose Up
-- +goose StatementBegin

INSERT INTO roles (code, name, description)
VALUES ('super_admin', 'Super Admin', '全量权限角色')
ON CONFLICT (code) DO NOTHING;

INSERT INTO permissions (code, name, resource, action) VALUES
('users.read',        '读取用户',        'users',       'read'),
('users.write',       '写入用户',        'users',       'write'),
('admins.read',       '读取管理员',      'admins',      'read'),
('admins.write',      '写入管理员',      'admins',      'write'),
('system.write',      '写入系统配置',    'system',      'write'),
('nodes.read',        '读取节点',        'nodes',       'read'),
('nodes.write',       '写入节点',        'nodes',       'write'),
('deployments.write', '执行节点发布',    'deployments', 'write'),
('plans.read',        '读取套餐',        'plans',       'read'),
('plans.write',       '写入套餐',        'plans',       'write'),
('audit.read',        '读取审计日志',    'audit',       'read')
ON CONFLICT (code) DO NOTHING;

INSERT INTO health_profiles (
  code, name,
  probe_interval_seconds, timeout_seconds,
  failure_threshold, recovery_threshold,
  probe_targets
) VALUES (
  'default-30s', '默认30秒探测',
  30, 10, 3, 2,
  '[{"type":"tcp","description":"TCP握手探测"}]'::jsonb
) ON CONFLICT (code) DO NOTHING;

INSERT INTO node_groups (code, name, description, visibility, sort_order)
VALUES
  ('global',   '全球节点',   '默认全球节点分组', 'public',  0),
  ('premium',  '高级节点',   '高级线路节点',     'public',  1),
  ('relay',    '中转节点',   '中转/落地分组',    'private', 2)
ON CONFLICT (code) DO NOTHING;

INSERT INTO subscription_templates (code, name, target_client, template_type, schema_version, content, status)
VALUES
  ('clash-meta', 'Clash Meta', 'clash-meta', 'yaml', 'v1',
   '# Clash Meta 订阅模板 v1
# 由订阅引擎动态渲染
proxies: []
proxy-groups: []
rules: []',
   'active'),
  ('sing-box', 'Sing-box', 'sing-box', 'json', 'v1',
   '{"log":{},"dns":{},"inbounds":[],"outbounds":[],"route":{}}',
   'active'),
  ('universal-uri', '通用 URI', 'universal', 'text', 'v1',
   '# 通用 URI 列表模板
# 每行一个节点 URI',
   'active')
ON CONFLICT (code) DO NOTHING;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM subscription_templates WHERE code IN ('clash-meta','sing-box','universal-uri');
DELETE FROM node_groups WHERE code IN ('global','premium','relay');
DELETE FROM health_profiles WHERE code = 'default-30s';
DELETE FROM permissions WHERE code IN (
  'users.read','users.write','admins.read','admins.write',
  'system.write','nodes.read','nodes.write','deployments.write',
  'plans.read','plans.write','audit.read'
);
DELETE FROM roles WHERE code = 'super_admin';
-- +goose StatementEnd
