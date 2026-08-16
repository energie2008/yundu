package service

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/airport-panel/identity-service/internal/model"
	"github.com/google/uuid"
)

// genTestRSAKey 生成临时 RSA 密钥对，返回 PKCS#8 私钥 / X.509 公钥的 base64（与平台格式一致）。
func genTestRSAKey(t *testing.T) (privB64, pubB64 string, priv *rsa.PrivateKey, pub *rsa.PublicKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	privDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(privDER),
		base64.StdEncoding.EncodeToString(pubDER),
		key, &key.PublicKey
}

func TestEpaySignContentOrdering(t *testing.T) {
	params := map[string]string{
		"sign":      "should-skip",
		"sign_type": "RSA",
		"empty":     "",
		"out_trade_no": "P123",
		"pid":       "1034",
		"money":     "6.01",
	}
	got := epaySignContent(params)
	want := "money=6.01&out_trade_no=P123&pid=1034"
	if got != want {
		t.Fatalf("sign content = %q, want %q", got, want)
	}
}

func TestEpaySignMD5Unchanged(t *testing.T) {
	// 与 V1 算法结果一致性（同一签名串 + 密钥取 MD5）
	params := map[string]string{"pid": "5", "money": "6.00", "out_trade_no": "T1"}
	key := "1422cef754d26d7a5630b9bd53e40103"
	if epaySign(params, key) != md5SignHex(params, key) {
		t.Fatal("epaySign 与 md5SignHex 结果应一致")
	}
	if epaySign(params, key) == epaySign(params, "other-key") {
		t.Fatal("不同密钥应产生不同签名")
	}
}

func TestRSAKeyParseNormalize(t *testing.T) {
	privB64, pubB64, priv, pub := genTestRSAKey(t)

	// 带 PEM 头尾与换行的粘贴输入也能解析
	privPEM := "-----BEGIN PRIVATE KEY-----\n" + insertN(privB64, 64) + "\n-----END PRIVATE KEY-----\n"
	pubPEM := "-----BEGIN PUBLIC KEY-----\n" + insertN(pubB64, 64) + "\n-----END PUBLIC KEY-----\n"
	if _, err := parsePKCS8Private(privPEM); err != nil {
		t.Fatalf("parse pem private: %v", err)
	}
	if _, err := parseX509Public(pubPEM); err != nil {
		t.Fatalf("parse pem public: %v", err)
	}

	// 签名/验签 roundtrip
	sig, err := rsaSignSHA256(priv, "money=6.01&pid=1034")
	if err != nil {
		t.Fatal(err)
	}
	if !rsaVerifySHA256(pub, "money=6.01&pid=1034", sig) {
		t.Fatal("roundtrip verify failed")
	}
	if rsaVerifySHA256(pub, "money=6.02&pid=1034", sig) {
		t.Fatal("tampered content should not verify")
	}
}

func insertN(s string, n int) string {
	var sb strings.Builder
	for i := 0; i < len(s); i += n {
		end := i + n
		if end > len(s) {
			end = len(s)
		}
		sb.WriteString(s[i:end])
		sb.WriteString("\n")
	}
	return strings.TrimSuffix(sb.String(), "\n")
}

func TestVerifyNotifyRSAAndMD5(t *testing.T) {
	privB64, pubB64, priv, _ := genTestRSAKey(t)
	gw := NewEpayV2Gateway(nil, http.DefaultClient, EpayV2Config{
		Pid:                "1034",
		SignType:           "RSA",
		MD5Key:             "md5-key-1",
		MerchantPrivateKey: privB64,
		PlatformPublicKey:  pubB64,
	}, "alipay")

	ts := strconv.FormatInt(time.Now().Unix(), 10)
	// RSA 通知
	notify := map[string]string{
		"pid":          "1034",
		"trade_no":     "T20260817",
		"out_trade_no": "P1",
		"money":        "6.01",
		"trade_status": "TRADE_SUCCESS",
		"timestamp":    ts,
	}
	sig, _ := rsaSignSHA256(priv, epaySignContent(notify))
	notify["sign"] = sig
	notify["sign_type"] = "RSA"
	got, err := gw.VerifyNotify(notify)
	if err != nil {
		t.Fatalf("rsa notify verify: %v", err)
	}
	if got.OutTradeNo != "P1" || got.Amount != 6.01 || got.Status != "TRADE_SUCCESS" {
		t.Fatalf("notify parse: %+v", got)
	}

	// 篡改后验签必须失败
	notify["money"] = "0.01"
	if _, err := gw.VerifyNotify(notify); err == nil {
		t.Fatal("tampered notify should fail")
	}

	// MD5 兼容通知（sign_type=MD5）
	m := map[string]string{
		"pid":          "1034",
		"trade_no":     "T2",
		"out_trade_no": "P2",
		"money":        "1.00",
		"trade_status": "TRADE_SUCCESS",
	}
	m["sign"] = md5SignHex(m, "md5-key-1")
	m["sign_type"] = "MD5"
	if _, err := gw.VerifyNotify(m); err != nil {
		t.Fatalf("md5 compat notify verify: %v", err)
	}
}

func TestVerifyNotifyTimestampSkew(t *testing.T) {
	privB64, pubB64, priv, _ := genTestRSAKey(t)
	gw := NewEpayV2Gateway(nil, http.DefaultClient, EpayV2Config{
		Pid:                "1034",
		MerchantPrivateKey: privB64,
		PlatformPublicKey:  pubB64,
	}, "alipay")

	notify := map[string]string{
		"pid":          "1034",
		"out_trade_no": "P1",
		"money":        "6.01",
		"trade_status": "TRADE_SUCCESS",
		"timestamp":    strconv.FormatInt(time.Now().Add(-10*time.Minute).Unix(), 10),
	}
	sig, _ := rsaSignSHA256(priv, epaySignContent(notify))
	notify["sign"] = sig
	notify["sign_type"] = "RSA"
	if _, err := gw.VerifyNotify(notify); err == nil {
		t.Fatal("stale timestamp should fail")
	}
}

func TestCreatePaymentV2Flow(t *testing.T) {
	privB64, pubB64, priv, _ := genTestRSAKey(t)

	// mock 平台：验请求签名（平台侧用商户公钥验签略），返回 code=0 + 平台签名响应
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pay/create" {
			http.Error(w, "not found", 404)
			return
		}
		_ = r.ParseForm()
		if r.Form.Get("pid") != "1034" || r.Form.Get("type") != "alipay" {
			http.Error(w, "bad params", 400)
			return
		}
		if r.Form.Get("clientip") == "" || r.Form.Get("timestamp") == "" {
			http.Error(w, "missing clientip/timestamp", 400)
			return
		}
		if r.Form.Get("sign_type") != "RSA" || r.Form.Get("sign") == "" {
			http.Error(w, "missing sign", 400)
			return
		}
		respParams := map[string]string{
			"code":      "0",
			"trade_no":  "T-V2-001",
			"pay_type":  "qrcode",
			"pay_info":  "https://qr.alipay.com/dynamic001",
			"timestamp": strconv.FormatInt(time.Now().Unix(), 10),
		}
		sig, _ := rsaSignSHA256(priv, epaySignContent(respParams))
		respParams["sign"] = sig
		respParams["sign_type"] = "RSA"
		out, _ := json.Marshal(respParams)
		_, _ = w.Write(out)
	}))
	defer srv.Close()

	gw := NewEpayV2Gateway(nil, srv.Client(), EpayV2Config{
		Pid:                "1034",
		GatewayURL:         srv.URL,
		MerchantPrivateKey: privB64,
		PlatformPublicKey:  pubB64,
		NotifyURL:          "https://panel.example.com/api/v1/payment/notify/alipay",
	}, "alipay")

	ctx := WithClientIP(context.Background(), "203.0.113.9")
	order := &model.PaymentOrder{ID: uuid.New(), OrderNo: "P202608179001", PlanName: "测试套餐", FinalAmount: 6.01}
	pay, err := gw.CreatePayment(ctx, order)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if pay.TradeNo != "T-V2-001" {
		t.Fatalf("trade_no = %s", pay.TradeNo)
	}
	if pay.QRCode != "https://qr.alipay.com/dynamic001" || pay.URL != "" {
		t.Fatalf("qrcode mapping wrong: %+v", pay)
	}
}

func TestQueryOrderV2Paid(t *testing.T) {
	privB64, pubB64, priv, _ := genTestRSAKey(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pay/query" {
			http.Error(w, "not found", 404)
			return
		}
		respParams := map[string]string{
			"code":         "0",
			"trade_no":     "T-V2-002",
			"out_trade_no": "P202608179002",
			"money":        "6.01",
			"status":       "1",
			"timestamp":    strconv.FormatInt(time.Now().Unix(), 10),
		}
		sig, _ := rsaSignSHA256(priv, epaySignContent(respParams))
		respParams["sign"] = sig
		out, _ := json.Marshal(respParams)
		_, _ = w.Write(out)
	}))
	defer srv.Close()

	gw := NewEpayV2Gateway(nil, srv.Client(), EpayV2Config{
		Pid:                "1034",
		GatewayURL:         srv.URL,
		MerchantPrivateKey: privB64,
		PlatformPublicKey:  pubB64,
	}, "alipay")

	tradeNo, paid, err := gw.QueryOrder(context.Background(), &model.PaymentOrder{OrderNo: "P202608179002"})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if !paid || tradeNo != "T-V2-002" {
		t.Fatalf("paid=%v tradeNo=%s", paid, tradeNo)
	}
}

// TestEpayV2Configured 校验 RSA/MD5 两种模式的配置完整性判断。
func TestEpayV2Configured(t *testing.T) {
	privB64, pubB64, _, _ := genTestRSAKey(t)
	rsaCfg := EpayV2Config{Pid: "1034", GatewayURL: "https://pay.ifz.cc", MerchantPrivateKey: privB64, PlatformPublicKey: pubB64}
	if !rsaCfg.ConfiguredRSA() || rsaCfg.ConfiguredMD5() {
		t.Fatal("rsa cfg flags wrong")
	}
	if rsaCfg.signMode() != "RSA" {
		t.Fatal("default sign mode should be RSA")
	}
	md5Cfg := EpayV2Config{Pid: "1034", GatewayURL: "https://pay.ifz.cc", SignType: "md5", MD5Key: "k"}
	if !md5Cfg.ConfiguredMD5() || md5Cfg.ConfiguredRSA() {
		t.Fatal("md5 cfg flags wrong")
	}
	if md5Cfg.signMode() != "MD5" {
		t.Fatal("md5 sign mode")
	}
}

// TestEpayV2LiveConnectivity 真实平台连通性验证（不产生订单，仅调商户信息查询）。
// 通过环境变量注入真实密钥后运行：EPAYV2_GATEWAY / EPAYV2_PID / EPAYV2_PRIV / EPAYV2_PUB
func TestEpayV2LiveConnectivity(t *testing.T) {
	gateway := os.Getenv("EPAYV2_GATEWAY")
	pid := os.Getenv("EPAYV2_PID")
	privB64 := os.Getenv("EPAYV2_PRIV")
	pubB64 := os.Getenv("EPAYV2_PUB")
	if gateway == "" || pid == "" || privB64 == "" || pubB64 == "" {
		t.Skip("live credentials not provided via EPAYV2_* env vars")
	}
	priv, err := parsePKCS8Private(privB64)
	if err != nil {
		t.Fatalf("merchant private key parse: %v", err)
	}
	if _, err := parseX509Public(pubB64); err != nil {
		t.Fatalf("platform public key parse: %v", err)
	}
	params := map[string]string{
		"pid":       pid,
		"timestamp": strconv.FormatInt(time.Now().Unix(), 10),
	}
	params["sign"] = mustSignRSA(priv, params)
	params["sign_type"] = "RSA"
	form := map[string]string{}
	for k, v := range params {
		form[k] = v
	}
	gw := NewEpayV2Gateway(nil, &http.Client{Timeout: 15 * time.Second}, EpayV2Config{GatewayURL: gateway}, "alipay")
	body, err := gw.postForm(gateway+"/api/merchant/info", params)
	if err != nil {
		t.Fatalf("merchant info request: %v", err)
	}
	t.Logf("live merchant info response: %s", truncateUTF8(string(body), 300))
	if strings.Contains(string(body), `"code":0`) {
		t.Log("live RSA signature accepted by platform")
	} else {
		t.Fatalf("platform rejected request: %s", truncateUTF8(string(body), 300))
	}
}

func TestFiatChannelConfigured(t *testing.T) {
	// v1: pid+url+md5key
	if (FiatChannel{Protocol: "v1", Pid: "1", GatewayURL: "https://a", MD5Key: "k"}).Configured() != true {
		t.Fatal("v1 should be configured")
	}
	if (FiatChannel{Protocol: "v1", Pid: "1", GatewayURL: "https://a"}).Configured() != false {
		t.Fatal("v1 without md5 key should not be configured")
	}
	// v2+RSA: pid+url+私钥+平台公钥
	privB64, pubB64, _, _ := genTestRSAKey(t)
	if (FiatChannel{Protocol: "v2", Pid: "1", GatewayURL: "https://a", MerchantPrivateKey: privB64, PlatformPublicKey: pubB64}).Configured() != true {
		t.Fatal("v2-rsa should be configured")
	}
	if (FiatChannel{Protocol: "v2", Pid: "1", GatewayURL: "https://a", MerchantPrivateKey: privB64}).Configured() != false {
		t.Fatal("v2-rsa without platform key should not be configured")
	}
	// v2+MD5 兼容: pid+url+md5key
	if (FiatChannel{Protocol: "v2", SignType: "MD5", Pid: "1", GatewayURL: "https://a", MD5Key: "k"}).Configured() != true {
		t.Fatal("v2-md5 should be configured")
	}
}

func TestFiatChannelsFindAndFallback(t *testing.T) {
	cfg := FiatChannelsConfig{
		Channels: []FiatChannel{
			{ID: "ifz", Name: "ifz", Protocol: "v2", GatewayURL: "https://pay.ifz.cc", Pid: "1034"},
			{ID: "qiupay", Name: "qiu-pay", Protocol: "v1", GatewayURL: "https://x", Pid: "5"},
		},
		AlipayChannel: "ifz",
		WechatChannel: "nope",
	}
	if cfg.FindChannel("ifz") == nil || cfg.FindChannel("absent") != nil {
		t.Fatal("FindChannel broken")
	}
	// 绑定回退逻辑在 fiatChannelsSnapshot 中，这里仅验证 FindChannel 语义
}

func TestBuildChannelGatewayDispatch(t *testing.T) {
	svc := &PaymentService{log: slog.New(slog.DiscardHandler), httpClient: http.DefaultClient}
	privB64, pubB64, _, _ := genTestRSAKey(t)

	// v2-rsa → EpayV2Gateway
	gw, err := svc.buildChannelGateway(&FiatChannel{
		ID: "ifz", Protocol: "v2", GatewayURL: "https://pay.ifz.cc", Pid: "1034",
		MerchantPrivateKey: privB64, PlatformPublicKey: pubB64,
	}, "alipay", "alipay")
	if err != nil || gw.Name() != "epayv2" {
		t.Fatalf("v2-rsa dispatch: %v name=%s", err, gw.Name())
	}

	// v2-md5 → EpayGateway（V1 端点）
	gw, err = svc.buildChannelGateway(&FiatChannel{
		ID: "ifz-v1", Protocol: "v2", SignType: "MD5", GatewayURL: "https://pay.ifz.cc", Pid: "1034", MD5Key: "k",
	}, "alipay", "alipay")
	if err != nil || gw.Name() != "epay" {
		t.Fatalf("v2-md5 dispatch: %v name=%s", err, gw.Name())
	}

	// v1 → EpayGateway
	gw, err = svc.buildChannelGateway(&FiatChannel{
		ID: "qiupay", Protocol: "v1", GatewayURL: "https://x", Pid: "5", MD5Key: "k",
	}, "wechat", "wxpay")
	if err != nil || gw.Name() != "epay" {
		t.Fatalf("v1 dispatch: %v name=%s", err, gw.Name())
	}

	// 未配置完整 → 报错
	if _, err := svc.buildChannelGateway(&FiatChannel{ID: "bad", Protocol: "v1", GatewayURL: "https://x"}, "alipay", "alipay"); err == nil {
		t.Fatal("incomplete channel should error")
	}
}

// TestEpayV2LiveCreate 真实统一下单验证（0.01元，不支付自然过期，无资金影响）。
// 环境变量：EPAYV2_GATEWAY / EPAYV2_PID / EPAYV2_PRIV / EPAYV2_PUB
func TestEpayV2LiveCreate(t *testing.T) {
	gateway := os.Getenv("EPAYV2_GATEWAY")
	pid := os.Getenv("EPAYV2_PID")
	privB64 := os.Getenv("EPAYV2_PRIV")
	pubB64 := os.Getenv("EPAYV2_PUB")
	if gateway == "" || pid == "" || privB64 == "" || pubB64 == "" {
		t.Skip("live credentials not provided via EPAYV2_* env vars")
	}
	gw := NewEpayV2Gateway(nil, &http.Client{Timeout: 15 * time.Second}, EpayV2Config{
		Pid:                pid,
		GatewayURL:         strings.TrimRight(gateway, "/"),
		SignType:           "RSA",
		MerchantPrivateKey: privB64,
		PlatformPublicKey:  pubB64,
		NotifyURL:          "https://6.tiktokplay.na.am/api/v1/payment/notify/alipay",
	}, "alipay")
	order := &model.PaymentOrder{
		OrderNo:     "TESTV2" + strconv.FormatInt(time.Now().UnixNano()%1000000, 10),
		PlanName:    "渠道验证",
		FinalAmount: 1.00,
	}
	pay, err := gw.CreatePayment(WithClientIP(context.Background(), "127.0.0.1"), order)
	if err != nil {
		t.Fatalf("live create: %v", err)
	}
	t.Logf("live create OK: trade_no=%s url=%s qrcode=%s", pay.TradeNo, pay.URL, pay.QRCode)
	if pay.URL == "" && pay.QRCode == "" {
		t.Fatal("no payment payload returned")
	}
}
