package handler

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/airport-panel/config/server"
	"github.com/airport-panel/identity-service/internal/repo"
	"github.com/airport-panel/identity-service/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// AdminPaymentHandler 管理员支付配置 handler
// 基于 system_settings 表管理 TRC20/ERC20 支付配置
type AdminPaymentHandler struct {
	settingRepo *repo.SettingRepo
	paymentSvc  *service.PaymentService
}

func NewAdminPaymentHandler(settingRepo *repo.SettingRepo, paymentSvc *service.PaymentService) *AdminPaymentHandler {
	return &AdminPaymentHandler{settingRepo: settingRepo, paymentSvc: paymentSvc}
}

// RegisterRoutesWithGroup 注册管理员支付配置路由
func (h *AdminPaymentHandler) RegisterRoutesWithGroup(rg *gin.RouterGroup) {
	payments := rg.Group("/payment-methods")
	{
		payments.GET("", h.ListPaymentMethods)
		payments.GET("/exchange-rate", h.GetExchangeRate)
		payments.PUT("/exchange-rate", h.UpdateExchangeRate)
		payments.GET("/:method", h.GetPaymentMethod)
		payments.PUT("/:method", h.UpdatePaymentMethod)
		payments.POST("/:method/toggle", h.TogglePaymentMethod)
	}
}

// ListPaymentMethods 列出所有支付方式配置
// GET /admin/payment-methods
func (h *AdminPaymentHandler) ListPaymentMethods(c *gin.Context) {
	trc20 := h.paymentSvc.GetTRC20Config()
	erc20 := h.paymentSvc.GetERC20Config()
	bep20 := h.paymentSvc.GetBEP20Config()
	wechat := h.paymentSvc.GetWechatConfig()
	alipay := h.paymentSvc.GetAlipayConfig()

	server.OK(c, gin.H{
		"methods": []gin.H{
			{
				"method":           "usdt_trc20",
				"name":             "USDT-TRC20",
				"enabled":          trc20.Enabled,
				"address":          trc20.Address,
				"amount_tolerance": trc20.AmountTolerance,
				"confirmations":    trc20.MinConfirmations,
				"network":          "tron",
				"auto_activate":    trc20.AutoActivate,
				"currency":         "USDT",
			},
			{
				"method":             "usdt_erc20",
				"name":               "USDT-EVM（Polygon / Arbitrum One 双通道）",
				"enabled":            erc20.Enabled,
				"address":            erc20.Address,
				"amount_tolerance":   erc20.AmountTolerance,
				"confirmations":      erc20.MinConfirmations,
				"network":            erc20.Network,
				"networks":           erc20.EnabledNetworks(),
				"auto_activate":      erc20.AutoActivate,
				"api_key_configured": erc20.EtherscanAPIKey != "",
				"currency":           "USDT",
			},
			{
				"method":           "usdt_bep20",
				"name":             "USDT-BEP20",
				"enabled":          bep20.Enabled,
				"address":          bep20.Address,
				"amount_tolerance": bep20.AmountTolerance,
				"confirmations":    bep20.MinConfirmations,
				"network":          "bsc",
				"auto_activate":    bep20.AutoActivate,
				"currency":         "USDT",
				"rpc_nodes":        bep20.BscRPC,
				"available":        true,
			},
			{
				"method":          "wechat",
				"name":            "微信支付",
				"enabled":         wechat.Enabled,
				"auto_activate":   wechat.AutoActivate,
				"currency":        "CNY",
				"epay":            epayConfigMap(wechat.Epay),
				"epay_configured": wechat.Epay.Configured(),
			},
			{
				"method":          "alipay",
				"name":            "支付宝",
				"enabled":         alipay.Enabled,
				"auto_activate":   alipay.AutoActivate,
				"currency":        "CNY",
				"epay":            epayConfigMap(alipay.Epay),
				"epay_configured": alipay.Epay.Configured(),
			},
		},
	})
}

// GetPaymentMethod 获取单个支付方式配置
// GET /admin/payment-methods/:method
func (h *AdminPaymentHandler) GetPaymentMethod(c *gin.Context) {
	method := c.Param("method")
	cfg := h.getMethodConfig(method)
	if cfg == nil {
		server.ValidationError(c, "unsupported payment method")
		return
	}
	server.OK(c, cfg)
}

// UpdatePaymentMethodRequest 更新支付方式请求
type UpdatePaymentMethodRequest struct {
	Enabled         *bool              `json:"enabled,omitempty"`
	Address         *string            `json:"address,omitempty"`
	AmountTolerance *float64           `json:"amount_tolerance,omitempty"`
	Confirmations   *int               `json:"confirmations,omitempty"`
	AutoActivate    *bool              `json:"auto_activate,omitempty"`
	Network         *string            `json:"network,omitempty"`
	APIKey          *string            `json:"api_key,omitempty"`
	Networks        *[]string          `json:"networks,omitempty"`
	RPCNodes        *[]string          `json:"rpc_nodes,omitempty"`
	Epay            *EpayUpdateRequest `json:"epay,omitempty"`
}

type EpayUpdateRequest struct {
	Pid        *string `json:"pid,omitempty"`
	Key        *string `json:"key,omitempty"`
	GatewayURL *string `json:"gateway_url,omitempty"`
	PayType    *string `json:"pay_type,omitempty"`
	NotifyURL  *string `json:"notify_url,omitempty"`
	ReturnURL  *string `json:"return_url,omitempty"`
}

// UpdatePaymentMethod 更新支付方式配置
// PUT /admin/payment-methods/:method
func (h *AdminPaymentHandler) UpdatePaymentMethod(c *gin.Context) {
	method := c.Param("method")
	var req UpdatePaymentMethodRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.ValidationError(c, err.Error())
		return
	}

	// 读取当前配置
	group := "payment"
	// 映射 method 名到 system_settings 的 setting_key
	// usdt_trc20 → trc20, usdt_erc20 → erc20, usdt_bep20 → bep20, wechat/alipay 保持不变
	key := method
	switch method {
	case "usdt_trc20":
		key = "trc20"
	case "usdt_erc20":
		key = "erc20"
	case "usdt_bep20":
		key = "bep20"
	}

	// 根据方法名确定配置结构
	cfg := h.getMethodConfig(method)
	if cfg == nil {
		server.ValidationError(c, "unsupported payment method")
		return
	}
	// api_key_configured 仅用于接口展示，不应写入持久化配置
	delete(cfg, "api_key_configured")
	// 保存其它字段时保留已配置的 EVM API Key，避免覆盖丢失
	if key == "erc20" {
		if cur := h.paymentSvc.GetERC20Config(); cur.EtherscanAPIKey != "" {
			cfg["etherscan_api_key"] = cur.EtherscanAPIKey
		}
	}

	// 应用更新
	if req.Enabled != nil {
		cfg["enabled"] = *req.Enabled
	}
	if req.Address != nil {
		cfg["address"] = strings.TrimSpace(*req.Address)
	}
	if req.AmountTolerance != nil {
		cfg["amount_tolerance"] = *req.AmountTolerance
	}
	if req.Confirmations != nil {
		cfg["min_confirmations"] = *req.Confirmations
	}
	if req.AutoActivate != nil {
		cfg["auto_activate"] = *req.AutoActivate
	}
	if req.Network != nil {
		cfg["network"] = *req.Network
	}
	if req.APIKey != nil {
		cfg["etherscan_api_key"] = *req.APIKey
	}
	if req.Networks != nil {
		nets := make([]string, 0, len(*req.Networks))
		for _, n := range *req.Networks {
			n = strings.ToLower(strings.TrimSpace(n))
			if n == "polygon" || n == "arbitrum" || n == "bsc" {
				nets = append(nets, n)
			}
		}
		cfg["networks"] = nets
		if len(nets) > 0 {
			cfg["network"] = nets[0]
		}
	}
	if req.RPCNodes != nil && key == "bep20" {
		nodes := make([]string, 0, len(*req.RPCNodes))
		for _, n := range *req.RPCNodes {
			if s := strings.TrimSpace(n); s != "" {
				nodes = append(nodes, s)
			}
		}
		if len(nodes) > 0 {
			cfg["rpc_nodes"] = nodes
		}
	}
	if req.Epay != nil && (method == "wechat" || method == "alipay") {
		var cur service.EpayConfig
		if method == "wechat" {
			cur = h.paymentSvc.GetWechatConfig().Epay
		} else {
			cur = h.paymentSvc.GetAlipayConfig().Epay
		}
		epayMap := map[string]interface{}{
			"pid":         cur.Pid,
			"gateway_url": cur.GatewayURL,
			"pay_type":    cur.PayType,
			"notify_url":  cur.NotifyURL,
			"return_url":  cur.ReturnURL,
		}
		if cur.Key != "" {
			epayMap["key"] = cur.Key
		}
		if req.Epay.Pid != nil {
			epayMap["pid"] = *req.Epay.Pid
		}
		if req.Epay.Key != nil {
			if *req.Epay.Key != "" {
				epayMap["key"] = *req.Epay.Key
			} else {
				delete(epayMap, "key")
			}
		}
		if req.Epay.GatewayURL != nil {
			epayMap["gateway_url"] = *req.Epay.GatewayURL
		}
		if req.Epay.PayType != nil {
			epayMap["pay_type"] = *req.Epay.PayType
		}
		if req.Epay.NotifyURL != nil {
			epayMap["notify_url"] = *req.Epay.NotifyURL
		}
		if req.Epay.ReturnURL != nil {
			epayMap["return_url"] = *req.Epay.ReturnURL
		}
		cfg["epay"] = epayMap
	}

	adminID := getAdminIDFromContext(c)
	desc := method + " payment configuration"
	_, err := h.settingRepo.SetByGroupKey(c.Request.Context(), group, key, cfg, false, &desc, &adminID)
	if err != nil {
		server.InternalError(c, "failed to save payment config")
		return
	}

	// 重新加载配置
	h.paymentSvc.ReloadConfigs()

	server.OK(c, gin.H{"method": method, "config": cfg, "updated": true})
}

// TogglePaymentMethod 启用/禁用支付方式
// POST /admin/payment-methods/:method/toggle
func (h *AdminPaymentHandler) TogglePaymentMethod(c *gin.Context) {
	method := c.Param("method")
	cfg := h.getMethodConfig(method)
	if cfg == nil {
		server.ValidationError(c, "unsupported payment method")
		return
	}
	// api_key_configured 仅用于接口展示，不应写入持久化配置
	delete(cfg, "api_key_configured")
	// toggle 时同样保留已配置的 EVM API Key
	if method == "usdt_erc20" || method == "erc20" {
		if cur := h.paymentSvc.GetERC20Config(); cur.EtherscanAPIKey != "" {
			cfg["etherscan_api_key"] = cur.EtherscanAPIKey
		}
	}

	currentEnabled, _ := cfg["enabled"].(bool)
	cfg["enabled"] = !currentEnabled

	// 映射 method 名到 system_settings 的 setting_key
	settingKey := method
	switch method {
	case "usdt_trc20":
		settingKey = "trc20"
	case "usdt_erc20":
		settingKey = "erc20"
	case "usdt_bep20":
		settingKey = "bep20"
	}

	adminID := getAdminIDFromContext(c)
	desc := method + " payment configuration"
	_, err := h.settingRepo.SetByGroupKey(c.Request.Context(), "payment", settingKey, cfg, false, &desc, &adminID)
	if err != nil {
		server.InternalError(c, "failed to toggle payment method")
		return
	}

	h.paymentSvc.ReloadConfigs()

	server.OK(c, gin.H{"method": method, "enabled": !currentEnabled})
}

// getMethodConfig 获取支付方式配置（返回 map 便于修改）
func (h *AdminPaymentHandler) getMethodConfig(method string) map[string]interface{} {
	switch method {
	case "usdt_trc20", "trc20":
		cfg := h.paymentSvc.GetTRC20Config()
		return map[string]interface{}{
			"method":            "usdt_trc20",
			"name":              "USDT-TRC20",
			"enabled":           cfg.Enabled,
			"address":           cfg.Address,
			"amount_tolerance":  cfg.AmountTolerance,
			"min_confirmations": cfg.MinConfirmations,
			"network":           "tron",
			"auto_activate":     cfg.AutoActivate,
		}
	case "usdt_erc20", "erc20":
		cfg := h.paymentSvc.GetERC20Config()
		return map[string]interface{}{
			"method":             "usdt_erc20",
			"name":               "USDT-EVM（Polygon / Arbitrum One 双通道）",
			"enabled":            cfg.Enabled,
			"address":            cfg.Address,
			"amount_tolerance":   cfg.AmountTolerance,
			"min_confirmations":  cfg.MinConfirmations,
			"network":            cfg.Network,
			"networks":           cfg.EnabledNetworks(),
			"auto_activate":      cfg.AutoActivate,
			"api_key_configured": cfg.EtherscanAPIKey != "",
		}
	case "usdt_bep20", "bep20":
		cfg := h.paymentSvc.GetBEP20Config()
		return map[string]interface{}{
			"method":            "usdt_bep20",
			"name":              "USDT-BEP20",
			"enabled":           cfg.Enabled,
			"address":           cfg.Address,
			"amount_tolerance":  cfg.AmountTolerance,
			"min_confirmations": cfg.MinConfirmations,
			"network":           "bsc",
			"auto_activate":     cfg.AutoActivate,
			"rpc_nodes":         cfg.BscRPC,
		}
	case "wechat":
		cfg := h.paymentSvc.GetWechatConfig()
		return map[string]interface{}{
			"method":             "wechat",
			"name":               "微信支付",
			"enabled":            cfg.Enabled,
			"auto_activate":      cfg.AutoActivate,
			"order_expiry_hours": cfg.OrderExpiryHours,
			"epay":               epayConfigMap(cfg.Epay),
			"epay_configured":    cfg.Epay.Configured(),
		}
	case "alipay":
		cfg := h.paymentSvc.GetAlipayConfig()
		return map[string]interface{}{
			"method":             "alipay",
			"name":               "支付宝",
			"enabled":            cfg.Enabled,
			"auto_activate":      cfg.AutoActivate,
			"order_expiry_hours": cfg.OrderExpiryHours,
			"epay":               epayConfigMap(cfg.Epay),
			"epay_configured":    cfg.Epay.Configured(),
		}
	default:
		return nil
	}
}

func epayConfigMap(cfg service.EpayConfig) map[string]interface{} {
	return map[string]interface{}{
		"pid":            cfg.Pid,
		"gateway_url":    cfg.GatewayURL,
		"pay_type":       cfg.PayType,
		"notify_url":     cfg.NotifyURL,
		"return_url":     cfg.ReturnURL,
		"key_configured": cfg.Key != "",
	}
}

// GetExchangeRate 获取 USDT 到 CNY 汇率配置
// GET /admin/payment-methods/exchange-rate
func (h *AdminPaymentHandler) GetExchangeRate(c *gin.Context) {
	data, err := h.settingRepo.GetJSON(c.Request.Context(), "payment", "exchange_rate")
	if err != nil {
		// 回退默认值
		server.OK(c, gin.H{
			"usdt_to_cny":  7.2,
			"auto_update":  false,
			"last_updated": nil,
		})
		return
	}
	var cfg map[string]interface{}
	_ = json.Unmarshal(data, &cfg)
	if cfg == nil {
		cfg = gin.H{"usdt_to_cny": 7.2, "auto_update": false}
	}
	server.OK(c, cfg)
}

// UpdateExchangeRateRequest 更新汇率请求
type UpdateExchangeRateRequest struct {
	USDTToCNY  float64 `json:"usdt_to_cny"`
	AutoUpdate *bool   `json:"auto_update,omitempty"`
}

// UpdateExchangeRate 更新 USDT 到 CNY 汇率
// PUT /admin/payment-methods/exchange-rate
func (h *AdminPaymentHandler) UpdateExchangeRate(c *gin.Context) {
	var req UpdateExchangeRateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.ValidationError(c, err.Error())
		return
	}
	if req.USDTToCNY <= 0 {
		server.ValidationError(c, "usdt_to_cny must be greater than 0")
		return
	}

	cfg := map[string]interface{}{
		"usdt_to_cny":  req.USDTToCNY,
		"auto_update":  false,
		"last_updated": time.Now().UTC().Format(time.RFC3339),
	}
	if req.AutoUpdate != nil {
		cfg["auto_update"] = *req.AutoUpdate
	}

	desc := "USDT到CNY汇率配置"
	adminID := getAdminIDFromContext(c)
	_, err := h.settingRepo.SetByGroupKey(c.Request.Context(), "payment", "exchange_rate", cfg, false, &desc, &adminID)
	if err != nil {
		server.InternalError(c, "failed to save exchange rate")
		return
	}

	// 重新加载配置（含汇率）
	h.paymentSvc.ReloadConfigs()

	server.OK(c, gin.H{"updated": true, "config": cfg})
}

// getAdminIDFromContext 从上下文获取管理员 ID
func getAdminIDFromContext(c *gin.Context) uuid.UUID {
	if v, exists := c.Get("admin_id"); exists {
		if id, ok := v.(uuid.UUID); ok {
			return id
		}
	}
	return uuid.Nil
}
