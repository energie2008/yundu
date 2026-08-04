package service

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/airport-panel/identity-service/internal/model"
)

// EpayConfig 易支付（彩虹易支付协议兼容）网关配置。
type EpayConfig struct {
	Pid        string `json:"pid,omitempty"`
	Key        string `json:"key,omitempty"`
	GatewayURL string `json:"gateway_url,omitempty"`
	PayType    string `json:"pay_type,omitempty"` // alipay / wxpay
	NotifyURL  string `json:"notify_url,omitempty"`
	ReturnURL  string `json:"return_url,omitempty"`
}

// Configured 判断易支付配置是否完整可用。
func (c EpayConfig) Configured() bool {
	return c.Pid != "" && c.Key != "" && c.GatewayURL != ""
}

// GatewayPayment 网关创建支付返回结果。
type GatewayPayment struct {
	URL     string
	QRCode  string
	TradeNo string
}

// GatewayNotify 网关异步回调解析结果。
type GatewayNotify struct {
	TradeNo    string
	OutTradeNo string
	Amount     float64
	Status     string
	PayType    string
}

// PayGateway 支付网关抽象：易支付、四方等第三方通道都通过该接口接入。
type PayGateway interface {
	Name() string
	CreatePayment(ctx context.Context, order *model.PaymentOrder) (*GatewayPayment, error)
	VerifyNotify(params map[string]string) (*GatewayNotify, error)
	QueryOrder(ctx context.Context, order *model.PaymentOrder) (tradeNo string, paid bool, err error)
}

// EpayGateway 彩虹易支付协议实现。
type EpayGateway struct {
	log     *slog.Logger
	client  *http.Client
	cfg     EpayConfig
	payType string // alipay / wxpay
}

func NewEpayGateway(log *slog.Logger, client *http.Client, cfg EpayConfig, payType string) *EpayGateway {
	return &EpayGateway{log: log, client: client, cfg: cfg, payType: payType}
}

func (g *EpayGateway) Name() string {
	return "epay"
}

// epaySign 彩虹易支付 MD5 签名：
// 参数按 ASCII 排序，sign/sign_type/空值不参与，末尾拼接商户密钥。
func epaySign(params map[string]string, key string) string {
	keys := make([]string, 0, len(params))
	for k, v := range params {
		if k == "sign" || k == "sign_type" || v == "" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	for i, k := range keys {
		if i > 0 {
			sb.WriteString("&")
		}
		sb.WriteString(k)
		sb.WriteString("=")
		sb.WriteString(params[k])
	}
	sb.WriteString(key)
	sum := md5.Sum([]byte(sb.String()))
	return hex.EncodeToString(sum[:])
}

func truncateUTF8(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

func (g *EpayGateway) CreatePayment(ctx context.Context, order *model.PaymentOrder) (*GatewayPayment, error) {
	params := map[string]string{
		"pid":          g.cfg.Pid,
		"type":         g.payType,
		"out_trade_no": order.OrderNo,
		"notify_url":   g.cfg.NotifyURL,
		"return_url":   g.cfg.ReturnURL,
		"name":         truncateUTF8(order.PlanName, 32),
		"money":        strconv.FormatFloat(order.FinalAmount, 'f', 2, 64),
		"sign_type":    "MD5",
	}
	params["sign"] = epaySign(params, g.cfg.Key)

	form := url.Values{}
	for k, v := range params {
		form.Set(k, v)
	}
	// 彩虹易支付：submit.php 用于表单自动跳转（返回 HTML/文本，服务端无法直接消费），
	// mapi.php 用于服务端 API 调用（返回 JSON）。这里使用 mapi.php。
	apiURL := strings.TrimRight(g.cfg.GatewayURL, "/") + "/mapi.php"
	resp, err := g.client.PostForm(apiURL, form)
	if err != nil {
		return nil, fmt.Errorf("epay create payment: %w", err)
	}
	defer resp.Body.Close()
	rawBody, _ := io.ReadAll(resp.Body)
	bodyText := strings.TrimSpace(string(rawBody))

	var result struct {
		Code     int    `json:"code"`
		Msg      string `json:"msg"`
		Message  string `json:"message"`
		TradeNo  string `json:"trade_no"`
		QRCode   string `json:"qrcode"`
		URL      string `json:"url"`
		Redirect string `json:"redirect"`
		Data     *struct {
			TradeNo  string `json:"trade_no"`
			QRCode   string `json:"qrcode"`
			URL      string `json:"url"`
			Redirect string `json:"redirect"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rawBody, &result); err != nil {
		// 部分易支付平台返回纯文本错误（如“没有找到商户信息”），直接透传便于排查
		if bodyText != "" {
			return nil, fmt.Errorf("epay create payment failed: %s", bodyText)
		}
		return nil, fmt.Errorf("epay create payment decode: %w", err)
	}
	msg := result.Msg
	if msg == "" {
		msg = result.Message
	}
	// 兼容新旧协议：code=1 为成功；部分平台错误也用 code=0，
	// 需结合 trade_no / 支付链接 / 二维码判断是否真正创建成功。
	tradeNo := result.TradeNo
	if tradeNo == "" && result.Data != nil {
		tradeNo = result.Data.TradeNo
	}
	payURL := result.URL
	if payURL == "" && result.Data != nil {
		payURL = result.Data.URL
	}
	if payURL == "" {
		payURL = result.Redirect
	}
	if payURL == "" && result.Data != nil {
		payURL = result.Data.Redirect
	}
	qrCode := result.QRCode
	if qrCode == "" && result.Data != nil {
		qrCode = result.Data.QRCode
	}
	success := result.Code == 1 || (tradeNo != "" && (payURL != "" || qrCode != ""))
	if !success {
		return nil, fmt.Errorf("epay create payment failed: code=%d msg=%s", result.Code, msg)
	}
	return &GatewayPayment{
		URL:     payURL,
		QRCode:  qrCode,
		TradeNo: tradeNo,
	}, nil
}

func (g *EpayGateway) VerifyNotify(params map[string]string) (*GatewayNotify, error) {
	sign := params["sign"]
	if sign == "" {
		return nil, errors.New("epay notify missing sign")
	}
	if !strings.EqualFold(epaySign(params, g.cfg.Key), sign) {
		return nil, errors.New("epay notify invalid signature")
	}
	if pid := params["pid"]; pid != "" && pid != g.cfg.Pid {
		return nil, fmt.Errorf("epay notify pid mismatch: %s", pid)
	}
	amount, err := strconv.ParseFloat(params["money"], 64)
	if err != nil {
		return nil, fmt.Errorf("epay notify invalid money: %s", params["money"])
	}
	return &GatewayNotify{
		TradeNo:    params["trade_no"],
		OutTradeNo: params["out_trade_no"],
		Amount:     amount,
		Status:     params["trade_status"],
		PayType:    params["type"],
	}, nil
}

func (g *EpayGateway) QueryOrder(ctx context.Context, order *model.PaymentOrder) (string, bool, error) {
	apiURL := strings.TrimRight(g.cfg.GatewayURL, "/") + "/api.php"
	// 彩虹易支付订单查询：sign = md5(out_trade_no + key)
	sum := md5.Sum([]byte(order.OrderNo + g.cfg.Key))
	form := url.Values{}
	form.Set("act", "order")
	form.Set("pid", g.cfg.Pid)
	form.Set("key", g.cfg.Key)
	form.Set("out_trade_no", order.OrderNo)
	form.Set("sign", hex.EncodeToString(sum[:]))

	resp, err := g.client.PostForm(apiURL, form)
	if err != nil {
		return "", false, fmt.Errorf("epay query order: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data *struct {
			Status   int    `json:"status"`
			TradeNo  string `json:"trade_no"`
			OutTrade string `json:"out_trade_no"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", false, fmt.Errorf("epay query order decode: %w", err)
	}
	if result.Code != 0 && result.Code != 1 {
		return "", false, fmt.Errorf("epay query order failed: code=%d msg=%s", result.Code, result.Msg)
	}
	if result.Data == nil {
		return "", false, nil
	}
	return result.Data.TradeNo, result.Data.Status == 1, nil
}
