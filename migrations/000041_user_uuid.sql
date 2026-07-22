-- +goose Up
-- +goose StatementBegin

-- 为 users 表新增 uuid 字段：每用户唯一 UUID，全节点共享（对齐 XBoard 模型）
-- 用于所有协议凭证：VLESS/VMess/TUIC 直接使用，Trojan/SS/Hysteria2/AnyTLS 直接使用，
-- SS2022 通过 serverKey:userKey 派生（派生算法在代码层实现）
ALTER TABLE users ADD COLUMN IF NOT EXISTS uuid VARCHAR(36);

-- 回填现有用户 UUID（随机生成 v4 UUID）
UPDATE users SET uuid = gen_random_uuid()::varchar WHERE uuid IS NULL OR uuid = '';

-- 添加 NOT NULL 约束和唯一索引
ALTER TABLE users ALTER COLUMN uuid SET NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_users_uuid ON users(uuid);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS uq_users_uuid;
ALTER TABLE users DROP COLUMN IF EXISTS uuid;
-- +goose StatementEnd
