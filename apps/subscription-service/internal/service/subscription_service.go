package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"time"

	"github.com/airport-panel/config"
	"github.com/airport-panel/subscription-service/internal/cache"
	"github.com/airport-panel/subscription-service/internal/client"
	"github.com/airport-panel/subscription-service/internal/lb"
	"github.com/airport-panel/subscription-service/internal/model"
	"github.com/airport-panel/subscription-service/internal/node"
	"github.com/airport-panel/subscription-service/internal/renderer"
	"github.com/airport-panel/subscription-service/internal/repo"
	"github.com/google/uuid"
)

var (
	ErrTokenNotFound    = errors.New("token not found")
	ErrTokenRevoked     = errors.New("token revoked")
	ErrTokenExpired     = errors.New("token expired")
	ErrNoActivePlan     = errors.New("no active subscription plan")
	ErrQuotaExceeded    = errors.New("traffic quota exceeded")
	ErrTemplateNotFound = errors.New("template not found")
	ErrUserBanned       = errors.New("user is banned or disabled")
	ErrIPMismatch       = errors.New("IP address mismatch")
	ErrUserNotFound     = errors.New("user not found")
)

type SubscriptionResult struct {
	Content     string
	ContentType string
	UserInfo    string
	NodeCount   int
	Degraded    bool
}

type SubscriptionService struct {
	tokenRepo      *repo.TokenRepo
	userRepo       *repo.UserRepo
	templateRepo   *repo.TemplateRepo
	accessLogRepo  *repo.AccessLogRepo
	shortCodeRepo  *repo.ShortCodeRepo
	nodeProvider   node.Provider
	cache          *cache.MemoryCache
	logger         *slog.Logger
	fallbackMode   bool
	nodeRenderer   *renderer.NodeRenderer
	lbService      *lb.LBService
	templateSvc    *TemplateService // 按名称索引的订阅模板服务（clash/clashmeta/singbox）
}

func NewSubscriptionService(
	tokenRepo *repo.TokenRepo,
	userRepo *repo.UserRepo,
	templateRepo *repo.TemplateRepo,
	accessLogRepo *repo.AccessLogRepo,
	shortCodeRepo *repo.ShortCodeRepo,
	nodeProvider node.Provider,
	cacheInstance *cache.MemoryCache,
	logger *slog.Logger,
	nodeRenderer *renderer.NodeRenderer,
	lbService *lb.LBService,
) *SubscriptionService {
	return &SubscriptionService{
		tokenRepo:      tokenRepo,
		userRepo:       userRepo,
		templateRepo:   templateRepo,
		accessLogRepo:  accessLogRepo,
		shortCodeRepo:  shortCodeRepo,
		nodeProvider:   nodeProvider,
		cache:          cacheInstance,
		logger:         logger,
		fallbackMode:   true,
		nodeRenderer:   nodeRenderer,
		lbService:      lbService,
	}
}

func (s *SubscriptionService) SetFallbackMode(enabled bool) {
	s.fallbackMode = enabled
}

// SetTemplateService 注入订阅模板服务（按名称索引，对齐 Xboard subscribe_template helper）。
// 渲染时根据客户端类型从模板服务获取对应的基础模板（clash/clashmeta/singbox），
// 将节点信息填入模板；若模板不存在则使用渲染器内置默认模板。
func (s *SubscriptionService) SetTemplateService(ts *TemplateService) {
	s.templateSvc = ts
}

func (s *SubscriptionService) InvalidateUserCache() {
	s.cache.Clear()
}

func generateTokenValue() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func MapServiceErrorToCode(err error) (config.ErrorCode, string) {
	switch {
	case errors.Is(err, ErrTokenNotFound), errors.Is(err, ErrUserNotFound), errors.Is(err, ErrTemplateNotFound):
		return config.CodeNotFound, err.Error()
	case errors.Is(err, ErrTokenRevoked), errors.Is(err, ErrUserBanned):
		return config.CodeForbidden, err.Error()
	case errors.Is(err, ErrTokenExpired), errors.Is(err, ErrQuotaExceeded), errors.Is(err, ErrNoActivePlan):
		return config.CodeForbidden, err.Error()
	default:
		return config.CodeInternalError, "internal error"
	}
}

func (s *SubscriptionService) GenerateToken(ctx context.Context, userID uuid.UUID, expiresAt *time.Time) (*model.SubscriptionToken, error) {
	value, err := generateTokenValue()
	if err != nil {
		return nil, err
	}
	t := &model.SubscriptionToken{
		ID:         uuid.New(),
		UserID:     userID,
		TokenValue: value,
		Status:     model.SubscriptionTokenStatusActive,
		ExpiresAt:  expiresAt,
	}
	if err := s.tokenRepo.Create(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

func (s *SubscriptionService) RevokeToken(ctx context.Context, id uuid.UUID) error {
	token, err := s.tokenRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if token == nil {
		return ErrTokenNotFound
	}
	if err := s.tokenRepo.RevokeToken(ctx, id); err != nil {
		return err
	}
	s.cache.Clear()
	return nil
}

func (s *SubscriptionService) ListTokens(ctx context.Context, page, pageSize int, status string, userID *uuid.UUID) ([]*model.SubscriptionToken, int, error) {
	return s.tokenRepo.ListAll(ctx, page, pageSize, status, userID)
}

func (s *SubscriptionService) ValidateToken(ctx context.Context, tokenValue string) (*model.SubscriptionToken, *model.UserSubscriptionInfo, error) {
	token, err := s.tokenRepo.GetByValue(ctx, tokenValue)
	if err != nil {
		return nil, nil, err
	}
	if token == nil {
		return nil, nil, ErrTokenNotFound
	}
	if token.Status == model.SubscriptionTokenStatusRevoked {
		return nil, nil, ErrTokenRevoked
	}
	if token.ExpiresAt != nil && token.ExpiresAt.Before(time.Now()) {
		return nil, nil, ErrTokenExpired
	}

	info := &model.UserSubscriptionInfo{
		UserID: token.UserID,
	}

	user, err := s.userRepo.GetByID(ctx, token.UserID)
	if err == nil && user != nil {
		if user.Status == model.UserStatusBanned || user.Status == model.UserStatusDisabled {
			return nil, nil, ErrUserBanned
		}
		// 将用户的会员分组ID传递下去，供 ListVisibleNodes 按 group_id 过滤节点
		info.GroupID = user.GroupID
	}

	sub, err := s.tokenRepo.GetActiveUserSubscription(ctx, token.UserID)
	if err == nil && sub != nil {
		info.PlanID = &sub.PlanID
		info.TrafficQuotaBytes = sub.TrafficQuotaBytes
		info.TrafficUsedBytes = sub.TrafficUsedBytes
		info.Upload = sub.UploadBytes
		info.Download = sub.DownloadBytes
		info.ExpiresAt = sub.ExpiresAt
		now := time.Now()
		if sub.ExpiresAt != nil && sub.ExpiresAt.Before(now) {
			info.IsExpired = true
		}
		if sub.TrafficQuotaBytes > 0 && sub.TrafficUsedBytes >= sub.TrafficQuotaBytes {
			info.IsOverQuota = true
		}
		if p, err := s.tokenRepo.GetPlanByID(ctx, sub.PlanID); err == nil && p != nil {
			info.PlanName = p.Name
			info.Total = p.TrafficBytes
		} else {
			info.Total = sub.TrafficQuotaBytes
		}
	}

	return token, info, nil
}

var versionRegex = regexp.MustCompile(`/([0-9]+\.[0-9]+(?:\.[0-9]+)?)`)

func extractClientVersion(userAgent string) string {
	matches := versionRegex.FindStringSubmatch(userAgent)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

func nodeInfoToCompatMap(n *model.NodeInfo) map[string]interface{} {
	m := make(map[string]interface{})
	m["protocol_type"] = n.ProtocolType
	m["transport_type"] = n.TransportType
	m["security_type"] = n.SecurityType
	m["status"] = n.Status
	if n.ConfigJSON != nil {
		for k, v := range n.ConfigJSON {
			m[k] = v
		}
	}
	return m
}

func (s *SubscriptionService) applyLBAndCompat(ctx context.Context, nodes []*model.NodeInfo, userID uuid.UUID, clientIP, clientCode, clientVersion string) []*model.NodeInfo {
	if s.lbService != nil {
		nodes = s.applyLB(ctx, nodes, userID, clientIP)
	}
	if s.nodeRenderer != nil && clientCode != "" {
		nodes = s.applyCompatFilter(ctx, nodes, clientCode, clientVersion)
	}
	return nodes
}

func (s *SubscriptionService) applyLB(ctx context.Context, nodes []*model.NodeInfo, userID uuid.UUID, clientIP string) []*model.NodeInfo {
	grouped := make(map[uuid.UUID][]*model.NodeInfo)
	var groupIDs []uuid.UUID
	zeroGroup := uuid.Nil
	for _, n := range nodes {
		gid := n.GroupID
		if gid == uuid.Nil {
			gid = zeroGroup
		}
		if _, ok := grouped[gid]; !ok {
			groupIDs = append(groupIDs, gid)
		}
		grouped[gid] = append(grouped[gid], n)
	}

	selectedByID := make(map[uuid.UUID]bool)
	var orderedResult []*model.NodeInfo

	for _, gid := range groupIDs {
		groupNodes := grouped[gid]
		if gid == zeroGroup {
			for _, n := range groupNodes {
				if !selectedByID[n.ID] {
					selectedByID[n.ID] = true
					orderedResult = append(orderedResult, n)
				}
			}
			continue
		}

		lbResult, err := s.lbService.SelectForSubscription(ctx, gid, userID.String(), clientIP)
		if err != nil || lbResult == nil {
			for _, n := range groupNodes {
				if !selectedByID[n.ID] {
					selectedByID[n.ID] = true
					orderedResult = append(orderedResult, n)
				}
			}
			continue
		}

		selectedIDs := make(map[uuid.UUID]bool)
		for _, cand := range lbResult.SelectedNodes {
			selectedIDs[cand.NodeID] = true
		}

		nodeMap := make(map[uuid.UUID]*model.NodeInfo)
		for _, n := range groupNodes {
			nodeMap[n.ID] = n
		}

		for _, cand := range lbResult.SelectedNodes {
			if n, ok := nodeMap[cand.NodeID]; ok {
				if !selectedByID[n.ID] {
					selectedByID[n.ID] = true
					orderedResult = append(orderedResult, n)
				}
			}
		}
	}

	return orderedResult
}

func (s *SubscriptionService) applyCompatFilter(ctx context.Context, nodes []*model.NodeInfo, clientCode, clientVersion string) []*model.NodeInfo {
	var filtered []*model.NodeInfo
	for _, n := range nodes {
		nodeMap := nodeInfoToCompatMap(n)
		rendered, _ := s.nodeRenderer.Render(ctx, nodeMap, clientCode, clientVersion)
		if rendered == nil {
			continue
		}
		if hidden, ok := rendered["_hidden"].(bool); ok && hidden {
			continue
		}
		filtered = append(filtered, n)
	}
	return filtered
}

func (s *SubscriptionService) GetSubscription(ctx context.Context, tokenValue string, clientType model.ClientType, userAgent string, clientIP string) (result *SubscriptionResult, err error) {
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("panic in GetSubscription", "panic", r)
			result = s.buildDegradedResponse(clientType)
			err = nil
		}
	}()

	ct := client.NormalizeClientType(string(clientType))
	clientCode := clientTypeToCompatCode(ct)
	clientVersion := extractClientVersion(userAgent)

	if cached, ok := s.cache.Get(tokenValue, string(ct)); ok {
		s.writeAccessLogAsync(ctx, tokenValue, ct, clientIP, userAgent, httpStatusOK, "", 0, true)
		return &SubscriptionResult{
			Content:     cached.Content,
			ContentType: cached.ContentType,
			UserInfo:    cached.UserInfo,
		}, nil
	}

	token, info, err := s.ValidateToken(ctx, tokenValue)
	if err != nil {
		if s.fallbackMode && !isAuthzError(err) {
			if stale, ok := s.cache.GetStale(tokenValue, string(ct)); ok {
				s.logger.Warn("serving stale subscription due to validate error", "error", err)
				s.writeAccessLogAsync(ctx, tokenValue, ct, clientIP, userAgent, httpStatusOK, "", 0, true)
				return &SubscriptionResult{
					Content:     stale.Content,
					ContentType: stale.ContentType,
					UserInfo:    stale.UserInfo,
					Degraded:    true,
				}, nil
			}
		}
		return nil, err
	}

	if info.IsExpired || info.IsOverQuota {
		return nil, ErrQuotaExceeded
	}

	if info.PlanID == nil {
		return nil, ErrNoActivePlan
	}

	var nodes []*model.NodeInfo
	nodes, err = s.nodeProvider.ListVisibleNodes(ctx, info.PlanID, info.GroupID)
	if err != nil {
		s.logger.Error("failed to list nodes", "error", err, "user", token.UserID)
		if s.fallbackMode {
			if stale, ok := s.cache.GetStale(tokenValue, string(ct)); ok {
				s.logger.Warn("serving stale subscription due to node list error", "error", err)
				s.writeAccessLogAsync(ctx, tokenValue, ct, clientIP, userAgent, httpStatusOK, "", 0, true)
				return &SubscriptionResult{
					Content:     stale.Content,
					ContentType: stale.ContentType,
					UserInfo:    stale.UserInfo,
					Degraded:    true,
				}, nil
			}
		}
		nodes = []*model.NodeInfo{}
	}

	nodes = s.applyLBAndCompat(ctx, nodes, token.UserID, clientIP, clientCode, clientVersion)

	// 为每个节点注入用户凭证（对齐 XBoard 模型）
	// 每用户一个 UUID 全节点共享：VLESS/VMess/TUIC/Trojan/SS/Hysteria2/AnyTLS 直接用 user.uuid
	// SS2022 通过 serverKey:userKey 派生（serverKey 来自 node.created_at，userKey 来自 user.uuid）
	s.injectUserCredentials(ctx, nodes, token.UserID)

	r := renderer.NewSubscriptionRenderer(ct)

	// 从数据库加载对应客户端类型的订阅模板（clash/clashmeta/singbox），
	// 作为渲染基础模板；若模板不存在则使用渲染器内置默认模板。
	if s.templateSvc != nil {
		rName := client.ClientToRenderer(ct)
		switch rName {
		case "clash", "clashmeta", "singbox":
			if tmpl, terr := s.templateSvc.GetTemplate(ctx, rName); terr == nil && tmpl != "" {
				r.WithBaseTemplate(tmpl)
			}
		}
	}

	expireTs := int64(0)
	if info.ExpiresAt != nil {
		expireTs = info.ExpiresAt.Unix()
	}
	rc := &model.RenderContext{
		Upload:   info.Upload,
		Download: info.Download,
		Total:    info.Total,
		Expire:   expireTs,
		Token:    tokenValue,
	}

	content, renderErr := r.Render(nodes, rc)
	if renderErr != nil {
		s.logger.Error("failed to render subscription", "error", renderErr)
		return s.buildEmptyResponse(ct), nil
	}

	userInfo := fmt.Sprintf("upload=%d; download=%d; total=%d; expire=%d", info.Upload, info.Download, info.Total, expireTs)
	if info.Total == 0 {
		userInfo = fmt.Sprintf("upload=%d; download=%d; total=0; expire=%d", info.Upload, info.Download, expireTs)
	}

	s.cache.Set(tokenValue, string(ct), content, r.ContentType(), userInfo)
	_ = s.tokenRepo.UpdateAccess(ctx, token.ID, clientIP, nil)
	s.writeAccessLogAsync(ctx, tokenValue, ct, clientIP, userAgent, httpStatusOK, "", len(nodes), false)

	return &SubscriptionResult{
		Content:     content,
		ContentType: r.ContentType(),
		UserInfo:    userInfo,
		NodeCount:   len(nodes),
	}, nil
}

// injectUserCredentials 为每个节点注入用户凭证（对齐 XBoard 模型）
// 每用户一个 UUID 全节点共享：
//   - VLESS/VMess/TUIC: 直接用 user.uuid 作为 uuid 字段
//   - Trojan/SS(普通)/Hysteria2/AnyTLS: 直接用 user.uuid 作为 password 字段
//   - SS2022(2022-blake3-*): 派生为 {serverKey}:{userKey}
//     serverKey = base64(substr(md5(node.created_at), 0, N))
//     userKey   = base64(substr(user.uuid, 0, N))
func (s *SubscriptionService) injectUserCredentials(ctx context.Context, nodes []*model.NodeInfo, userID uuid.UUID) {
	if len(nodes) == 0 {
		return
	}

	// 查询用户 UUID（全节点共享）
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil || user == nil || user.UUID == "" {
		s.logger.Error("failed to get user uuid for credential injection", "error", err, "user", userID)
		// 凭证获取失败时降级为使用节点原始凭证（不影响订阅可用性）
		return
	}
	userUUID := user.UUID

	// 为每个节点注入凭证
	for _, n := range nodes {
		if n == nil {
			continue
		}
		if n.ConfigJSON == nil {
			n.ConfigJSON = make(map[string]interface{})
		}

		proto := n.ProtocolType
		switch proto {
		case "vless", "vmess":
			// UUID 类协议：直接用 user.uuid
			n.ConfigJSON["uuid"] = userUUID

		case "tuic":
			// TUIC 同时需要 uuid 和 password，对齐 XBoard：两者都用 user.uuid
			n.ConfigJSON["uuid"] = userUUID
			n.ConfigJSON["password"] = userUUID

		case "trojan", "hysteria2", "hy2", "anytls":
			// 密码类协议：直接用 user.uuid
			n.ConfigJSON["password"] = userUUID

		case "shadowsocks", "ss":
			// SS：判断是否 SS2022 加密
			method, _ := n.ConfigJSON["method"].(string)
			if method == "" {
				if m, ok := n.ConfigJSON["cipher"].(string); ok {
					method = m
				}
			}
			if isSS2022Cipher(method) {
				// SS2022: 派生 serverKey:userKey
				n.ConfigJSON["password"] = generateSS2022Password(
					userUUID,
					n.CreatedAt.Format("2006-01-02 15:04:05"),
					method,
				)
			} else {
				// 普通 SS：直接用 user.uuid
				n.ConfigJSON["password"] = userUUID
			}
		}
	}
}

func clientTypeToCompatCode(ct model.ClientType) string {
	switch ct {
	case model.ClientTypeClashMeta, model.ClientTypeMihomo, model.ClientTypeMihomoParty,
		model.ClientTypeClashVerge, model.ClientTypeVergeRev, model.ClientTypeNyanpasu,
		model.ClientTypeClashForAndroid, model.ClientTypeClashXPro, model.ClientTypeCFW,
		model.ClientTypeFlClash, model.ClientTypeKaring, model.ClientTypeStash,
		model.ClientTypeHiddify, model.ClientTypeHiddifyNext:
		return "clash-meta"
	case model.ClientTypeClash, model.ClientTypeClashX:
		return "clash"
	case model.ClientTypeSingBox, model.ClientTypeSFA, model.ClientTypeSFI, model.ClientTypeSFM:
		return "sing-box"
	case model.ClientTypeShadowrocket:
		return "shadowrocket"
	case model.ClientTypeV2RayN, model.ClientTypeV2RayNG, model.ClientTypeNekoBox, model.ClientTypeNekoRay, model.ClientTypeV2Box:
		return "v2rayn"
	case model.ClientTypeQuantumult, model.ClientTypeQuantumultX:
		return "quantumult"
	case model.ClientTypeSurge:
		return "surge"
	case model.ClientTypeLoon, model.ClientTypeLoonLite:
		return "loon"
	case model.ClientTypeSurfboard, model.ClientTypeStreisand, model.ClientTypeMomo,
		model.ClientTypeSubStore, model.ClientTypeXBrowser:
		return ""
	default:
		return ""
	}
}

const httpStatusOK = 200

func (s *SubscriptionService) buildEmptyResponse(ct model.ClientType) *SubscriptionResult {
	content := ""
	rName := client.ClientToRenderer(ct)
	switch rName {
	case "clash", "clashmeta":
		content = "proxies: []\nproxy-groups: []\nrules: []\n"
	case "singbox":
		content = `{"log":{"level":"info"},"inbounds":[{"type":"mixed","tag":"mixed-in","listen_port":7890}],"outbounds":[{"type":"direct","tag":"direct"}],"route":{"rules":[]}}`
	default:
		content = ""
	}
	return &SubscriptionResult{
		Content:     content,
		ContentType: contentTypeForClient(ct),
		UserInfo:    "upload=0; download=0; total=0; expire=0",
		NodeCount:   0,
		Degraded:    true,
	}
}

func (s *SubscriptionService) buildDegradedResponse(ct model.ClientType) *SubscriptionResult {
	return s.buildEmptyResponse(ct)
}

func contentTypeForClient(ct model.ClientType) string {
	rName := client.ClientToRenderer(ct)
	switch rName {
	case "clash", "clashmeta":
		return "text/yaml; charset=utf-8"
	case "singbox":
		return "application/json; charset=utf-8"
	default:
		return "text/plain; charset=utf-8"
	}
}

func (s *SubscriptionService) writeAccessLogAsync(ctx context.Context, tokenValue string, ct model.ClientType, ip, ua string, status int, templateCode string, nodeCount int, cacheHit bool) {
	go func() {
		token, err := s.tokenRepo.GetByValue(ctx, tokenValue)
		if err != nil || token == nil {
			return
		}
		tokenID := token.ID
		userID := token.UserID
		clientType := string(ct)
		requestIP := ip
		userAgent := ua
		tplCode := templateCode
		l := &model.SubscriptionAccessLog{
			ID:                 uuid.New(),
			TokenID:            &tokenID,
			UserID:             &userID,
			ClientType:         &clientType,
			RequestIP:          &requestIP,
			UserAgent:          &userAgent,
			ResponseStatus:     status,
			TemplateCode:       &tplCode,
			GeneratedNodeCount: nodeCount,
			CacheHit:           cacheHit,
		}
		_ = s.accessLogRepo.Create(ctx, l)
	}()
}

func isAuthzError(err error) bool {
	return errors.Is(err, ErrTokenNotFound) ||
		errors.Is(err, ErrTokenRevoked) ||
		errors.Is(err, ErrTokenExpired) ||
		errors.Is(err, ErrUserBanned) ||
		errors.Is(err, ErrUserNotFound) ||
		errors.Is(err, ErrQuotaExceeded) ||
		errors.Is(err, ErrNoActivePlan) ||
		errors.Is(err, ErrIPMismatch)
}

func (s *SubscriptionService) ListTemplates(ctx context.Context, page, pageSize int, clientType model.ClientType) ([]*model.SubscriptionTemplate, int, error) {
	return s.templateRepo.ListAll(ctx, page, pageSize, clientType)
}

func (s *SubscriptionService) CreateTemplate(ctx context.Context, req *model.CreateTemplateRequest, adminID *uuid.UUID) (*model.SubscriptionTemplate, error) {
	t := &model.SubscriptionTemplate{
		ID:             uuid.New(),
		Code:           req.Code,
		Name:           req.Name,
		TargetClient:   req.TargetClient,
		TemplateType:   "subscription",
		Content:        req.Content,
		Status:         model.TemplateStatusActive,
		SchemaVersion:  "v1",
		CreatedByAdmin: adminID,
	}
	if err := s.templateRepo.Create(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

func (s *SubscriptionService) UpdateTemplate(ctx context.Context, req *model.UpdateTemplateRequest) (*model.SubscriptionTemplate, error) {
	tmpl, err := s.templateRepo.GetByID(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	if tmpl == nil {
		return nil, ErrTemplateNotFound
	}

	if req.Code != "" {
		tmpl.Code = req.Code
	}
	if req.Name != "" {
		tmpl.Name = req.Name
	}
	if req.TargetClient != "" {
		tmpl.TargetClient = req.TargetClient
	}
	if req.Content != "" {
		tmpl.Content = req.Content
	}
	if req.Status != "" {
		tmpl.Status = model.TemplateStatus(req.Status)
	}

	if err := s.templateRepo.Update(ctx, tmpl); err != nil {
		return nil, err
	}
	return tmpl, nil
}

func (s *SubscriptionService) SetDefaultTemplate(ctx context.Context, id uuid.UUID) error {
	return s.templateRepo.SetDefault(ctx, id)
}

func (s *SubscriptionService) IsDefaultTemplate(t *model.SubscriptionTemplate) bool {
	return s.templateRepo.IsDefault(t)
}

func (s *SubscriptionService) ResolveShortCode(ctx context.Context, code string) (string, error) {
	sc, err := s.shortCodeRepo.GetByCode(ctx, code)
	if err != nil {
		return "", err
	}
	if sc == nil {
		return "", ErrTokenNotFound
	}
	token, err := s.tokenRepo.GetByID(ctx, sc.TokenID)
	if err != nil {
		return "", err
	}
	if token == nil {
		return "", ErrTokenNotFound
	}
	return token.TokenValue, nil
}

func (s *SubscriptionService) GenerateShortCode(ctx context.Context, tokenValue string, expiresIn int, description string) (*model.SubscriptionShortCode, error) {
	token, err := s.tokenRepo.GetByValue(ctx, tokenValue)
	if err != nil {
		return nil, err
	}
	if token == nil {
		return nil, ErrTokenNotFound
	}

	existing, err := s.shortCodeRepo.GetByTokenID(ctx, token.ID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}

	var expiresAt *time.Time
	if expiresIn > 0 {
		t := time.Now().Add(time.Duration(expiresIn) * time.Second)
		expiresAt = &t
	} else {
		expiresAt = token.ExpiresAt
	}

	sc := &model.SubscriptionShortCode{
		ID:          uuid.New(),
		TokenID:     token.ID,
		UserID:      token.UserID,
		Description: description,
		ExpiresAt:   expiresAt,
	}
	if err := s.shortCodeRepo.Create(ctx, sc); err != nil {
		return nil, err
	}
	return sc, nil
}

func (s *SubscriptionService) RevokeShortCode(ctx context.Context, tokenID uuid.UUID) error {
	return s.shortCodeRepo.RevokeByTokenID(ctx, tokenID)
}

func (s *SubscriptionService) RevokeShortCodeByCode(ctx context.Context, code string) error {
	return s.shortCodeRepo.RevokeByCode(ctx, code)
}

func (s *SubscriptionService) GetUserAccessStats(ctx context.Context, userID uuid.UUID, days int) (*model.AccessLogStats, error) {
	if days <= 0 {
		days = 7
	}
	return s.accessLogRepo.GetStatsByUserID(ctx, userID, days)
}

func (s *SubscriptionService) GetAccessOverview(ctx context.Context, startTime, endTime time.Time) (*model.AccessLogOverview, error) {
	now := time.Now()
	if startTime.IsZero() {
		startTime = now.AddDate(0, 0, -7)
	}
	if endTime.IsZero() {
		endTime = now
	}
	return s.accessLogRepo.GetOverview(ctx, startTime, endTime)
}

func (s *SubscriptionService) ListAccessLogs(ctx context.Context, userID *uuid.UUID, page, pageSize int) ([]*model.SubscriptionAccessLog, int, error) {
	return s.accessLogRepo.List(ctx, userID, page, pageSize)
}
