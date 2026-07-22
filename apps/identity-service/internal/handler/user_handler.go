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
		"user_id":              result.User.ID,
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

	order, err := h.paymentSvc.CreateOrder(c.Request.Context(), userID, req)
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
	wechat := h.paymentSvc.GetWechatConfig()
	alipay := h.paymentSvc.GetAlipayConfig()
	rate := h.paymentSvc.GetExchangeRate()

	type paymentMethod struct {
		Method   string  `json:"method"`
		Name     string  `json:"name"`
		Currency string  `json:"currency"`
		Enabled  bool    `json:"enabled"`
		Fiat     bool    `json:"fiat"`
	}
	methods := []paymentMethod{
		{Method: model.PaymentMethodAlipay, Name: "支付宝", Currency: "CNY", Enabled: alipay.Enabled, Fiat: true},
		{Method: model.PaymentMethodWechat, Name: "微信支付", Currency: "CNY", Enabled: wechat.Enabled, Fiat: true},
		{Method: model.PaymentMethodUSDTTRC20, Name: "USDT-TRC20", Currency: "USDT", Enabled: trc20.Enabled && trc20.Address != "", Fiat: false},
		{Method: model.PaymentMethodUSDTERC20, Name: "USDT-ERC20", Currency: "USDT", Enabled: erc20.Enabled && erc20.Address != "", Fiat: false},
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
