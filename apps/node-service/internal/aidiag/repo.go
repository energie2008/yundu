package aidiag

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ============================================================================
// Repo
// ============================================================================

type Repo struct {
	pool *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

// Create 创建诊断会话
func (r *Repo) Create(ctx context.Context, s *DiagnosisSession) error {
	suggestionsJSON, _ := json.Marshal(s.Suggestions)
	if len(suggestionsJSON) == 0 {
		suggestionsJSON = []byte("[]")
	}
	docLinksJSON, _ := json.Marshal(s.DocLinks)
	if len(docLinksJSON) == 0 {
		docLinksJSON = []byte("[]")
	}
	metricsJSON, _ := json.Marshal(s.RawMetrics)
	if len(metricsJSON) == 0 {
		metricsJSON = []byte("{}")
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO diagnosis_sessions
			(id, server_id, node_id, status, trigger_source, time_window_start, time_window_end,
			 raw_logs, raw_metrics, llm_provider, llm_model, root_cause_category, root_cause_description,
			 confidence, suggestions, doc_links, knowledge_entry_id, autofix_applied, autofix_result,
			 duration_ms, created_by_admin_id, created_at, completed_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23)
	`,
		s.ID, s.ServerID, s.NodeID, s.Status, s.TriggerSource, s.TimeWindowStart, s.TimeWindowEnd,
		s.RawLogs, metricsJSON, s.LLMProvider, s.LLMModel, s.RootCauseCategory, s.RootCauseDescription,
		s.Confidence, suggestionsJSON, docLinksJSON, s.KnowledgeEntryID, s.AutofixApplied, nil,
		s.DurationMs, s.CreatedByAdminID, s.CreatedAt, s.CompletedAt,
	)
	return err
}

// Update 更新诊断会话（分析完成后）
func (r *Repo) Update(ctx context.Context, s *DiagnosisSession) error {
	suggestionsJSON, _ := json.Marshal(s.Suggestions)
	if len(suggestionsJSON) == 0 {
		suggestionsJSON = []byte("[]")
	}
	docLinksJSON, _ := json.Marshal(s.DocLinks)
	if len(docLinksJSON) == 0 {
		docLinksJSON = []byte("[]")
	}
	metricsJSON, _ := json.Marshal(s.RawMetrics)
	if len(metricsJSON) == 0 {
		metricsJSON = []byte("{}")
	}
	var autofixResult interface{}
	if s.AutofixResult != nil {
		autofixResultJSON, _ := json.Marshal(s.AutofixResult)
		autofixResult = autofixResultJSON
	}
	_, err := r.pool.Exec(ctx, `
		UPDATE diagnosis_sessions SET
			status = $1, raw_logs = $2, raw_metrics = $3,
			root_cause_category = $4, root_cause_description = $5,
			confidence = $6, suggestions = $7, doc_links = $8,
			knowledge_entry_id = $9, autofix_applied = $10, autofix_result = $11,
			duration_ms = $12, completed_at = $13, llm_model = COALESCE($14, llm_model)
		WHERE id = $15
	`,
		s.Status, s.RawLogs, metricsJSON,
		s.RootCauseCategory, s.RootCauseDescription,
		s.Confidence, suggestionsJSON, docLinksJSON,
		s.KnowledgeEntryID, s.AutofixApplied, autofixResult,
		s.DurationMs, s.CompletedAt, s.LLMModel, s.ID,
	)
	return err
}

// GetByID 按 ID 获取
func (r *Repo) GetByID(ctx context.Context, id uuid.UUID) (*DiagnosisSession, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, server_id, node_id, status, trigger_source, time_window_start, time_window_end,
			   raw_logs, raw_metrics, llm_provider, llm_model, root_cause_category, root_cause_description,
			   confidence, suggestions, doc_links, knowledge_entry_id, autofix_applied, autofix_result,
			   duration_ms, created_by_admin_id, created_at, completed_at
		FROM diagnosis_sessions WHERE id = $1
	`, id)
	s := &DiagnosisSession{}
	var serverID, nodeID, knowledgeID, adminID *string
	var timeStart, timeEnd, completedAt *time.Time
	var confidence *float64
	var durationMs *int
	var rawMetrics, suggestions, docLinks, autofixResult []byte
	var rawLogs *string
	var llmModel *string
	if err := row.Scan(
		&s.ID, &serverID, &nodeID, &s.Status, &s.TriggerSource, &timeStart, &timeEnd,
		&rawLogs, &rawMetrics, &s.LLMProvider, &llmModel, &s.RootCauseCategory, &s.RootCauseDescription,
		&confidence, &suggestions, &docLinks, &knowledgeID, &s.AutofixApplied, &autofixResult,
		&durationMs, &adminID, &s.CreatedAt, &completedAt,
	); err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if serverID != nil {
		id, _ := uuid.Parse(*serverID)
		s.ServerID = &id
	}
	if nodeID != nil {
		id, _ := uuid.Parse(*nodeID)
		s.NodeID = &id
	}
	if knowledgeID != nil {
		id, _ := uuid.Parse(*knowledgeID)
		s.KnowledgeEntryID = &id
	}
	if adminID != nil {
		id, _ := uuid.Parse(*adminID)
		s.CreatedByAdminID = &id
	}
	if timeStart != nil {
		s.TimeWindowStart = timeStart
	}
	if timeEnd != nil {
		s.TimeWindowEnd = timeEnd
	}
	if completedAt != nil {
		s.CompletedAt = completedAt
	}
	if confidence != nil {
		s.Confidence = confidence
	}
	if durationMs != nil {
		s.DurationMs = durationMs
	}
	if rawLogs != nil {
		s.RawLogs = *rawLogs
	}
	if llmModel != nil {
		s.LLMModel = *llmModel
	}
	if len(rawMetrics) > 0 {
		_ = json.Unmarshal(rawMetrics, &s.RawMetrics)
	}
	if len(suggestions) > 0 {
		_ = json.Unmarshal(suggestions, &s.Suggestions)
	}
	if len(docLinks) > 0 {
		_ = json.Unmarshal(docLinks, &s.DocLinks)
	}
	if len(autofixResult) > 0 {
		_ = json.Unmarshal(autofixResult, &s.AutofixResult)
	}
	return s, nil
}

// List 分页列表
func (r *Repo) List(ctx context.Context, q *ListSessionsQuery) ([]*DiagnosisSession, int, error) {
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 || q.PageSize > 100 {
		q.PageSize = 20
	}
	offset := (q.Page - 1) * q.PageSize

	where := "WHERE 1=1"
	args := []interface{}{}
	argPos := 1
	if q.NodeID != nil {
		where += fmt.Sprintf(" AND node_id = $%d", argPos)
		args = append(args, *q.NodeID)
		argPos++
	}
	if q.ServerID != nil {
		where += fmt.Sprintf(" AND server_id = $%d", argPos)
		args = append(args, *q.ServerID)
		argPos++
	}
	if q.Status != "" {
		where += fmt.Sprintf(" AND status = $%d", argPos)
		args = append(args, q.Status)
		argPos++
	}

	var total int
	if err := r.pool.QueryRow(ctx, fmt.Sprintf(`SELECT COUNT(*) FROM diagnosis_sessions %s`, where), args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	listSQL := fmt.Sprintf(`
		SELECT id, server_id, node_id, status, trigger_source, time_window_start, time_window_end,
			   raw_logs, raw_metrics, llm_provider, llm_model, root_cause_category, root_cause_description,
			   confidence, suggestions, doc_links, knowledge_entry_id, autofix_applied, autofix_result,
			   duration_ms, created_by_admin_id, created_at, completed_at
		FROM diagnosis_sessions %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, where, argPos, argPos+1)
	args = append(args, q.PageSize, offset)

	rows, err := r.pool.Query(ctx, listSQL, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []*DiagnosisSession
	for rows.Next() {
		s := &DiagnosisSession{}
		var serverID, nodeID, knowledgeID, adminID *string
		var timeStart, timeEnd, completedAt *time.Time
		var confidence *float64
		var durationMs *int
		var rawMetrics, suggestions, docLinks, autofixResult []byte
		var rawLogs *string
		var llmModel *string
		if err := rows.Scan(
			&s.ID, &serverID, &nodeID, &s.Status, &s.TriggerSource, &timeStart, &timeEnd,
			&rawLogs, &rawMetrics, &s.LLMProvider, &llmModel, &s.RootCauseCategory, &s.RootCauseDescription,
			&confidence, &suggestions, &docLinks, &knowledgeID, &s.AutofixApplied, &autofixResult,
			&durationMs, &adminID, &s.CreatedAt, &completedAt,
		); err != nil {
			return nil, 0, err
		}
		if serverID != nil {
			id, _ := uuid.Parse(*serverID)
			s.ServerID = &id
		}
		if nodeID != nil {
			id, _ := uuid.Parse(*nodeID)
			s.NodeID = &id
		}
		if knowledgeID != nil {
			id, _ := uuid.Parse(*knowledgeID)
			s.KnowledgeEntryID = &id
		}
		if adminID != nil {
			id, _ := uuid.Parse(*adminID)
			s.CreatedByAdminID = &id
		}
		if timeStart != nil {
			s.TimeWindowStart = timeStart
		}
		if timeEnd != nil {
			s.TimeWindowEnd = timeEnd
		}
		if completedAt != nil {
			s.CompletedAt = completedAt
		}
		if confidence != nil {
			s.Confidence = confidence
		}
		if durationMs != nil {
			s.DurationMs = durationMs
		}
		if rawLogs != nil {
			s.RawLogs = *rawLogs
		}
		if llmModel != nil {
			s.LLMModel = *llmModel
		}
		if len(rawMetrics) > 0 {
			_ = json.Unmarshal(rawMetrics, &s.RawMetrics)
		}
		if len(suggestions) > 0 {
			_ = json.Unmarshal(suggestions, &s.Suggestions)
		}
		if len(docLinks) > 0 {
			_ = json.Unmarshal(docLinks, &s.DocLinks)
		}
		if len(autofixResult) > 0 {
			_ = json.Unmarshal(autofixResult, &s.AutofixResult)
		}
		items = append(items, s)
	}
	return items, total, nil
}

// ListKnowledge 列出知识库
func (r *Repo) ListKnowledge(ctx context.Context, category string, onlyVerified bool, page, pageSize int) ([]*KnowledgeEntry, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	where := "WHERE 1=1"
	args := []interface{}{}
	argPos := 1
	if category != "" {
		where += fmt.Sprintf(" AND category = $%d", argPos)
		args = append(args, category)
		argPos++
	}
	if onlyVerified {
		where += " AND is_verified = true"
	}

	var total int
	if err := r.pool.QueryRow(ctx, fmt.Sprintf(`SELECT COUNT(*) FROM diagnosis_knowledge %s`, where), args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	listSQL := fmt.Sprintf(`
		SELECT id, title, category, root_cause_pattern, solution, auto_fix_action,
			   doc_links, hit_count, is_verified, created_at, updated_at
		FROM diagnosis_knowledge %s
		ORDER BY hit_count DESC, created_at DESC
		LIMIT $%d OFFSET $%d
	`, where, argPos, argPos+1)
	args = append(args, pageSize, offset)

	rows, err := r.pool.Query(ctx, listSQL, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []*KnowledgeEntry
	for rows.Next() {
		k := &KnowledgeEntry{}
		var autoFixAction *string
		var docLinks []byte
		if err := rows.Scan(
			&k.ID, &k.Title, &k.Category, &k.RootCausePattern, &k.Solution,
			&autoFixAction, &docLinks, &k.HitCount, &k.IsVerified, &k.CreatedAt, &k.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		if autoFixAction != nil {
			k.AutoFixAction = autoFixAction
		}
		if len(docLinks) > 0 {
			_ = json.Unmarshal(docLinks, &k.DocLinks)
		}
		items = append(items, k)
	}
	return items, total, nil
}

// MatchKnowledge 按根因关键词匹配知识库
func (r *Repo) MatchKnowledge(ctx context.Context, rootCauseCategory, description string) (*KnowledgeEntry, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, title, category, root_cause_pattern, solution, auto_fix_action,
			   doc_links, hit_count, is_verified, created_at, updated_at
		FROM diagnosis_knowledge
		WHERE category = $1 AND is_verified = true
		ORDER BY hit_count DESC
		LIMIT 1
	`, rootCauseCategory)
	k := &KnowledgeEntry{}
	var autoFixAction *string
	var docLinks []byte
	if err := row.Scan(
		&k.ID, &k.Title, &k.Category, &k.RootCausePattern, &k.Solution,
		&autoFixAction, &docLinks, &k.HitCount, &k.IsVerified, &k.CreatedAt, &k.UpdatedAt,
	); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if autoFixAction != nil {
		k.AutoFixAction = autoFixAction
	}
	if len(docLinks) > 0 {
		_ = json.Unmarshal(docLinks, &k.DocLinks)
	}
	return k, nil
}

// IncrementKnowledgeHit 知识库命中计数 +1
func (r *Repo) IncrementKnowledgeHit(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `UPDATE diagnosis_knowledge SET hit_count = hit_count + 1, updated_at = now() WHERE id = $1`, id)
	return err
}
