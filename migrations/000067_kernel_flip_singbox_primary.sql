-- +goose Up
-- +goose StatementBegin

-- P4-E: 内核翻转 runtime_id 迁移（CTE 优化版）
-- 目标：非 XHTTP 节点的 runtime_id 迁移到 sing-box runtime。
-- XHTTP 节点保持 xray runtime（sing-box 不支持 XHTTP）。

-- Step 1: 为没有 sing-box runtime 的 server 创建 sing-box runtime
-- 使用 CTE 避免循环依赖
WITH servers_needing_singbox AS (
    SELECT DISTINCT s.id AS server_id, s.listen_host, s.listen_port
    FROM servers s
    JOIN runtimes r ON r.server_id = s.id
    WHERE r.runtime_type IN ('xray', 'xray-core')
      AND s.deleted_at IS NULL
      AND r.deleted_at IS NULL
      AND NOT EXISTS (
          SELECT 1 FROM runtimes r2
          WHERE r2.server_id = s.id
            AND r2.runtime_type IN ('sing-box', 'singbox')
            AND r2.deleted_at IS NULL
      )
)
INSERT INTO runtimes (id, server_id, runtime_type, listen_host, listen_port, status, version, config_hash, created_at, updated_at)
SELECT
    gen_random_uuid(),
    server_id,
    'sing-box',
    listen_host,
    listen_port,
    'created',
    '',
    '',
    NOW(),
    NOW()
FROM servers_needing_singbox;

-- Step 2: 迁移非 XHTTP 节点到 sing-box runtime
-- 非XHTTP = transport_type != 'xhttp' AND protocol NOT IN ('xhttp' 相关)
UPDATE nodes n SET
    runtime_id = (
        SELECT r2.id FROM runtimes r2
        WHERE r2.server_id = (
            SELECT r.server_id FROM runtimes r WHERE r.id = n.runtime_id
        )
        AND r2.runtime_type IN ('sing-box', 'singbox')
        AND r2.deleted_at IS NULL
        LIMIT 1
    )
WHERE n.transport_type != 'xhttp'
  AND n.protocol_type NOT IN ('xhttp')
  AND n.deleted_at IS NULL
  AND EXISTS (
      SELECT 1 FROM runtimes r
      WHERE r.id = n.runtime_id
        AND r.runtime_type IN ('xray', 'xray-core')
        AND r.deleted_at IS NULL
  )
  AND EXISTS (
      SELECT 1 FROM runtimes r3
      WHERE r3.server_id = (
          SELECT r.server_id FROM runtimes r WHERE r.id = n.runtime_id
      )
      AND r3.runtime_type IN ('sing-box', 'singbox')
      AND r3.deleted_at IS NULL
  );

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- 回滚：将非 XHTTP 节点迁回 xray runtime
UPDATE nodes n SET
    runtime_id = (
        SELECT r2.id FROM runtimes r2
        WHERE r2.server_id = (
            SELECT r.server_id FROM runtimes r WHERE r.id = n.runtime_id
        )
        AND r2.runtime_type IN ('xray', 'xray-core')
        AND r2.deleted_at IS NULL
        LIMIT 1
    )
WHERE n.transport_type != 'xhttp'
  AND n.deleted_at IS NULL;
-- +goose StatementEnd
