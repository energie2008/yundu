-- 000057: Add device_limit, padding_scheme, rate_time, transfer_enable to nodes table
-- Supports P0 (device_limit for speed/device limit chain) and P3 (padding_scheme, rate_time, transfer_enable)

-- device_limit: max concurrent devices per user on this node (0 = unlimited)
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS device_limit INTEGER DEFAULT 0;

-- padding_scheme: AnyTLS padding scheme (max-0 to max-8), NULL for other protocols
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS padding_scheme VARCHAR(20);

-- rate_time_enable: enable time-based traffic rate multiplier
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS rate_time_enable BOOLEAN DEFAULT FALSE;

-- rate_time_ranges: JSON array of {start, end, multiplier} for time-based rate
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS rate_time_ranges JSONB;

-- transfer_enable_bytes: node-level total traffic quota in bytes (0 = unlimited)
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS transfer_enable_bytes BIGINT DEFAULT 0;
