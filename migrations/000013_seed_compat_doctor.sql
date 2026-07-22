-- +goose Up
-- +goose StatementBegin

-- 客户端档案
INSERT INTO client_profiles (code, name, platform, min_version) VALUES
('clash-meta',    'Clash Meta',        'multi',     '1.18.0'),
('sing-box',      'Sing-box',          'multi',     '1.9.0'),
('v2rayn',        'v2rayN',            'windows',   '6.0.0'),
('v2rayng',       'v2rayNG',           'android',   '1.8.0'),
('shadowrocket',  'Shadowrocket',      'ios',       '2.2.0'),
('stash',         'Stash',             'ios',       '2.5.0'),
('hiddify',       'Hiddify',           'multi',     '2.0.0'),
('nekobox',       'NekoBox',           'android',   '1.3.0'),
('loon',          'Loon',              'ios',       '3.1.0')
ON CONFLICT (code) DO NOTHING;

-- 兼容矩阵
INSERT INTO client_compat_matrix (client_code, feature_code, supported, supported_since_version, notes) VALUES
('clash-meta',   'utls',                true,  '1.18.0',  'chrome/firefox/safari/edge/ios/android/randomized'),
('clash-meta',   'reality',             true,  '1.18.0',  NULL),
('clash-meta',   'vless_encryption',    false, NULL,       '暂不支持'),
('clash-meta',   'ech',                 false, NULL,       '尚未实装'),
('clash-meta',   'xhttp',               true,  '1.19.0',  NULL),
('clash-meta',   'ws',                  true,  '1.18.0',  NULL),
('clash-meta',   'grpc',                true,  '1.18.0',  NULL),
('clash-meta',   'hysteria2',           true,  '1.18.0',  NULL),
('clash-meta',   'tuic_v5',             true,  '1.18.0',  NULL),

('sing-box',     'utls',                true,  '1.9.0',   NULL),
('sing-box',     'reality',             true,  '1.9.0',   NULL),
('sing-box',     'vless_encryption',    true,  '1.10.0',  '需服务端同版本以上'),
('sing-box',     'ech',                 true,  '1.11.0',  '实验性'),
('sing-box',     'xhttp',               true,  '1.10.0',  NULL),
('sing-box',     'ws',                  true,  '1.9.0',   NULL),
('sing-box',     'grpc',                true,  '1.9.0',   NULL),
('sing-box',     'hysteria2',           true,  '1.9.0',   NULL),
('sing-box',     'tuic_v5',             true,  '1.9.0',   NULL),

('v2rayn',       'utls',                true,  '6.0.0',   NULL),
('v2rayn',       'reality',             true,  '6.0.0',   NULL),
('v2rayn',       'vless_encryption',    false, NULL,       NULL),
('v2rayn',       'ech',                 false, NULL,       NULL),
('v2rayn',       'xhttp',               true,  '7.0.0',   NULL),
('v2rayn',       'ws',                  true,  '6.0.0',   NULL),
('v2rayn',       'hysteria2',           true,  '6.5.0',   NULL),
('v2rayn',       'tuic_v5',             true,  '6.5.0',   NULL),

('v2rayng',      'utls',                true,  '1.8.0',   NULL),
('v2rayng',      'reality',             true,  '1.8.0',   NULL),
('v2rayng',      'vless_encryption',    false, NULL,       NULL),
('v2rayng',      'ech',                 false, NULL,       NULL),
('v2rayng',      'ws',                  true,  '1.8.0',   NULL),
('v2rayng',      'hysteria2',           true,  '1.8.5',   NULL),
('v2rayng',      'tuic_v5',             false, NULL,       NULL),

('shadowrocket', 'utls',                true,  '2.2.0',   NULL),
('shadowrocket', 'reality',             true,  '2.2.0',   NULL),
('shadowrocket', 'vless_encryption',    false, NULL,       NULL),
('shadowrocket', 'ech',                 false, NULL,       NULL),
('shadowrocket', 'ws',                  true,  '2.2.0',   NULL),
('shadowrocket', 'hysteria2',           true,  '2.2.5',   NULL),
('shadowrocket', 'tuic_v5',             true,  '2.2.5',   NULL),

('hiddify',      'utls',                true,  '2.0.0',   NULL),
('hiddify',      'reality',             true,  '2.0.0',   NULL),
('hiddify',      'vless_encryption',    true,  '2.2.0',   '跟随 sing-box 能力'),
('hiddify',      'ech',                 true,  '2.3.0',   '实验性'),
('hiddify',      'ws',                  true,  '2.0.0',   NULL),
('hiddify',      'hysteria2',           true,  '2.0.0',   NULL),
('hiddify',      'tuic_v5',             true,  '2.0.0',   NULL)
ON CONFLICT (client_code, feature_code) DO NOTHING;

-- 暴露方式兼容矩阵
INSERT INTO exposure_compat_rules (protocol_type, transport_type, security_type, exposure_mode, is_allowed, reason) VALUES
('vless',  'tcp',      'reality',  'direct_public_ip',         true,  NULL),
('vless',  'tcp',      'reality',  'nginx_reverse_proxy',      false, 'REALITY 不兼容 Nginx 反代'),
('vless',  'tcp',      'reality',  'cloudflare_tunnel_fixed',  false, 'REALITY 不兼容 CF Tunnel'),
('vless',  'ws',       'tls',      'direct_public_ip',         true,  NULL),
('vless',  'ws',       'tls',      'nginx_reverse_proxy',      true,  NULL),
('vless',  'ws',       'tls',      'cloudflare_tunnel_fixed',  true,  NULL),
('vless',  'xhttp',    'tls',      'direct_public_ip',         true,  NULL),
('vless',  'xhttp',    'tls',      'nginx_reverse_proxy',      true,  NULL),
('vless',  'xhttp',    'tls',      'cloudflare_tunnel_fixed',  true,  NULL),
('trojan', 'ws',       'tls',      'nginx_reverse_proxy',      true,  NULL),
('trojan', 'ws',       'tls',      'cloudflare_tunnel_fixed',  true,  NULL),
('trojan', 'tcp',      'tls',      'direct_public_ip',         true,  NULL),
('trojan', 'tcp',      'tls',      'nginx_reverse_proxy',      true,  NULL),
('hysteria2', 'udp',   NULL,       'direct_public_ip',         true,  NULL),
('hysteria2', 'udp',   NULL,       'nginx_reverse_proxy',      false, 'Hysteria2 为 UDP，Nginx 不支持 UDP 代理'),
('hysteria2', 'udp',   NULL,       'cloudflare_tunnel_fixed',  false, 'CF Tunnel 不支持 UDP 协议'),
('tuic',   'udp',      NULL,       'direct_public_ip',         true,  NULL),
('tuic',   'udp',      NULL,       'cloudflare_tunnel_fixed',  false, 'CF Tunnel 不支持 UDP 协议'),
('shadowsocks', 'tcp', NULL,       'direct_public_ip',         true,  NULL),
('shadowsocks', 'ws',  'tls',      'nginx_reverse_proxy',      true,  NULL),
('shadowsocks', 'ws',  'tls',      'cloudflare_tunnel_fixed',  true,  NULL)
ON CONFLICT (protocol_type, transport_type, security_type, exposure_mode) DO NOTHING;

-- Node Doctor 检查项定义
INSERT INTO node_doctor_check_defs (code, name, check_category, severity, applicable_exposure_modes, auto_fix_available, sort_order) VALUES
('dns_resolve',           'DNS 解析检查',             'network',    'fail', '["*"]',                             false, 10),
('public_port_reachable', '公网端口连通性',            'network',    'fail', '["*"]',                             false, 20),
('tls_cert_valid',        'TLS 证书有效性',            'tls',        'fail', '["direct_public_ip","nginx_reverse_proxy"]', false, 30),
('tls_cert_expiry',       '证书到期时间（<21天）',     'tls',        'warn', '["direct_public_ip","nginx_reverse_proxy"]', true,  31),
('tls_sni_match',         'SNI 与证书 CN/SAN 匹配',   'tls',        'fail', '["direct_public_ip","nginx_reverse_proxy"]', false, 32),
('reality_config_valid',  'REALITY 配置完整性',        'tls',        'fail', '["direct_public_ip"]',             false, 40),
('nginx_upstream',        'Nginx 上游端口可达',         'nginx',      'fail', '["nginx_reverse_proxy"]',          false, 50),
('nginx_ws_path_match',   'Nginx WS 路径与节点一致',   'nginx',      'fail', '["nginx_reverse_proxy"]',          true,  51),
('nginx_host_header',     'Nginx Host 头配置',         'nginx',      'warn', '["nginx_reverse_proxy"]',          true,  52),
('cf_tunnel_online',      'CF Tunnel 在线状态',        'tunnel',     'fail', '["cloudflare_tunnel_fixed"]',      false, 60),
('cf_tunnel_hostname',    'CF Tunnel 域名解析',        'tunnel',     'fail', '["cloudflare_tunnel_fixed"]',      false, 61),
('cf_origin_reachable',   'CF Tunnel 源站可达',        'tunnel',     'fail', '["cloudflare_tunnel_fixed"]',      false, 62),
('xray_listen_port',      'Xray 本地监听端口',         'runtime',    'fail', '["*"]',                             false, 70),
('xray_process_running',  'Xray 进程运行状态',         'runtime',    'fail', '["*"]',                             true,  71),
('sub_render_sni',        '订阅 SNI 字段一致性',       'subscription','warn', '["*"]',                            true,  80),
('sub_render_port',       '订阅端口字段一致性',        'subscription','fail', '["*"]',                             true,  81),
('sub_render_path',       '订阅 WS 路径字段一致性',   'subscription','warn', '["nginx_reverse_proxy","cloudflare_tunnel_fixed"]', true, 82),
('port_conflict',         '端口占用冲突检测',          'system',     'fail', '["*"]',                             false, 90),
('warp_sidecar_status',   'WARP 出站可用性',           'outbound',   'warn', '["*"]',                             false, 100)
ON CONFLICT (code) DO NOTHING;

-- uTLS 指纹内置模板种子
INSERT INTO system_settings (setting_group, setting_key, value_json, description) VALUES
('node_defaults', 'utls_fingerprint_options',
 '["chrome","firefox","safari","edge","ios","android","randomized","qq"]'::jsonb,
 'uTLS 指纹选项列表'),
('node_defaults', 'default_utls_fingerprint',
 '"chrome"'::jsonb,
 '默认 uTLS 指纹'),
('node_defaults', 'ech_experimental_enabled',
 'false'::jsonb,
 'ECH 实验特性全局开关（需单节点再开启）'),
('cert_management', 'auto_renew_days_before',
 '21'::jsonb,
 '证书自动续期提前天数'),
('cert_management', 'renew_check_interval_hours',
 '12'::jsonb,
 '证书续期检查间隔（小时）')
ON CONFLICT (setting_group, setting_key) DO NOTHING;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM system_settings WHERE setting_group IN ('node_defaults','cert_management');
DELETE FROM node_doctor_check_defs WHERE code IN (
  'dns_resolve','public_port_reachable','tls_cert_valid','tls_cert_expiry',
  'tls_sni_match','reality_config_valid','nginx_upstream','nginx_ws_path_match',
  'nginx_host_header','cf_tunnel_online','cf_tunnel_hostname','cf_origin_reachable',
  'xray_listen_port','xray_process_running','sub_render_sni','sub_render_port',
  'sub_render_path','port_conflict','warp_sidecar_status'
);
DELETE FROM exposure_compat_rules;
DELETE FROM client_compat_matrix;
DELETE FROM client_profiles WHERE code IN (
  'clash-meta','sing-box','v2rayn','v2rayng',
  'shadowrocket','stash','hiddify','nekobox','loon'
);
-- +goose StatementEnd
