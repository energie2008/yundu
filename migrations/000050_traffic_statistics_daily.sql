-- +goose Up
-- +goose StatementBegin

-- 每日流量统计汇总表，由 traffic-service 的 StatisticsService.DailyStatistics 写入。
CREATE TABLE IF NOT EXISTS traffic_statistics_daily (
    stat_date       DATE         NOT NULL PRIMARY KEY,
    upload_bytes    BIGINT       NOT NULL DEFAULT 0,
    download_bytes  BIGINT       NOT NULL DEFAULT 0,
    total_bytes     BIGINT       NOT NULL DEFAULT 0,
    active_users    INTEGER      NOT NULL DEFAULT 0,
    online_count    BIGINT       NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT now()
);

COMMENT ON TABLE traffic_statistics_daily IS '每日流量统计汇总（按天唯一）';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS traffic_statistics_daily;

-- +goose StatementEnd
