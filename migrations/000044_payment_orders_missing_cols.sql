-- migration 000044: 补齐 payment_orders 缺失列
-- 这些列在 model.PaymentOrder 中已定义，但 payment_orders 表缺失，导致 admin/orders API 500

ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS plan_name VARCHAR(128) DEFAULT '';
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS final_amount NUMERIC(18,2) DEFAULT 0;

-- 历史订单回填 final_amount
UPDATE payment_orders
SET final_amount = amount_usdt - COALESCE(discount_amount, 0)
WHERE final_amount = 0 AND amount_usdt > 0;
