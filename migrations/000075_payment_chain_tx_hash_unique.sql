-- migration 000075: 支付订单按“网络+交易哈希”保证唯一
-- pay_currency 支持多网络（USDT-Polygon / USDT-Arbitrum One / USDT-BEP20），
-- 同一收款地址在不同链上交易哈希互不冲突，唯一约束必须带网络维度。
-- Ethereum 主网手续费过高已下线，历史 USDT-ERC20 测试订单归入 Polygon 语义。

-- +goose Up
-- +goose StatementBegin
ALTER TABLE payment_orders ALTER COLUMN pay_currency TYPE VARCHAR(32);

UPDATE payment_orders
SET pay_currency = 'USDT-Polygon'
WHERE pay_currency = 'USDT-ERC20';

DROP INDEX IF EXISTS uq_payment_orders_real_tx_hash;

CREATE UNIQUE INDEX IF NOT EXISTS uq_payment_orders_chain_tx_hash
  ON payment_orders(pay_currency, tx_hash)
  WHERE tx_hash IS NOT NULL AND tx_hash <> '' AND tx_hash NOT LIKE 'ZERO%';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS uq_payment_orders_chain_tx_hash;
CREATE UNIQUE INDEX IF NOT EXISTS uq_payment_orders_real_tx_hash
  ON payment_orders(tx_hash)
  WHERE tx_hash IS NOT NULL AND tx_hash <> '' AND tx_hash NOT LIKE 'ZERO%';
-- +goose StatementEnd
