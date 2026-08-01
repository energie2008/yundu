-- migration 000074: 真实链上交易哈希唯一索引
-- 防止同一笔链上转账被匹配到多个同金额 pending 订单导致重复激活订阅。
-- ZERO 前缀为 0 元订单内部标记，不参与链上唯一约束。

-- +goose Up
-- +goose StatementBegin
CREATE UNIQUE INDEX IF NOT EXISTS uq_payment_orders_real_tx_hash
  ON payment_orders(tx_hash)
  WHERE tx_hash IS NOT NULL AND tx_hash <> '' AND tx_hash NOT LIKE 'ZERO%';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS uq_payment_orders_real_tx_hash;
-- +goose StatementEnd
