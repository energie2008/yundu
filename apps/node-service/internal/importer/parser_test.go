package importer

import (
	"testing"
)

func TestXrayConfigParser_ExtractsInboundProtocolAndPort(t *testing.T) {
	// 最小 Xray config.json
	content := `{
		"inbounds": [
			{
				"protocol": "vless",
				"port": 443,
				"settings": {
					"clients": [
						{"id": "b831381d-6324-4d53-ad4f-8cda48b30811"},
						{"id": "f1c5f2d3-1234-5678-9abc-def012345678"}
					]
				},
				"streamSettings": {
					"network": "ws",
					"security": "tls",
					"tlsSettings": {
						"certificates": [
							{"certificateFile": "/etc/xray/cert/fullchain.pem"}
						]
					},
					"wsSettings": {
						"path": "/ws"
					}
				}
			}
		]
	}`

	spec, err := XrayConfigParser(content)
	if err != nil {
		t.Fatalf("XrayConfigParser failed: %v", err)
	}
	if spec.ProtocolType != "vless" {
		t.Errorf("ProtocolType = %q, want vless", spec.ProtocolType)
	}
	if spec.ListenPort != 443 {
		t.Errorf("ListenPort = %d, want 443", spec.ListenPort)
	}
	if spec.TransportType != "ws" {
		t.Errorf("TransportType = %q, want ws", spec.TransportType)
	}
	if spec.SecurityType != "tls" {
		t.Errorf("SecurityType = %q, want tls", spec.SecurityType)
	}
	if len(spec.UUIDs) != 2 {
		t.Fatalf("len(UUIDs) = %d, want 2", len(spec.UUIDs))
	}
	if spec.UUIDs[0] != "b831381d-6324-4d53-ad4f-8cda48b30811" {
		t.Errorf("UUIDs[0] = %q", spec.UUIDs[0])
	}
	if spec.CertPath != "/etc/xray/cert/fullchain.pem" {
		t.Errorf("CertPath = %q, want /etc/xray/cert/fullchain.pem", spec.CertPath)
	}
	// vless + tls + 有证书路径：不应报缺失
	if len(spec.MissingFields) != 0 {
		t.Errorf("MissingFields = %v, want empty", spec.MissingFields)
	}
}

func TestXrayConfigParser_MissingUUID_ReportsMissingField(t *testing.T) {
	content := `{
		"inbounds": [
			{
				"protocol": "vmess",
				"port": 8080,
				"settings": {"clients": []},
				"streamSettings": {"network": "tcp", "security": "none"}
			}
		]
	}`
	spec, err := XrayConfigParser(content)
	if err != nil {
		t.Fatalf("XrayConfigParser failed: %v", err)
	}
	if spec.ProtocolType != "vmess" {
		t.Errorf("ProtocolType = %q, want vmess", spec.ProtocolType)
	}
	found := false
	for _, f := range spec.MissingFields {
		if f == "uuids" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'uuids' in MissingFields, got %v", spec.MissingFields)
	}
}

func TestSingBoxConfigParser_ExtractsTypeAndPort(t *testing.T) {
	content := `{
		"inbounds": [
			{
				"type": "hysteria2",
				"listen_port": 8443,
				"tls": {"enabled": true, "server_name": "example.com"}
			}
		]
	}`
	spec, err := SingBoxConfigParser(content)
	if err != nil {
		t.Fatalf("SingBoxConfigParser failed: %v", err)
	}
	if spec.ProtocolType != "hysteria2" {
		t.Errorf("ProtocolType = %q, want hysteria2", spec.ProtocolType)
	}
	if spec.ListenPort != 8443 {
		t.Errorf("ListenPort = %d, want 8443", spec.ListenPort)
	}
	if spec.TransportType != "udp" {
		t.Errorf("TransportType = %q, want udp", spec.TransportType)
	}
	if spec.SecurityType != "tls" {
		t.Errorf("SecurityType = %q, want tls", spec.SecurityType)
	}
	if spec.SNI != "example.com" {
		t.Errorf("SNI = %q, want example.com", spec.SNI)
	}
}

func TestNginxConfParser_ExtractsServerNameAndListen(t *testing.T) {
	content := `server {
		listen 443 ssl http2;
		server_name example.com;
		ssl_certificate /etc/nginx/ssl/example.crt;
		location /ws {
			proxy_pass http://127.0.0.1:10000;
		}
	}`
	spec, err := NginxConfParser(content)
	if err != nil {
		t.Fatalf("NginxConfParser failed: %v", err)
	}
	if spec.SNI != "example.com" {
		t.Errorf("SNI = %q, want example.com", spec.SNI)
	}
	if spec.ListenPort != 443 {
		t.Errorf("ListenPort = %d, want 443", spec.ListenPort)
	}
	if spec.CertPath != "/etc/nginx/ssl/example.crt" {
		t.Errorf("CertPath = %q", spec.CertPath)
	}
	if spec.SecurityType != "tls" {
		t.Errorf("SecurityType = %q, want tls", spec.SecurityType)
	}
	if spec.TransportType != "ws" {
		t.Errorf("TransportType = %q, want ws", spec.TransportType)
	}
	locations, ok := spec.RawMetadata["locations"].([]map[string]interface{})
	if !ok || len(locations) != 1 {
		t.Fatalf("expected 1 location, got %v", spec.RawMetadata["locations"])
	}
	if locations[0]["path"] != "/ws" {
		t.Errorf("location path = %v", locations[0]["path"])
	}
}

func TestCloudflaredConfigParser_ExtractsTunnelAndIngress(t *testing.T) {
	content := `tunnel:
  id: abc-123
  token: secret-token-value
credentials-file: /etc/cloudflared/credentials.json
ingress:
  - hostname: example.com
    service: http://127.0.0.1:10000
  - service: http_status:404
`
	spec, err := CloudflaredConfigParser(content)
	if err != nil {
		t.Fatalf("CloudflaredConfigParser failed: %v", err)
	}
	if spec.SNI != "example.com" {
		t.Errorf("SNI = %q, want example.com", spec.SNI)
	}
	token, _ := spec.RawMetadata["tunnel_token"].(string)
	if token != "secret-token-value" {
		t.Errorf("tunnel_token = %q, want secret-token-value", token)
	}
	ingress, ok := spec.RawMetadata["ingress"].([]map[string]interface{})
	if !ok || len(ingress) != 2 {
		t.Fatalf("expected 2 ingress entries, got %v", spec.RawMetadata["ingress"])
	}
	if ingress[0]["hostname"] != "example.com" {
		t.Errorf("ingress[0].hostname = %v", ingress[0]["hostname"])
	}
	if ingress[0]["service"] != "http://127.0.0.1:10000" {
		t.Errorf("ingress[0].service = %v", ingress[0]["service"])
	}
}

func TestParserForSourceType_Unsupported(t *testing.T) {
	_, err := ParserForSourceType("unknown")
	if err == nil {
		t.Errorf("expected error for unsupported source type")
	}
}

func TestBuildNodeSpec_MergesMultipleSpecs(t *testing.T) {
	spec1 := &NodeSpec{ProtocolType: "vless", ListenPort: 443, UUIDs: []string{"uuid-1"}}
	spec2 := &NodeSpec{TransportType: "ws", SecurityType: "tls", SNI: "example.com", CertPath: "/c.pem"}
	merged := BuildNodeSpec([]*NodeSpec{spec1, spec2})
	if merged.ProtocolType != "vless" {
		t.Errorf("ProtocolType = %q", merged.ProtocolType)
	}
	if merged.TransportType != "ws" {
		t.Errorf("TransportType = %q", merged.TransportType)
	}
	if merged.SecurityType != "tls" {
		t.Errorf("SecurityType = %q", merged.SecurityType)
	}
	if merged.ListenPort != 443 {
		t.Errorf("ListenPort = %d", merged.ListenPort)
	}
	if len(merged.UUIDs) != 1 || merged.UUIDs[0] != "uuid-1" {
		t.Errorf("UUIDs = %v", merged.UUIDs)
	}
	if merged.SNI != "example.com" {
		t.Errorf("SNI = %q", merged.SNI)
	}
}
