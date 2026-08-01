-- Migration 000070: A-约束 修复重复 runtime 根因
-- 根因：runtimes 表 UNIQUE(server_id, runtime_type, provider_ref)（见 000004 L57）在 PostgreSQL 中
-- 对 NULL 不生效（NULL != NULL），导致 provider_ref IS NULL 的记录可重复插入。
-- 注意：runtimes 表无 deleted_at 列（用 ON DELETE CASCADE 硬删除），与 nodes 表不同。
-- 配合 runtime_service.go 的 provider_ref 回退（新注册不产生 NULL），从存储层根治根因 #2。
-- 关联方案：YunDu-架构缺陷根治方案-简化版-最小改动零SSH-20260801.md §3

BEGIN;

-- 1. 迁移重复 NULL runtime 的节点到主 runtime（同 server+runtime_type 的非 NULL runtime）
--    必须在 DELETE runtime 之前完成，否则 ON DELETE CASCADE 会级联删除 nodes
UPDATE nodes SET runtime_id = main_rt.id
FROM runtimes dup_rt
JOIN runtimes main_rt
  ON main_rt.server_id = dup_rt.server_id
  AND main_rt.runtime_type = dup_rt.runtime_type
  AND main_rt.provider_ref IS NOT NULL
WHERE dup_rt.provider_ref IS NULL
  AND nodes.runtime_id = dup_rt.id
  AND nodes.deleted_at IS NULL;

-- 2. DELETE 重复的 NULL runtime（节点已迁移到主 runtime；runtimes 无软删除，用 DELETE）
DELETE FROM runtimes
WHERE provider_ref IS NULL
  AND EXISTS (
    SELECT 1 FROM runtimes main_rt
    WHERE main_rt.server_id = runtimes.server_id
      AND main_rt.runtime_type = runtimes.runtime_type
      AND main_rt.provider_ref IS NOT NULL
  );

-- 3. 回填剩余 NULL provider_ref（无主 runtime 的情况，根据 runtime_type 派生，
--    与 runtime_service.go:204 的 normalizedType 回退逻辑一致）
UPDATE runtimes SET provider_ref = runtime_type
WHERE provider_ref IS NULL;

-- 4. 部分唯一约束兜底：每 server+runtime_type 只允许一条 NULL（防御性，回填后应无 NULL）
--    runtimes 无 deleted_at，约束不加该条件
CREATE UNIQUE INDEX IF NOT EXISTS uq_runtimes_server_type_null_ref
ON runtimes (server_id, runtime_type)
WHERE provider_ref IS NULL;

COMMIT;
