package repo

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/airport-panel/identity-service/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PlanRepo struct {
	pool *pgxpool.Pool
}

func NewPlanRepo(pool *pgxpool.Pool) *PlanRepo {
	return &PlanRepo{pool: pool}
}

const planColumns = `id, code, name, COALESCE(description,''), COALESCE(content,''), status, billing_type, traffic_bytes, speed_limit_mbps,
	device_limit, ip_limit, reset_cycle, duration_days, can_renew,
	sort_order, group_id, tags, COALESCE(features,'[]'::jsonb), feature_flags, created_at, updated_at, deleted_at`

func scanPlan(row pgx.Row, p *model.Plan) error {
	var tags []string
	var featureFlags []byte
	var featuresJSON []byte
	err := row.Scan(
		&p.ID, &p.Code, &p.Name, &p.Description, &p.Content, &p.Status, &p.BillingType, &p.TrafficBytes,
		&p.SpeedLimitMbps, &p.DeviceLimit, &p.IPLimit, &p.ResetCycle, &p.DurationDays,
		&p.CanRenew, &p.SortOrder, &p.GroupID, &tags, &featuresJSON, &featureFlags,
		&p.CreatedAt, &p.UpdatedAt, &p.DeletedAt,
	)
	if err != nil {
		return err
	}
	if tags == nil {
		tags = []string{}
	}
	p.Tags = tags
	if featuresJSON != nil {
		_ = json.Unmarshal(featuresJSON, &p.Features)
	}
	if p.Features == nil {
		p.Features = []string{}
	}
	if featureFlags != nil {
		p.FeatureFlags = featureFlags
	} else {
		p.FeatureFlags = json.RawMessage(`{}`)
	}
	return nil
}

func (r *PlanRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.Plan, error) {
	query := fmt.Sprintf(`SELECT %s FROM plans WHERE id = $1 AND deleted_at IS NULL`, planColumns)
	p := &model.Plan{}
	err := scanPlan(r.pool.QueryRow(ctx, query, id), p)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return p, nil
}

func (r *PlanRepo) GetByCode(ctx context.Context, code string) (*model.Plan, error) {
	query := fmt.Sprintf(`SELECT %s FROM plans WHERE code = $1 AND deleted_at IS NULL`, planColumns)
	p := &model.Plan{}
	err := scanPlan(r.pool.QueryRow(ctx, query, code), p)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return p, nil
}

func (r *PlanRepo) ListActive(ctx context.Context) ([]*model.Plan, error) {
	query := fmt.Sprintf(`SELECT %s FROM plans WHERE status = 'active' AND deleted_at IS NULL
		ORDER BY sort_order ASC, created_at ASC`, planColumns)
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var plans []*model.Plan
	for rows.Next() {
		p := &model.Plan{}
		if err := scanPlan(rows, p); err != nil {
			return nil, err
		}
		plans = append(plans, p)
	}
	return plans, rows.Err()
}

func (r *PlanRepo) List(ctx context.Context, page, pageSize int, query model.PlanListQuery) ([]*model.Plan, int, error) {
	var where []string
	var args []interface{}
	argIdx := 1

	where = append(where, "deleted_at IS NULL")

	if query.Status != "" {
		where = append(where, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, query.Status)
		argIdx++
	}
	if query.BillingType != "" {
		where = append(where, fmt.Sprintf("billing_type = $%d", argIdx))
		args = append(args, query.BillingType)
		argIdx++
	}

	whereClause := strings.Join(where, " AND ")

	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM plans WHERE %s`, whereClause)
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	listQuery := fmt.Sprintf(`SELECT %s FROM plans WHERE %s ORDER BY sort_order ASC, created_at ASC LIMIT $%d OFFSET $%d`,
		planColumns, whereClause, argIdx, argIdx+1)
	args = append(args, pageSize, (page-1)*pageSize)

	rows, err := r.pool.Query(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []*model.Plan
	for rows.Next() {
		p := &model.Plan{}
		if err := scanPlan(rows, p); err != nil {
			return nil, 0, err
		}
		items = append(items, p)
	}
	return items, total, rows.Err()
}

func (r *PlanRepo) Create(ctx context.Context, p *model.Plan) error {
	var featureFlags []byte
	if p.FeatureFlags != nil {
		featureFlags = p.FeatureFlags
	} else {
		featureFlags = []byte(`{}`)
	}
	tags := p.Tags
	if tags == nil {
		tags = []string{}
	}

	query := `
		INSERT INTO plans (code, name, description, content, status, billing_type, traffic_bytes, speed_limit_mbps,
			device_limit, ip_limit, reset_cycle, duration_days, can_renew,
			sort_order, group_id, tags, feature_flags)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
		RETURNING id, created_at, updated_at`
	return r.pool.QueryRow(ctx, query,
		p.Code, p.Name, p.Description, p.Content, p.Status, p.BillingType, p.TrafficBytes, p.SpeedLimitMbps,
		p.DeviceLimit, p.IPLimit, p.ResetCycle, p.DurationDays, p.CanRenew,
		p.SortOrder, p.GroupID, tags, featureFlags,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
}

func (r *PlanRepo) Update(ctx context.Context, p *model.Plan) error {
	var featureFlags []byte
	if p.FeatureFlags != nil {
		featureFlags = p.FeatureFlags
	} else {
		featureFlags = []byte(`{}`)
	}
	tags := p.Tags
	if tags == nil {
		tags = []string{}
	}

	query := `
		UPDATE plans SET
			name = $2, description = $3, content = $4, status = $5, billing_type = $6, traffic_bytes = $7,
			speed_limit_mbps = $8, device_limit = $9, ip_limit = $10,
			reset_cycle = $11, duration_days = $12, can_renew = $13,
			sort_order = $14, group_id = $15, tags = $16, feature_flags = $17, updated_at = now()
		WHERE id = $1 AND deleted_at IS NULL`
	_, err := r.pool.Exec(ctx, query,
		p.ID, p.Name, p.Description, p.Content, p.Status, p.BillingType, p.TrafficBytes,
		p.SpeedLimitMbps, p.DeviceLimit, p.IPLimit, p.ResetCycle, p.DurationDays,
		p.CanRenew, p.SortOrder, p.GroupID, tags, featureFlags,
	)
	return err
}

func (r *PlanRepo) Delete(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE plans SET deleted_at = now(), updated_at = now() WHERE id = $1 AND deleted_at IS NULL`
	_, err := r.pool.Exec(ctx, query, id)
	return err
}

func (r *PlanRepo) GetPrices(ctx context.Context, planID uuid.UUID) (map[string]model.PlanPriceEntry, error) {
	query := `SELECT period_code, amount_minor, amount_cny FROM plan_prices WHERE plan_id = $1 AND currency_code = 'USDT-TRC20' AND is_active = true`
	rows, err := r.pool.Query(ctx, query, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// 获取汇率用于 amount_cny 为 NULL 时的回退计算
	rate := r.GetExchangeRate(ctx)

	prices := make(map[string]model.PlanPriceEntry)
	for rows.Next() {
		var period string
		var amountMinor int64
		var amountCNY *int64
		if err := rows.Scan(&period, &amountMinor, &amountCNY); err != nil {
			return nil, err
		}
		usdt := float64(amountMinor) / 100.0
		var cny float64
		if amountCNY != nil {
			cny = float64(*amountCNY) / 100.0
		} else {
			cny = math.Round(usdt*rate*100) / 100
		}
		prices[period] = model.PlanPriceEntry{USDT: usdt, CNY: cny}
	}
	return prices, rows.Err()
}

func (r *PlanRepo) SetPrices(ctx context.Context, planID uuid.UUID, prices map[string]model.PlanPriceEntry) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM plan_prices WHERE plan_id = $1 AND currency_code = 'USDT-TRC20'`, planID); err != nil {
		return err
	}

	// 获取汇率：当管理员只录入 CNY 时，自动换算 USDT
	rate := r.GetExchangeRate(ctx)

	for period, entry := range prices {
		usdt := entry.USDT
		cny := entry.CNY
		// 若只录入了 CNY（USDT=0），按汇率反算 USDT
		if usdt == 0 && cny > 0 && rate > 0 {
			usdt = math.Round(cny/rate*100) / 100
		}
		// 若只录入了 USDT（CNY=0），按汇率正算 CNY
		if cny == 0 && usdt > 0 && rate > 0 {
			cny = math.Round(usdt*rate*100) / 100
		}
		amountMinor := int64(math.Round(usdt * 100))
		var amountCNY *int64
		if cny > 0 {
			v := int64(math.Round(cny * 100))
			amountCNY = &v
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO plan_prices (plan_id, period_code, currency_code, amount_minor, amount_cny, is_active) VALUES ($1, $2, 'USDT-TRC20', $3, $4, true)
			 ON CONFLICT (plan_id, period_code, currency_code) DO UPDATE SET amount_minor = EXCLUDED.amount_minor, amount_cny = EXCLUDED.amount_cny, is_active = true`,
			planID, period, amountMinor, amountCNY); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

// GetExchangeRate 从 system_settings 读取 ('payment', 'exchange_rate') 配置，返回 usdt_to_cny 汇率，默认 7.2
func (r *PlanRepo) GetExchangeRate(ctx context.Context) float64 {
	const defaultRate = 7.2
	var data []byte
	err := r.pool.QueryRow(ctx, `SELECT value_json FROM system_settings WHERE setting_group = $1 AND setting_key = $2`, "payment", "exchange_rate").Scan(&data)
	if err != nil {
		return defaultRate
	}
	var cfg struct {
		USDTToCNY float64 `json:"usdt_to_cny"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return defaultRate
	}
	if cfg.USDTToCNY <= 0 {
		return defaultRate
	}
	return cfg.USDTToCNY
}

func (r *PlanRepo) CountNodesForPlan(ctx context.Context, planID uuid.UUID) (int, error) {
	var n int
	// 通过 group_id 分组关联：套餐有 group_id 时返回该分组节点，无 group_id 时返回全部节点
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM nodes n
		WHERE n.is_enabled = true AND n.is_visible = true AND n.deleted_at IS NULL
		AND ((SELECT group_id FROM plans WHERE id = $1 AND deleted_at IS NULL) IS NULL
			OR n.group_id = (SELECT group_id FROM plans WHERE id = $1 AND deleted_at IS NULL))`, planID).Scan(&n)
	return n, err
}

func (r *PlanRepo) ListNodesForPlan(ctx context.Context, planID uuid.UUID) ([]*model.PlanNodeInfo, error) {
	rows, err := r.pool.Query(ctx, `SELECT n.id, n.name,
		COALESCE(n.protocol_type,''),
		COALESCE(n.traffic_rate,1.0),
		COALESCE(n.last_seen_at IS NOT NULL AND n.last_seen_at > now() - interval '2 minutes', false),
		COALESCE(r.country_code,''),
		COALESCE(n.tags,'{}'::text[])
	FROM nodes n
	LEFT JOIN regions r ON r.id = n.region_id
	WHERE n.is_enabled = true AND n.is_visible = true AND n.deleted_at IS NULL
		AND ((SELECT group_id FROM plans WHERE id = $1 AND deleted_at IS NULL) IS NULL
			OR n.group_id = (SELECT group_id FROM plans WHERE id = $1 AND deleted_at IS NULL))
	ORDER BY n.priority ASC NULLS LAST, n.name ASC`, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.PlanNodeInfo
	for rows.Next() {
		pn := &model.PlanNodeInfo{Rate: 1.0}
		var countryCode string
		var tags []string
		if err := rows.Scan(&pn.ID, &pn.Name, &pn.Protocol, &pn.Rate, &pn.IsOnline, &countryCode, &tags); err != nil {
			return nil, err
		}
		pn.CountryCode = countryCode
		pn.Tags = tags
		pn.CountryFlag = countryCodeToFlag(countryCode)
		out = append(out, pn)
	}
	return out, rows.Err()
}

func countryCodeToFlag(code string) string {
	flags := map[string]string{
		"JP": "🇯🇵", "HK": "🇭🇰", "CN": "🇨🇳", "TW": "🇹🇼", "SG": "🇸🇬",
		"US": "🇺🇸", "KR": "🇰🇷", "DE": "🇩🇪", "GB": "🇬🇧", "FR": "🇫🇷",
		"CA": "🇨🇦", "AU": "🇦🇺", "RU": "🇷🇺", "NL": "🇳🇱", "CH": "🇨🇭",
		"SE": "🇸🇪", "NO": "🇳🇴", "FI": "🇫🇮", "DK": "🇩🇰", "IT": "🇮🇹",
		"ES": "🇪🇸", "TR": "🇹🇷", "IN": "🇮🇳", "TH": "🇹🇭", "VN": "🇻🇳",
		"PH": "🇵🇭", "MY": "🇲🇾", "ID": "🇮🇩", "BR": "🇧🇷", "MX": "🇲🇽",
		"AR": "🇦🇷", "CL": "🇨🇱", "ZA": "🇿🇦", "AE": "🇦🇪", "IL": "🇮🇱",
	}
	if f, ok := flags[strings.ToUpper(code)]; ok {
		return f
	}
	return "🌐"
}

// ReplacePlanNodes 批量替换套餐的节点绑定（事务：先删除旧关联，再插入新关联）
// nodeIDs 为空时表示解绑所有节点
func (r *PlanRepo) ReplacePlanNodes(ctx context.Context, planID uuid.UUID, nodeIDs []uuid.UUID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM plan_nodes WHERE plan_id = $1`, planID); err != nil {
		return err
	}

	if len(nodeIDs) > 0 {
		for _, nid := range nodeIDs {
			if _, err := tx.Exec(ctx,
				`INSERT INTO plan_nodes (plan_id, node_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
				planID, nid); err != nil {
				return err
			}
		}
	}

	return tx.Commit(ctx)
}

// ListPlanCodesForNode 返回节点已绑定的套餐 code 列表（用于 NodeResponse 展示）
func (r *PlanRepo) ListPlanCodesForNode(ctx context.Context, nodeID uuid.UUID) ([]string, error) {
	rows, err := r.pool.Query(ctx, `SELECT p.code FROM plan_nodes pn
		JOIN plans p ON p.id = pn.plan_id
		WHERE pn.node_id = $1 ORDER BY p.code`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var codes []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		codes = append(codes, c)
	}
	return codes, rows.Err()
}

func (r *PlanRepo) GetUserTrafficLogs(ctx context.Context, userID uuid.UUID, days int) ([]*model.TrafficLog, error) {
	if days <= 0 || days > 365 {
		days = 7
	}
	startDate := time.Now().AddDate(0, 0, -days).Truncate(24 * time.Hour)
	query := `
		SELECT usage_date, COALESCE(SUM(upload_bytes), 0), COALESCE(SUM(download_bytes), 0)
		FROM traffic_usage_daily
		WHERE user_id = $1 AND usage_date >= $2
		GROUP BY usage_date
		ORDER BY usage_date ASC`
	rows, err := r.pool.Query(ctx, query, userID, startDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	logMap := make(map[string]*model.TrafficLog)
	var dates []string
	for rows.Next() {
		var date time.Time
		var up, down int64
		if err := rows.Scan(&date, &up, &down); err != nil {
			return nil, err
		}
		ds := date.Format("2006-01-02")
		logMap[ds] = &model.TrafficLog{
			Date:     ds,
			Upload:   up,
			Download: down,
			Total:    up + down,
		}
		dates = append(dates, ds)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var logs []*model.TrafficLog
	for i := days - 1; i >= 0; i-- {
		d := time.Now().AddDate(0, 0, -i).Truncate(24 * time.Hour)
		ds := d.Format("2006-01-02")
		if l, ok := logMap[ds]; ok {
			logs = append(logs, l)
		} else {
			logs = append(logs, &model.TrafficLog{Date: ds, Upload: 0, Download: 0, Total: 0})
		}
	}
	return logs, nil
}
