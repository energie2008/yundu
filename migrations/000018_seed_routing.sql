-- +goose Up
-- +goose StatementBegin

-- 内置路由规则集（8 个）
INSERT INTO route_rule_sets (code, name, description, rule_type, source_type, content) VALUES
('cn-direct',       '国内直连',     '国内 IP 与域名直连',           'builtin', 'geoip',   '["geoip:cn","geosite:cn"]'::jsonb),
('private-direct',  '私有 IP 直连', '局域网与私有 IP 直连',          'builtin', 'inline',  '["geoip:private"]'::jsonb),
('streaming-unlock','流媒体域名',    'Netflix/Hulu/Disney/HBO 域名',  'builtin', 'geosite', '["geosite:netflix","geosite:hulu","geosite:disney","geosite:hbo"]'::jsonb),
('openai',          'OpenAI 域名',   'OpenAI/Anthropic/Gemini 域名',  'builtin', 'inline',  '["domain_suffix:openai.com","domain_suffix:anthropic.com","domain_suffix:gemini.google.com"]'::jsonb),
('telegram',        'Telegram',      'Telegram IP 段',               'builtin', 'geoip',   '["geoip:telegram"]'::jsonb),
('ads-block',       '广告屏蔽',      '广告域名黑洞',                  'builtin', 'geosite', '["geosite:category-ads-all"]'::jsonb),
('malware-block',   '恶意域名屏蔽',  '恶意域名黑洞',                  'builtin', 'geosite', '["geosite:malware"]'::jsonb),
('warp-recommend',  '建议走 WARP',   'OpenAI/Copilot/Netflix 走 WARP','builtin', 'inline',  '["domain_suffix:openai.com","domain_suffix:copilot.microsoft.com","geosite:netflix"]'::jsonb)
ON CONFLICT (code) DO NOTHING;

-- 内置模板 A：标准机场（国内直连 + 广告屏蔽 + 其余代理）
INSERT INTO route_policies (code, name, description, policy_type)
VALUES ('tpl-standard', '标准机场', '国内直连 + 广告屏蔽 + 其余代理', 'builtin_template')
ON CONFLICT (code) DO NOTHING;

WITH p AS (SELECT id FROM route_policies WHERE code='tpl-standard'),
     cn AS (SELECT id FROM route_rule_sets WHERE code='cn-direct'),
     prv AS (SELECT id FROM route_rule_sets WHERE code='private-direct'),
     ads AS (SELECT id FROM route_rule_sets WHERE code='ads-block')
INSERT INTO route_policy_rules (policy_id, sort_order, rule_source, rule_set_id, outbound_action)
VALUES
  ((SELECT id FROM p), 10, 'rule_set', (SELECT id FROM prv), 'direct'),
  ((SELECT id FROM p), 20, 'rule_set', (SELECT id FROM ads), 'blackhole'),
  ((SELECT id FROM p), 30, 'rule_set', (SELECT id FROM cn),  'direct'),
  ((SELECT id FROM p), 99, 'inline',   NULL,                 'proxy')
ON CONFLICT DO NOTHING;

-- 内置模板 B：流媒体解锁（国内直连 + 流媒体解锁 + OpenAI 走 WARP）
INSERT INTO route_policies (code, name, description, policy_type)
VALUES ('tpl-streaming', '流媒体解锁', '国内直连 + 流媒体解锁 + OpenAI 走 WARP', 'builtin_template')
ON CONFLICT (code) DO NOTHING;

WITH p AS (SELECT id FROM route_policies WHERE code='tpl-streaming'),
     cn AS (SELECT id FROM route_rule_sets WHERE code='cn-direct'),
     prv AS (SELECT id FROM route_rule_sets WHERE code='private-direct'),
     ads AS (SELECT id FROM route_rule_sets WHERE code='ads-block'),
     stm AS (SELECT id FROM route_rule_sets WHERE code='streaming-unlock'),
     oai AS (SELECT id FROM route_rule_sets WHERE code='openai')
INSERT INTO route_policy_rules (policy_id, sort_order, rule_source, rule_set_id, outbound_action, outbound_tag)
VALUES
  ((SELECT id FROM p), 10, 'rule_set', (SELECT id FROM prv), 'direct',    NULL),
  ((SELECT id FROM p), 20, 'rule_set', (SELECT id FROM ads), 'blackhole', NULL),
  ((SELECT id FROM p), 30, 'rule_set', (SELECT id FROM oai), 'warp',      NULL),
  ((SELECT id FROM p), 40, 'rule_set', (SELECT id FROM stm), 'tag',       'streaming-out'),
  ((SELECT id FROM p), 50, 'rule_set', (SELECT id FROM cn),  'direct',    NULL),
  ((SELECT id FROM p), 99, 'inline',   NULL,                 'proxy',     NULL)
ON CONFLICT DO NOTHING;

-- 内置模板 C：全局代理（仅私有 IP 直连，其余全代理）
INSERT INTO route_policies (code, name, description, policy_type)
VALUES ('tpl-global', '全局代理', '仅私有 IP 直连，其余全代理', 'builtin_template')
ON CONFLICT (code) DO NOTHING;

WITH p AS (SELECT id FROM route_policies WHERE code='tpl-global'),
     prv AS (SELECT id FROM route_rule_sets WHERE code='private-direct')
INSERT INTO route_policy_rules (policy_id, sort_order, rule_source, rule_set_id, outbound_action)
VALUES
  ((SELECT id FROM p), 10, 'rule_set', (SELECT id FROM prv), 'direct'),
  ((SELECT id FROM p), 99, 'inline',   NULL,                 'proxy')
ON CONFLICT DO NOTHING;

-- 默认节点组负载均衡策略（round_robin，min_score=30）
INSERT INTO node_group_lb_policies (group_id, lb_strategy, min_score_threshold)
SELECT id, 'round_robin', 30 FROM node_groups
WHERE NOT EXISTS (SELECT 1 FROM node_group_lb_policies WHERE group_id = node_groups.id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM route_policy_rules WHERE policy_id IN (SELECT id FROM route_policies WHERE policy_type = 'builtin_template');
DELETE FROM route_policies WHERE policy_type = 'builtin_template';
DELETE FROM route_rule_sets WHERE rule_type = 'builtin';
-- 不删除 node_group_lb_policies（由用户手动管理）
-- +goose StatementEnd
