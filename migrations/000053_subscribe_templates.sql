-- +goose Up
-- +goose StatementBegin

-- 订阅模板表（按名称索引，对齐 Xboard v2_subscribe_templates）。
-- 与 subscription_templates（按 code+target_client）互补：本表供渲染器按内核/格式名
-- 直接取模板内容，简化 subscribe_template('singbox') 类调用。
CREATE TABLE IF NOT EXISTS subscribe_templates (
  id          UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
  name        VARCHAR(64)  NOT NULL UNIQUE,
  content     TEXT         NOT NULL DEFAULT '',
  is_builtin  BOOLEAN      NOT NULL DEFAULT false,
  enabled     BOOLEAN      NOT NULL DEFAULT true,
  created_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_subscribe_templates_name    ON subscribe_templates(name);
CREATE INDEX IF NOT EXISTS idx_subscribe_templates_enabled ON subscribe_templates(enabled) WHERE enabled = true;

-- 预置 Clash 基础模板（含 proxy-groups + rules 框架）
INSERT INTO subscribe_templates (name, content, is_builtin, enabled)
VALUES ('clash', $tmpl$# Clash 基础模板
mixed-port: 7890
allow-lan: false
mode: rule
log-level: info
external-controller: 127.0.0.1:9090

proxies: []

proxy-groups:
  - name: "🚀 节点选择"
    type: select
    proxies:
      - "♻️ 自动选择"
      - DIRECT
      - "🐟 漏网之鱼"
  - name: "♻️ 自动选择"
    type: url-test
    url: http://www.gstatic.com/generate_204
    interval: 300
    tolerance: 50
    proxies: []
  - name: "🐟 漏网之鱼"
    type: select
    proxies:
      - "🚀 节点选择"
      - DIRECT
  - name: "🛑 广告拦截"
    type: select
    proxies:
      - REJECT
      - DIRECT

rule-providers:
  reject:
    type: http
    behavior: domain
    url: https://raw.githubusercontent.com/Loyalsoldier/clash-rules/release/reject.txt
    path: ./ruleset/reject.yaml
    interval: 86400
  proxy:
    type: http
    behavior: domain
    url: https://raw.githubusercontent.com/Loyalsoldier/clash-rules/release/proxy.txt
    path: ./ruleset/proxy.yaml
    interval: 86400
  direct:
    type: http
    behavior: domain
    url: https://raw.githubusercontent.com/Loyalsoldier/clash-rules/release/direct.txt
    path: ./ruleset/direct.yaml
    interval: 86400
  cncidr:
    type: http
    behavior: ipcidr
    url: https://raw.githubusercontent.com/Loyalsoldier/clash-rules/release/cncidr.txt
    path: ./ruleset/cncidr.yaml
    interval: 86400

rules:
  - RULE-SET,reject,🛑 广告拦截
  - RULE-SET,direct,DIRECT
  - RULE-SET,proxy,🚀 节点选择
  - GEOIP,CN,DIRECT
  - MATCH,🐟 漏网之鱼
$tmpl$, true, true)
ON CONFLICT (name) DO NOTHING;

-- 预置 Clash Meta 模板
INSERT INTO subscribe_templates (name, content, is_builtin, enabled)
VALUES ('clashmeta', $tmpl$# Clash Meta 模板
mixed-port: 7890
allow-lan: false
mode: rule
log-level: info
unified-delay: true
external-controller: 127.0.0.1:9090
geodata-mode: true
geox-url:
  geoip: "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/release/geoip.dat"
  geosite: "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/release/geosite.dat"

proxies: []

proxy-groups:
  - name: "🚀 节点选择"
    type: select
    proxies:
      - "♻️ 自动选择"
      - DIRECT
  - name: "♻️ 自动选择"
    type: url-test
    url: http://www.gstatic.com/generate_204
    interval: 300
    tolerance: 50
    proxies: []
  - name: "🐟 漏网之鱼"
    type: select
    proxies:
      - "🚀 节点选择"
      - DIRECT
  - name: "🛑 广告拦截"
    type: select
    proxies:
      - REJECT
      - DIRECT

rule-providers:
  reject:
    type: http
    behavior: domain
    url: "https://raw.githubusercontent.com/Loyalsoldier/clash-rules/release/reject.txt"
    path: ./ruleset/reject.yaml
    interval: 86400
  proxy:
    type: http
    behavior: domain
    url: "https://raw.githubusercontent.com/Loyalsoldier/clash-rules/release/proxy.txt"
    path: ./ruleset/proxy.yaml
    interval: 86400
  direct:
    type: http
    behavior: domain
    url: "https://raw.githubusercontent.com/Loyalsoldier/clash-rules/release/direct.txt"
    path: ./ruleset/direct.yaml
    interval: 86400
  geosite-cn:
    type: http
    behavior: domain
    url: "https://raw.githubusercontent.com/Loyalsoldier/clash-rules/release/cn.txt"
    path: ./ruleset/cn.yaml
    interval: 86400
  geoip-cn:
    type: http
    behavior: ipcidr
    url: "https://raw.githubusercontent.com/Loyalsoldier/clash-rules/release/cncidr.txt"
    path: ./ruleset/cncidr.yaml
    interval: 86400

rules:
  - RULE-SET,reject,🛑 广告拦截
  - RULE-SET,direct,DIRECT
  - RULE-SET,geosite-cn,DIRECT
  - RULE-SET,proxy,🚀 节点选择
  - GEOIP,CN,DIRECT
  - MATCH,🐟 漏网之鱼
$tmpl$, true, true)
ON CONFLICT (name) DO NOTHING;

-- 预置 SingBox 模板（1.12+ 格式基准，渲染时按版本降级）
INSERT INTO subscribe_templates (name, content, is_builtin, enabled)
VALUES ('singbox', $tmpl${
  "log": {
    "level": "info",
    "timestamp": true
  },
  "dns": {
    "servers": [
      {
        "type": "https",
        "server": "1.1.1.1",
        "tag": "dns_proxy",
        "detour": "🚀 节点选择"
      },
      {
        "type": "https",
        "server": "doh.pub",
        "tag": "dns_direct",
        "detour": "direct"
      },
      {
        "type": "udp",
        "server": "223.5.5.5",
        "tag": "dns_local",
        "detour": "direct"
      }
    ],
    "rules": [
      {
        "rule_set": "geosite-cn",
        "server": "dns_direct"
      },
      {
        "rule_set": "geosite-category-ads-all",
        "action": "reject"
      }
    ],
    "final": "dns_proxy",
    "strategy": "prefer_ipv4"
  },
  "inbounds": [
    {
      "type": "mixed",
      "tag": "mixed-in",
      "listen": "127.0.0.1",
      "listen_port": 7890,
      "sniff": true,
      "domain_strategy": "prefer_ipv4"
    },
    {
      "type": "tun",
      "tag": "tun-in",
      "address": ["172.19.0.1/30", "fd00::1/126"],
      "auto_route": true,
      "stack": "mixed",
      "strict_route": true,
      "sniff": true,
      "domain_strategy": "prefer_ipv4"
    }
  ],
  "outbounds": [
    {
      "type": "selector",
      "tag": "🚀 节点选择",
      "outbounds": ["♻️ 自动选择", "direct"],
      "default": "♻️ 自动选择"
    },
    {
      "type": "urltest",
      "tag": "♻️ 自动选择",
      "outbounds": [],
      "url": "http://www.gstatic.com/generate_204",
      "interval": "5m",
      "tolerance": 50,
      "interrupt_exist_connections": true
    },
    {
      "type": "selector",
      "tag": "🐟 漏网之鱼",
      "outbounds": ["🚀 节点选择", "direct"]
    },
    {
      "type": "selector",
      "tag": "🛑 广告拦截",
      "outbounds": ["block", "direct"]
    },
    {
      "type": "direct",
      "tag": "direct"
    }
  ],
  "route": {
    "rule_set": [
      {
        "type": "remote",
        "tag": "geosite-cn",
        "format": "binary",
        "url": "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-cn.srs"
      },
      {
        "type": "remote",
        "tag": "geoip-cn",
        "format": "binary",
        "url": "https://raw.githubusercontent.com/SagerNet/sing-geoip/rule-set/geoip-cn.srs"
      },
      {
        "type": "remote",
        "tag": "geosite-category-ads-all",
        "format": "binary",
        "url": "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-category-ads-all.srs"
      }
    ],
    "rules": [
      {
        "action": "hijack-dns",
        "protocol": "dns"
      },
      {
        "action": "reject",
        "rule_set": "geosite-category-ads-all"
      },
      {
        "rule_set": ["geosite-cn", "geoip-cn"],
        "outbound": "direct"
      },
      {
        "ip_is_private": true,
        "outbound": "direct"
      },
      {
        "match": true,
        "outbound": "🐟 漏网之鱼"
      }
    ],
    "final": "🚀 节点选择",
    "auto_detect_interface": true
  },
  "experimental": {
    "cache_file": {
      "enabled": true
    }
  }
}
$tmpl$, true, true)
ON CONFLICT (name) DO NOTHING;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS subscribe_templates;
-- +goose StatementEnd
