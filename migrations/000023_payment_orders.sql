-- +goose Up
-- +goose StatementBegin

-- 1. 创建支付订单表
CREATE TABLE payment_orders (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  order_no VARCHAR(64) NOT NULL UNIQUE,
  user_id UUID NOT NULL REFERENCES users(id),
  plan_id UUID NOT NULL REFERENCES plans(id),
  period_code VARCHAR(32) NOT NULL,
  amount_usdt NUMERIC(18,2) NOT NULL,
  pay_address VARCHAR(64) NOT NULL,
  pay_currency VARCHAR(16) NOT NULL DEFAULT 'USDT-TRC20',
  status VARCHAR(32) NOT NULL DEFAULT 'pending',
  tx_hash VARCHAR(128),
  block_number BIGINT,
  paid_amount NUMERIC(18,6),
  confirmations INTEGER DEFAULT 0,
  paid_at TIMESTAMPTZ,
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_payment_orders_user ON payment_orders(user_id, created_at DESC);
CREATE INDEX idx_payment_orders_status ON payment_orders(status);
CREATE INDEX idx_payment_orders_expires ON payment_orders(expires_at) WHERE status='pending';
CREATE INDEX idx_payment_orders_tx ON payment_orders(tx_hash) WHERE tx_hash IS NOT NULL;

-- 2. 扩展 plan_prices.currency_code 长度以支持 USDT-TRC20
ALTER TABLE plan_prices ALTER COLUMN currency_code TYPE VARCHAR(16);

-- 3. 插入 USDT-TRC20 定价种子数据（仅当 plans 表有数据时才插入）
DO $$
DECLARE
  plan_rec RECORD;
BEGIN
  IF EXISTS (SELECT 1 FROM plans WHERE deleted_at IS NULL) THEN
    FOR plan_rec IN SELECT id FROM plans WHERE deleted_at IS NULL LOOP
      INSERT INTO plan_prices (plan_id, period_code, currency_code, amount_minor, is_active) VALUES
        (plan_rec.id, 'monthly',      'USDT-TRC20',  999, true),
        (plan_rec.id, 'quarterly',    'USDT-TRC20', 2797, true),
        (plan_rec.id, 'half_yearly',  'USDT-TRC20', 5295, true),
        (plan_rec.id, 'yearly',       'USDT-TRC20', 9993, true)
      ON CONFLICT (plan_id, period_code, currency_code) DO NOTHING;
    END LOOP;
  END IF;
END $$;

-- 4. 插入 TRC20 支付系统设置
INSERT INTO system_settings (setting_group, setting_key, value_json, description) VALUES
(
  'payment',
  'trc20',
  jsonb_build_object(
    'enabled',              false,
    'address',              '',
    'usdt_contract',        'TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t',
    'trongrid_api',         'https://api.trongrid.io',
    'min_confirmations',    1,
    'poll_interval_seconds',15,
    'amount_tolerance',     '0.01',
    'order_expiry_hours',   24,
    'auto_activate',        true
  ),
  'TRC20-USDT支付配置'
)
ON CONFLICT (setting_group, setting_key) DO NOTHING;

-- 5. 增加 users.write 权限码（如果不存在）并绑定到 super_admin
INSERT INTO permissions (code, name, resource, action) VALUES
  ('users.write', '写入用户', 'users', 'write')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code = 'users.write'
WHERE r.code = 'super_admin'
  AND NOT EXISTS (
    SELECT 1 FROM role_permissions rp
    WHERE rp.role_id = r.id AND rp.permission_id = p.id
  );

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- 1. 解绑 users.write 权限
DELETE FROM role_permissions
WHERE permission_id = (SELECT id FROM permissions WHERE code = 'users.write');

-- 2. 删除 users.write 权限
DELETE FROM permissions WHERE code = 'users.write';

-- 3. 删除 TRC20 支付系统设置
DELETE FROM system_settings WHERE setting_group = 'payment' AND setting_key = 'trc20';

-- 4. 删除 USDT-TRC20 定价种子数据
DELETE FROM plan_prices WHERE currency_code = 'USDT-TRC20';

-- 5. 删除支付订单表
DROP TABLE IF EXISTS payment_orders;

-- +goose StatementEnd
