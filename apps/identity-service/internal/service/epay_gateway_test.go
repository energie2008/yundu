package service

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/airport-panel/identity-service/internal/model"
)

func newEpayMock(t *testing.T, submitBody, queryBody string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/submit.php", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(submitBody))
	})
	// CreatePayment 使用 mapi.php（服务端 JSON API），submit.php 返回 HTML/文本
	mux.HandleFunc("/mapi.php", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(submitBody))
	})
	mux.HandleFunc("/api.php", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(queryBody))
	})
	return httptest.NewServer(mux)
}

func testEpayGateway(serverURL string, payType string) *EpayGateway {
	cfg := EpayConfig{
		Pid:        "1001",
		Key:        "test-secret",
		GatewayURL: serverURL,
		PayType:    payType,
		NotifyURL:  "https://panel.test/api/v1/payment/notify/alipay",
		ReturnURL:  "https://panel.test/dashboard/orders",
	}
	return NewEpayGateway(slog.Default(), http.DefaultClient, cfg, payType)
}

func TestEpayNotifyVerify(t *testing.T) {
	server := newEpayMock(t, `{}`, `{}`)
	defer server.Close()
	gw := testEpayGateway(server.URL, "alipay")

	params := map[string]string{
		"pid":          "1001",
		"trade_no":     "T202608020001",
		"out_trade_no": "P202608020001",
		"type":         "alipay",
		"name":         "测试套餐",
		"money":        "10.00",
		"trade_status": "TRADE_SUCCESS",
	}
	params["sign"] = epaySign(params, "test-secret")

	notify, err := gw.VerifyNotify(params)
	if err != nil {
		t.Fatalf("verify notify: %v", err)
	}
	if notify.Amount != 10 || notify.TradeNo != "T202608020001" || notify.OutTradeNo != "P202608020001" {
		t.Fatalf("unexpected notify: %+v", notify)
	}
}

func TestEpayNotifyRejectBadSign(t *testing.T) {
	server := newEpayMock(t, `{}`, `{}`)
	defer server.Close()
	gw := testEpayGateway(server.URL, "wxpay")

	params := map[string]string{
		"pid":          "1001",
		"trade_no":     "T1",
		"out_trade_no": "P1",
		"money":        "5.00",
		"trade_status": "TRADE_SUCCESS",
		"sign":         "deadbeef",
	}
	if _, err := gw.VerifyNotify(params); err == nil {
		t.Fatal("expected invalid signature error")
	}
}

func TestEpayCreatePayment(t *testing.T) {
	server := newEpayMock(t,
		`{"code":1,"msg":"ok","trade_no":"T1","qrcode":"https://qr.example/abc","url":"https://pay.example/checkout?order=P1"}`,
		`{}`,
	)
	defer server.Close()
	gw := testEpayGateway(server.URL, "alipay")

	order := &model.PaymentOrder{OrderNo: "P202608020001", PlanName: "轻量-66G-月付", FinalAmount: 10}
	pay, err := gw.CreatePayment(context.Background(), order)
	if err != nil {
		t.Fatalf("create payment: %v", err)
	}
	if pay.URL == "" || pay.QRCode == "" || pay.TradeNo != "T1" {
		t.Fatalf("unexpected payment: %+v", pay)
	}
}

func TestEpayCreatePaymentAlipayScheme(t *testing.T) {
	scheme := "alipays://platformapi/startapp?appId=20000028&bizData=%7B%22u%22%3A%222088001234567890%22%2C%22a%22%3A%226.01%22%7D"
	server := newEpayMock(t,
		`{"code":1,"msg":"ok","trade_no":"T2","qrcode":"https://qr.alipay.com/static","money":"6.01","alipay_scheme":"`+scheme+`"}`,
		`{}`,
	)
	defer server.Close()
	gw := testEpayGateway(server.URL, "alipay")

	order := &model.PaymentOrder{OrderNo: "P202608170001", PlanName: "轻量-66G-月付", FinalAmount: 6}
	pay, err := gw.CreatePayment(context.Background(), order)
	if err != nil {
		t.Fatalf("create payment: %v", err)
	}
	if pay.AlipayScheme != scheme {
		t.Fatalf("alipay scheme not parsed: %+v", pay)
	}
	if pay.Money != 6.01 {
		t.Fatalf("adjusted money not parsed: %v", pay.Money)
	}
	// 免输金额模式下 mapi 无跳转 URL，应兜底构造 qiu-pay 自有收银台链接
	if pay.URL != server.URL+"/v1/pay/T2" {
		t.Fatalf("expected cashier fallback URL, got %q", pay.URL)
	}
	if pay.QRCode != "https://qr.alipay.com/static" {
		t.Fatalf("static qrcode lost: %+v", pay)
	}
}

func TestEpayQueryOrderPaid(t *testing.T) {
	server := newEpayMock(t, `{}`,
		`{"code":1,"msg":"ok","data":{"trade_no":"T1","out_trade_no":"P202608020001","status":1}}`,
	)
	defer server.Close()
	gw := testEpayGateway(server.URL, "alipay")

	order := &model.PaymentOrder{OrderNo: "P202608020001"}
	tradeNo, paid, err := gw.QueryOrder(context.Background(), order)
	if err != nil {
		t.Fatalf("query order: %v", err)
	}
	if !paid || tradeNo != "T1" {
		t.Fatalf("unexpected query result: trade=%s paid=%v", tradeNo, paid)
	}
}

func TestEpayQueryOrderNotPaid(t *testing.T) {
	server := newEpayMock(t, `{}`,
		`{"code":1,"msg":"ok","data":{"trade_no":"T1","out_trade_no":"P202608020001","status":0}}`,
	)
	defer server.Close()
	gw := testEpayGateway(server.URL, "wxpay")

	order := &model.PaymentOrder{OrderNo: "P202608020001"}
	_, paid, err := gw.QueryOrder(context.Background(), order)
	if err != nil {
		t.Fatalf("query order: %v", err)
	}
	if paid {
		t.Fatal("expected unpaid order")
	}
}
