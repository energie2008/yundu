package service

import (
	"context"
	"testing"

	"github.com/airport-panel/node-service/internal/model"
)

func TestCertConsistencyWarningsSelfSigned(t *testing.T) {
	s := &DeploymentService{}
	sec := "tls"
	sni := "cn-hnzz-cm-01-01.bilivideo.com"
	n := &model.Node{
		Code:          "test-self",
		ProtocolType:  "trojan",
		TransportType: "tcp",
		SecurityType:  &sec,
		SNI:           &sni,
		ConfigJSON: map[string]interface{}{
			"security_type": "tls",
			"exposure_mode": "direct",
		},
	}

	ws := s.CertConsistencyWarnings(context.Background(), n)
	if len(ws) == 0 {
		t.Fatalf("expected self-signed consistency warning, got none")
	}

	// allow_insecure=true 时不应再告警
	n.ConfigJSON["allow_insecure"] = true
	if ws := s.CertConsistencyWarnings(context.Background(), n); len(ws) != 0 {
		t.Fatalf("expected no warning when allow_insecure=true, got %v", ws)
	}

	// pin_sha256 配置后也不应告警
	n.ConfigJSON["allow_insecure"] = false
	n.ConfigJSON["pin_sha256"] = "aabbccdd"
	if ws := s.CertConsistencyWarnings(context.Background(), n); len(ws) != 0 {
		t.Fatalf("expected no warning when pin_sha256 set, got %v", ws)
	}
}

func TestCertSANMatchWildcard(t *testing.T) {
	cases := []struct {
		certSAN string
		sni     string
		want    bool
	}{
		{"*.xinti.na.am", "seed4.xinti.na.am", true},
		{"*.xinti.na.am", "seed.xinti.na.am", true},
		{"*.xinti.na.am", "xinti.na.am", false},
		{"*.xinti.na.am", "a.b.xinti.na.am", false},
		{"seed4.xinti.na.am", "seed4.xinti.na.am", true},
		{"seed4.xinti.na.am", "seed5.xinti.na.am", false},
		{"*.XINTI.NA.AM", "Seed4.xinti.na.am", true},
	}
	for _, c := range cases {
		if got := certSANMatch(c.certSAN, c.sni); got != c.want {
			t.Fatalf("certSANMatch(%q, %q) = %v, want %v", c.certSAN, c.sni, got, c.want)
		}
	}
}
