-- +goose Up
-- +goose StatementBegin

-- 1. 新增优惠券管理权限
INSERT INTO permissions (code, name, resource, action) VALUES
  ('coupons.read',  '读取优惠券',  'coupons', 'read'),
  ('coupons.write', '写入优惠券',  'coupons', 'write')
ON CONFLICT (code) DO NOTHING;

-- 2. 为 super_admin 角色绑定新权限（代码层 super_admin 直接短路，DB 映射为审计完整性）
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE r.code = 'super_admin'
  AND p.code IN ('coupons.read', 'coupons.write')
  AND NOT EXISTS (
    SELECT 1 FROM role_permissions rp
    WHERE rp.role_id = r.id AND rp.permission_id = p.id
  );

-- 3. operator 角色：运维相关，包含优惠券读写
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code IN ('coupons.read', 'coupons.write')
WHERE r.code = 'operator'
  AND NOT EXISTS (
    SELECT 1 FROM role_permissions rp
    WHERE rp.role_id = r.id AND rp.permission_id = p.id
  );

-- 4. support 角色：客服可查看优惠券（仅读）
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code = 'coupons.read'
WHERE r.code = 'support'
  AND NOT EXISTS (
    SELECT 1 FROM role_permissions rp
    WHERE rp.role_id = r.id AND rp.permission_id = p.id
  );

-- 5. viewer 角色：只读
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code = 'coupons.read'
WHERE r.code = 'viewer'
  AND NOT EXISTS (
    SELECT 1 FROM role_permissions rp
    WHERE rp.role_id = r.id AND rp.permission_id = p.id
  );

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- 删除所有角色对 coupons.* 权限的映射
DELETE FROM role_permissions
WHERE permission_id IN (
  SELECT id FROM permissions WHERE code IN ('coupons.read', 'coupons.write')
);

-- 删除权限定义
DELETE FROM permissions WHERE code IN ('coupons.read', 'coupons.write');

-- +goose StatementEnd
