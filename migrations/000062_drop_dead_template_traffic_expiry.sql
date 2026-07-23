-- +goose Up
-- +goose StatementBegin
-- 清理死模板：traffic_expiry 在 000019 预置，但代码实际使用 traffic_warning（000056 预置），
-- traffic_expiry 从未被任何服务引用，删除以避免混淆。
DELETE FROM notification_templates WHERE code = 'traffic_expiry';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- 恢复 traffic_expiry 模板（与 000019 原始定义一致）
INSERT INTO notification_templates (code, name, description, category, channel, title_template, body_template, variables) VALUES
('traffic_expiry', '流量到期提醒', '当用户流量即将用尽时触发', 'traffic_expiry', 'in_app',
 '流量即将用尽', '您的套餐剩余流量已不足 {{remaining_bytes}} MB，请及时续费或升级套餐。',
 '[{"key":"remaining_bytes","label":"剩余流量(MB)","type":"number"}]')
ON CONFLICT (code) DO NOTHING;
-- +goose StatementEnd
