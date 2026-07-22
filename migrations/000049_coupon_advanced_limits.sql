-- +goose Up
-- +goose StatementBegin

-- 补齐 coupons 表的高级限制字段，对齐 Xboard CouponService 的校验能力：
--   limit_period   : 限制可用周期（month/quarter/year 等），空数组=不限制
--   max_discount   : 最大折扣金额上限（0=不限制），防止百分比优惠券折扣过大
--   is_repeatable  : 是否可重复使用（false=一次性券，全局仅可使用一次）
ALTER TABLE coupons ADD COLUMN IF NOT EXISTS limit_period  TEXT[]        DEFAULT '{}';
ALTER TABLE coupons ADD COLUMN IF NOT EXISTS max_discount  NUMERIC(18,2) DEFAULT 0;
ALTER TABLE coupons ADD COLUMN IF NOT EXISTS is_repeatable BOOLEAN       DEFAULT true;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE coupons DROP COLUMN IF EXISTS limit_period;
ALTER TABLE coupons DROP COLUMN IF EXISTS max_discount;
ALTER TABLE coupons DROP COLUMN IF EXISTS is_repeatable;

-- +goose StatementEnd
