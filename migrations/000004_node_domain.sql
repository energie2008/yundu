-- +goose Up
-- +goose StatementBegin

CREATE TABLE regions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  code VARCHAR(32) NOT NULL UNIQUE,
  name VARCHAR(128) NOT NULL,
  country_code VARCHAR(8),
  city_code VARCHAR(32),
  isp_code VARCHAR(32),
  tags TEXT[] NOT NULL DEFAULT '{}',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE servers (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  code VARCHAR(64) NOT NULL UNIQUE,
  name VARCHAR(128) NOT NULL,
  region_id UUID REFERENCES regions(id),
  provider VARCHAR(64),
  host VARCHAR(255) NOT NULL,
  ipv4 INET,
  ipv6 INET,
  ssh_port INTEGER,
  os_name VARCHAR(64),
  os_version VARCHAR(64),
  arch VARCHAR(32),
  status VARCHAR(32) NOT NULL DEFAULT 'provisioning',
  role VARCHAR(32) NOT NULL DEFAULT 'node',
  labels JSONB NOT NULL DEFAULT '{}'::jsonb,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  last_heartbeat_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at TIMESTAMPTZ
);
CREATE INDEX idx_servers_region_id ON servers(region_id);
CREATE INDEX idx_servers_status ON servers(status);

CREATE TABLE runtimes (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  server_id UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
  runtime_type VARCHAR(32) NOT NULL,
  runtime_version VARCHAR(64),
  provider_type VARCHAR(32) NOT NULL DEFAULT 'node-agent',
  provider_ref VARCHAR(128),
  listen_host VARCHAR(255),
  api_port INTEGER,
  status VARCHAR(32) NOT NULL DEFAULT 'inactive',
  capabilities JSONB NOT NULL DEFAULT '{}'::jsonb,
  config_schema_version VARCHAR(32) NOT NULL DEFAULT 'v1',
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  last_heartbeat_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (server_id, runtime_type, provider_ref)
);
CREATE INDEX idx_runtimes_server_id ON runtimes(server_id);
CREATE INDEX idx_runtimes_provider_type ON runtimes(provider_type);

CREATE TABLE node_groups (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  code VARCHAR(64) NOT NULL UNIQUE,
  name VARCHAR(128) NOT NULL,
  description TEXT,
  visibility VARCHAR(32) NOT NULL DEFAULT 'public',
  sort_order INTEGER NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE nodes (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  code VARCHAR(64) NOT NULL UNIQUE,
  name VARCHAR(128) NOT NULL,
  runtime_id UUID NOT NULL REFERENCES runtimes(id) ON DELETE CASCADE,
  region_id UUID REFERENCES regions(id),
  group_id UUID REFERENCES node_groups(id),
  node_type VARCHAR(32) NOT NULL DEFAULT 'standard',
  protocol_type VARCHAR(32) NOT NULL,
  transport_type VARCHAR(32) NOT NULL,
  security_type VARCHAR(32),
  address VARCHAR(255) NOT NULL,
  port INTEGER NOT NULL,
  sni VARCHAR(255),
  alpn TEXT[] NOT NULL DEFAULT '{}',
  path VARCHAR(255),
  host_header VARCHAR(255),
  flow VARCHAR(64),
  is_enabled BOOLEAN NOT NULL DEFAULT true,
  is_visible BOOLEAN NOT NULL DEFAULT true,
  allow_udp BOOLEAN NOT NULL DEFAULT true,
  speed_limit_mbps INTEGER,
  traffic_rate NUMERIC(8,2) NOT NULL DEFAULT 1.00,
  priority INTEGER NOT NULL DEFAULT 100,
  capacity_score INTEGER NOT NULL DEFAULT 100,
  protocol_schema_version VARCHAR(32) NOT NULL DEFAULT 'v1',
  config_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  tags TEXT[] NOT NULL DEFAULT '{}',
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  last_published_version BIGINT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at TIMESTAMPTZ
);
CREATE INDEX idx_nodes_runtime_id ON nodes(runtime_id);
CREATE INDEX idx_nodes_region_id ON nodes(region_id);
CREATE INDEX idx_nodes_group_id ON nodes(group_id);
CREATE INDEX idx_nodes_protocol_type ON nodes(protocol_type);
CREATE INDEX idx_nodes_enabled_visible ON nodes(is_enabled, is_visible);
CREATE INDEX idx_nodes_config_json_gin ON nodes USING GIN(config_json);

CREATE TABLE health_profiles (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  code VARCHAR(64) NOT NULL UNIQUE,
  name VARCHAR(128) NOT NULL,
  probe_interval_seconds INTEGER NOT NULL DEFAULT 30,
  timeout_seconds INTEGER NOT NULL DEFAULT 10,
  failure_threshold INTEGER NOT NULL DEFAULT 3,
  recovery_threshold INTEGER NOT NULL DEFAULT 2,
  probe_targets JSONB NOT NULL DEFAULT '[]'::jsonb,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE proxy_chains (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  code VARCHAR(64) NOT NULL UNIQUE,
  name VARCHAR(128) NOT NULL,
  status VARCHAR(32) NOT NULL DEFAULT 'active',
  chain_mode VARCHAR(32) NOT NULL DEFAULT 'single',
  strategy VARCHAR(32) NOT NULL DEFAULT 'ordered',
  max_hops INTEGER NOT NULL DEFAULT 1,
  health_policy_id UUID REFERENCES health_profiles(id),
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE proxy_chain_hops (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  chain_id UUID NOT NULL REFERENCES proxy_chains(id) ON DELETE CASCADE,
  hop_index INTEGER NOT NULL,
  hop_type VARCHAR(32) NOT NULL,
  upstream_node_id UUID REFERENCES nodes(id),
  upstream_runtime_id UUID REFERENCES runtimes(id),
  outbound_protocol_type VARCHAR(32),
  outbound_config_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (chain_id, hop_index)
);
CREATE INDEX idx_proxy_chain_hops_chain_id ON proxy_chain_hops(chain_id);

CREATE TABLE node_chain_bindings (
  node_id UUID NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  chain_id UUID NOT NULL REFERENCES proxy_chains(id) ON DELETE CASCADE,
  bind_mode VARCHAR(32) NOT NULL DEFAULT 'default',
  priority INTEGER NOT NULL DEFAULT 100,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (node_id, chain_id)
);

CREATE TABLE node_health_status (
  node_id UUID PRIMARY KEY REFERENCES nodes(id) ON DELETE CASCADE,
  overall_status VARCHAR(32) NOT NULL DEFAULT 'unknown',
  heartbeat_status VARCHAR(32) NOT NULL DEFAULT 'unknown',
  probe_status VARCHAR(32) NOT NULL DEFAULT 'unknown',
  availability_score INTEGER NOT NULL DEFAULT 0,
  latency_score INTEGER NOT NULL DEFAULT 0,
  loss_score INTEGER NOT NULL DEFAULT 0,
  handshake_score INTEGER NOT NULL DEFAULT 0,
  chain_score INTEGER NOT NULL DEFAULT 0,
  stability_score INTEGER NOT NULL DEFAULT 0,
  current_rtt_ms INTEGER,
  current_loss_ratio NUMERIC(6,4),
  current_online_users INTEGER NOT NULL DEFAULT 0,
  current_cpu_percent NUMERIC(5,2),
  current_mem_percent NUMERIC(5,2),
  current_disk_percent NUMERIC(5,2),
  last_heartbeat_at TIMESTAMPTZ,
  last_probe_at TIMESTAMPTZ,
  last_state_changed_at TIMESTAMPTZ,
  last_error_code VARCHAR(64),
  last_error_message TEXT,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE node_health_events (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  node_id UUID NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  event_type VARCHAR(32) NOT NULL,
  severity VARCHAR(32) NOT NULL,
  from_status VARCHAR(32),
  to_status VARCHAR(32),
  metrics JSONB NOT NULL DEFAULT '{}'::jsonb,
  message TEXT,
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_node_health_events_node_id ON node_health_events(node_id, occurred_at DESC);

CREATE TABLE config_versions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  scope_type VARCHAR(32) NOT NULL,
  scope_id UUID NOT NULL,
  version_no BIGINT NOT NULL,
  status VARCHAR(32) NOT NULL DEFAULT 'draft',
  source VARCHAR(32) NOT NULL DEFAULT 'system',
  schema_version VARCHAR(32) NOT NULL DEFAULT 'v1',
  content_json JSONB NOT NULL,
  content_hash VARCHAR(128) NOT NULL,
  created_by_admin_id UUID REFERENCES admins(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at TIMESTAMPTZ,
  UNIQUE (scope_type, scope_id, version_no)
);
CREATE INDEX idx_config_versions_scope ON config_versions(scope_type, scope_id);

CREATE TABLE deployment_batches (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  scope_type VARCHAR(32) NOT NULL,
  scope_id UUID NOT NULL,
  target_version_id UUID NOT NULL REFERENCES config_versions(id),
  strategy VARCHAR(32) NOT NULL DEFAULT 'rolling',
  batch_plan JSONB NOT NULL DEFAULT '[]'::jsonb,
  status VARCHAR(32) NOT NULL DEFAULT 'pending',
  started_at TIMESTAMPTZ,
  finished_at TIMESTAMPTZ,
  created_by_admin_id UUID REFERENCES admins(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE deployment_targets (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  deployment_batch_id UUID NOT NULL REFERENCES deployment_batches(id) ON DELETE CASCADE,
  target_type VARCHAR(32) NOT NULL,
  target_id UUID NOT NULL,
  target_version_id UUID NOT NULL REFERENCES config_versions(id),
  previous_version_id UUID REFERENCES config_versions(id),
  phase_no INTEGER NOT NULL DEFAULT 1,
  status VARCHAR(32) NOT NULL DEFAULT 'pending',
  precheck_result JSONB NOT NULL DEFAULT '{}'::jsonb,
  apply_result JSONB NOT NULL DEFAULT '{}'::jsonb,
  rollback_result JSONB NOT NULL DEFAULT '{}'::jsonb,
  started_at TIMESTAMPTZ,
  finished_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_deployment_targets_batch_id ON deployment_targets(deployment_batch_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS deployment_targets;
DROP TABLE IF EXISTS deployment_batches;
DROP TABLE IF EXISTS config_versions;
DROP TABLE IF EXISTS node_health_events;
DROP TABLE IF EXISTS node_health_status;
DROP TABLE IF EXISTS node_chain_bindings;
DROP TABLE IF EXISTS proxy_chain_hops;
DROP TABLE IF EXISTS proxy_chains;
DROP TABLE IF EXISTS health_profiles;
DROP TABLE IF EXISTS nodes;
DROP TABLE IF EXISTS node_groups;
DROP TABLE IF EXISTS runtimes;
DROP TABLE IF EXISTS servers;
DROP TABLE IF EXISTS regions;
-- +goose StatementEnd
