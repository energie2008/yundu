-- migration 000076: 支付订单法币网关字段
-- 支付宝/微信走易支付网关，需持久化网关标识、网关流水号与支付跳转地址。

-- +goose Up
-- +goose StatementBegin
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS gateway VARCHAR(32) NOT NULL DEFAULT '';
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS gateway_trade_no VARCHAR(128) NOT NULL DEFAULT '';
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS payment_uri TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE payment_orders DROP COLUMN IF EXISTS payment_uri;
ALTER TABLE payment_orders DROP COLUMN IF EXISTS gateway_trade_no;
ALTER TABLE payment_orders DROP COLUMN IF EXISTS gateway;
-- +goose StatementEnd
