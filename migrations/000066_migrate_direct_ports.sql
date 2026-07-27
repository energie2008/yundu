-- +goose Up
-- +goose StatementBegin

-- P4-D: 端口迁移 9450-9600 → 30000-30200
-- 修正 SQL 循环逻辑：使用 generate_series 生成端口映射，避免循环依赖。
-- 9450→30000, 9451→30001, ..., 9600→30200（偏移量 +20550）

-- Step 1: 迁移 nodes.server_port（存量直连节点）
UPDATE nodes SET
    server_port = 30000 + (server_port - 9450),
    port = CASE
        WHEN port = server_port THEN 30000 + (server_port - 9450)
        ELSE port
    END,
    config_json = jsonb_set(
        COALESCE(config_json, '{}'::jsonb),
        '{server_port}',
        to_jsonb(30000 + (server_port - 9450))
    )
WHERE server_port BETWEEN 9450 AND 9600
  AND deleted_at IS NULL;

-- Step 2: 迁移 nodes.port（如果 port 也在 9450-9600 范围且不等于 server_port，例如 CDN 节点用 443 的不受影响）
UPDATE nodes SET
    port = 30000 + (port - 9450)
WHERE port BETWEEN 9450 AND 9600
  AND port != server_port
  AND deleted_at IS NULL;

-- Step 3: 迁移 deployments.config_json 中的 server_port
UPDATE deployments SET
    config_json = jsonb_set(
        config_json,
        '{server_port}',
        to_jsonb(30000 + ((config_json->>'server_port')::integer - 9450))
    )
WHERE (config_json->>'server_port')::integer BETWEEN 9450 AND 9600
  AND deleted_at IS NULL;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
UPDATE nodes SET
    server_port = 9450 + (server_port - 30000),
    port = CASE
        WHEN port = server_port THEN 9450 + (server_port - 30000)
        ELSE port
    END
WHERE server_port BETWEEN 30000 AND 30200
  AND deleted_at IS NULL;

UPDATE nodes SET
    port = 9450 + (port - 30000)
WHERE port BETWEEN 30000 AND 30200
  AND port != server_port
  AND deleted_at IS NULL;
-- +goose StatementEnd
