-- +goose Up
-- +goose StatementBegin

-- 补齐 plans 表的 description/content/features 列
-- 这三列原本由临时脚本 tmp-bin/add_plan_fields.sql 添加，未进入正式 migration
-- 导致 plan_repo.go 的 planColumns SELECT 在未执行临时脚本的数据库上报错
ALTER TABLE plans ADD COLUMN IF NOT EXISTS description TEXT DEFAULT '';
ALTER TABLE plans ADD COLUMN IF NOT EXISTS content TEXT DEFAULT '';
ALTER TABLE plans ADD COLUMN IF NOT EXISTS features JSONB DEFAULT '[]'::jsonb;

-- 补齐 payment_orders 表与 Go model 对齐的列
-- payment_order.go OrderResponse 引用了 DiscountAmount/CouponCode/PaymentMethod/PaymentUri
-- 但 migration 000023 未创建这些列
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS discount_amount NUMERIC(18,2) DEFAULT 0;
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS coupon_code VARCHAR(64) DEFAULT '';
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS payment_method VARCHAR(32) DEFAULT '';
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS payment_uri TEXT DEFAULT '';

-- 补齐 coupons 表与 Coupons.tsx UI 对齐的列
-- UI 使用 limit_use_by_user / limit_plan_ids / new_user_only，但 migration 000030 未创建
ALTER TABLE coupons ADD COLUMN IF NOT EXISTS limit_use_by_user INTEGER DEFAULT 1;
ALTER TABLE coupons ADD COLUMN IF NOT EXISTS limit_plan_ids UUID[] DEFAULT '{}';
ALTER TABLE coupons ADD COLUMN IF NOT EXISTS new_user_only BOOLEAN DEFAULT false;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE plans DROP COLUMN IF EXISTS description;
ALTER TABLE plans DROP COLUMN IF EXISTS content;
ALTER TABLE plans DROP COLUMN IF EXISTS features;

ALTER TABLE payment_orders DROP COLUMN IF EXISTS discount_amount;
ALTER TABLE payment_orders DROP COLUMN IF EXISTS coupon_code;
ALTER TABLE payment_orders DROP COLUMN IF EXISTS payment_method;
ALTER TABLE payment_orders DROP COLUMN IF EXISTS payment_uri;

ALTER TABLE coupons DROP COLUMN IF EXISTS limit_use_by_user;
ALTER TABLE coupons DROP COLUMN IF EXISTS limit_plan_ids;
ALTER TABLE coupons DROP COLUMN IF EXISTS new_user_only;

-- +goose StatementEnd
