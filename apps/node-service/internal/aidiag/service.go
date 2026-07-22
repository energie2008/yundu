package aidiag

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/airport-panel/node-service/internal/metrics"
	"github.com/google/uuid"
)

// ============================================================================
// Service - AI 诊断编排
// ============================================================================

type Service struct {
	repo      *Repo
	llm       LLMClient
	collector LogCollector
	dispatcher ActionDispatcher
	logger    *slog.Logger
}

func NewService(repo *Repo, llm LLMClient, collector LogCollector, logger *slog.Logger) *Service {
	return &Service{
		repo:      repo,
		llm:       llm,
		collector: collector,
		logger:    logger.With("component", "aidiag"),
	}
}

// SetLLMClient 运行时替换 LLM 客户端（可选）
func (s *Service) SetLLMClient(llm LLMClient) { s.llm = llm }

// SetLogCollector 运行时替换日志采集器（可选）
func (s *Service) SetLogCollector(c LogCollector) { s.collector = c }

// SetActionDispatcher 注入自动修复派发器
// 必须在 grpcserver 启动后注入（GRPCDispatcher 依赖 *AgentServer.PushToMachine）。
// 未注入时 ApplyAutofix 仅记录 "dispatcher not configured" 不实际下发。
func (s *Service) SetActionDispatcher(d ActionDispatcher) { s.dispatcher = d }

// CreateSession 创建诊断会话并立即开始异步分析
func (s *Service) CreateSession(ctx context.Context, req *CreateSessionRequest, adminID *uuid.UUID) (*DiagnosisSession, error) {
	now := time.Now()
	windowMins := req.TimeWindowMins
	if windowMins <= 0 {
		windowMins = 30
	}
	windowStart := now.Add(-time.Duration(windowMins) * time.Minute)
	windowEnd := now

	session := &DiagnosisSession{
		ID:              uuid.New(),
		ServerID:        req.ServerID,
		NodeID:          req.NodeID,
		Status:          "pending",
		TriggerSource:   "manual",
		TimeWindowStart: &windowStart,
		TimeWindowEnd:   &windowEnd,
		RawMetrics:       map[string]interface{}{},
		LLMProvider:      "none",
		Suggestions:      []Suggestion{},
		DocLinks:         []DocLink{},
		CreatedByAdminID: adminID,
		CreatedAt:        now,
	}
	if s.llm != nil {
		session.LLMProvider = s.llm.Provider()
	}

	if err := s.repo.Create(ctx, session); err != nil {
		return nil, err
	}

	// 异步执行诊断（不阻塞 HTTP 请求）
	go s.runDiagnosis(context.Background(), session.ID)

	return session, nil
}

// runDiagnosis 实际诊断流程：采集日志/指标 → 调 LLM → 解析 → 持久化
func (s *Service) runDiagnosis(ctx context.Context, sessionID uuid.UUID) {
	start := time.Now()

	// 1. 加载 session
	session, err := s.repo.GetByID(ctx, sessionID)
	if err != nil {
		s.logger.Error("diagnosis: load session failed", "session_id", sessionID, "error", err)
		return
	}

	// 2. 更新状态为 collecting
	session.Status = "collecting"
	_ = s.repo.Update(ctx, session)

	// 3. 采集日志和指标
	if s.collector != nil && session.ServerID != nil {
		logs, err := s.collector.CollectLogs(ctx, *session.ServerID, session.NodeID, *session.TimeWindowStart, *session.TimeWindowEnd)
		if err != nil {
			s.logger.Warn("diagnosis: collect logs failed", "error", err)
		}
		session.RawLogs = logs

		metrics, err := s.collector.CollectMetrics(ctx, *session.ServerID, session.NodeID, *session.TimeWindowStart, *session.TimeWindowEnd)
		if err == nil {
			session.RawMetrics = metrics
		}
	} else {
		session.RawLogs = "[no log collector configured] channel health and doctor reports available via admin API"
	}

	// 4. 调 LLM 分析
	if s.llm == nil {
		session.Status = "failed"
		session.RootCauseCategory = "unknown"
		session.RootCauseDescription = "LLM client not configured"
		now := time.Now()
		session.CompletedAt = &now
		d := int(time.Since(start).Milliseconds())
		session.DurationMs = &d
		_ = s.repo.Update(ctx, session)
		return
	}

	session.Status = "analyzing"
	_ = s.repo.Update(ctx, session)

	// 5. 构建 Prompt
	systemPrompt := buildSystemPrompt()
	userPrompt := buildUserPrompt(session)

	resp, err := s.llm.Chat(ctx, &LLMRequest{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		MaxTokens:    2048,
		Temperature:  0.2,
	})
	if err != nil {
		s.logger.Error("diagnosis: LLM call failed", "error", err)
		session.Status = "failed"
		session.RootCauseDescription = fmt.Sprintf("LLM 调用失败: %v", err)
		now := time.Now()
		session.CompletedAt = &now
		d := int(time.Since(start).Milliseconds())
		session.DurationMs = &d
		_ = s.repo.Update(ctx, session)
		return
	}

	// 6. 解析 LLM 输出
	result := parseLLMResponse(resp.Content)
	session.Status = "done"
	session.RootCauseCategory = result.Category
	session.RootCauseDescription = result.Description
	metrics.DiagnosisSessionsTotal.WithLabelValues(result.Category).Inc()
	session.Confidence = &result.Confidence
	session.Suggestions = result.Suggestions
	session.DocLinks = result.DocLinks
	if s.llm != nil {
		session.LLMModel = resp.FinishReason // 复用字段存 finish_reason 信息（实际应从配置读取模型名）
	}

	// 7. 匹配知识库
	if result.Category != "normal" && result.Category != "unknown" {
		if knowledge, err := s.repo.MatchKnowledge(ctx, result.Category, result.Description); err == nil && knowledge != nil {
			session.KnowledgeEntryID = &knowledge.ID
			_ = s.repo.IncrementKnowledgeHit(ctx, knowledge.ID)
			// 如果知识库有自动修复动作且会话没有建议，补充知识库的方案
			if len(session.Suggestions) == 0 && knowledge.Solution != "" {
				session.Suggestions = []Suggestion{{
					Title:       knowledge.Title,
					Description: knowledge.Solution,
					Action:      derefString(knowledge.AutoFixAction),
					AutoFixable: knowledge.AutoFixAction != nil,
				}}
			}
		}
	}

	now := time.Now()
	session.CompletedAt = &now
	d := int(time.Since(start).Milliseconds())
	session.DurationMs = &d

	if err := s.repo.Update(ctx, session); err != nil {
		s.logger.Error("diagnosis: persist result failed", "error", err)
	}
}

// GetSession 获取会话详情
func (s *Service) GetSession(ctx context.Context, id uuid.UUID) (*DiagnosisSession, error) {
	return s.repo.GetByID(ctx, id)
}

// ListSessions 列表
func (s *Service) ListSessions(ctx context.Context, q *ListSessionsQuery) ([]*DiagnosisSession, int, error) {
	return s.repo.List(ctx, q)
}

// ListKnowledge 知识库列表
func (s *Service) ListKnowledge(ctx context.Context, category string, onlyVerified bool, page, pageSize int) ([]*KnowledgeEntry, int, error) {
	return s.repo.ListKnowledge(ctx, category, onlyVerified, page, pageSize)
}

// ApplyAutofix 应用自动修复
//
// 根据 suggestion.Action 派发到具体修复器：
//   - restart_kernel → dispatcher.RestartKernel（gRPC MaintenanceCommand.ACTION_RESTART）
//   - reload_config  → dispatcher.ReloadConfig（ACTION_RESTART，reason 标注 config reload）
//   - renew_cert     → dispatcher.RenewCert（触发面板侧证书续期）
//
// 派发结果（含 dispatch_status/dispatch_error/dispatched_at）写入 session.AutofixResult。
// 若未注入 dispatcher，仅记录 "dispatcher not configured" 不实际下发。
func (s *Service) ApplyAutofix(ctx context.Context, sessionID uuid.UUID, suggestionIndex int) (*DiagnosisSession, error) {
	session, err := s.repo.GetByID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session.Status != "done" {
		return nil, ErrSessionNotDone
	}
	if suggestionIndex < 0 || suggestionIndex >= len(session.Suggestions) {
		return nil, ErrInvalidSuggestion
	}
	sug := session.Suggestions[suggestionIndex]
	if !sug.AutoFixable {
		return nil, fmt.Errorf("suggestion %d is not auto-fixable", suggestionIndex)
	}

	result := map[string]interface{}{
		"applied_at": time.Now().Format(time.RFC3339),
		"action":     sug.Action,
		"suggestion": sug.Title,
	}

	if s.dispatcher == nil {
		// 未注入派发器：仅记录，不下发（保留旧行为以便降级）
		result["status"] = "skipped"
		result["dispatch_status"] = "dispatcher_not_configured"
		result["note"] = "auto-fix skipped: ActionDispatcher not injected; record only"
		s.logger.Warn("autofix skipped: dispatcher not configured",
			"session_id", sessionID, "action", sug.Action)
		metrics.DiagnosisAutofixTotal.WithLabelValues(sug.Action, "skipped").Inc()
	} else if err := s.dispatchAction(ctx, session, sug); err != nil {
		// 派发失败：记录错误但不阻塞会话更新
		result["status"] = "failed"
		result["dispatch_status"] = "failed"
		result["dispatch_error"] = err.Error()
		s.logger.Error("autofix dispatch failed",
			"session_id", sessionID, "action", sug.Action, "error", err)
		metrics.DiagnosisAutofixTotal.WithLabelValues(sug.Action, "failed").Inc()
	} else {
		result["status"] = "dispatched"
		result["dispatch_status"] = "dispatched"
		s.logger.Info("autofix dispatched",
			"session_id", sessionID, "action", sug.Action, "suggestion", sug.Title)
		metrics.DiagnosisAutofixTotal.WithLabelValues(sug.Action, "dispatched").Inc()
	}

	session.AutofixApplied = true
	session.AutofixResult = result
	if err := s.repo.Update(ctx, session); err != nil {
		return nil, err
	}
	return session, nil
}

// dispatchAction 根据 action 编码派发到 dispatcher 对应方法
func (s *Service) dispatchAction(ctx context.Context, session *DiagnosisSession, sug Suggestion) error {
	if session.ServerID == nil {
		return fmt.Errorf("session has no server_id, cannot resolve machineID")
	}
	serverID := *session.ServerID
	switch sug.Action {
	case "restart_kernel":
		reason := sug.Title
		if reason == "" {
			reason = "AI diagnosis: kernel restart required"
		}
		return s.dispatcher.RestartKernel(ctx, serverID, reason)
	case "reload_config":
		return s.dispatcher.ReloadConfig(ctx, serverID)
	case "renew_cert":
		return s.dispatcher.RenewCert(ctx, serverID, session.NodeID)
	case "", "none":
		return fmt.Errorf("action %q is not dispatchable (manual fix only)", sug.Action)
	default:
		return fmt.Errorf("unknown action %q (supported: restart_kernel/reload_config/renew_cert)", sug.Action)
	}
}

// ============================================================================
// Prompt 工程
// ============================================================================

func buildSystemPrompt() string {
	return `你是一名资深的代理服务器运维专家，专注于 VLESS/VMess/Trojan/Hysteria2/TUIC/Shadowsocks 协议、Xray/Sing-box 双内核、Cloudflare CDN/WARP、Nginx 中转等机场系统的故障诊断。

你的任务是：根据节点的日志和指标，分析根因，并给出可执行的修复建议。

输出必须是严格 JSON 格式，不要包含任何其他文本。JSON 结构如下：
{
  "category": "config_error|network_issue|cert_expired|kernel_compat|rate_limit|resource_exhausted|dns_issue|firewall_block|normal",
  "description": "对根因的简明描述，1-2 句话",
  "confidence": 0.0 到 1.0 之间的浮点数,
  "suggestions": [
    {
      "title": "建议标题",
      "description": "详细步骤",
      "action": "可选的自动修复动作编码，如 restart_kernel/reload_config/renew_cert/none",
      "auto_fixable": true 或 false
    }
  ],
  "doc_links": [
    {"title": "文档标题", "url": "相关文档URL（可留空）"}
  ]
}

判断依据：
- TLS 握手失败、证书过期 → cert_expired
- 连接超时、端口不通 → network_issue
- 配置校验失败、协议参数错误 → config_error
- 内核版本不兼容、Reality key 无效 → kernel_compat
- 连接池耗尽、文件描述符不足 → resource_exhausted
- DNS 解析慢、解析失败 → dns_issue
- 防火墙阻断、端口被占用 → firewall_block
- 一切正常 → normal`
}

func buildUserPrompt(s *DiagnosisSession) string {
	var b strings.Builder
	b.WriteString("请诊断以下节点的故障：\n\n")
	if s.NodeID != nil {
		b.WriteString(fmt.Sprintf("节点 ID: %s\n", s.NodeID))
	}
	if s.ServerID != nil {
		b.WriteString(fmt.Sprintf("服务器 ID: %s\n", s.ServerID))
	}
	if s.TimeWindowStart != nil {
		b.WriteString(fmt.Sprintf("诊断时间窗口: %s 至 %s\n", s.TimeWindowStart.Format(time.RFC3339), s.TimeWindowEnd.Format(time.RFC3339)))
	}
	b.WriteString("\n--- 节点日志（最近）---\n")
	if s.RawLogs != "" {
		// 截断过长的日志，避免超出 LLM 上下文
		logs := s.RawLogs
		if len(logs) > 8000 {
			logs = logs[:8000] + "\n... [truncated]"
		}
		b.WriteString(logs)
	} else {
		b.WriteString("(无日志)")
	}
	b.WriteString("\n\n--- 节点指标 ---\n")
	if len(s.RawMetrics) > 0 {
		metricsJSON, _ := json.MarshalIndent(s.RawMetrics, "", "  ")
		b.WriteString(string(metricsJSON))
	} else {
		b.WriteString("(无指标)")
	}
	b.WriteString("\n\n请分析根因并给出修复建议，输出严格 JSON。")
	return b.String()
}

// parseLLMResponse 解析 LLM 返回的 JSON
type parsedResult struct {
	Category     string       `json:"category"`
	Description  string       `json:"description"`
	Confidence   float64      `json:"confidence"`
	Suggestions  []Suggestion `json:"suggestions"`
	DocLinks     []DocLink    `json:"doc_links"`
}

func parseLLMResponse(content string) parsedResult {
	result := parsedResult{
		Category:    "unknown",
		Suggestions: []Suggestion{},
		DocLinks:    []DocLink{},
	}

	// 尝试提取 JSON（LLM 可能输出额外文本）
	content = strings.TrimSpace(content)
	// 找到第一个 { 和最后一个 }
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start < 0 || end < 0 || end <= start {
		result.Description = content
		return result
	}
	jsonStr := content[start : end+1]

	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		result.Description = content
		result.Category = "unknown"
		return result
	}
	if result.Suggestions == nil {
		result.Suggestions = []Suggestion{}
	}
	if result.DocLinks == nil {
		result.DocLinks = []DocLink{}
	}
	if result.Category == "" {
		result.Category = "unknown"
	}
	return result
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
