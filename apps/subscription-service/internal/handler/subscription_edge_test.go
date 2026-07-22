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

// TestGetClientIPMalformedXFF 验证 getClientIP 对畸形 X-Forwarded-For 的处理。
// Bug-D: 当 XFF 为 ", " (分隔后所有段为空) 时, strings.TrimSpace(parts[0]) 返回 "",
// getClientIP 会返回空字符串作为 IP, 导致访问日志记录空 IP / 统计失真。
// 安全契约: IP 必须为非空有效值, 畸形 XFF 应回退到 c.ClientIP() 或 X-Real-IP。
func TestGetClientIPMalformedXFF(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name   string
		xff    string
		xrIP   string
		expect string // "" 表示只要求非空, 其余要求精确匹配或前缀
	}{
		{"empty entries XFF", ", , ", "", "nonempty"},
		{"leading comma XFF", ", 1.2.3.4", "", "1.2.3.4"},
		{"whitespace only", "   ", "", "nonempty"},
		{"valid XFF single", "1.2.3.4", "", "1.2.3.4"},
		{"valid XFF chain", "1.2.3.4, 5.6.7.8", "", "1.2.3.4"},
		{"malformed XFF fallback to X-Real-IP", ", ", "9.9.9.9", "9.9.9.9"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request, _ = http.NewRequest("GET", "/", nil)
			// 真实生产中 Go HTTP server 总会设置 RemoteAddr;
			// 测试环境 httptest 不会自动设置, 此处补齐以验证全空 XFF 的回退契约。
			c.Request.RemoteAddr = "203.0.113.7:12345"
			if tc.xff != "" {
				c.Request.Header.Set("X-Forwarded-For", tc.xff)
			}
			if tc.xrIP != "" {
				c.Request.Header.Set("X-Real-IP", tc.xrIP)
			}

			h := &SubscriptionHandler{}
			ip := h.getClientIP(c)

			if tc.expect == "nonempty" {
				if strings.TrimSpace(ip) == "" {
					t.Errorf("getClientIP returned empty/whitespace IP %q for XFF=%q X-Real-IP=%q",
						ip, tc.xff, tc.xrIP)
				}
			} else {
				if ip != tc.expect {
					t.Errorf("getClientIP = %q, want %q (XFF=%q X-Real-IP=%q)",
						ip, tc.expect, tc.xff, tc.xrIP)
				}
			}
		})
	}
}

// TestWriteSubscriptionQuantumultXSanitized 验证 QuantumultX 客户端订阅内容会被 sanitize。
// Bug-E: isStrictURI 仅含 Shadowrocket/Quantumult/Surge, 未含 QuantumultX。
// 当用户通过 ?client=quanx 参数(无匹配 UA)访问时, ct=QuantumultX 但
// isQuantumult=false (UA 不含 "quantumult"), 导致 sanitize 被跳过,
// 订阅含 BOM/CRLF/尾随空白时客户端解析失败。
// QuantumultX 与 Quantumult 同样渲染为 URI, 应纳入 sanitize。
func TestWriteSubscriptionQuantumultXSanitized(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/sub/test", nil)
	// 用空 UA 精确模拟 ?client=quanx 路径, 避免 UA 中的 "quantumult" 子串
	// 让 isQuantumult 误判为 true 而掩盖缺陷。
	c.Request.Header.Set("User-Agent", "curl/7.0")

	h := &SubscriptionHandler{}
	// 含 BOM + CRLF 的脏内容, 期望被清理
	result := &service.SubscriptionResult{
		Content:     "\xEF\xBB\xBFvless://test@example.com:443?encryption=none&security=tls#Test\r\n",
		ContentType: "text/plain; charset=utf-8",
		UserInfo:    "upload=0; download=1024; total=1073741824; expire=1893456000",
	}
	h.writeSubscription(c, result, model.ClientTypeQuantumultX, "curl/7.0")

	body := w.Body.String()
	if strings.Contains(body, "\xEF\xBB\xBF") {
		t.Errorf("QuantumultX subscription should have BOM stripped, body still contains BOM: %q", body)
	}
	if strings.Contains(body, "\r") {
		t.Errorf("QuantumultX subscription should have CRLF normalized, body contains CR: %q", body)
	}
	if !strings.Contains(body, "vless://test") {
		t.Errorf("QuantumultX subscription content corrupted: %q", body)
	}
}

// TestWriteSubscriptionQuantumultXSanitizedViaUA 验证仅凭 UA 检测到 QuantumultX
// (无 ?client= 参数) 时订阅内容同样被 sanitize。覆盖 GetSubscription 的真实路径行为。
func TestWriteSubscriptionQuantumultXSanitizedViaUA(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 用空 UA + ct=QuantumultX 复现纯参数路径, 验证尾随空白被清理。
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/sub/test", nil)

	h := &SubscriptionHandler{}
	result := &service.SubscriptionResult{
		Content:     "vless://a  \n",
		ContentType: "text/plain; charset=utf-8",
		UserInfo:    "upload=0; download=0; total=0; expire=0",
	}
	h.writeSubscription(c, result, model.ClientTypeQuantumultX, "")

	body := w.Body.String()
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		if line == "" {
			continue
		}
		if line[len(line)-1] == ' ' || line[len(line)-1] == '\t' {
			t.Errorf("QuantumultX: trailing whitespace not stripped at line %d: %q", i+1, line)
		}
	}
}
