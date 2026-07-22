package importer

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestParseVLESSWS(t *testing.T) {
	uri := "vless://bda53aa0-e18b-4429-a85b-e827df61d9a0@162.159.160.46:443?encryption=none&security=tls&sni=doubao.yundu.space&fp=ios&insecure=0&allowInsecure=0&type=ws&host=doubao.yundu.space&path=%2Fdoubaobda53aa0-e18b-4429-a85b#gogogo"

	spec, err := ParseURI(uri)
	if err != nil {
		t.Fatalf("ParseURI failed: %v", err)
	}

	if spec.ProtocolType != "vless" {
		t.Errorf("ProtocolType = %q, want vless", spec.ProtocolType)
	}
	if spec.TransportType != "ws" {
		t.Errorf("TransportType = %q, want ws", spec.TransportType)
	}
	if spec.SecurityType != "tls" {
		t.Errorf("SecurityType = %q, want tls", spec.SecurityType)
	}
	if spec.Name != "gogogo" {
		t.Errorf("Name = %q, want gogogo", spec.Name)
	}
	if spec.Host != "162.159.160.46" {
		t.Errorf("Host = %q, want 162.159.160.46", spec.Host)
	}
	if spec.Port != 443 {
		t.Errorf("Port = %d, want 443", spec.Port)
	}
	if spec.UUID != "bda53aa0-e18b-4429-a85b-e827df61d9a0" {
		t.Errorf("UUID = %q, want bda53aa0-e18b-4429-a85b-e827df61d9a0", spec.UUID)
	}

	tlsCfg, ok := spec.ConfigJSON["tls"].(map[string]interface{})
	if !ok {
		t.Fatal("ConfigJSON.tls is missing")
	}
	if tlsCfg["server_name"] != "doubao.yundu.space" {
		t.Errorf("tls.server_name = %q, want doubao.yundu.space", tlsCfg["server_name"])
	}
	if tlsCfg["fingerprint"] != "ios" {
		t.Errorf("tls.fingerprint = %q, want ios", tlsCfg["fingerprint"])
	}
	if insecure, ok := tlsCfg["allow_insecure"].(bool); !ok || insecure {
		t.Errorf("tls.allow_insecure = %v, want false", tlsCfg["allow_insecure"])
	}

	if spec.ConfigJSON["ws_path"] != "/doubaobda53aa0-e18b-4429-a85b" {
		t.Errorf("ws_path = %q", spec.ConfigJSON["ws_path"])
	}
	if spec.ConfigJSON["ws_host"] != "doubao.yundu.space" {
		t.Errorf("ws_host = %q", spec.ConfigJSON["ws_host"])
	}
}

func TestParseTrojanTCPTLS(t *testing.T) {
	uri := "trojan://9acd564f-c4b7-4ab7-81c0-ed0a1b715757@c0165d.douyincdn.25hp9v9.top:41696?security=tls&sni=cn-hnzz-cm-01-01.bilivideo.com&fp=chrome&alpn=h2%2Chttp%2F1.1&insecure=1&allowInsecure=1&type=tcp&headerType=none#新加坡02中转"

	spec, err := ParseURI(uri)
	if err != nil {
		t.Fatalf("ParseURI failed: %v", err)
	}

	if spec.ProtocolType != "trojan" {
		t.Errorf("ProtocolType = %q, want trojan", spec.ProtocolType)
	}
	if spec.TransportType != "tcp" {
		t.Errorf("TransportType = %q, want tcp", spec.TransportType)
	}
	if spec.SecurityType != "tls" {
		t.Errorf("SecurityType = %q, want tls", spec.SecurityType)
	}
	if spec.Name != "新加坡02中转" {
		t.Errorf("Name = %q, want 新加坡02中转", spec.Name)
	}
	if spec.Host != "c0165d.douyincdn.25hp9v9.top" {
		t.Errorf("Host = %q", spec.Host)
	}
	if spec.Port != 41696 {
		t.Errorf("Port = %d, want 41696", spec.Port)
	}
	if spec.Password != "9acd564f-c4b7-4ab7-81c0-ed0a1b715757" {
		t.Errorf("Password = %q", spec.Password)
	}

	tlsCfg, ok := spec.ConfigJSON["tls"].(map[string]interface{})
	if !ok {
		t.Fatal("ConfigJSON.tls is missing")
	}
	if tlsCfg["server_name"] != "cn-hnzz-cm-01-01.bilivideo.com" {
		t.Errorf("tls.server_name = %q", tlsCfg["server_name"])
	}
	if tlsCfg["fingerprint"] != "chrome" {
		t.Errorf("tls.fingerprint = %q", tlsCfg["fingerprint"])
	}
	alpn, ok := tlsCfg["alpn"].([]string)
	if !ok || len(alpn) != 2 {
		t.Fatalf("tls.alpn = %v, want [h2 http/1.1]", tlsCfg["alpn"])
	}
	if alpn[0] != "h2" || alpn[1] != "http/1.1" {
		t.Errorf("tls.alpn = %v", alpn)
	}
	if insecure, ok := tlsCfg["allow_insecure"].(bool); !ok || !insecure {
		t.Errorf("tls.allow_insecure = %v, want true", tlsCfg["allow_insecure"])
	}

	if spec.ConfigJSON["tcp_header_type"] != "none" {
		t.Errorf("tcp_header_type = %q, want none", spec.ConfigJSON["tcp_header_type"])
	}
}

func TestParseVLESSReality(t *testing.T) {
	uri := "vless://test-uuid@example.com:443?security=reality&sni=example.com&fp=chrome&pbk=public-key-here&sid=01234567&spx=%2F&type=tcp&flow=xtls-rprx-vision#Reality-Node"

	spec, err := ParseURI(uri)
	if err != nil {
		t.Fatalf("ParseURI failed: %v", err)
	}

	if spec.ProtocolType != "vless" {
		t.Errorf("ProtocolType = %q", spec.ProtocolType)
	}
	if spec.SecurityType != "reality" {
		t.Errorf("SecurityType = %q, want reality", spec.SecurityType)
	}
	if spec.Name != "Reality-Node" {
		t.Errorf("Name = %q", spec.Name)
	}

	realityCfg, ok := spec.ConfigJSON["reality"].(map[string]interface{})
	if !ok {
		t.Fatal("ConfigJSON.reality is missing")
	}
	if realityCfg["public_key"] != "public-key-here" {
		t.Errorf("reality.public_key = %q", realityCfg["public_key"])
	}
	shortIDs, ok := realityCfg["short_ids"].([]string)
	if !ok || len(shortIDs) == 0 || shortIDs[0] != "01234567" {
		t.Errorf("reality.short_ids = %v", realityCfg["short_ids"])
	}
	if realityCfg["spider_x"] != "/" {
		t.Errorf("reality.spider_x = %q", realityCfg["spider_x"])
	}
	if spec.ConfigJSON["flow"] != "xtls-rprx-vision" {
		t.Errorf("flow = %q", spec.ConfigJSON["flow"])
	}
}

func TestParseHysteria2(t *testing.T) {
	uri := "hysteria2://mypassword@example.com:443?sni=example.com&insecure=0&obfs=salamander&obfs-password=obfskey&up=100&down=100#Hysteria2-Node"

	spec, err := ParseURI(uri)
	if err != nil {
		t.Fatalf("ParseURI failed: %v", err)
	}

	if spec.ProtocolType != "hysteria2" {
		t.Errorf("ProtocolType = %q", spec.ProtocolType)
	}
	if spec.TransportType != "udp" {
		t.Errorf("TransportType = %q, want udp", spec.TransportType)
	}
	if spec.SecurityType != "tls" {
		t.Errorf("SecurityType = %q, want tls", spec.SecurityType)
	}
	if spec.Name != "Hysteria2-Node" {
		t.Errorf("Name = %q", spec.Name)
	}
	if spec.Host != "example.com" {
		t.Errorf("Host = %q", spec.Host)
	}
	if spec.Port != 443 {
		t.Errorf("Port = %d", spec.Port)
	}
	if spec.Password != "mypassword" {
		t.Errorf("Password = %q", spec.Password)
	}

	tlsCfg, ok := spec.ConfigJSON["tls"].(map[string]interface{})
	if !ok {
		t.Fatal("ConfigJSON.tls is missing")
	}
	alpn, ok := tlsCfg["alpn"].([]string)
	if !ok || len(alpn) == 0 || alpn[0] != "h3" {
		t.Errorf("tls.alpn = %v, want [h3]", tlsCfg["alpn"])
	}

	obfsCfg, ok := spec.ConfigJSON["obfs"].(map[string]interface{})
	if !ok {
		t.Fatal("ConfigJSON.obfs is missing")
	}
	if obfsCfg["type"] != "salamander" {
		t.Errorf("obfs.type = %q", obfsCfg["type"])
	}
	if obfsCfg["password"] != "obfskey" {
		t.Errorf("obfs.password = %q", obfsCfg["password"])
	}

	if spec.ConfigJSON["up_mbps"] != 100 {
		t.Errorf("up_mbps = %v", spec.ConfigJSON["up_mbps"])
	}
	if spec.ConfigJSON["down_mbps"] != 100 {
		t.Errorf("down_mbps = %v", spec.ConfigJSON["down_mbps"])
	}
}

func TestParseTUIC(t *testing.T) {
	uri := "tuic://uuid-here:password@example.com:443?sni=example.com&congestion_control=bbr&udp_relay_mode=native#TUIC-Node"

	spec, err := ParseURI(uri)
	if err != nil {
		t.Fatalf("ParseURI failed: %v", err)
	}

	if spec.ProtocolType != "tuic" {
		t.Errorf("ProtocolType = %q", spec.ProtocolType)
	}
	if spec.TransportType != "udp" {
		t.Errorf("TransportType = %q, want udp", spec.TransportType)
	}
	if spec.SecurityType != "tls" {
		t.Errorf("SecurityType = %q, want tls", spec.SecurityType)
	}
	if spec.Name != "TUIC-Node" {
		t.Errorf("Name = %q", spec.Name)
	}
	if spec.Host != "example.com" {
		t.Errorf("Host = %q", spec.Host)
	}
	if spec.Port != 443 {
		t.Errorf("Port = %d", spec.Port)
	}
	if spec.UUID != "uuid-here" {
		t.Errorf("UUID = %q", spec.UUID)
	}
	if spec.Password != "password" {
		t.Errorf("Password = %q", spec.Password)
	}

	tlsCfg, ok := spec.ConfigJSON["tls"].(map[string]interface{})
	if !ok {
		t.Fatal("ConfigJSON.tls is missing")
	}
	alpn, ok := tlsCfg["alpn"].([]string)
	if !ok || len(alpn) == 0 || alpn[0] != "h3" {
		t.Errorf("tls.alpn = %v, want [h3]", tlsCfg["alpn"])
	}

	if spec.ConfigJSON["zero_rtt_handshake"] != true {
		t.Errorf("zero_rtt_handshake should be true")
	}
	if spec.ConfigJSON["congestion_control"] != "bbr" {
		t.Errorf("congestion_control = %v", spec.ConfigJSON["congestion_control"])
	}
	if spec.ConfigJSON["udp_relay_mode"] != "native" {
		t.Errorf("udp_relay_mode = %v", spec.ConfigJSON["udp_relay_mode"])
	}
}

func TestParseShadowsocks(t *testing.T) {
	userInfo := "chacha20-ietf-poly1305:mypassword"
	encodedUser := base64.StdEncoding.EncodeToString([]byte(userInfo))
	uri := "ss://" + encodedUser + "@example.com:8388#SS-Node"

	spec, err := ParseURI(uri)
	if err != nil {
		t.Fatalf("ParseURI failed: %v", err)
	}

	if spec.ProtocolType != "ss" {
		t.Errorf("ProtocolType = %q", spec.ProtocolType)
	}
	if spec.Name != "SS-Node" {
		t.Errorf("Name = %q", spec.Name)
	}
	if spec.Host != "example.com" {
		t.Errorf("Host = %q", spec.Host)
	}
	if spec.Port != 8388 {
		t.Errorf("Port = %d", spec.Port)
	}
	if spec.Password != "mypassword" {
		t.Errorf("Password = %q", spec.Password)
	}
	if spec.ConfigJSON["method"] != "chacha20-ietf-poly1305" {
		t.Errorf("method = %q", spec.ConfigJSON["method"])
	}
}

func TestParseVMessJSON(t *testing.T) {
	vmessObj := map[string]string{
		"v":    "2",
		"ps":   "VMess-Node",
		"add":  "example.com",
		"port": "443",
		"id":   "vmess-uuid-here",
		"aid":  "0",
		"scy":  "auto",
		"net":  "ws",
		"type": "none",
		"host": "example.com",
		"path": "/ws",
		"tls":  "tls",
		"sni":  "example.com",
		"alpn": "h2,http/1.1",
		"fp":   "chrome",
	}
	data, _ := json.Marshal(vmessObj)
	encoded := base64.StdEncoding.EncodeToString(data)
	uri := "vmess://" + encoded

	spec, err := ParseURI(uri)
	if err != nil {
		t.Fatalf("ParseURI failed: %v", err)
	}

	if spec.ProtocolType != "vmess" {
		t.Errorf("ProtocolType = %q", spec.ProtocolType)
	}
	if spec.Name != "VMess-Node" {
		t.Errorf("Name = %q", spec.Name)
	}
	if spec.Host != "example.com" {
		t.Errorf("Host = %q", spec.Host)
	}
	if spec.Port != 443 {
		t.Errorf("Port = %d", spec.Port)
	}
	if spec.UUID != "vmess-uuid-here" {
		t.Errorf("UUID = %q", spec.UUID)
	}
	if spec.TransportType != "ws" {
		t.Errorf("TransportType = %q, want ws", spec.TransportType)
	}
	if spec.SecurityType != "tls" {
		t.Errorf("SecurityType = %q, want tls", spec.SecurityType)
	}

	tlsCfg, ok := spec.ConfigJSON["tls"].(map[string]interface{})
	if !ok {
		t.Fatal("ConfigJSON.tls is missing")
	}
	if tlsCfg["server_name"] != "example.com" {
		t.Errorf("tls.server_name = %q", tlsCfg["server_name"])
	}

	if spec.ConfigJSON["ws_path"] != "/ws" {
		t.Errorf("ws_path = %q", spec.ConfigJSON["ws_path"])
	}
	if spec.ConfigJSON["ws_host"] != "example.com" {
		t.Errorf("ws_host = %q", spec.ConfigJSON["ws_host"])
	}
}

func TestParseMultiple(t *testing.T) {
	content := `# This is a comment
vless://uuid1@host1:443?encryption=none&security=tls&type=ws&host=host1&path=%2Fpath1#Node1

trojan://pass1@host2:443?security=tls&type=tcp#Node2

invalid-uri

ss://Y2hhY2hhMjAtaWV0Zi1wb2x5MTMwNTpwYXNz@host3:8388#Node3
`

	nodes, errs := ParseURIs(content)
	if len(nodes) != 3 {
		t.Errorf("expected 3 nodes, got %d", len(nodes))
	}
	if len(errs) != 1 {
		t.Errorf("expected 1 error, got %d", len(errs))
	}
	if nodes[0].ProtocolType != "vless" || nodes[0].Name != "Node1" {
		t.Errorf("nodes[0] = %v", nodes[0])
	}
	if nodes[1].ProtocolType != "trojan" || nodes[1].Name != "Node2" {
		t.Errorf("nodes[1] = %v", nodes[1])
	}
	if nodes[2].ProtocolType != "ss" || nodes[2].Name != "Node3" {
		t.Errorf("nodes[2] = %v", nodes[2])
	}
}

func TestRenderVLESSWS(t *testing.T) {
	originalURI := "vless://bda53aa0-e18b-4429-a85b-e827df61d9a0@162.159.160.46:443?encryption=none&security=tls&sni=doubao.yundu.space&fp=ios&insecure=0&allowInsecure=0&type=ws&host=doubao.yundu.space&path=%2Fdoubaobda53aa0-e18b-4429-a85b#gogogo"

	spec, err := ParseURI(originalURI)
	if err != nil {
		t.Fatalf("ParseURI failed: %v", err)
	}

	rendered, err := RenderURI(spec)
	if err != nil {
		t.Fatalf("RenderURI failed: %v", err)
	}

	parsedAgain, err := ParseURI(rendered)
	if err != nil {
		t.Fatalf("ParseURI(rendered) failed: %v", err)
	}

	if parsedAgain.ProtocolType != spec.ProtocolType {
		t.Errorf("ProtocolType mismatch: %q vs %q", parsedAgain.ProtocolType, spec.ProtocolType)
	}
	if parsedAgain.TransportType != spec.TransportType {
		t.Errorf("TransportType mismatch: %q vs %q", parsedAgain.TransportType, spec.TransportType)
	}
	if parsedAgain.SecurityType != spec.SecurityType {
		t.Errorf("SecurityType mismatch: %q vs %q", parsedAgain.SecurityType, spec.SecurityType)
	}
	if parsedAgain.Name != spec.Name {
		t.Errorf("Name mismatch: %q vs %q", parsedAgain.Name, spec.Name)
	}
	if parsedAgain.Host != spec.Host {
		t.Errorf("Host mismatch: %q vs %q", parsedAgain.Host, spec.Host)
	}
	if parsedAgain.Port != spec.Port {
		t.Errorf("Port mismatch: %d vs %d", parsedAgain.Port, spec.Port)
	}
	if parsedAgain.UUID != spec.UUID {
		t.Errorf("UUID mismatch: %q vs %q", parsedAgain.UUID, spec.UUID)
	}

	tlsCfg1, _ := spec.ConfigJSON["tls"].(map[string]interface{})
	tlsCfg2, _ := parsedAgain.ConfigJSON["tls"].(map[string]interface{})
	if tlsCfg1["server_name"] != tlsCfg2["server_name"] {
		t.Errorf("tls.server_name mismatch: %v vs %v", tlsCfg2["server_name"], tlsCfg1["server_name"])
	}
	if tlsCfg1["fingerprint"] != tlsCfg2["fingerprint"] {
		t.Errorf("tls.fingerprint mismatch: %v vs %v", tlsCfg2["fingerprint"], tlsCfg1["fingerprint"])
	}
	if spec.ConfigJSON["ws_path"] != parsedAgain.ConfigJSON["ws_path"] {
		t.Errorf("ws_path mismatch: %v vs %v", parsedAgain.ConfigJSON["ws_path"], spec.ConfigJSON["ws_path"])
	}
}
