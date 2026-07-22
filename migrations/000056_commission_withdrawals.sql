-- +goose Up
-- +goose StatementBegin

-- 佣金提现表（与 commission_withdraw_repo.go 查询字段匹配）
CREATE TABLE IF NOT EXISTS commission_withdrawals (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    amount      NUMERIC(18,2) NOT NULL,
    method      VARCHAR(32) NOT NULL,
    account     VARCHAR(255) NOT NULL,
    real_name   VARCHAR(128) NOT NULL DEFAULT '',
    status      INTEGER NOT NULL DEFAULT 0,
    remark      TEXT NOT NULL DEFAULT '',
    handled_by  UUID REFERENCES admins(user_id) ON DELETE SET NULL,
    handled_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_commission_withdrawals_user_id ON commission_withdrawals(user_id);
CREATE INDEX IF NOT EXISTS idx_commission_withdrawals_status ON commission_withdrawals(status);

-- 补充通知模板：payment_success（支付成功站内信通知）
-- 与 000019 的通知模板保持同一张表 notification_templates
INSERT INTO notification_templates (code, name, description, category, channel, title_template, body_template, variables, enabled) VALUES
('payment_success', '支付成功提醒', '用户支付成功并激活订阅后触发', 'billing', 'in_app',
 '支付成功', '您的订单 {{order_no}} 已支付成功，套餐「{{plan_name}}」已激活，支付金额 {{amount}} USDT。',
 '[{"key":"order_no","label":"订单号","type":"string"},{"key":"plan_name","label":"套餐名称","type":"string"},{"key":"amount","label":"支付金额","type":"number"}]'),
('traffic_warning', '流量耗尽预警', '用户已用流量超过 80% 时触发', 'traffic_expiry', 'in_app',
 '流量即将耗尽', '您已使用 {{used}} / {{total}} ，已用流量占比 {{percentage}}%，请及时续费或升级套餐。',
 '[{"key":"used","label":"已用流量","type":"string"},{"key":"total","label":"总流量","type":"string"},{"key":"percentage","label":"已用百分比","type":"number"}]')
ON CONFLICT (code) DO NOTHING;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS commission_withdrawals;
DELETE FROM notification_templates WHERE code IN ('payment_success', 'traffic_warning');
-- +goose StatementEnd
