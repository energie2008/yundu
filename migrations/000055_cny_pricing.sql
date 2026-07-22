-- +goose Up
-- +goose StatementBegin

-- 1. 插入 USDT 到 CNY 汇率配置
INSERT INTO system_settings (setting_group, setting_key, value_json, description) VALUES
(
  'payment',
  'exchange_rate',
  jsonb_build_object(
    'usdt_to_cny',  7.2,
    'auto_update',  false,
    'last_updated', to_char(now() at time zone 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
  ),
  'USDT到CNY汇率配置'
)
ON CONFLICT (setting_group, setting_key) DO NOTHING;

-- 2. 在 plan_prices 表增加 amount_cny 列（以分为单位的 CNY 价格），默认 NULL
ALTER TABLE plan_prices ADD COLUMN IF NOT EXISTS amount_cny BIGINT;

-- 3. 在 payment_orders 表增加 exchange_rate 和 amount_cny 列
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS exchange_rate NUMERIC(10,4) DEFAULT 7.2;
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS amount_cny NUMERIC(18,2) DEFAULT 0;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE payment_orders DROP COLUMN IF EXISTS exchange_rate;
ALTER TABLE plan_prices DROP COLUMN IF EXISTS amount_cny;
DELETE FROM system_settings WHERE setting_group = 'payment' AND setting_key = 'exchange_rate';

-- +goose StatementEnd
