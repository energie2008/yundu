-- +goose Up
-- +goose StatementBegin
-- 新增 exposure_mode（上行暴露方式）独立列，从 config_json 回填
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS exposure_mode VARCHAR(32) DEFAULT NULL;
COMMENT ON COLUMN nodes.exposure_mode IS '上行暴露方式: direct/cdn/cdn_saas/argo_tunnel。P1 阶段从 config_json.exposure_mode 回填，后续 standardizeNodeFields 同时写入此列和 config_json 保持同步';

-- 新增 downstream_exposure_mode（下行暴露方式）独立列
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS downstream_exposure_mode VARCHAR(32) DEFAULT NULL;
COMMENT ON COLUMN nodes.downstream_exposure_mode IS '下行暴露方式（XHTTP split mode 专用）: direct/reality/none。非分离节点为 NULL。渲染逻辑按 inbound tag + 此字段决定是否剥离 TLS，替代 P0 临时 tag 后缀判定';

-- 新增 is_split_mode（上下行分离开关）独立列
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS is_split_mode BOOLEAN DEFAULT false;
COMMENT ON COLUMN nodes.is_split_mode IS '是否启用上下行分离（XHTTP split mode）。前端表单开关，控制下行暴露方式下拉框显示。【重要约束】此字段不进入渲染安全判定逻辑，剥离判定仅依赖 downstream_exposure_mode 是否有值';

-- 从 config_json 回填 exposure_mode（已通过 P0 批量回填脚本设置）
UPDATE nodes
SET exposure_mode = COALESCE(config_json->>'exposure_mode', 'direct')
WHERE exposure_mode IS NULL AND deleted_at IS NULL;

-- 识别已有 split mode 节点（有 xhttp.extra.downloadSettings 且下行 security 与上行不同），
-- 自动设置 is_split_mode=true 和 downstream_exposure_mode='reality'
-- 当前已知：p06 (usvps206p06-xhttp-up-cdn-down-reality) 是唯一已配置的 split 节点
UPDATE nodes
SET is_split_mode = true,
    downstream_exposure_mode = 'reality'
WHERE is_split_mode = false
  AND deleted_at IS NULL
  AND config_json IS NOT NULL
  AND config_json::jsonb @> '{"xhttp":{"extra":{"downloadSettings":{"security":"reality"}}}}'::jsonb;

-- 为 exposure_mode 创建索引（面板列表按暴露方式筛选）
CREATE INDEX IF NOT EXISTS idx_nodes_exposure_mode ON nodes(exposure_mode) WHERE deleted_at IS NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_nodes_exposure_mode;
ALTER TABLE nodes DROP COLUMN IF EXISTS is_split_mode;
ALTER TABLE nodes DROP COLUMN IF EXISTS downstream_exposure_mode;
ALTER TABLE nodes DROP COLUMN IF EXISTS exposure_mode;
-- +goose StatementEnd
