package lb

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/google/uuid"
)

// ===================== 内存 fake 实现 =====================

// fakeDataReader 是 NodeDataReader 的内存实现，不依赖真实数据库 / redis
type fakeDataReader struct {
	policy    *LBPolicy
	nodes     []NodeCandidate
	counter   int64
	policyErr error
	nodesErr  error
	incrErr   error
}

func (f *fakeDataReader) GetPolicy(ctx context.Context, groupID uuid.UUID) (*LBPolicy, error) {
	if f.policyErr != nil {
		return nil, f.policyErr
	}
	return f.policy, nil
}

func (f *fakeDataReader) GetGroupNodes(ctx context.Context, groupID uuid.UUID) ([]NodeCandidate, error) {
	if f.nodesErr != nil {
		return nil, f.nodesErr
	}
	return f.nodes, nil
}

func (f *fakeDataReader) IncrCounter(ctx context.Context, key string) (int64, error) {
	if f.incrErr != nil {
		return 0, f.incrErr
	}
	f.counter++
	return f.counter, nil
}

// fakeGeoResolver 是 GeoResolver 的内存实现
type fakeGeoResolver struct {
	ipToCountry map[string]string
}

func (g *fakeGeoResolver) CountryCode(ip string) string {
	return g.ipToCountry[ip]
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(&discardWriter{}, &slog.HandlerOptions{Level: slog.LevelError}))
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

func ptrInt(v int) *int { return &v }
func ptrStr(v string) *string { return &v }

func mkCandidate(code string, priority, sortOrder, rtt, online, score int, country string) NodeCandidate {
	return NodeCandidate{
		NodeID:            uuid.New(),
		Code:              code,
		Name:              code,
		GroupID:           uuid.New(),
		Priority:          priority,
		SortOrder:         sortOrder,
		RTTms:             rtt,
		OnlineUsers:       online,
		HealthScore:       score,
		Capacity:          100,
		RegionCountryCode: country,
	}
}

// ===================== 测试用例 =====================

// 1. round_robin：3 个节点轮转 3 次拿到不同起始位置
func TestRoundRobin_RotatesStartingPosition(t *testing.T) {
	groupID := uuid.New()
	nodes := []NodeCandidate{
		mkCandidate("A", 100, 0, 50, 10, 80, "HK"),
		mkCandidate("B", 100, 1, 50, 10, 80, "HK"),
		mkCandidate("C", 100, 2, 50, 10, 80, "HK"),
	}
	reader := &fakeDataReader{
		policy: &LBPolicy{GroupID: groupID, LBStrategy: StrategyRoundRobin, WeightField: "priority", MinScoreThreshold: 0},
		nodes:  nodes,
	}
	engine := NewLBEngine(reader, testLogger())

	req := &LBRequest{GroupID: groupID}
	var firstCodes []string
	for i := 0; i < 3; i++ {
		res, err := engine.SelectNodes(context.Background(), req)
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		if len(res.SelectedNodes) != 3 {
			t.Fatalf("call %d: expected 3 nodes, got %d", i, len(res.SelectedNodes))
		}
		firstCodes = append(firstCodes, res.SelectedNodes[0].Code)
	}
	// 三次起始节点应两两不同
	seen := map[string]bool{}
	for _, c := range firstCodes {
		if seen[c] {
			t.Fatalf("expected 3 distinct starting nodes, got duplicates: %v", firstCodes)
		}
		seen[c] = true
	}
	if len(seen) != 3 {
		t.Fatalf("expected 3 distinct starting nodes, got %v", firstCodes)
	}
}

// 2. weighted：高 priority 节点被选到首位的概率更高（统计 1000 次）
func TestWeighted_HighPrioritySelectedMore(t *testing.T) {
	groupID := uuid.New()
	nodes := []NodeCandidate{
		mkCandidate("low1", 1, 0, 50, 10, 80, "HK"),
		mkCandidate("HIGH", 100, 1, 50, 10, 80, "HK"),
		mkCandidate("low2", 1, 2, 50, 10, 80, "HK"),
	}
	reader := &fakeDataReader{
		policy: &LBPolicy{GroupID: groupID, LBStrategy: StrategyWeighted, WeightField: "priority", MinScoreThreshold: 0},
		nodes:  nodes,
	}
	engine := NewLBEngine(reader, testLogger())

	req := &LBRequest{GroupID: groupID}
	counts := map[string]int{}
	for i := 0; i < 1000; i++ {
		res, err := engine.SelectNodes(context.Background(), req)
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		counts[res.SelectedNodes[0].Code]++
	}
	high := counts["HIGH"]
	low1 := counts["low1"]
	low2 := counts["low2"]
	if high <= low1 || high <= low2 {
		t.Fatalf("expected HIGH node selected first more often, got HIGH=%d low1=%d low2=%d", high, low1, low2)
	}
	// 权重 1:100:1，HIGH 应占据绝大多数
	if high < 800 {
		t.Fatalf("expected HIGH selected first >=800 times, got %d", high)
	}
}

// 3. least_conn：online_users 最少的排首位
func TestLeastConn_LeastOnlineUsersFirst(t *testing.T) {
	groupID := uuid.New()
	nodes := []NodeCandidate{
		mkCandidate("A", 100, 0, 50, 50, 80, "HK"),
		mkCandidate("B", 100, 1, 50, 10, 80, "HK"),
		mkCandidate("C", 100, 2, 50, 30, 80, "HK"),
	}
	reader := &fakeDataReader{
		policy: &LBPolicy{GroupID: groupID, LBStrategy: StrategyLeastConn, WeightField: "priority", MinScoreThreshold: 0},
		nodes:  nodes,
	}
	engine := NewLBEngine(reader, testLogger())

	res, err := engine.SelectNodes(context.Background(), &LBRequest{GroupID: groupID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.SelectedNodes[0].Code != "B" {
		t.Fatalf("expected B (10 users) first, got %s (users=%d)", res.SelectedNodes[0].Code, res.SelectedNodes[0].OnlineUsers)
	}
	if res.SelectedNodes[1].Code != "C" {
		t.Fatalf("expected C (30 users) second, got %s", res.SelectedNodes[1].Code)
	}
	if res.SelectedNodes[2].Code != "A" {
		t.Fatalf("expected A (50 users) last, got %s", res.SelectedNodes[2].Code)
	}
}

// 4. latency：rtt 最低的排首位
func TestLatency_LowestRTTFirst(t *testing.T) {
	groupID := uuid.New()
	nodes := []NodeCandidate{
		mkCandidate("A", 100, 0, 100, 10, 80, "HK"),
		mkCandidate("B", 100, 1, 20, 10, 80, "HK"),
		mkCandidate("C", 100, 2, 0, 10, 80, "HK"), // RTT=0 视为未知，排最后
	}
	reader := &fakeDataReader{
		policy: &LBPolicy{GroupID: groupID, LBStrategy: StrategyLatency, WeightField: "priority", MinScoreThreshold: 0},
		nodes:  nodes,
	}
	engine := NewLBEngine(reader, testLogger())

	res, err := engine.SelectNodes(context.Background(), &LBRequest{GroupID: groupID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.SelectedNodes[0].Code != "B" {
		t.Fatalf("expected B (rtt=20) first, got %s (rtt=%d)", res.SelectedNodes[0].Code, res.SelectedNodes[0].RTTms)
	}
	if res.SelectedNodes[2].Code != "C" {
		t.Fatalf("expected C (rtt unknown) last, got %s", res.SelectedNodes[2].Code)
	}
}

// 5. sticky_user：同一 user_id 两次拿到相同首节点
func TestStickyUser_SameUserSameNode(t *testing.T) {
	groupID := uuid.New()
	nodes := []NodeCandidate{
		mkCandidate("A", 100, 0, 50, 10, 80, "HK"),
		mkCandidate("B", 100, 1, 50, 10, 80, "HK"),
		mkCandidate("C", 100, 2, 50, 10, 80, "HK"),
	}
	reader := &fakeDataReader{
		policy: &LBPolicy{GroupID: groupID, LBStrategy: StrategyStickyUser, WeightField: "priority", MinScoreThreshold: 0, StickyBy: ptrStr(StickyByUserID)},
		nodes:  nodes,
	}
	engine := NewLBEngine(reader, testLogger())

	req := &LBRequest{GroupID: groupID, UserID: "user-123"}
	res1, err := engine.SelectNodes(context.Background(), req)
	if err != nil {
		t.Fatalf("call 1: %v", err)
	}
	res2, err := engine.SelectNodes(context.Background(), req)
	if err != nil {
		t.Fatalf("call 2: %v", err)
	}
	if res1.SelectedNodes[0].NodeID != res2.SelectedNodes[0].NodeID {
		t.Fatalf("expected same first node for same user, got %s vs %s", res1.SelectedNodes[0].Code, res2.SelectedNodes[0].Code)
	}
	// 不同 user 大概率拿到不同首节点（这里不强制，但同 user 必须稳定）
}

// 6. geo_affinity：用户 IP 匹配 region 的节点优先
func TestGeoAffinity_MatchingCountryFirst(t *testing.T) {
	groupID := uuid.New()
	nodes := []NodeCandidate{
		mkCandidate("HK-1", 100, 0, 50, 10, 80, "HK"),
		mkCandidate("US-1", 100, 1, 50, 10, 80, "US"),
		mkCandidate("HK-2", 100, 2, 50, 10, 80, "HK"),
	}
	reader := &fakeDataReader{
		policy: &LBPolicy{GroupID: groupID, LBStrategy: StrategyGeoAffinity, WeightField: "priority", MinScoreThreshold: 0},
		nodes:  nodes,
	}
	engine := NewLBEngine(reader, testLogger()).WithGeoResolver(&fakeGeoResolver{
		ipToCountry: map[string]string{"1.2.3.4": "HK"},
	})

	res, err := engine.SelectNodes(context.Background(), &LBRequest{GroupID: groupID, UserIP: "1.2.3.4"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.SelectedNodes[0].RegionCountryCode != "HK" {
		t.Fatalf("expected first node country HK, got %s", res.SelectedNodes[0].RegionCountryCode)
	}
	if res.SelectedNodes[1].RegionCountryCode != "HK" {
		t.Fatalf("expected second node country HK, got %s", res.SelectedNodes[1].RegionCountryCode)
	}
	if res.SelectedNodes[2].RegionCountryCode != "US" {
		t.Fatalf("expected last node country US, got %s", res.SelectedNodes[2].RegionCountryCode)
	}
}

// 7. min_score_threshold：低于阈值的被过滤
func TestMinScoreThreshold_FiltersLowScore(t *testing.T) {
	groupID := uuid.New()
	nodes := []NodeCandidate{
		mkCandidate("good1", 100, 0, 50, 10, 80, "HK"),
		mkCandidate("bad1", 100, 1, 50, 10, 30, "HK"),
		mkCandidate("good2", 100, 2, 50, 10, 60, "HK"),
		mkCandidate("bad2", 100, 3, 50, 10, 20, "HK"),
	}
	reader := &fakeDataReader{
		policy: &LBPolicy{GroupID: groupID, LBStrategy: StrategyRoundRobin, WeightField: "priority", MinScoreThreshold: 50},
		nodes:  nodes,
	}
	engine := NewLBEngine(reader, testLogger())

	res, err := engine.SelectNodes(context.Background(), &LBRequest{GroupID: groupID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.SelectedNodes) != 2 {
		t.Fatalf("expected 2 selected nodes, got %d", len(res.SelectedNodes))
	}
	if len(res.FilteredOut) != 2 {
		t.Fatalf("expected 2 filtered out nodes, got %d", len(res.FilteredOut))
	}
	for _, c := range res.SelectedNodes {
		if c.HealthScore < 50 {
			t.Fatalf("selected node %s score %d below threshold", c.Code, c.HealthScore)
		}
	}
	for _, c := range res.FilteredOut {
		if c.HealthScore >= 50 {
			t.Fatalf("filtered node %s score %d above threshold", c.Code, c.HealthScore)
		}
	}
}

// 8. max_nodes_per_subscription：截断到指定数量
func TestMaxNodesPerSubscription_Truncates(t *testing.T) {
	groupID := uuid.New()
	nodes := []NodeCandidate{
		mkCandidate("A", 100, 0, 50, 10, 80, "HK"),
		mkCandidate("B", 100, 1, 50, 10, 80, "HK"),
		mkCandidate("C", 100, 2, 50, 10, 80, "HK"),
		mkCandidate("D", 100, 3, 50, 10, 80, "HK"),
	}
	reader := &fakeDataReader{
		policy: &LBPolicy{GroupID: groupID, LBStrategy: StrategyRoundRobin, WeightField: "priority", MinScoreThreshold: 0, MaxNodesPerSubscription: ptrInt(2)},
		nodes:  nodes,
	}
	engine := NewLBEngine(reader, testLogger())

	res, err := engine.SelectNodes(context.Background(), &LBRequest{GroupID: groupID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.SelectedNodes) != 2 {
		t.Fatalf("expected 2 selected nodes after truncation, got %d", len(res.SelectedNodes))
	}
	if len(res.FilteredOut) != 2 {
		t.Fatalf("expected 2 truncated nodes in filtered out, got %d", len(res.FilteredOut))
	}
}

// 9. 无候选节点返回 ErrNoCandidates
func TestNoCandidates_ReturnsError(t *testing.T) {
	groupID := uuid.New()
	reader := &fakeDataReader{
		policy: &LBPolicy{GroupID: groupID, LBStrategy: StrategyRoundRobin, MinScoreThreshold: 0},
		nodes:  nil,
	}
	engine := NewLBEngine(reader, testLogger())
	_, err := engine.SelectNodes(context.Background(), &LBRequest{GroupID: groupID})
	if !errors.Is(err, ErrNoCandidates) {
		t.Fatalf("expected ErrNoCandidates, got %v", err)
	}
}

// 10. 不支持的策略返回 ErrInvalidStrategy
func TestInvalidStrategy_ReturnsError(t *testing.T) {
	groupID := uuid.New()
	reader := &fakeDataReader{
		policy: &LBPolicy{GroupID: groupID, LBStrategy: "bogus_strategy", MinScoreThreshold: 0},
		nodes:  []NodeCandidate{mkCandidate("A", 100, 0, 50, 10, 80, "HK")},
	}
	engine := NewLBEngine(reader, testLogger())
	_, err := engine.SelectNodes(context.Background(), &LBRequest{GroupID: groupID})
	if !errors.Is(err, ErrInvalidStrategy) {
		t.Fatalf("expected ErrInvalidStrategy, got %v", err)
	}
}

// 11. 未配置策略时使用默认 round_robin（验证默认策略可用）
func TestDefaultPolicy_WhenPolicyNil(t *testing.T) {
	groupID := uuid.New()
	reader := &fakeDataReader{
		policy: nil, // 未配置
		nodes:  []NodeCandidate{mkCandidate("A", 100, 0, 50, 10, 80, "HK")},
	}
	engine := NewLBEngine(reader, testLogger())
	res, err := engine.SelectNodes(context.Background(), &LBRequest{GroupID: groupID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Strategy != StrategyRoundRobin {
		t.Fatalf("expected default strategy round_robin, got %s", res.Strategy)
	}
}
