-- +goose Up
-- +goose StatementBegin

-- 补齐 coupons 表缺失的列（代码已引用但数据库未创建）
-- 这些列对应 Xboard CouponService 的校验逻辑：
--   limit_use_by_user : 每用户可用次数（0=不限）
--   limit_plan_ids    : 限定可用套餐 ID 数组（空=不限）
--   new_user_only     : 仅限新用户使用
ALTER TABLE coupons ADD COLUMN IF NOT EXISTS limit_use_by_user INTEGER NOT NULL DEFAULT 0;
ALTER TABLE coupons ADD COLUMN IF NOT EXISTS limit_plan_ids   UUID[]   NOT NULL DEFAULT '{}';
ALTER TABLE coupons ADD COLUMN IF NOT EXISTS new_user_only    BOOLEAN  NOT NULL DEFAULT false;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE coupons DROP COLUMN IF EXISTS limit_use_by_user;
ALTER TABLE coupons DROP COLUMN IF EXISTS limit_plan_ids;
ALTER TABLE coupons DROP COLUMN IF EXISTS new_user_only;

-- +goose StatementEnd