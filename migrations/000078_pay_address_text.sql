-- +goose Up
-- pay_address 扩容为 TEXT：原 VARCHAR(64) 只够静态收款码 URL，
-- qiu-pay 免输金额方案启用后存 alipays:// scheme（约 270 字符），超长导致下单 500。
-- +goose StatementBegin
ALTER TABLE payment_orders ALTER COLUMN pay_address TYPE TEXT;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE payment_orders ALTER COLUMN pay_address SET DEFAULT '';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE payment_orders ALTER COLUMN pay_address TYPE VARCHAR(64);
-- +goose StatementEnd
