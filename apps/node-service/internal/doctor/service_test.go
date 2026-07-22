package doctor

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// fakeReportStore 内存实现
type fakeReportStore struct {
	reports map[uuid.UUID][]*DoctorReport
	last   *DoctorReport
}

func newFakeReportStore() *fakeReportStore {
	return &fakeReportStore{reports: map[uuid.UUID][]*DoctorReport{}}
}

func (f *fakeReportStore) Create(ctx context.Context, rep *DoctorReport) error {
	f.reports[rep.NodeID] = append(f.reports[rep.NodeID], rep)
	f.last = rep
	return nil
}
func (f *fakeReportStore) GetByID(ctx context.Context, id uuid.UUID) (*DoctorReport, error) {
	for _, list := range f.reports {
		for _, r := range list {
			if r.ID == id {
				return r, nil
			}
		}
	}
	return nil, nil
}
func (f *fakeReportStore) GetLatestByNodeID(ctx context.Context, nodeID uuid.UUID) (*DoctorReport, error) {
	list := f.reports[nodeID]
	if len(list) == 0 {
		return nil, nil
	}
	return list[len(list)-1], nil
}
func (f *fakeReportStore) ListByNodeID(ctx context.Context, nodeID uuid.UUID, page, pageSize int) ([]*DoctorReport, int, error) {
	list := f.reports[nodeID]
	return list, len(list), nil
}

// fakeDefStore 内存实现
type fakeDefStore struct {
	defs []*DoctorCheckDef
}

func (f *fakeDefStore) ListEnabled(ctx context.Context) ([]*DoctorCheckDef, error) {
	var out []*DoctorCheckDef
	for _, d := range f.defs {
		if d.IsEnabled {
			out = append(out, d)
		}
	}
	return out, nil
}
func (f *fakeDefStore) GetByCode(ctx context.Context, code string) (*DoctorCheckDef, error) {
	for _, d := range f.defs {
		if d.Code == code {
			return d, nil
		}
	}
	return nil, nil
}
func (f *fakeDefStore) GetByCodes(ctx context.Context, codes []string) ([]*DoctorCheckDef, error) {
	var out []*DoctorCheckDef
	for _, d := range f.defs {
		for _, c := range codes {
			if d.Code == c {
				out = append(out, d)
				break
			}
		}
	}
	return out, nil
}

// fakeNodeFetcher 可配置返回 info（nil 表示不适用）
type fakeNodeFetcher struct {
	info *NodeExposureInfo
}

func (f *fakeNodeFetcher) Fetch(ctx context.Context, nodeID uuid.UUID) (*NodeExposureInfo, error) {
	if f.info == nil {
		return nil, nil
	}
	return f.info, nil
}

func TestRunSingleCheck_SkipWhenNotApplicable_ReturnsSkip(t *testing.T) {
	defStore := &fakeDefStore{
		defs: []*DoctorCheckDef{
			{
				ID:                      uuid.New(),
				Code:                    "tls_cert_valid",
				Name:                    "TLS 证书有效性",
				CheckCategory:           "tls",
				Severity:                "fail",
				ApplicableExposureModes: []string{"direct_public_ip", "nginx_reverse_proxy"},
				IsEnabled:               true,
			},
		},
	}
	reportStore := newFakeReportStore()
	// 节点当前 exposure_mode 为 cloudflare_tunnel_fixed，不适用 tls_cert_valid
	nodeFetcher := &fakeNodeFetcher{info: &NodeExposureInfo{
		NodeID:       uuid.New(),
		ExposureMode: "cloudflare_tunnel_fixed",
		ProtocolType: "vless",
	}}

	svc := NewDoctorService(reportStore, defStore, nodeFetcher, nil, nil)
	nodeID := uuid.New()
	rep, err := svc.RunSingleCheck(context.Background(), nodeID, "tls_cert_valid", "manual")
	if err != nil {
		t.Fatalf("RunSingleCheck failed: %v", err)
	}
	if len(rep.Checks) != 1 {
		t.Fatalf("expected 1 check result, got %d", len(rep.Checks))
	}
	if rep.Checks[0].Status != "skip" {
		t.Errorf("Status = %q, want skip", rep.Checks[0].Status)
	}
	if rep.OverallStatus != "healthy" {
		t.Errorf("OverallStatus = %q, want healthy (skip 不计入 fail/warn)", rep.OverallStatus)
	}
	if rep.SummaryOK != 0 {
		t.Errorf("SummaryOK = %d, want 0", rep.SummaryOK)
	}
	if rep.SummaryFail != 0 {
		t.Errorf("SummaryFail = %d, want 0", rep.SummaryFail)
	}
}

func TestRunSingleCheck_Applicable_ReturnsPass(t *testing.T) {
	defStore := &fakeDefStore{
		defs: []*DoctorCheckDef{
			{
				ID:                      uuid.New(),
				Code:                    "dns_resolve",
				Name:                    "DNS 解析检查",
				CheckCategory:           "network",
				Severity:                "fail",
				ApplicableExposureModes: []string{"*"},
				IsEnabled:               true,
			},
		},
	}
	reportStore := newFakeReportStore()
	nodeFetcher := &fakeNodeFetcher{info: &NodeExposureInfo{
		NodeID:       uuid.New(),
		ExposureMode: "direct_public_ip",
		ProtocolType: "vless",
	}}

	svc := NewDoctorService(reportStore, defStore, nodeFetcher, nil, nil)
	nodeID := uuid.New()
	rep, err := svc.RunSingleCheck(context.Background(), nodeID, "dns_resolve", "manual")
	if err != nil {
		t.Fatalf("RunSingleCheck failed: %v", err)
	}
	if len(rep.Checks) != 1 {
		t.Fatalf("expected 1 check result, got %d", len(rep.Checks))
	}
	if rep.Checks[0].Status != "pass" {
		t.Errorf("Status = %q, want pass", rep.Checks[0].Status)
	}
	if rep.Checks[0].Message != stubPassMessage {
		t.Errorf("Message = %q, want %q", rep.Checks[0].Message, stubPassMessage)
	}
	if rep.SummaryOK != 1 {
		t.Errorf("SummaryOK = %d, want 1", rep.SummaryOK)
	}
	if rep.OverallStatus != "healthy" {
		t.Errorf("OverallStatus = %q, want healthy", rep.OverallStatus)
	}
}

func TestRunFullCheck_FiltersByExposureMode(t *testing.T) {
	defStore := &fakeDefStore{
		defs: []*DoctorCheckDef{
			{
				Code: "dns_resolve", Name: "DNS", CheckCategory: "network", Severity: "fail",
				ApplicableExposureModes: []string{"*"}, IsEnabled: true,
			},
			{
				Code: "nginx_upstream", Name: "Nginx upstream", CheckCategory: "nginx", Severity: "fail",
				ApplicableExposureModes: []string{"nginx_reverse_proxy"}, IsEnabled: true,
			},
		},
	}
	reportStore := newFakeReportStore()
	// 节点为 direct_public_ip，nginx_upstream 不适用
	nodeFetcher := &fakeNodeFetcher{info: &NodeExposureInfo{
		ExposureMode: "direct_public_ip",
		ProtocolType: "vless",
	}}

	svc := NewDoctorService(reportStore, defStore, nodeFetcher, nil, nil)
	rep, err := svc.RunFullCheck(context.Background(), uuid.New(), "scheduled")
	if err != nil {
		t.Fatalf("RunFullCheck failed: %v", err)
	}
	// 应只跑 dns_resolve（nginx_upstream 被过滤掉）
	if len(rep.Checks) != 1 {
		t.Fatalf("expected 1 applicable check, got %d", len(rep.Checks))
	}
	if rep.Checks[0].CheckCode != "dns_resolve" {
		t.Errorf("CheckCode = %q, want dns_resolve", rep.Checks[0].CheckCode)
	}
	if rep.SummaryOK != 1 {
		t.Errorf("SummaryOK = %d, want 1", rep.SummaryOK)
	}
}

func TestAutoFix_ManualRequiredForAutoFixAvailable(t *testing.T) {
	defStore := &fakeDefStore{
		defs: []*DoctorCheckDef{
			{
				Code: "tls_cert_expiry", Name: "cert expiry", CheckCategory: "tls", Severity: "warn",
				ApplicableExposureModes: []string{"*"}, AutoFixAvailable: true, IsEnabled: true,
			},
			{
				Code: "dns_resolve", Name: "dns", CheckCategory: "network", Severity: "fail",
				ApplicableExposureModes: []string{"*"}, AutoFixAvailable: false, IsEnabled: true,
			},
		},
	}
	reportStore := newFakeReportStore()
	svc := NewDoctorService(reportStore, defStore, nil, nil, nil)

	resp, err := svc.AutoFix(context.Background(), uuid.New(), nil)
	if err != nil {
		t.Fatalf("AutoFix failed: %v", err)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(resp.Items))
	}
	// 找到 auto_fix_available=true 的项
	var foundManual, foundNotApplicable bool
	for _, item := range resp.Items {
		if item.CheckCode == "tls_cert_expiry" {
			if item.FixStatus == "manual_required" {
				foundManual = true
			}
		}
		if item.CheckCode == "dns_resolve" {
			if item.FixStatus == "not_applicable" {
				foundNotApplicable = true
			}
		}
	}
	if !foundManual {
		t.Errorf("expected tls_cert_expiry to have fix_status=manual_required")
	}
	if !foundNotApplicable {
		t.Errorf("expected dns_resolve to have fix_status=not_applicable")
	}
}
