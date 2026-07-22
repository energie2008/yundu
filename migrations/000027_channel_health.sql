-- +goose Up
-- +goose StatementBegin

-- ============================================================================
-- 通道健康快照：node-agent 每 10s 上报一次通道状态（gRPC/WS/HTTP 三通道）
-- ============================================================================
CREATE TABLE channel_health_snapshots (
  id BIGSERIAL PRIMARY KEY,
  server_id UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
  runtime_id UUID REFERENCES runtimes(id) ON DELETE SET NULL,
  active_channel VARCHAR(16) NOT NULL,             -- grpc / ws / http
  channel_state VARCHAR(16) NOT NULL DEFAULT 'unknown', -- healthy / degraded / unhealthy / unknown
  rtt_ms INTEGER,
  fail_count_1h INTEGER NOT NULL DEFAULT 0,
  online_users INTEGER NOT NULL DEFAULT 0,
  last_error TEXT,
  reported_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_channel_health_server_time ON channel_health_snapshots(server_id, reported_at DESC);
CREATE INDEX idx_channel_health_runtime_time ON channel_health_snapshots(runtime_id, reported_at DESC);

-- 当前通道状态（每服务器一条，UPSERT）
CREATE TABLE channel_health_current (
  server_id UUID PRIMARY KEY REFERENCES servers(id) ON DELETE CASCADE,
  runtime_id UUID REFERENCES runtimes(id) ON DELETE SET NULL,
  active_channel VARCHAR(16) NOT NULL,
  channel_state VARCHAR(16) NOT NULL DEFAULT 'unknown',
  rtt_ms INTEGER,
  fail_count_1h INTEGER NOT NULL DEFAULT 0,
  online_users INTEGER NOT NULL DEFAULT 0,
  failover_count_1h INTEGER NOT NULL DEFAULT 0,
  failover_count_24h INTEGER NOT NULL DEFAULT 0,
  last_error TEXT,
  last_failover_at TIMESTAMPTZ,
  last_failover_from VARCHAR(16),
  last_failover_to VARCHAR(16),
  last_failover_reason VARCHAR(32),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ============================================================================
-- 通道降级事件：记录每次 failover
-- ============================================================================
CREATE TABLE channel_failover_events (
  id BIGSERIAL PRIMARY KEY,
  server_id UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
  runtime_id UUID REFERENCES runtimes(id) ON DELETE SET NULL,
  from_channel VARCHAR(16) NOT NULL,
  to_channel VARCHAR(16) NOT NULL,
  reason VARCHAR(32) NOT NULL DEFAULT 'heartbeat_timeout', -- heartbeat_timeout / connection_error / manual_switch / auto_recovery / initial_connect
  detail TEXT,
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_failover_events_server_time ON channel_failover_events(server_id, occurred_at DESC);
CREATE INDEX idx_failover_events_time ON channel_failover_events(occurred_at DESC);

-- ============================================================================
-- AI 诊断会话：每次诊断创建一个 session
-- ============================================================================
CREATE TABLE diagnosis_sessions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  server_id UUID REFERENCES servers(id) ON DELETE SET NULL,
  node_id UUID REFERENCES nodes(id) ON DELETE SET NULL,
  status VARCHAR(32) NOT NULL DEFAULT 'pending', -- pending / collecting / analyzing / done / failed
  trigger_source VARCHAR(32) NOT NULL DEFAULT 'manual', -- manual / scheduled / alert
  time_window_start TIMESTAMPTZ,
  time_window_end TIMESTAMPTZ,
  raw_logs TEXT,
  raw_metrics JSONB NOT NULL DEFAULT '{}'::jsonb,
  llm_provider VARCHAR(32) NOT NULL DEFAULT 'glm',
  llm_model VARCHAR(64),
  root_cause_category VARCHAR(64),  -- config_error / network_issue / cert_expired / kernel_compat / rate_limit / normal
  root_cause_description TEXT,
  confidence FLOAT,                  -- 0.0 ~ 1.0
  suggestions JSONB NOT NULL DEFAULT '[]'::jsonb, -- [{title, description, action, auto_fixable}]
  doc_links JSONB NOT NULL DEFAULT '[]'::jsonb,
  knowledge_entry_id UUID,           -- 匹配到的历史知识库条目
  autofix_applied BOOLEAN NOT NULL DEFAULT false,
  autofix_result JSONB,
  duration_ms INTEGER,
  created_by_admin_id UUID REFERENCES admins(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ
);
CREATE INDEX idx_diagnosis_sessions_node_time ON diagnosis_sessions(node_id, created_at DESC);
CREATE INDEX idx_diagnosis_sessions_server_time ON diagnosis_sessions(server_id, created_at DESC);
CREATE INDEX idx_diagnosis_sessions_status ON diagnosis_sessions(status);

-- ============================================================================
-- AI 诊断知识库：历史诊断沉淀的可复用方案
-- ============================================================================
CREATE TABLE diagnosis_knowledge (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  title VARCHAR(256) NOT NULL,
  category VARCHAR(64) NOT NULL,             -- 与 root_cause_category 对齐
  root_cause_pattern TEXT NOT NULL,          -- 用于匹配的根因关键词/正则
  solution TEXT NOT NULL,                    -- 解决方案描述
  auto_fix_action VARCHAR(64),              -- 可自动修复的动作编码
  doc_links JSONB NOT NULL DEFAULT '[]'::jsonb,
  hit_count INTEGER NOT NULL DEFAULT 0,
  is_verified BOOLEAN NOT NULL DEFAULT false, -- 人工核验后才进入推荐
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_diagnosis_knowledge_category ON diagnosis_knowledge(category);
CREATE INDEX idx_diagnosis_knowledge_hit ON diagnosis_knowledge(hit_count DESC);

-- ============================================================================
-- 节点体验评分：每 5 分钟计算一次
-- ============================================================================
CREATE TABLE node_experience_scores (
  id BIGSERIAL PRIMARY KEY,
  node_id UUID NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  overall_score FLOAT NOT NULL DEFAULT 0,       -- 0 ~ 100 综合体验分
  latency_score FLOAT NOT NULL DEFAULT 0,       -- 0 ~ 100
  stability_score FLOAT NOT NULL DEFAULT 0,     -- 0 ~ 100
  speed_score FLOAT NOT NULL DEFAULT 0,          -- 0 ~ 100
  success_rate_score FLOAT NOT NULL DEFAULT 0,   -- 0 ~ 100
  -- 原始指标快照
  p50_latency_ms FLOAT,
  p95_latency_ms FLOAT,
  p99_latency_ms FLOAT,
  heartbeat_success_rate FLOAT,                 -- 0.0 ~ 1.0
  channel_failover_count_24h INTEGER,
  measured_bandwidth_mbps FLOAT,
  connection_success_rate FLOAT,                -- 0.0 ~ 1.0
  -- 体验分级（excellent / good / fair / poor / critical）
  grade VARCHAR(16) NOT NULL DEFAULT 'unknown',
  -- 是否被 LB 自动隔离（overall_score < 阈值）
  isolated BOOLEAN NOT NULL DEFAULT false,
  calculated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_experience_node_time ON node_experience_scores(node_id, calculated_at DESC);
CREATE INDEX idx_experience_calculated_at ON node_experience_scores(calculated_at DESC);

-- 当前节点体验分（每节点一条，UPSERT）
CREATE TABLE node_experience_current (
  node_id UUID PRIMARY KEY REFERENCES nodes(id) ON DELETE CASCADE,
  overall_score FLOAT NOT NULL DEFAULT 0,
  latency_score FLOAT NOT NULL DEFAULT 0,
  stability_score FLOAT NOT NULL DEFAULT 0,
  speed_score FLOAT NOT NULL DEFAULT 0,
  success_rate_score FLOAT NOT NULL DEFAULT 0,
  p50_latency_ms FLOAT,
  p95_latency_ms FLOAT,
  p99_latency_ms FLOAT,
  heartbeat_success_rate FLOAT,
  channel_failover_count_24h INTEGER,
  measured_bandwidth_mbps FLOAT,
  connection_success_rate FLOAT,
  grade VARCHAR(16) NOT NULL DEFAULT 'unknown',
  isolated BOOLEAN NOT NULL DEFAULT false,
  calculated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ============================================================================
-- 节点体验评分配置（系统设置）
-- ============================================================================
CREATE TABLE node_experience_config (
  id SMALLINT PRIMARY KEY DEFAULT 1,
  -- 权重（总和 1.0）
  weight_latency FLOAT NOT NULL DEFAULT 0.30,
  weight_stability FLOAT NOT NULL DEFAULT 0.25,
  weight_speed FLOAT NOT NULL DEFAULT 0.25,
  weight_success_rate FLOAT NOT NULL DEFAULT 0.20,
  -- 阈值
  excellent_threshold FLOAT NOT NULL DEFAULT 85.0,
  good_threshold FLOAT NOT NULL DEFAULT 70.0,
  fair_threshold FLOAT NOT NULL DEFAULT 60.0,
  poor_threshold FLOAT NOT NULL DEFAULT 40.0,
  isolate_threshold FLOAT NOT NULL DEFAULT 30.0,
  -- 计算/探针周期
  calc_interval_seconds INTEGER NOT NULL DEFAULT 300,
  probe_interval_seconds INTEGER NOT NULL DEFAULT 60,
  -- 是否启用 LB 自动隔离
  auto_isolate_enabled BOOLEAN NOT NULL DEFAULT true,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT single_row CHECK (id = 1)
);
INSERT INTO node_experience_config (id) VALUES (1) ON CONFLICT DO NOTHING;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS node_experience_config;
DROP TABLE IF EXISTS node_experience_current;
DROP TABLE IF EXISTS node_experience_scores;
DROP TABLE IF EXISTS diagnosis_knowledge;
DROP TABLE IF EXISTS diagnosis_sessions;
DROP TABLE IF EXISTS channel_failover_events;
DROP TABLE IF EXISTS channel_health_current;
DROP TABLE IF EXISTS channel_health_snapshots;
-- +goose StatementEnd
