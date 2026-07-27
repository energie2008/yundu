-- +goose Up
-- +goose StatementBegin

-- P1-B: exposure_mode 双源同步 trigger（v3 修正版）
-- 原则：config_json.exposure_mode 是唯一真相源，独立列仅为 DB 索引投影。
-- v3 修正：
--   1. 先修复存量数据（trigger 尚未创建，不会递归）
--   2. 再创建 trigger（含 NULL 防御）

-- Step 1: 先修复存量数据（trigger 尚未创建，不会递归）
UPDATE nodes SET exposure_mode = config_json->>'exposure_mode'
WHERE exposure_mode IS DISTINCT FROM (config_json->>'exposure_mode')
  AND config_json->>'exposure_mode' IS NOT NULL
  AND deleted_at IS NULL;

UPDATE nodes SET downstream_exposure_mode = config_json->>'downstream_exposure_mode'
WHERE downstream_exposure_mode IS DISTINCT FROM (config_json->>'downstream_exposure_mode')
  AND config_json->>'downstream_exposure_mode' IS NOT NULL
  AND deleted_at IS NULL;

-- Step 2: 再创建 trigger（含 NULL 防御）
CREATE OR REPLACE FUNCTION sync_exposure_mode() RETURNS TRIGGER AS $$
BEGIN
    -- 仅在 config_json.exposure_mode 非 NULL 时同步到独立列
    IF NEW.config_json->>'exposure_mode' IS NOT NULL THEN
        IF NEW.config_json->>'exposure_mode' IS DISTINCT FROM OLD.config_json->>'exposure_mode' THEN
            NEW.exposure_mode := NEW.config_json->>'exposure_mode';
        END IF;
    END IF;
    -- 独立列变更 → 同步 config_json（同样防御 NULL）
    IF NEW.exposure_mode IS NOT NULL AND NEW.exposure_mode IS DISTINCT FROM OLD.exposure_mode THEN
        NEW.config_json := jsonb_set(
            COALESCE(NEW.config_json, '{}'::jsonb),
            '{exposure_mode}',
            to_jsonb(NEW.exposure_mode)
        );
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER nodes_exposure_mode_sync
    BEFORE INSERT OR UPDATE ON nodes
    FOR EACH ROW EXECUTE FUNCTION sync_exposure_mode();

-- downstream_exposure_mode 同理（含 NULL 防御）
CREATE OR REPLACE FUNCTION sync_downstream_exposure_mode() RETURNS TRIGGER AS $$
BEGIN
    IF NEW.config_json->>'downstream_exposure_mode' IS NOT NULL THEN
        IF NEW.config_json->>'downstream_exposure_mode' IS DISTINCT FROM OLD.config_json->>'downstream_exposure_mode' THEN
            NEW.downstream_exposure_mode := NEW.config_json->>'downstream_exposure_mode';
        END IF;
    END IF;
    IF NEW.downstream_exposure_mode IS NOT NULL AND NEW.downstream_exposure_mode IS DISTINCT FROM OLD.downstream_exposure_mode THEN
        NEW.config_json := jsonb_set(
            COALESCE(NEW.config_json, '{}'::jsonb),
            '{downstream_exposure_mode}',
            to_jsonb(NEW.downstream_exposure_mode)
        );
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER nodes_downstream_exposure_mode_sync
    BEFORE INSERT OR UPDATE ON nodes
    FOR EACH ROW EXECUTE FUNCTION sync_downstream_exposure_mode();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS nodes_exposure_mode_sync ON nodes;
DROP FUNCTION IF EXISTS sync_exposure_mode();
DROP TRIGGER IF EXISTS nodes_downstream_exposure_mode_sync ON nodes;
DROP FUNCTION IF EXISTS sync_downstream_exposure_mode();
-- +goose StatementEnd
