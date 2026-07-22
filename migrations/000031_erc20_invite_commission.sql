-- +goose Up
-- +goose StatementBegin

-- 1. Ensure payment_orders has needed columns
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS discount_amount NUMERIC(18,2) NOT NULL DEFAULT 0;
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS coupon_code VARCHAR(32);
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS payment_method VARCHAR(32) NOT NULL DEFAULT 'usdt_trc20';

-- 2. Invite codes
CREATE TABLE IF NOT EXISTS invite_codes (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  code VARCHAR(32) NOT NULL UNIQUE,
  pv INTEGER NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_invite_codes_user ON invite_codes(user_id);
CREATE INDEX IF NOT EXISTS idx_invite_codes_code ON invite_codes(code);

-- 3. Commission logs
CREATE TABLE IF NOT EXISTS commission_logs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  inviter_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  invitee_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  order_id UUID REFERENCES payment_orders(id) ON DELETE SET NULL,
  trade_no VARCHAR(64),
  order_amount NUMERIC(18,2) NOT NULL DEFAULT 0,
  get_amount NUMERIC(18,2) NOT NULL DEFAULT 0,
  commission_balance NUMERIC(18,2) NOT NULL DEFAULT 0,
  status INTEGER NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_commission_inviter ON commission_logs(inviter_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_commission_invitee ON commission_logs(invitee_id);
CREATE INDEX IF NOT EXISTS idx_commission_status ON commission_logs(status);

-- 4. User commission/invite fields
ALTER TABLE users ADD COLUMN IF NOT EXISTS commission_balance NUMERIC(18,2) NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN IF NOT EXISTS commission_total NUMERIC(18,2) NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN IF NOT EXISTS inviter_id UUID REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE users ADD COLUMN IF NOT EXISTS registered_at TIMESTAMPTZ;
UPDATE users SET registered_at = created_at WHERE registered_at IS NULL;

-- 5. Coupon enhancements
ALTER TABLE coupons ADD COLUMN IF NOT EXISTS limit_use_by_user INTEGER NOT NULL DEFAULT 0;
ALTER TABLE coupons ADD COLUMN IF NOT EXISTS limit_plan_ids UUID[] DEFAULT NULL;
ALTER TABLE coupons ADD COLUMN IF NOT EXISTS new_user_only BOOLEAN NOT NULL DEFAULT false;

-- 6. Insert ERC20 payment config
INSERT INTO system_settings (setting_group, setting_key, value_json)
VALUES ('payment', 'erc20',
  '{"enabled":true,"address":"0xeB784Ac722D7bF65A6F5aCa5a47591f54EF323Eb","usdt_contract":"0xdAC17F958D2ee523a2206206994597C13D831ec7","etherscan_api":"https://api.etherscan.io/api","etherscan_api_key":"","min_confirmations":3,"poll_interval_seconds":60,"amount_tolerance":0.01,"order_expiry_hours":6,"auto_activate":true,"network":"Ethereum(ERC20)"}'::jsonb)
ON CONFLICT (setting_group, setting_key) DO UPDATE SET value_json = EXCLUDED.value_json;

-- 7. Insert invite/commission config
INSERT INTO system_settings (setting_group, setting_key, value_json)
VALUES ('invite', 'commission',
  '{"enabled":true,"rate":20,"first_pullback":10,"register_reward":0,"invite_reward":0,"confirm_days":3,"withdraw_enable":false,"min_withdraw":10}'::jsonb)
ON CONFLICT (setting_group, setting_key) DO UPDATE SET value_json = EXCLUDED.value_json;

-- 8. Insert email SMTP config
INSERT INTO system_settings (setting_group, setting_key, value_json)
VALUES ('email', 'smtp',
  '{"enabled":false,"host":"","port":465,"username":"","password":"","from_address":"","from_name":"YunDu","encryption":"ssl"}'::jsonb)
ON CONFLICT (setting_group, setting_key) DO UPDATE SET value_json = EXCLUDED.value_json;

-- 9. Ensure TRC20 config exists
INSERT INTO system_settings (setting_group, setting_key, value_json)
VALUES ('payment', 'trc20',
  '{"enabled":true,"address":"","usdt_contract":"TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t","trongrid_api":"https://api.trongrid.io","trongrid_api_key":"","min_confirmations":6,"poll_interval_seconds":60,"amount_tolerance":0.01,"order_expiry_hours":2,"auto_activate":true}'::jsonb)
ON CONFLICT (setting_group, setting_key) DO NOTHING;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS commission_logs;
DROP TABLE IF EXISTS invite_codes;
ALTER TABLE payment_orders DROP COLUMN IF EXISTS payment_method;
ALTER TABLE users DROP COLUMN IF EXISTS commission_balance;
ALTER TABLE users DROP COLUMN IF EXISTS commission_total;
ALTER TABLE users DROP COLUMN IF EXISTS inviter_id;
ALTER TABLE users DROP COLUMN IF EXISTS registered_at;
ALTER TABLE coupons DROP COLUMN IF EXISTS limit_use_by_user;
ALTER TABLE coupons DROP COLUMN IF EXISTS limit_plan_ids;
ALTER TABLE coupons DROP COLUMN IF EXISTS new_user_only;
DELETE FROM system_settings WHERE setting_group='payment' AND setting_key='erc20';
DELETE FROM system_settings WHERE setting_group='invite' AND setting_key='commission';
DELETE FROM system_settings WHERE setting_group='email' AND setting_key='smtp';
-- +goose StatementEnd
