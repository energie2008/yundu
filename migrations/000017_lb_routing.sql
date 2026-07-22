-- +goose Up
-- +goose StatementBegin

-- 路由规则集（可复用的规则集合：内置 / 自定义 / 远程 URL 同步）
CREATE TABLE route_rule_sets (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  code VARCHAR(64) NOT NULL UNIQUE,
  name VARCHAR(128) NOT NULL,
  description TEXT,
  rule_type VARCHAR(32) NOT NULL,       -- builtin / custom
  source_type VARCHAR(32) NOT NULL,     -- inline / geosite / geoip / remote_url
  source_url TEXT,                      -- 远程规则集地址（source_type=remote_url 时）
  content JSONB NOT NULL DEFAULT '[]'::jsonb,
  auto_update BOOLEAN NOT NULL DEFAULT false,
  last_synced_at TIMESTAMPTZ,
  status VARCHAR(32) NOT NULL DEFAULT 'active',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 路由策略（把"哪些流量"映射到"哪个出站"）
CREATE TABLE route_policies (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  code VARCHAR(64) NOT NULL UNIQUE,
  name VARCHAR(128) NOT NULL,
  description TEXT,
  policy_type VARCHAR(32) NOT NULL DEFAULT 'custom', -- builtin_template / custom
  base_template_code VARCHAR(64),
  status VARCHAR(32) NOT NULL DEFAULT 'active',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 路由策略规则条目（有序）
CREATE TABLE route_policy_rules (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  policy_id UUID NOT NULL REFERENCES route_policies(id) ON DELETE CASCADE,
  sort_order INTEGER NOT NULL DEFAULT 0,
  rule_source VARCHAR(32) NOT NULL,             -- rule_set / inline
  rule_set_id UUID REFERENCES route_rule_sets(id) ON DELETE SET NULL,
  inline_type VARCHAR(32),                      -- domain/domain_suffix/domain_keyword/geosite/geoip/ip_cidr/port/protocol
  inline_values JSONB NOT NULL DEFAULT '[]'::jsonb,
  outbound_action VARCHAR(32) NOT NULL,         -- proxy/direct/blackhole/warp/tag/balancer
  outbound_tag VARCHAR(64),
  notes TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_route_policy_rules_policy_id ON route_policy_rules(policy_id, sort_order);

-- 节点绑定路由策略
CREATE TABLE node_route_bindings (
  node_id UUID NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  policy_id UUID NOT NULL REFERENCES route_policies(id) ON DELETE CASCADE,
  bind_scope VARCHAR(32) NOT NULL DEFAULT 'all', -- all / inbound_tag
  inbound_tag VARCHAR(64),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (node_id, policy_id)
);

-- 节点组负载均衡策略
CREATE TABLE node_group_lb_policies (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  group_id UUID NOT NULL REFERENCES node_groups(id) ON DELETE CASCADE,
  lb_strategy VARCHAR(32) NOT NULL DEFAULT 'round_robin',
  weight_field VARCHAR(32) NOT NULL DEFAULT 'priority',
  geo_affinity BOOLEAN NOT NULL DEFAULT false,
  sticky_by VARCHAR(32),                         -- user_id / subscription_token / null
  min_score_threshold INTEGER NOT NULL DEFAULT 30,
  max_nodes_per_subscription INTEGER,
  extra_config JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (group_id)
);

-- 出站策略组（出站均衡器，对应 xray balancer）
CREATE TABLE outbound_groups (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  node_id UUID NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  tag VARCHAR(64) NOT NULL,
  lb_strategy VARCHAR(32) NOT NULL DEFAULT 'leastPing',
  probe_url TEXT NOT NULL DEFAULT 'https://www.google.com/generate_204',
  probe_interval_seconds INTEGER NOT NULL DEFAULT 60,
  members JSONB NOT NULL DEFAULT '[]'::jsonb,    -- [{"tag": "hk-isp1", "weight": 1}]
  status VARCHAR(32) NOT NULL DEFAULT 'active',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (node_id, tag)
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS outbound_groups;
DROP TABLE IF EXISTS node_group_lb_policies;
DROP TABLE IF EXISTS node_route_bindings;
DROP TABLE IF EXISTS route_policy_rules;
DROP TABLE IF EXISTS route_policies;
DROP TABLE IF EXISTS route_rule_sets;
-- +goose StatementEnd
