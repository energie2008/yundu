package service

import (
	"context"
	"crypto"
	"crypto/md5"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/airport-panel/identity-service/internal/model"
)

// EpayV2Config 易支付 V2 渠道配置（pay.ifz.cc 等 V2 平台，SHA256WithRSA 签名）。
// SignType 支持两种版本：rsa（V2 接口，api/pay/* 端点）与 md5（V1 兼容，
// mapi.php/submit.php/api.php 端点）——md5 模式由 epayGatewayFor 直接分发到
// 现有 EpayGateway 实现，本结构只需保留 V1 密钥。
type EpayV2Config struct {
	Pid                string `json:"pid,omitempty"`
	GatewayURL         string `json:"gateway_url,omitempty"`
	SignType           string `json:"sign_type,omitempty"`              // rsa(默认) | md5
	MD5Key             string `json:"key,omitempty"`                    // md5 模式商户密钥
	MerchantPrivateKey string `json:"merchant_private_key,omitempty"`   // rsa 模式 PKCS#8 base64
	PlatformPublicKey  string `json:"platform_public_key,omitempty"`    // rsa 模式 X.509 base64（验签）
	PayType            string `json:"pay_type,omitempty"`               // alipay / wxpay
	NotifyURL          string `json:"notify_url,omitempty"`
	ReturnURL          string `json:"return_url,omitempty"`
	Method             string `json:"method,omitempty"` // 接口类型 web/jump，默认 web
	Device             string `json:"device,omitempty"` // 设备类型 pc/mobile，默认 pc
}

// ConfiguredV2 RSA 模式要求 pid + 网关地址 + 商户私钥 + 平台公钥齐全。
func (c EpayV2Config) ConfiguredRSA() bool {
	return c.Pid != "" && c.GatewayURL != "" &&
		strings.TrimSpace(c.MerchantPrivateKey) != "" && strings.TrimSpace(c.PlatformPublicKey) != ""
}

// ConfiguredMD5 V1 兼容模式要求 pid + 网关地址 + MD5 密钥。
func (c EpayV2Config) ConfiguredMD5() bool {
	return c.Pid != "" && c.GatewayURL != "" && c.MD5Key != ""
}

func (c EpayV2Config) signMode() string {
	if strings.EqualFold(c.SignType, "md5") {
		return "MD5"
	}
	return "RSA"
}

// clientIPKey 用于在 ctx 中传递下单用户 IP（V2 统一下单 clientip 必填）。
type clientIPKey struct{}

// WithClientIP 将客户端 IP 注入 ctx（用户下单 handler 调用）。
func WithClientIP(ctx context.Context, ip string) context.Context {
	if ip == "" {
		return ctx
	}
	return context.WithValue(ctx, clientIPKey{}, ip)
}

func clientIPFrom(ctx context.Context) string {
	if v, ok := ctx.Value(clientIPKey{}).(string); ok && v != "" {
		return v
	}
	return "127.0.0.1"
}

// EpayV2Gateway 易支付 V2 网关（RSA 签名）。实现 PayGateway 接口。
// 协议要点（对照平台 SDK EpayCore.class.php）：
//   - 下单 POST /api/pay/create，响应 code==0 为成功且必须用平台公钥验签、
//     校验 timestamp（±300s）；
//   - 查单 POST /api/pay/query，status==1 为已支付；
//   - 异步通知为 GET 回调，签名规则与请求一致（sign/sign_type 除外、非空、ASCII 排序）。
type EpayV2Gateway struct {
	log     *slog.Logger
	client  *http.Client
	cfg     EpayV2Config
	payType string
}

func NewEpayV2Gateway(log *slog.Logger, client *http.Client, cfg EpayV2Config, payType string) *EpayV2Gateway {
	return &EpayV2Gateway{log: log, client: client, cfg: cfg, payType: payType}
}

func (g *EpayV2Gateway) Name() string {
	return "epayv2"
}

// parsePKCS8Private 解析 PKCS#8 base64 商户私钥（-----BEGIN PRIVATE KEY----- 内容）。
func parsePKCS8Private(b64 string) (*rsa.PrivateKey, error) {
	raw := normalizeKeyB64(b64)
	der, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("merchant private key base64 decode: %w", err)
	}
	key, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil, fmt.Errorf("merchant private key parse (expect PKCS#8): %w", err)
	}
	rk, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("merchant private key is not RSA")
	}
	return rk, nil
}

// parseX509Public 解析 X.509 SubjectPublicKeyInfo base64 平台公钥。
func parseX509Public(b64 string) (*rsa.PublicKey, error) {
	raw := normalizeKeyB64(b64)
	der, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("platform public key base64 decode: %w", err)
	}
	pub, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return nil, fmt.Errorf("platform public key parse (expect X.509 SPKI): %w", err)
	}
	rk, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("platform public key is not RSA")
	}
	return rk, nil
}

// normalizeKeyB64 容错处理密钥输入：去 PEM 头尾/换行/空格。
func normalizeKeyB64(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "-----BEGIN PRIVATE KEY-----", "")
	s = strings.ReplaceAll(s, "-----END PRIVATE KEY-----", "")
	s = strings.ReplaceAll(s, "-----BEGIN PUBLIC KEY-----", "")
	s = strings.ReplaceAll(s, "-----END PUBLIC KEY-----", "")
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", "")
	s = strings.ReplaceAll(s, " ", "")
	return s
}

// rsaSignSHA256 商户私钥 SHA256WithRSA 签名（base64 输出）。
func rsaSignSHA256(priv *rsa.PrivateKey, data string) (string, error) {
	digest := sha256.Sum256([]byte(data))
	sig, err := rsa.SignPKCS1v15(nil, priv, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(sig), nil
}

// rsaVerifySHA256 平台公钥 SHA256WithRSA 验签。
func rsaVerifySHA256(pub *rsa.PublicKey, data, signB64 string) bool {
	sig, err := base64.StdEncoding.DecodeString(signB64)
	if err != nil {
		return false
	}
	digest := sha256.Sum256([]byte(data))
	return rsa.VerifyPKCS1v15(pub, crypto.SHA256, digest[:], sig) == nil
}

// md5SignHex V1 兼容 MD5 签名（与 EpayGateway 一致：签名串+密钥取 MD5）。
func md5SignHex(params map[string]string, key string) string {
	sum := md5.Sum([]byte(epaySignContent(params) + key))
	return hex.EncodeToString(sum[:])
}

// flexInt 兼容 JSON 数字与字符串数字（不同平台对 code/status 序列化不一）。
type flexInt int

func (f *flexInt) UnmarshalJSON(data []byte) error {
	s := strings.Trim(strings.TrimSpace(string(data)), `"`)
	if s == "" || s == "null" {
		*f = 0
		return nil
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return err
	}
	*f = flexInt(v)
	return nil
}

// CreatePayment 统一下单 POST {gw}/api/pay/create。
func (g *EpayV2Gateway) CreatePayment(ctx context.Context, order *model.PaymentOrder) (*GatewayPayment, error) {
	priv, err := parsePKCS8Private(g.cfg.MerchantPrivateKey)
	if err != nil {
		return nil, err
	}
	returnURL := g.cfg.ReturnURL
	if returnURL == "" {
		returnURL = defaultEpayReturnURL("")
	}
	method := g.cfg.Method
	if method == "" {
		method = "web"
	}
	device := g.cfg.Device
	if device == "" {
		device = "pc"
	}
	params := map[string]string{
		"pid":          g.cfg.Pid,
		"method":       method,
		"device":       device,
		"type":         g.payType,
		"out_trade_no": order.OrderNo,
		"notify_url":   g.cfg.NotifyURL,
		"return_url":   returnURL,
		"name":         truncateUTF8(order.PlanName, 32),
		"money":        strconv.FormatFloat(order.FinalAmount, 'f', 2, 64),
		"clientip":     clientIPFrom(ctx),
		"timestamp":    strconv.FormatInt(time.Now().Unix(), 10),
	}
	params["sign"] = mustSignRSA(priv, params)
	params["sign_type"] = "RSA"

	body, err := g.postForm(g.cfg.GatewayURL + "/api/pay/create", params)
	if err != nil {
		return nil, fmt.Errorf("epayv2 create payment: %w", err)
	}
	var result struct {
		Code      flexInt `json:"code"`
		Msg       string  `json:"msg"`
		TradeNo   string  `json:"trade_no"`
		PayType   string  `json:"pay_type"`
		PayInfo   string  `json:"pay_info"`
		Timestamp string  `json:"timestamp"`
		Sign      string  `json:"sign"`
		SignType  string  `json:"sign_type"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("epayv2 create response decode: %w: %s", err, truncateUTF8(string(body), 120))
	}
	if result.Code != 0 {
		return nil, fmt.Errorf("epayv2 create failed: code=%d msg=%s", result.Code, result.Msg)
	}
	// 响应验签（平台公钥）+ 时间戳校验，与 SDK 行为一致。
	// 验签字段取响应全量（数字字段按字符串参与，与平台侧序列化一致）。
	pub, err := parseX509Public(g.cfg.PlatformPublicKey)
	if err != nil {
		return nil, err
	}
	var respParams map[string]string
	if err := json.Unmarshal(body, &jsonStringMap{m: &respParams}); err != nil {
		return nil, fmt.Errorf("epayv2 create resp to map: %w", err)
	}
	if !rsaVerifySHA256(pub, epaySignContent(respParams), result.Sign) {
		return nil, errors.New("epayv2 create response signature verify failed")
	}
	if ts, err := strconv.ParseInt(result.Timestamp, 10, 64); err == nil {
		if drift := time.Now().Unix() - ts; drift > 300 || drift < -300 {
			return nil, fmt.Errorf("epayv2 create response timestamp out of range: %d", ts)
		}
	}

	// pay_type → 支付展示映射：qrcode 为二维码内容；jump/html 为跳转链接；
	// 其余（urlscheme/jsapi/wxplugin 等）内容多为链接或参数串，优先当跳转/二维码内容自适应。
	pay := &GatewayPayment{TradeNo: result.TradeNo}
	switch result.PayType {
	case "qrcode":
		pay.QRCode = result.PayInfo
	case "jump", "html":
		pay.URL = result.PayInfo
	default:
		if strings.HasPrefix(result.PayInfo, "http://") || strings.HasPrefix(result.PayInfo, "https://") {
			pay.URL = result.PayInfo
		} else {
			pay.QRCode = result.PayInfo
		}
	}
	return pay, nil
}

func mustSignRSA(priv *rsa.PrivateKey, params map[string]string) string {
	sig, err := rsaSignSHA256(priv, epaySignContent(params))
	if err != nil {
		// 私钥已在构造前校验可解析，签名失败属于不可恢复错误
		panic(fmt.Sprintf("epayv2 rsa sign: %v", err))
	}
	return sig
}

// QueryOrder POST /api/pay/query，status==1 视为已支付。
func (g *EpayV2Gateway) QueryOrder(ctx context.Context, order *model.PaymentOrder) (string, bool, error) {
	priv, err := parsePKCS8Private(g.cfg.MerchantPrivateKey)
	if err != nil {
		return "", false, err
	}
	params := map[string]string{
		"pid":          g.cfg.Pid,
		"out_trade_no": order.OrderNo,
		"timestamp":    strconv.FormatInt(time.Now().Unix(), 10),
	}
	params["sign"] = mustSignRSA(priv, params)
	params["sign_type"] = "RSA"

	body, err := g.postForm(g.cfg.GatewayURL + "/api/pay/query", params)
	if err != nil {
		return "", false, fmt.Errorf("epayv2 query: %w", err)
	}
	var result struct {
		Code      flexInt `json:"code"`
		Msg       string  `json:"msg"`
		TradeNo   string  `json:"trade_no"`
		Status    flexInt `json:"status"`
		Money     string  `json:"money"`
		Timestamp string  `json:"timestamp"`
		Sign      string  `json:"sign"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", false, fmt.Errorf("epayv2 query decode: %w: %s", err, truncateUTF8(string(body), 120))
	}
	if result.Code != 0 {
		return "", false, fmt.Errorf("epayv2 query failed: code=%d msg=%s", result.Code, result.Msg)
	}
	pub, err := parseX509Public(g.cfg.PlatformPublicKey)
	if err != nil {
		return "", false, err
	}
	// 全量响应字段参与验签（平台可能增加扩展字段，必须原样参与而非挑选固定字段）
	var respParams map[string]string
	if err := json.Unmarshal(body, &jsonStringMap{m: &respParams}); err != nil {
		return "", false, fmt.Errorf("epayv2 query resp to map: %w", err)
	}
	if !rsaVerifySHA256(pub, epaySignContent(respParams), result.Sign) {
		return "", false, errors.New("epayv2 query response signature verify failed")
	}
	return result.TradeNo, result.Status == 1, nil
}

// jsonStringMap 将 JSON 对象解码为 map[string]string（数字/布尔转字符串，其余跳过）。
type jsonStringMap struct {
	m *map[string]string
}

func (j *jsonStringMap) UnmarshalJSON(data []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	m := map[string]string{}
	for k, v := range raw {
		switch x := v.(type) {
		case string:
			m[k] = x
		case float64:
			if x == float64(int64(x)) {
				m[k] = strconv.FormatInt(int64(x), 10)
			} else {
				m[k] = strconv.FormatFloat(x, 'f', -1, 64)
			}
		case bool:
			m[k] = strconv.FormatBool(x)
		}
	}
	*j.m = m
	return nil
}

// VerifyNotify 校验 V2 异步通知（GET 参数）。sign_type=RSA 用平台公钥验签；
// 兼容 sign_type=MD5 的 V1 风格通知（用 V1 密钥）。
func (g *EpayV2Gateway) VerifyNotify(params map[string]string) (*GatewayNotify, error) {
	sign := params["sign"]
	if sign == "" {
		return nil, errors.New("epayv2 notify missing sign")
	}
	if pid := params["pid"]; pid != "" && pid != g.cfg.Pid {
		return nil, fmt.Errorf("epayv2 notify pid mismatch: %s", pid)
	}
	signType := params["sign_type"]
	if signType == "" {
		signType = "RSA"
	}
	switch strings.ToUpper(signType) {
	case "MD5":
		if g.cfg.MD5Key == "" {
			return nil, errors.New("epayv2 notify md5 mode but v1 key not configured")
		}
		if !strings.EqualFold(md5SignHex(params, g.cfg.MD5Key), sign) {
			return nil, errors.New("epayv2 notify invalid md5 signature")
		}
	default:
		pub, err := parseX509Public(g.cfg.PlatformPublicKey)
		if err != nil {
			return nil, err
		}
		if !rsaVerifySHA256(pub, epaySignContent(params), sign) {
			return nil, errors.New("epayv2 notify invalid rsa signature")
		}
		// 通知时间戳校验（±300s），缺失则跳过（老平台兼容）
		if ts := params["timestamp"]; ts != "" {
			if t, err := strconv.ParseInt(ts, 10, 64); err == nil {
				if drift := time.Now().Unix() - t; drift > 300 || drift < -300 {
					return nil, fmt.Errorf("epayv2 notify timestamp out of range: %d", t)
				}
			}
		}
	}
	amount, err := strconv.ParseFloat(params["money"], 64)
	if err != nil {
		return nil, fmt.Errorf("epayv2 notify invalid money: %s", params["money"])
	}
	return &GatewayNotify{
		TradeNo:    params["trade_no"],
		OutTradeNo: params["out_trade_no"],
		Amount:     amount,
		Status:     params["trade_status"],
		PayType:    params["type"],
	}, nil
}

// postForm 以 application/x-www-form-urlencoded 提交并返回响应体。
func (g *EpayV2Gateway) postForm(apiURL string, params map[string]string) ([]byte, error) {
	form := url.Values{}
	for k, v := range params {
		form.Set(k, v)
	}
	resp, err := g.client.PostForm(apiURL, form)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return body, nil
}
