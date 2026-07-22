-- +goose Up
-- +goose StatementBegin

-- 1. 新增标准角色（super_admin 已在 000006 创建，这里只新增 operator/support/viewer）
INSERT INTO roles (code, name, description) VALUES
  ('operator', '运维管理员', '负责节点/服务器/部署/路由等运维操作'),
  ('support',  '客服管理员', '负责工单/公告/通知/用户支持'),
  ('viewer',   '只读观察员', '只能查看各类数据，不能修改')
ON CONFLICT (code) DO NOTHING;

-- 2. 为 super_admin 角色绑定全部已知权限（虽然代码层 super_admin 直接短路 ["*"]，
--    但数据库层也应该保持完整，方便通过 admin_roles 查询审计）
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE r.code = 'super_admin'
  AND NOT EXISTS (
    SELECT 1 FROM role_permissions rp
    WHERE rp.role_id = r.id AND rp.permission_id = p.id
  );

-- 3. operator 角色：运维相关权限
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code IN (
  'nodes.read', 'nodes.write',
  'deployments.write',
  'system.write',
  'plans.read', 'plans.write',
  'audit.read',
  'announcements.read',
  'notifications.read'
)
WHERE r.code = 'operator'
  AND NOT EXISTS (
    SELECT 1 FROM role_permissions rp
    WHERE rp.role_id = r.id AND rp.permission_id = p.id
  );

-- 4. support 角色：客服相关权限
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code IN (
  'users.read',
  'tickets.read', 'tickets.write',
  'announcements.read', 'announcements.write',
  'notifications.read', 'notifications.write',
  'plans.read'
)
WHERE r.code = 'support'
  AND NOT EXISTS (
    SELECT 1 FROM role_permissions rp
    WHERE rp.role_id = r.id AND rp.permission_id = p.id
  );

-- 5. viewer 角色：所有 .read 权限
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code LIKE '%.read'
WHERE r.code = 'viewer'
  AND NOT EXISTS (
    SELECT 1 FROM role_permissions rp
    WHERE rp.role_id = r.id AND rp.permission_id = p.id
  );

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- 删除 operator/support/viewer 角色的 role_permissions（保留 super_admin 的映射，因为它本来就该全权）
DELETE FROM role_permissions
WHERE role_id IN (SELECT id FROM roles WHERE code IN ('operator', 'support', 'viewer'));

-- 删除新增的角色
DELETE FROM roles WHERE code IN ('operator', 'support', 'viewer');

-- 可选：清空 super_admin 的 role_permissions（因为代码层短路 ["*"]，DB 映射非必需）
-- 这里保留，避免影响已分配 super_admin 角色的 admin 审计链
-- 如需清理，取消下面注释：
-- DELETE FROM role_permissions WHERE role_id IN (SELECT id FROM roles WHERE code = 'super_admin');

-- +goose StatementEnd
