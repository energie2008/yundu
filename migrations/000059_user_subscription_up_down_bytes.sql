-- +goose Up
-- +goose StatementBegin

ALTER TABLE user_plan_subscriptions
    ADD COLUMN IF NOT EXISTS upload_bytes BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS download_bytes BIGINT NOT NULL DEFAULT 0;

-- Backfill: aggregate from traffic_usage_daily for active subscriptions
UPDATE user_plan_subscriptions ups
SET upload_bytes = COALESCE(agg.up, 0),
    download_bytes = COALESCE(agg.down, 0)
FROM (
    SELECT subscription_id, SUM(upload_bytes) as up, SUM(download_bytes) as down
    FROM traffic_usage_daily
    WHERE subscription_id IS NOT NULL
    GROUP BY subscription_id
) agg
WHERE ups.id = agg.subscription_id
  AND ups.status = 'active';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE user_plan_subscriptions
    DROP COLUMN IF EXISTS upload_bytes,
    DROP COLUMN IF EXISTS download_bytes;
-- +goose StatementEnd
