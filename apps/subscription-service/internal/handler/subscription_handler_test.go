package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/airport-panel/subscription-service/internal/model"
	"github.com/airport-panel/subscription-service/internal/service"
	"github.com/gin-gonic/gin"
)

func TestSanitizeSubscriptionContent_RemovesBOM(t *testing.T) {
	input := "\xEF\xBB\xBFvless://test1\nvless://test2"
	out := sanitizeSubscriptionContent(input)
	if len(out) > 0 && out[0] == 0xEF {
		t.Error("BOM not removed")
	}
	if !containsString(out, "vless://test1") {
		t.Error("content corrupted after BOM removal")
	}
}

func TestSanitizeSubscriptionContent_NormalizesLineEndings(t *testing.T) {
	tests := []struct{ name, input, expectContains string }{
		{"CRLF", "a\r\nb\r\n", "a\nb\n"},
		{"CR only", "a\rb\r", "a\nb\n"},
		{"mixed", "a\r\nb\rc\n", "a\nb\nc\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := sanitizeSubscriptionContent(tc.input)
			if out != tc.expectContains {
				t.Errorf("expected %q, got %q", tc.expectContains, out)
			}
		})
	}
}

func TestSanitizeSubscriptionContent_StripsTrailingSpaces(t *testing.T) {
	input := "vless://a  \nvless://b\t\n"
	out := sanitizeSubscriptionContent(input)
	for _, line := range splitLines(out) {
		for i := len(line) - 1; i >= 0; i-- {
			if line[i] == ' ' || line[i] == '\t' {
				if i == len(line)-1 {
					t.Errorf("trailing whitespace not stripped: %q", line)
				}
			}
			break
		}
	}
}

func TestSanitizeSubscriptionContent_EndsWithNewline(t *testing.T) {
	out := sanitizeSubscriptionContent("vless://a")
	if len(out) == 0 || out[len(out)-1] != '\n' {
		t.Errorf("output should end with newline, got %q", out)
	}
}

func TestSanitizeSubscriptionContent_NoMultipleBlankLines(t *testing.T) {
	input := "vless://a\n\n\n\nvless://b\n\n"
	out := sanitizeSubscriptionContent(input)
	if containsString(out, "\n\n\n") {
		t.Errorf("consecutive blank lines should be collapsed: %q", out)
	}
}

func TestSanitizeSubscriptionContent_MultipleBOMs(t *testing.T) {
	input := "\xEF\xBB\xBF\xEF\xBB\xBFvless://test"
	out := sanitizeSubscriptionContent(input)
	if containsString(out, "\xEF\xBB\xBF") {
		t.Errorf("multiple BOMs not fully removed")
	}
}

func TestSanitizeSubscriptionContent_Empty(t *testing.T) {
	out := sanitizeSubscriptionContent("")
	if out != "" {
		t.Errorf("empty input should produce empty output, got %q", out)
	}
}

func containsString(s, sub string) bool {
	return len(s) >= len(sub) && searchString(s, sub)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func TestShadowrocketHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		userAgent      string
		clientType     model.ClientType
		expectCache    string
		expectInterval string
	}{
		{
			name:           "Shadowrocket UA",
			userAgent:      "Shadowrocket/2023",
			clientType:     model.ClientTypeURI,
			expectCache:    "private, no-store, no-cache, must-revalidate, max-age=0",
			expectInterval: "6",
		},
		{
			name:           "Shadowrocket client param",
			userAgent:      "Unknown",
			clientType:     model.ClientTypeShadowrocket,
			expectCache:    "private, no-store, no-cache, must-revalidate, max-age=0",
			expectInterval: "6",
		},
		{
			name:           "Surge UA",
			userAgent:      "Surge/5.0",
			clientType:     model.ClientTypeURI,
			expectCache:    "private, max-age=3600",
			expectInterval: "6",
		},
		{
			name:           "Default client",
			userAgent:      "curl/7.0",
			clientType:     model.ClientTypeURI,
			expectCache:    "private, no-store",
			expectInterval: "6",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request, _ = http.NewRequest("GET", "/sub/test", nil)
			c.Request.Header.Set("User-Agent", tc.userAgent)

			h := &SubscriptionHandler{}
			result := &service.SubscriptionResult{
				Content:     "vless://test@example.com:443?encryption=none&security=tls#Test",
				ContentType: "text/plain; charset=utf-8",
				UserInfo:    "upload=0; download=1024; total=1073741824; expire=1893456000",
			}
			h.writeSubscription(c, result, tc.clientType, tc.userAgent)

			cacheControl := w.Header().Get("Cache-Control")
			if cacheControl != tc.expectCache {
				t.Errorf("Cache-Control: expected %q, got %q", tc.expectCache, cacheControl)
			}

			interval := w.Header().Get("Profile-Update-Interval")
			if interval != tc.expectInterval {
				t.Errorf("Profile-Update-Interval: expected %q, got %q", tc.expectInterval, interval)
			}

			contentDisp := w.Header().Get("Content-Disposition")
			// 所有订阅响应都应附带 Content-Disposition: attachment; filename="..."
			if !strings.HasPrefix(contentDisp, `attachment; filename="`) {
				t.Errorf("Content-Disposition should be attachment format, got %q", contentDisp)
			}

			cors := w.Header().Get("Access-Control-Allow-Origin")
			if tc.clientType == model.ClientTypeShadowrocket || strings.Contains(strings.ToLower(tc.userAgent), "shadowrocket") {
				if cors != "*" {
					t.Errorf("Shadowrocket should have Access-Control-Allow-Origin: *, got %q", cors)
				}
			}

			userinfo := w.Header().Get("Subscription-Userinfo")
			if userinfo != result.UserInfo {
				t.Errorf("Subscription-Userinfo: expected %q, got %q", result.UserInfo, userinfo)
			}
		})
	}
}

func TestShadowrocketSanitize100Times(t *testing.T) {
	testContent := "\xEF\xBB\xBFvless://test1@hk1.example.com:443?encryption=none&security=tls&sni=hk1.example.com&fp=chrome&type=ws&path=/ws&host=hk1.example.com#HK-Node\r\n" +
		"vmess://00000000-0000-0000-0000-000000000004@uk1.example.com:443?alterId=0&v=2&security=tls&sni=uk1.example.com&type=ws&path=/vmess  \r\n" +
		"trojan://pass@us1.example.com:443?security=tls&type=ws&path=/trojan\t\n" +
		"\n" +
		"\n" +
		"ss://aes-256-gcm:pass@sg1.example.com:8388#SG-Node\r\n"

	for i := 0; i < 100; i++ {
		out := sanitizeSubscriptionContent(testContent)

		if len(out) >= 3 && out[0] == 0xEF && out[1] == 0xBB && out[2] == 0xBF {
			t.Fatalf("iteration %d: BOM not removed", i)
		}

		if strings.Contains(out, "\r\n") || strings.Contains(out, "\r") {
			t.Fatalf("iteration %d: contains CRLF/CR", i)
		}

		lines := strings.Split(out, "\n")
		for j, line := range lines {
			if line == "" {
				continue
			}
			if line[len(line)-1] == ' ' || line[len(line)-1] == '\t' {
				t.Fatalf("iteration %d, line %d: trailing whitespace: %q", i, j+1, line)
			}
		}

		if strings.Contains(out, "\n\n\n") {
			t.Fatalf("iteration %d: consecutive blank lines", i)
		}

		if len(out) == 0 || out[len(out)-1] != '\n' {
			t.Fatalf("iteration %d: should end with newline", i)
		}

		if !strings.Contains(out, "vless://") || !strings.Contains(out, "vmess://") ||
			!strings.Contains(out, "trojan://") || !strings.Contains(out, "ss://") {
			t.Fatalf("iteration %d: content corrupted, missing protocols", i)
		}
	}
}

func TestShadowrocketUserinfoHeaderFormat(t *testing.T) {
	tests := []struct {
		name   string
		value  string
		expect bool
	}{
		{"valid standard", "upload=0; download=1024; total=1073741824; expire=1893456000", true},
		{"valid no-space", "upload=123;download=456;total=789;expire=9999999999", true},
		{"valid no-expire-zero", "upload=0; download=0; total=0; expire=0", true},
		{"missing upload", "download=1024; total=1073741824; expire=1893456000", false},
		{"missing download", "upload=0; total=1073741824; expire=1893456000", false},
		{"missing total", "upload=0; download=1024; expire=1893456000", false},
		{"missing expire", "upload=0; download=1024; total=1073741824", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parts := strings.Split(tc.value, ";")
			hasUpload, hasDownload, hasTotal, hasExpire := false, false, false, false
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if strings.HasPrefix(p, "upload=") {
					hasUpload = true
				} else if strings.HasPrefix(p, "download=") {
					hasDownload = true
				} else if strings.HasPrefix(p, "total=") {
					hasTotal = true
				} else if strings.HasPrefix(p, "expire=") {
					hasExpire = true
				}
			}
			valid := hasUpload && hasDownload && hasTotal && hasExpire
			if valid != tc.expect {
				t.Errorf("expected valid=%v, got %v for %q", tc.expect, valid, tc.value)
			}
		})
	}
}
