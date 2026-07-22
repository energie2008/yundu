-- 000058: Add last_seen_at column to nodes table
-- Allows SQL-level queries to determine node online status directly from the nodes table,
-- without joining node_health_status. Updated by health_service.ReportHeartbeat on each agent heartbeat.

ALTER TABLE nodes ADD COLUMN IF NOT EXISTS last_seen_at TIMESTAMPTZ;

-- Index for fast online/offline filtering
CREATE INDEX IF NOT EXISTS idx_nodes_last_seen_at ON nodes (last_seen_at) WHERE last_seen_at IS NOT NULL;

-- Backfill: set last_seen_at from node_health_status.last_heartbeat_at for existing nodes
UPDATE nodes
SET last_seen_at = nhs.last_heartbeat_at
FROM node_health_status nhs
WHERE nodes.id = nhs.node_id
  AND nodes.last_seen_at IS NULL
  AND nhs.last_heartbeat_at IS NOT NULL;
