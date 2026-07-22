package handler

import (
	"bytes"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/airport-panel/config/server"
	"github.com/airport-panel/subscription-service/internal/client"
	"github.com/airport-panel/subscription-service/internal/middleware"
	"github.com/airport-panel/subscription-service/internal/model"
	"github.com/airport-panel/subscription-service/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type SubscriptionHandler struct {
	subService *service.SubscriptionService
}

func NewSubscriptionHandler(subService *service.SubscriptionService) *SubscriptionHandler {
	return &SubscriptionHandler{
		subService: subService,
	}
}

func (h *SubscriptionHandler) getClientIP(c *gin.Context) string {
	// X-Forwarded-For: 取最左侧(最原始)的非空客户端 IP。跳过畸形 XFF
	// 中的空段(如 ", 1.2.3.4" 的首段为空), 取首个有效 IP;
	// 若所有段都为空/空白, 回退到 X-Real-IP / ClientIP。
	if xff := c.GetHeader("X-Forwarded-For"); xff != "" {
		for _, part := range strings.Split(xff, ",") {
			if ip := strings.TrimSpace(part); ip != "" {
				return ip
			}
		}
	}
	if xr := strings.TrimSpace(c.GetHeader("X-Real-IP")); xr != "" {
		return xr
	}
	return c.ClientIP()
}

func (h *SubscriptionHandler) GetSubscription(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		c.String(http.StatusBadRequest, "invalid token")
		return
	}

	clientParam := c.Query("client")
	var clientType model.ClientType
	if clientParam != "" {
		clientType = client.NormalizeClientType(clientParam)
	} else {
		clientType = client.DetectClient(c.GetHeader("User-Agent"))
		if clientType == model.ClientTypeURI && c.GetHeader("Client-Type") != "" {
			clientType = client.NormalizeClientType(c.GetHeader("Client-Type"))
		}
	}
	userAgent := c.GetHeader("User-Agent")
	clientIP := h.getClientIP(c)

	result, err := h.subService.GetSubscription(c.Request.Context(), token, clientType, userAgent, clientIP)
	if err != nil {
		code, msg := service.MapServiceErrorToCode(err)
		c.String(code.HTTPStatus(), msg)
		return
	}

	h.writeSubscription(c, result, clientType, userAgent)
}

func (h *SubscriptionHandler) writeSubscription(c *gin.Context, result *service.SubscriptionResult, ct model.ClientType, userAgent string) {
	content := result.Content
	contentType := result.ContentType
	userInfo := result.UserInfo

	ua := strings.ToLower(userAgent)
	isShadowrocket := ct == model.ClientTypeShadowrocket || strings.Contains(ua, "shadowrocket")
	// Quantumult 与 QuantumultX 均渲染为 URI 格式, 都需要 sanitize。
	// 注意: UA 含 "quantumult" 时两者都匹配, 但纯 ?client=quanx (无 UA) 路径下
	// 仅 ct==QuantumultX 为真, 必须显式纳入否则 sanitize 被跳过 (Bug-E)。
	isQuantumult := ct == model.ClientTypeQuantumult || ct == model.ClientTypeQuantumultX || strings.Contains(ua, "quantumult")
	isSurge := ct == model.ClientTypeSurge || strings.Contains(ua, "surge")
	isStrictURI := isShadowrocket || isQuantumult || isSurge

	if isStrictURI {
		content = sanitizeSubscriptionContent(content)
	}

	c.Header("Content-Type", contentType)
	c.Header("Subscription-Userinfo", userInfo)
	// Profile-Update-Interval: 客户端订阅更新间隔（小时），所有客户端统一为 6 小时
	c.Header("Profile-Update-Interval", "6")
	// Content-Disposition: 所有订阅响应都附带附件文件名，便于客户端保存
	c.Header("Content-Disposition", `attachment; filename="`+subscriptionFilename(ct)+`"`)
	// Profile-Title / Profile-Web-Page-URL: 订阅元信息头，便于客户端展示订阅来源。
	// 当前 subscription-service 未注入站点设置存储，使用硬编码默认站点名 "YunDu"
	// 与请求 Host 作为回退；如后续接入 settings 存储，可改为从配置读取 site_name/site_url。
	c.Header("Profile-Title", "YunDu")
	c.Header("Profile-Web-Page-URL", c.Request.Host)
	if isShadowrocket {
		c.Header("Cache-Control", "private, no-store, no-cache, must-revalidate, max-age=0")
		c.Header("Access-Control-Allow-Origin", "*")
	} else if isSurge {
		c.Header("Cache-Control", "private, max-age=3600")
	} else {
		c.Header("Cache-Control", "private, no-store")
	}

	if strings.HasPrefix(contentType, "text/") {
		c.String(http.StatusOK, content)
	} else {
		c.Data(http.StatusOK, contentType, []byte(content))
	}
}

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// subscriptionFilename 根据客户端类型返回合适的订阅文件名。
func subscriptionFilename(ct model.ClientType) string {
	switch client.ClientToRenderer(ct) {
	case "clash", "clashmeta":
		return "subscription.yaml"
	case "singbox":
		return "subscription.json"
	case "surge":
		return "subscription.conf"
	default:
		return "subscription"
	}
}

func sanitizeSubscriptionContent(content string) string {
	b := []byte(content)
	b = bytes.TrimPrefix(b, utf8BOM)
	for bytes.HasPrefix(b, utf8BOM) {
		b = bytes.TrimPrefix(b, utf8BOM)
	}
	s := string(b)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	lines := strings.Split(s, "\n")
	var cleaned []string
	for _, line := range lines {
		trimmed := strings.TrimRight(line, " \t")
		if strings.TrimSpace(trimmed) == "" && (len(cleaned) == 0 || len(cleaned) > 0 && cleaned[len(cleaned)-1] == "") {
			continue
		}
		cleaned = append(cleaned, trimmed)
	}
	for len(cleaned) > 0 && cleaned[len(cleaned)-1] == "" {
		cleaned = cleaned[:len(cleaned)-1]
	}
	if len(cleaned) == 0 {
		return ""
	}
	return strings.Join(cleaned, "\n") + "\n"
}

func (h *SubscriptionHandler) GetSubscriptionInfo(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		server.BadRequest(c, "invalid token")
		return
	}

	_, info, err := h.subService.ValidateToken(c.Request.Context(), token)
	if err != nil {
		code, msg := service.MapServiceErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	upload := int64(0)
	download := info.TrafficUsedBytes
	total := info.TrafficQuotaBytes
	if total < download {
		total = download
	}

	server.OK(c, model.SubscriptionInfoResponse{
		Upload:            upload,
		Download:          download,
		Total:             total,
		Expire:            info.ExpiresAt,
		TrafficQuotaBytes: info.TrafficQuotaBytes,
		TrafficUsedBytes:  info.TrafficUsedBytes,
		IsExpired:         info.IsExpired,
		IsOverQuota:       info.IsOverQuota,
	})
}

func (h *SubscriptionHandler) RegisterPublicRoutes(r *gin.Engine) {
	r.GET("/sub/:token", h.GetSubscription)
	r.GET("/sub/:token/info", h.GetSubscriptionInfo)
}

func (h *SubscriptionHandler) ListTokens(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	status := c.Query("status")
	var userID *uuid.UUID
	if uidStr := c.Query("user_id"); uidStr != "" {
		uid, err := uuid.Parse(uidStr)
		if err == nil {
			userID = &uid
		}
	}

	tokens, total, err := h.subService.ListTokens(c.Request.Context(), page, pageSize, status, userID)
	if err != nil {
		server.InternalError(c, "")
		return
	}

	items := make([]model.TokenResponse, len(tokens))
	for i, t := range tokens {
		items[i] = model.NewTokenResponse(t)
	}

	server.OK(c, model.PaginationResponse{
		Page:     page,
		PageSize: pageSize,
		Total:    total,
		Items:    items,
	})
}

func (h *SubscriptionHandler) CreateToken(c *gin.Context) {
	var req model.CreateTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}

	token, err := h.subService.GenerateToken(c.Request.Context(), req.UserID, req.ExpiresAt)
	if err != nil {
		server.InternalError(c, err.Error())
		return
	}

	server.Created(c, model.NewTokenResponseWithValue(token))
}

func (h *SubscriptionHandler) RevokeToken(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		server.BadRequest(c, "invalid token id")
		return
	}

	if err := h.subService.RevokeToken(c.Request.Context(), id); err != nil {
		code, msg := service.MapServiceErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.NoContent(c)
}

func (h *SubscriptionHandler) ListTemplates(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	clientType := model.ClientType(c.Query("client_type"))

	templates, total, err := h.subService.ListTemplates(c.Request.Context(), page, pageSize, clientType)
	if err != nil {
		server.InternalError(c, "")
		return
	}

	items := make([]model.TemplateResponse, len(templates))
	for i, t := range templates {
		items[i] = model.NewTemplateResponse(t, h.subService.IsDefaultTemplate(t))
	}

	server.OK(c, model.PaginationResponse{
		Page:     page,
		PageSize: pageSize,
		Total:    total,
		Items:    items,
	})
}

func (h *SubscriptionHandler) CreateTemplate(c *gin.Context) {
	var req model.CreateTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}

	adminID := middleware.GetAdminID(c)
	tmpl, err := h.subService.CreateTemplate(c.Request.Context(), &req, &adminID)
	if err != nil {
		server.InternalError(c, err.Error())
		return
	}

	server.Created(c, model.NewTemplateResponse(tmpl, false))
}

func (h *SubscriptionHandler) SetDefaultTemplate(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		server.BadRequest(c, "invalid template id")
		return
	}

	if err := h.subService.SetDefaultTemplate(c.Request.Context(), id); err != nil {
		code, msg := service.MapServiceErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, gin.H{"status": "ok"})
}

// GetShortCode 按 token 获取（或生成）短码
func (h *SubscriptionHandler) GetShortCode(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		server.BadRequest(c, "invalid token")
		return
	}
	// 直接生成短码（默认 7 天有效期）
	sc, err := h.subService.GenerateShortCode(c.Request.Context(), token, 7*24*3600, "")
	if err != nil {
		code, msg := service.MapServiceErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.OK(c, gin.H{
		"short_code": sc.ShortCode,
		"expires_at": sc.ExpiresAt,
	})
}

// GetUserAccessStats 用户访问统计
func (h *SubscriptionHandler) GetUserAccessStats(c *gin.Context) {
	userID := middleware.GetUserID(c)
	days, _ := strconv.Atoi(c.DefaultQuery("days", "7"))
	if days <= 0 || days > 90 {
		days = 7
	}
	stats, err := h.subService.GetUserAccessStats(c.Request.Context(), userID, days)
	if err != nil {
		server.InternalError(c, err.Error())
		return
	}
	server.OK(c, stats)
}

// UpdateTemplate 更新模板
func (h *SubscriptionHandler) UpdateTemplate(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		server.BadRequest(c, "invalid template id")
		return
	}
	var req model.UpdateTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}
	req.ID = id
	tmpl, err := h.subService.UpdateTemplate(c.Request.Context(), &req)
	if err != nil {
		code, msg := service.MapServiceErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.OK(c, model.NewTemplateResponse(tmpl, h.subService.IsDefaultTemplate(tmpl)))
}

// GenerateShortCode 管理端生成短码
func (h *SubscriptionHandler) GenerateShortCode(c *gin.Context) {
	var req model.ShortCodeCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}
	if req.ExpiresIn <= 0 {
		req.ExpiresIn = 7 * 24 * 3600
	}
	sc, err := h.subService.GenerateShortCode(c.Request.Context(), req.Token, req.ExpiresIn, req.Description)
	if err != nil {
		code, msg := service.MapServiceErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.Created(c, sc)
}

// RevokeShortCode 按短码撤销
func (h *SubscriptionHandler) RevokeShortCode(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		server.BadRequest(c, "invalid short code")
		return
	}
	if err := h.subService.RevokeShortCodeByCode(c.Request.Context(), code); err != nil {
		code, msg := service.MapServiceErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.NoContent(c)
}

// GetAccessOverview 访问概览
func (h *SubscriptionHandler) GetAccessOverview(c *gin.Context) {
	startStr := c.Query("start")
	endStr := c.Query("end")
	if endStr == "" {
		server.BadRequest(c, "end is required")
		return
	}
	end, err := time.Parse(time.RFC3339, endStr)
	if err != nil {
		server.BadRequest(c, "invalid end time")
		return
	}
	var start time.Time
	if startStr == "" {
		start = end.AddDate(0, 0, -7)
	} else {
		start, err = time.Parse(time.RFC3339, startStr)
		if err != nil {
			server.BadRequest(c, "invalid start time")
			return
		}
	}
	overview, err := h.subService.GetAccessOverview(c.Request.Context(), start, end)
	if err != nil {
		server.InternalError(c, err.Error())
		return
	}
	server.OK(c, overview)
}

// GetAccessLogs 访问日志列表
func (h *SubscriptionHandler) GetAccessLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	var userID *uuid.UUID
	if uidStr := c.Query("user_id"); uidStr != "" {
		uid, err := uuid.Parse(uidStr)
		if err == nil {
			userID = &uid
		}
	}
	logs, total, err := h.subService.ListAccessLogs(c.Request.Context(), userID, page, pageSize)
	if err != nil {
		server.InternalError(c, err.Error())
		return
	}
	server.OK(c, model.PaginationResponse{
		Page:     page,
		PageSize: pageSize,
		Total:    total,
		Items:    logs,
	})
}

// GetSubscriptionQR 订阅二维码（返回订阅 URL 文本）
func (h *SubscriptionHandler) GetSubscriptionQR(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		server.BadRequest(c, "invalid token")
		return
	}
	// 订阅 URL 必须用固定的 SUB_BASE_URL，不能用 c.Request.Host
	//（Request.Host 会随访问域名变化，导致订阅地址不稳定）
	subBaseURL := os.Getenv("SUB_BASE_URL")
	if subBaseURL == "" {
		subBaseURL = "https://ad.tiktokplay.na.am"
	}
	subURL := subBaseURL + "/sub/" + token
	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.String(http.StatusOK, subURL)
}

// ResolveShortCode 短码解析（302 重定向到 /sub/:token）
func (h *SubscriptionHandler) ResolveShortCode(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		server.BadRequest(c, "invalid short code")
		return
	}
	tokenValue, err := h.subService.ResolveShortCode(c.Request.Context(), code)
	if err != nil {
		code, msg := service.MapServiceErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	c.Redirect(http.StatusFound, "/sub/"+tokenValue)
}
