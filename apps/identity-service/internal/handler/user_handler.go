package handler

import (
	"net/http"

	"github.com/airport-panel/config/server"
	"github.com/airport-panel/identity-service/internal/middleware"
	"github.com/airport-panel/identity-service/internal/model"
	"github.com/airport-panel/identity-service/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type UserHandler struct {
	userSvc    *service.UserService
	paymentSvc *service.PaymentService
}

func NewUserHandler(userSvc *service.UserService, paymentSvc *service.PaymentService) *UserHandler {
	return &UserHandler{
		userSvc:    userSvc,
		paymentSvc: paymentSvc,
	}
}

func (h *UserHandler) Register(c *gin.Context) {
	var req model.UserRegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.ValidationError(c, err.Error())
		return
	}

	ip := c.ClientIP()
	result, err := h.userSvc.Register(c.Request.Context(), &req, ip)
	if err != nil {
		code, msg := service.MapErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	resp := gin.H{
		"user_id":               result.User.ID,
		"requires_verification": result.User.Status == model.UserStatusPending,
	}
	if result.SubscriptionToken != "" {
		resp["subscription_token"] = result.SubscriptionToken
	}
	server.Created(c, resp)
}

func (h *UserHandler) VerifyEmail(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		server.ValidationError(c, "token is required")
		return
	}

	if err := h.userSvc.VerifyEmail(c.Request.Context(), token); err != nil {
		code, msg := service.MapErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	frontendBase := c.GetHeader("Origin")
	if frontendBase == "" {
		frontendBase = "/"
	}
	c.Redirect(http.StatusFound, frontendBase+"/login?verified=1")
}

func (h *UserHandler) ForgotPassword(c *gin.Context) {
	var req model.ForgotPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.ValidationError(c, err.Error())
		return
	}

	if err := h.userSvc.ForgotPassword(c.Request.Context(), req.Email); err != nil {
		code, msg := service.MapErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, gin.H{"message": "if email exists, reset link sent"})
}

func (h *UserHandler) ResetPassword(c *gin.Context) {
	var req model.ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.ValidationError(c, err.Error())
		return
	}

	if err := h.userSvc.ResetPassword(c.Request.Context(), req.Token, req.NewPassword); err != nil {
		code, msg := service.MapErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, gin.H{"message": "password reset successful"})
}

func (h *UserHandler) GetMe(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		server.Unauthorized(c, "")
		return
	}

	user, profile, sub, err := h.userSvc.GetUserDetail(c.Request.Context(), userID)
	if err != nil {
		code, msg := service.MapErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, model.NewUserDetailResponse(user, profile, sub))
}

func (h *UserHandler) UpdateMe(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		server.Unauthorized(c, "")
		return
	}

	var req model.UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.ValidationError(c, err.Error())
		return
	}

	ip := c.ClientIP()
	if err := h.userSvc.UpdateProfile(c.Request.Context(), userID, &req, ip); err != nil {
		code, msg := service.MapErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	user, profile, sub, err := h.userSvc.GetUserDetail(c.Request.Context(), userID)
	if err != nil {
		server.InternalError(c, "")
		return
	}

	server.OK(c, model.NewUserDetailResponse(user, profile, sub))
}

func (h *UserHandler) GetSubscription(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		server.Unauthorized(c, "")
		return
	}

	sub, err := h.userSvc.GetSubscription(c.Request.Context(), userID)
	if err != nil {
		code, msg := service.MapErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, sub)
}

func (h *UserHandler) ListSubscriptionTokens(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		server.Unauthorized(c, "")
		return
	}

	tokens, err := h.userSvc.ListSubscriptionTokens(c.Request.Context(), userID)
	if err != nil {
		server.InternalError(c, "")
		return
	}

	resp := make([]model.SubscriptionTokenResponse, len(tokens))
	for i, t := range tokens {
		resp[i] = model.NewSubscriptionTokenResponse(t)
	}

	server.OK(c, resp)
}

func (h *UserHandler) CreateSubscriptionToken(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		server.Unauthorized(c, "")
		return
	}

	var req model.CreateTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.ValidationError(c, err.Error())
		return
	}

	ip := c.ClientIP()
	token, rawToken, err := h.userSvc.CreateSubscriptionToken(c.Request.Context(), userID, req.ClientHint, ip)
	if err != nil {
		code, msg := service.MapErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	resp := model.NewSubscriptionTokenResponse(token)
	resp.Token = rawToken

	server.Created(c, resp)
}

func (h *UserHandler) RevokeSubscriptionToken(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		server.Unauthorized(c, "")
		return
	}

	tokenIDStr := c.Param("id")
	tokenID, err := uuid.Parse(tokenIDStr)
	if err != nil {
		server.ValidationError(c, "invalid token id")
		return
	}

	ip := c.ClientIP()
	if err := h.userSvc.RevokeSubscriptionToken(c.Request.Context(), userID, tokenID, ip); err != nil {
		code, msg := service.MapErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, nil)
}

func (h *UserHandler) ResetSubscriptionToken(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		server.Unauthorized(c, "")
		return
	}

	tokenIDStr := c.Param("id")
	tokenID, err := uuid.Parse(tokenIDStr)
	if err != nil {
		server.ValidationError(c, "invalid token id")
		return
	}

	ip := c.ClientIP()
	token, rawToken, err := h.userSvc.ResetSubscriptionToken(c.Request.Context(), userID, tokenID, ip)
	if err != nil {
		code, msg := service.MapErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	resp := model.NewSubscriptionTokenResponse(token)
	resp.Token = rawToken

	server.OK(c, resp)
}

func (h *UserHandler) ResetAllSubscriptionTokens(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		server.Unauthorized(c, "")
		return
	}

	ip := c.ClientIP()
	token, rawToken, err := h.userSvc.ResetAllSubscriptionTokens(c.Request.Context(), userID, ip)
	if err != nil {
		code, msg := service.MapErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	resp := model.NewSubscriptionTokenResponse(token)
	resp.Token = rawToken

	server.OK(c, resp)
}

func (h *UserHandler) ListPlans(c *gin.Context) {
	plans, err := h.userSvc.ListActivePlans(c.Request.Context())
	if err != nil {
		server.InternalError(c, "")
		return
	}

	resp := make([]model.PlanResponse, len(plans))
	for i, p := range plans {
		pr := model.NewPlanResponse(p)
		prices := make([]model.PlanPrice, 0)
		for period, entry := range p.Prices {
			prices = append(prices, model.PlanPrice{
				PeriodCode: period,
				PriceUSDT:  entry.USDT,
				PriceCNY:   entry.CNY,
			})
		}
		pr.Prices = prices
		resp[i] = pr
	}

	server.OK(c, resp)
}

func (h *UserHandler) GetPlan(c *gin.Context) {
	planIDStr := c.Param("id")
	planID, err := uuid.Parse(planIDStr)
	if err != nil {
		server.ValidationError(c, "invalid plan id")
		return
	}

	plan, err := h.userSvc.GetPlan(c.Request.Context(), planID)
	if err != nil {
		code, msg := service.MapErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	pr := model.NewPlanResponse(plan)
	prices := make([]model.PlanPrice, 0)
	for period, entry := range plan.Prices {
		prices = append(prices, model.PlanPrice{
			PeriodCode: period,
			PriceUSDT:  entry.USDT,
			PriceCNY:   entry.CNY,
		})
	}
	pr.Prices = prices

	server.OK(c, pr)
}

func (h *UserHandler) CreateOrder(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		server.Unauthorized(c, "")
		return
	}

	var req model.CreateOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.ValidationError(c, err.Error())
		return
	}

	// V2 易支付统一下单 clientip 必填：把客户端 IP 注入 ctx 供网关读取
	ctx := service.WithClientIP(c.Request.Context(), c.ClientIP())
	order, err := h.paymentSvc.CreateOrder(ctx, userID, req)
	if err != nil {
		code, msg := service.MapErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.Created(c, model.NewOrderResponse(order))
}

func (h *UserHandler) ListOrders(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		server.Unauthorized(c, "")
		return
	}

	var query model.OrderListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		server.ValidationError(c, err.Error())
		return
	}

	if query.Page <= 0 {
		query.Page = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = 20
	}

	orders, total, err := h.paymentSvc.ListUserOrders(c.Request.Context(), userID, query.Page, query.PageSize, query.Status)
	if err != nil {
		server.InternalError(c, "")
		return
	}

	items := make([]model.OrderResponse, len(orders))
	for i, o := range orders {
		items[i] = model.NewOrderResponse(o)
	}

	server.OK(c, model.PaginationResponse{
		Page:     query.Page,
		PageSize: query.PageSize,
		Total:    total,
		Items:    items,
	})
}

func (h *UserHandler) GetOrder(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		server.Unauthorized(c, "")
		return
	}

	orderIDStr := c.Param("id")
	orderID, err := uuid.Parse(orderIDStr)
	if err != nil {
		server.ValidationError(c, "invalid order id")
		return
	}

	order, err := h.paymentSvc.GetOrder(c.Request.Context(), userID, orderID)
	if err != nil {
		code, msg := service.MapErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, model.NewOrderResponse(order))
}

// ListPaymentMethods 列出用户可用的支付方式
// GET /api/v1/user/payment-methods
func (h *UserHandler) ListPaymentMethods(c *gin.Context) {
	trc20 := h.paymentSvc.GetTRC20Config()
	erc20 := h.paymentSvc.GetERC20Config()
	bep20 := h.paymentSvc.GetBEP20Config()
	wechat := h.paymentSvc.GetWechatConfig()
	alipay := h.paymentSvc.GetAlipayConfig()
	rate := h.paymentSvc.GetExchangeRate()

	type paymentMethod struct {
		Method   string   `json:"method"`
		Name     string   `json:"name"`
		Currency string   `json:"currency"`
		Enabled  bool     `json:"enabled"`
		Fiat     bool     `json:"fiat"`
		Network  string   `json:"network,omitempty"`
		Networks []string `json:"networks,omitempty"`
		Hint     string   `json:"hint,omitempty"`
	}
	// USDT 通用提示：用户端结算界面展示（请勿选错网络 / 手续费极低 / 优惠码见公告）
	const usdtHint = "请勿选错网络，USDT手续费极低真实可用，优惠码见系统公告"
	// 支付宝/微信仅在绑定的法币渠道真实配置后才对用户开放，避免“显示启用但无法支付”；
	// BEP20(BSC) 因公共 RPC 无法查询 USDT 日志（日志量超限）无法自动到账，按 08-02 设计停用，不再展示给用户。
	channels := h.paymentSvc.GetFiatChannels()
	channelBound := func(channelID string) bool {
		ch := channels.FindChannel(channelID)
		return ch != nil && ch.Configured()
	}
	methods := []paymentMethod{
		{Method: model.PaymentMethodAlipay, Name: "支付宝", Currency: "CNY", Enabled: alipay.Enabled && channelBound(channels.AlipayChannel), Fiat: true},
		{Method: model.PaymentMethodWechat, Name: "微信支付", Currency: "CNY", Enabled: wechat.Enabled && channelBound(channels.WechatChannel), Fiat: true},
		{Method: model.PaymentMethodUSDTTRC20, Name: "USDT-TRC20", Currency: "USDT", Enabled: trc20.Enabled && trc20.Address != "", Fiat: false, Network: "tron", Hint: usdtHint},
		{Method: model.PaymentMethodUSDTBEP20, Name: "USDT-BEP20", Currency: "USDT", Enabled: bep20.Enabled && bep20.Address != "", Fiat: false, Network: "bsc", Hint: usdtHint},
		{Method: model.PaymentMethodUSDTERC20, Name: "USDT", Currency: "USDT", Enabled: erc20.Enabled && erc20.Address != "" && len(erc20.EnabledNetworks()) > 0, Fiat: false, Network: erc20.Network, Networks: erc20.EnabledNetworks(), Hint: usdtHint},
	}

	// 只返回启用的支付方式
	enabled := make([]paymentMethod, 0, len(methods))
	for _, m := range methods {
		if m.Enabled {
			enabled = append(enabled, m)
		}
	}

	server.OK(c, gin.H{
		"methods":       enabled,
		"exchange_rate": rate,
		"base_currency": "CNY",
	})
}
