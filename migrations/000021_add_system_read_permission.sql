-- +goose Up
-- +goose StatementBegin

-- 1. 新增 system.read 权限（之前只有 system.write，导致 GET /admin/system/settings 无可用权限码）
INSERT INTO permissions (code, name, resource, action) VALUES
  ('system.read', '读取系统配置', 'system', 'read')
ON CONFLICT (code) DO NOTHING;

-- 2. 绑定到 super_admin（保持 DB 层完整，虽然代码层短路 ["*"]）
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code = 'system.read'
WHERE r.code = 'super_admin'
  AND NOT EXISTS (
    SELECT 1 FROM role_permissions rp
    WHERE rp.role_id = r.id AND rp.permission_id = p.id
  );

-- 3. 绑定到 operator（运维需要看系统配置）
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code = 'system.read'
WHERE r.code = 'operator'
  AND NOT EXISTS (
    SELECT 1 FROM role_permissions rp
    WHERE rp.role_id = r.id AND rp.permission_id = p.id
  );

-- 4. 绑定到 viewer（只读角色应该能读所有 .read 权限，包括 system.read）
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code = 'system.read'
WHERE r.code = 'viewer'
  AND NOT EXISTS (
    SELECT 1 FROM role_permissions rp
    WHERE rp.role_id = r.id AND rp.permission_id = p.id
  );

-- 注：support 角色不需要 system.read（客服不接触系统配置）

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- 解绑 system.read 权限
DELETE FROM role_permissions
WHERE permission_id = (SELECT id FROM permissions WHERE code = 'system.read');

-- 删除 system.read 权限
DELETE FROM permissions WHERE code = 'system.read';

-- +goose StatementEnd
