-- +goose Up
-- +goose StatementBegin

-- P0-6: 内核能力矩阵表
-- 记录每个内核（xray/sing-box）对各协议+传输+安全+特性组合的支持级别
CREATE TABLE IF NOT EXISTS kernel_capabilities (
    id BIGSERIAL PRIMARY KEY,
    kernel VARCHAR(16) NOT NULL,          -- xray / sing-box
    min_version VARCHAR(32),              -- 最低支持版本（NULL = 任意版本）
    max_version VARCHAR(32),              -- 最高支持版本（NULL = 当前最新）
    protocol VARCHAR(32) NOT NULL,        -- vless / vmess / trojan / shadowsocks / hysteria2 / tuic / anytls / mieru
    transport VARCHAR(32) NOT NULL,       -- tcp / ws / grpc / http / httpupgrade / xhttp / quic
    security VARCHAR(32) NOT NULL,        -- none / tls / reality
    feature VARCHAR(64) NOT NULL DEFAULT '*',  -- 特性名（* = 通配，vision / xmux / split / packet-up / stream-up / ech）
    support_level VARCHAR(16) NOT NULL,   -- native / degradable / blocked
    downgrade_to JSONB,                   -- 降级目标（如 xmux→mux）
    message TEXT,                         -- 说明信息
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(kernel, protocol, transport, security, feature)
);

CREATE INDEX idx_kernel_capabilities_lookup ON kernel_capabilities(kernel, protocol, transport, security);

-- P0-6: 证书包表（PEM-only 证书总线）
-- 替代 config_json 中的 cert_file/key_file 路径引用
CREATE TABLE IF NOT EXISTS cert_bundles (
    id UUID PRIMARY KEY,
    provider VARCHAR(32) NOT NULL,        -- acme / self / content / cf-origin
    mode VARCHAR(16) NOT NULL,            -- file / content / acme / self
    cert_pem TEXT NOT NULL,               -- 证书 PEM 内容
    key_pem TEXT NOT NULL,                -- 私钥 PEM 内容
    san JSONB NOT NULL DEFAULT '[]',      -- SAN 列表
    not_after TIMESTAMPTZ,                -- 过期时间
    version INT NOT NULL DEFAULT 1,       -- 版本号（证书续期后递增）
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_cert_bundles_provider ON cert_bundles(provider);
CREATE INDEX idx_cert_bundles_not_after ON cert_bundles(not_after);

-- +goose StatementEnd

-- P0-6: 种子数据——Xray 能力矩阵
-- 基于 Xray 26.3+（项目固定版本，支持最多协议）
INSERT INTO kernel_capabilities (kernel, min_version, protocol, transport, security, feature, support_level, message) VALUES
-- VLESS
('xray', '26.3', 'vless', 'tcp', 'none', '*', 'native', 'VLESS over TCP'),
('xray', '26.3', 'vless', 'tcp', 'tls', '*', 'native', 'VLESS+TLS over TCP'),
('xray', '26.3', 'vless', 'tcp', 'reality', '*', 'native', 'VLESS+REALITY over TCP'),
('xray', '26.3', 'vless', 'tcp', 'reality', 'vision', 'native', 'REALITY Vision flow'),
('xray', '26.3', 'vless', 'ws', 'tls', '*', 'native', 'VLESS+WS+TLS'),
('xray', '26.3', 'vless', 'ws', 'none', '*', 'native', 'VLESS+WS (CDN)'),
('xray', '26.3', 'vless', 'grpc', 'tls', '*', 'native', 'VLESS+gRPC+TLS'),
('xray', '26.3', 'vless', 'grpc', 'none', '*', 'native', 'VLESS+gRPC (CDN)'),
('xray', '26.3', 'vless', 'httpupgrade', 'tls', '*', 'native', 'VLESS+HTTPUpgrade+TLS'),
('xray', '26.3', 'vless', 'httpupgrade', 'none', '*', 'native', 'VLESS+HTTPUpgrade (CDN)'),
('xray', '26.3', 'vless', 'xhttp', 'tls', '*', 'native', 'VLESS+XHTTP+TLS'),
('xray', '26.3', 'vless', 'xhttp', 'none', '*', 'native', 'VLESS+XHTTP (CDN)'),
('xray', '26.3', 'vless', 'xhttp', 'tls', 'packet-up', 'native', 'XHTTP packet-up mode (CDN 推荐)'),
('xray', '26.3', 'vless', 'xhttp', 'tls', 'stream-up', 'native', 'XHTTP stream-up mode (直连+REALITY)'),
('xray', '26.3', 'vless', 'xhttp', 'reality', '*', 'native', 'VLESS+XHTTP+REALITY'),
('xray', '26.3', 'vless', 'xhttp', 'reality', 'stream-up', 'native', 'XHTTP+REALITY stream-up (直连推荐)'),
('xray', '26.3', 'vless', 'xhttp', 'tls', 'auto', 'blocked', 'XHTTP mode auto 禁止使用（连接不稳定）'),
-- VMess
('xray', '26.3', 'vmess', 'tcp', 'none', '*', 'native', 'VMess over TCP'),
('xray', '26.3', 'vmess', 'tcp', 'tls', '*', 'native', 'VMess+TLS over TCP'),
('xray', '26.3', 'vmess', 'ws', 'tls', '*', 'native', 'VMess+WS+TLS'),
('xray', '26.3', 'vmess', 'ws', 'none', '*', 'native', 'VMess+WS (CDN)'),
('xray', '26.3', 'vmess', 'grpc', 'tls', '*', 'native', 'VMess+gRPC+TLS'),
('xray', '26.3', 'vmess', 'httpupgrade', 'tls', '*', 'native', 'VMess+HTTPUpgrade+TLS'),
-- Trojan
('xray', '26.3', 'trojan', 'tcp', 'tls', '*', 'native', 'Trojan+TLS over TCP'),
('xray', '26.3', 'trojan', 'ws', 'tls', '*', 'native', 'Trojan+WS+TLS'),
('xray', '26.3', 'trojan', 'grpc', 'tls', '*', 'native', 'Trojan+gRPC+TLS'),
('xray', '26.3', 'trojan', 'xhttp', 'tls', '*', 'native', 'Trojan+XHTTP+TLS'),
-- Shadowsocks
('xray', '26.3', 'shadowsocks', 'tcp', 'none', '*', 'native', 'SS over TCP'),
('xray', '26.3', 'shadowsocks', 'tcp', 'tls', '*', 'native', 'SS+TLS'),
('xray', '26.3', 'shadowsocks', 'tcp', 'reality', '*', 'native', 'SS+REALITY'),
-- Hysteria2
('xray', '26.3', 'hysteria2', 'quic', 'tls', '*', 'native', 'Hysteria2 over QUIC'),
-- 不支持的协议
('xray', NULL, 'tuic', 'quic', 'tls', '*', 'blocked', 'Xray 不支持 TUIC'),
('xray', NULL, 'anytls', '*', '*', '*', 'blocked', 'Xray 不支持 AnyTLS'),
('xray', NULL, 'mieru', '*', '*', '*', 'blocked', 'Xray 不支持 Mieru'),
-- ECH
('xray', NULL, '*', '*', 'tls', 'ech', 'blocked', 'Xray 不支持 ECH')
ON CONFLICT (kernel, protocol, transport, security, feature) DO NOTHING;

-- P0-6: 种子数据——sing-box 能力矩阵
INSERT INTO kernel_capabilities (kernel, min_version, protocol, transport, security, feature, support_level, message) VALUES
-- VLESS
('sing-box', '1.8', 'vless', 'tcp', 'none', '*', 'native', 'VLESS over TCP'),
('sing-box', '1.8', 'vless', 'tcp', 'tls', '*', 'native', 'VLESS+TLS over TCP'),
('sing-box', '1.8', 'vless', 'tcp', 'reality', '*', 'native', 'VLESS+REALITY over TCP'),
('sing-box', '1.8', 'vless', 'tcp', 'reality', 'vision', 'native', 'REALITY Vision flow'),
('sing-box', '1.8', 'vless', 'ws', 'tls', '*', 'native', 'VLESS+WS+TLS'),
('sing-box', '1.8', 'vless', 'ws', 'none', '*', 'native', 'VLESS+WS (CDN)'),
('sing-box', '1.8', 'vless', 'grpc', 'tls', '*', 'native', 'VLESS+gRPC+TLS'),
('sing-box', '1.8', 'vless', 'httpupgrade', 'tls', '*', 'native', 'VLESS+HTTPUpgrade+TLS'),
-- sing-box 不支持 XHTTP
('sing-box', NULL, 'vless', 'xhttp', '*', '*', 'blocked', 'sing-box 不支持 XHTTP'),
('sing-box', NULL, '*', 'xhttp', '*', 'xmux', 'blocked', 'sing-box 不支持 XMUX'),
('sing-box', NULL, '*', 'xhttp', '*', 'split', 'blocked', 'sing-box 不支持 XHTTP split'),
-- VMess
('sing-box', '1.8', 'vmess', 'tcp', 'none', '*', 'native', 'VMess over TCP'),
('sing-box', '1.8', 'vmess', 'tcp', 'tls', '*', 'native', 'VMess+TLS over TCP'),
('sing-box', '1.8', 'vmess', 'ws', 'tls', '*', 'native', 'VMess+WS+TLS'),
('sing-box', '1.8', 'vmess', 'grpc', 'tls', '*', 'native', 'VMess+gRPC+TLS'),
-- Trojan
('sing-box', '1.8', 'trojan', 'tcp', 'tls', '*', 'native', 'Trojan+TLS over TCP'),
('sing-box', '1.8', 'trojan', 'ws', 'tls', '*', 'native', 'Trojan+WS+TLS'),
('sing-box', '1.8', 'trojan', 'grpc', 'tls', '*', 'native', 'Trojan+gRPC+TLS'),
-- Shadowsocks
('sing-box', '1.8', 'shadowsocks', 'tcp', 'none', '*', 'native', 'SS over TCP'),
('sing-box', '1.8', 'shadowsocks', 'tcp', 'tls', '*', 'native', 'SS+TLS'),
-- Hysteria2
('sing-box', '1.8', 'hysteria2', 'quic', 'tls', '*', 'native', 'Hysteria2 over QUIC'),
-- TUIC
('sing-box', '1.8', 'tuic', 'quic', 'tls', '*', 'native', 'TUIC over QUIC'),
-- AnyTLS
('sing-box', '1.10', 'anytls', 'tcp', 'tls', '*', 'native', 'AnyTLS+TLS'),
-- ECH
('sing-box', '1.8', '*', '*', 'tls', 'ech', 'native', 'sing-box 支持 ECH')
ON CONFLICT (kernel, protocol, transport, security, feature) DO NOTHING;

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS cert_bundles;
DROP TABLE IF EXISTS kernel_capabilities;
-- +goose StatementEnd
