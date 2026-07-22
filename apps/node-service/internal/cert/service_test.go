package cert

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// fakeCertStore 是 CertificateStore 的内存实现，用于测试
type fakeCertStore struct {
	byCode   map[string]*Certificate
	byID     map[uuid.UUID]*Certificate
	createErr error
}

func newFakeCertStore() *fakeCertStore {
	return &fakeCertStore{
		byCode: make(map[string]*Certificate),
		byID:   make(map[uuid.UUID]*Certificate),
	}
}

func (f *fakeCertStore) Create(ctx context.Context, c *Certificate) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.byCode[c.Code] = c
	f.byID[c.ID] = c
	return nil
}
func (f *fakeCertStore) GetByID(ctx context.Context, id uuid.UUID) (*Certificate, error) {
	if c, ok := f.byID[id]; ok {
		return c, nil
	}
	return nil, nil
}
func (f *fakeCertStore) GetByCode(ctx context.Context, code string) (*Certificate, error) {
	if c, ok := f.byCode[code]; ok {
		return c, nil
	}
	return nil, nil
}
func (f *fakeCertStore) Update(ctx context.Context, c *Certificate) error {
	f.byCode[c.Code] = c
	f.byID[c.ID] = c
	return nil
}
func (f *fakeCertStore) SoftDelete(ctx context.Context, id uuid.UUID) error {
	if c, ok := f.byID[id]; ok {
		c.Status = "deleted"
	}
	return nil
}
func (f *fakeCertStore) SetRenewPending(ctx context.Context, id uuid.UUID) error {
	if c, ok := f.byID[id]; ok {
		c.RenewStatus = "pending"
	}
	return nil
}
func (f *fakeCertStore) UpdateRenewal(ctx context.Context, c *Certificate) error {
	f.byCode[c.Code] = c
	f.byID[c.ID] = c
	return nil
}
func (f *fakeCertStore) SetRenewFailed(ctx context.Context, id uuid.UUID, errMsg string) error {
	if c, ok := f.byID[id]; ok {
		c.RenewStatus = "failed"
		c.RenewLastError = &errMsg
	}
	return nil
}
func (f *fakeCertStore) List(ctx context.Context, page, pageSize int, status string, days int) ([]*Certificate, int, error) {
	var out []*Certificate
	for _, c := range f.byID {
		out = append(out, c)
	}
	return out, len(out), nil
}
func (f *fakeCertStore) ListExpiringSoon(ctx context.Context, days int) ([]*Certificate, error) {
	return nil, nil
}

// fakeProfileStore 是 ProfileStore 的内存实现
type fakeProfileStore struct {
	byCode    map[string]*TLSProfile
	byID      map[uuid.UUID]*TLSProfile
	usageCount int
}

func newFakeProfileStore() *fakeProfileStore {
	return &fakeProfileStore{
		byCode: make(map[string]*TLSProfile),
		byID:   make(map[uuid.UUID]*TLSProfile),
	}
}

func (f *fakeProfileStore) Create(ctx context.Context, p *TLSProfile) error {
	f.byCode[p.Code] = p
	f.byID[p.ID] = p
	return nil
}
func (f *fakeProfileStore) GetByID(ctx context.Context, id uuid.UUID) (*TLSProfile, error) {
	if p, ok := f.byID[id]; ok {
		return p, nil
	}
	return nil, nil
}
func (f *fakeProfileStore) GetByCode(ctx context.Context, code string) (*TLSProfile, error) {
	if p, ok := f.byCode[code]; ok {
		return p, nil
	}
	return nil, nil
}
func (f *fakeProfileStore) Update(ctx context.Context, p *TLSProfile) error {
	f.byCode[p.Code] = p
	f.byID[p.ID] = p
	return nil
}
func (f *fakeProfileStore) Delete(ctx context.Context, id uuid.UUID) error {
	if p, ok := f.byID[id]; ok {
		delete(f.byID, id)
		delete(f.byCode, p.Code)
	}
	return nil
}
func (f *fakeProfileStore) CountUsageInExposures(ctx context.Context, id uuid.UUID) (int, error) {
	return f.usageCount, nil
}
func (f *fakeProfileStore) List(ctx context.Context, page, pageSize int) ([]*TLSProfile, int, error) {
	var out []*TLSProfile
	for _, p := range f.byID {
		out = append(out, p)
	}
	return out, len(out), nil
}

// fakeDeployStore
type fakeDeployStore struct {
	records map[uuid.UUID][]*CertDeployRecord
}

func newFakeDeployStore() *fakeDeployStore {
	return &fakeDeployStore{records: make(map[uuid.UUID][]*CertDeployRecord)}
}

func (f *fakeDeployStore) ListByCertificateID(ctx context.Context, certID uuid.UUID) ([]*CertDeployRecord, error) {
	return f.records[certID], nil
}
func (f *fakeDeployStore) Upsert(ctx context.Context, rec *CertDeployRecord) error {
	f.records[rec.CertificateID] = append(f.records[rec.CertificateID], rec)
	return nil
}

// recordingAudit 记录 audit 调用以便断言
type recordingAudit struct {
	calls []auditCall
}
type auditCall struct {
	action, resource string
}

func (a *recordingAudit) Audit(ctx context.Context, action, resource string, before, after interface{}) {
	a.calls = append(a.calls, auditCall{action: action, resource: resource})
}

func TestCreateCertificate_Happy(t *testing.T) {
	store := newFakeCertStore()
	svc := NewCertificateService(store, newFakeProfileStore(), newFakeDeployStore(), nil, nil)

	req := &CreateCertificateRequest{
		Code:       "cert-001",
		Name:       "example.com",
		CertType:   "domain",
		CommonName: "example.com",
		Provider:   "custom",
	}
	cert, err := svc.CreateCertificate(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateCertificate failed: %v", err)
	}
	if cert.Code != "cert-001" {
		t.Errorf("Code = %q, want cert-001", cert.Code)
	}
	if cert.Status != "active" {
		t.Errorf("Status = %q, want active", cert.Status)
	}
	if cert.Provider != "custom" {
		t.Errorf("Provider = %q, want custom", cert.Provider)
	}
	if cert.RenewStatus != "unknown" {
		t.Errorf("RenewStatus = %q, want unknown", cert.RenewStatus)
	}
	if cert.RenewDaysBefore != 21 {
		t.Errorf("RenewDaysBefore = %d, want 21", cert.RenewDaysBefore)
	}
	if cert.DeployMode != "agent_push" {
		t.Errorf("DeployMode = %q, want agent_push", cert.DeployMode)
	}
}

func TestCreateCertificate_DuplicateCode_ReturnsConflict(t *testing.T) {
	store := newFakeCertStore()
	// 预置一条已存在的证书
	existing := &Certificate{ID: uuid.New(), Code: "dup-code", Name: "old", Status: "active"}
	store.byCode["dup-code"] = existing
	store.byID[existing.ID] = existing

	svc := NewCertificateService(store, newFakeProfileStore(), newFakeDeployStore(), nil, nil)
	req := &CreateCertificateRequest{
		Code:       "dup-code",
		Name:       "new",
		CertType:   "domain",
		CommonName: "example.com",
	}
	_, err := svc.CreateCertificate(context.Background(), req)
	if !errors.Is(err, ErrCertAlreadyExists) {
		t.Fatalf("expected ErrCertAlreadyExists, got %v", err)
	}
}

func TestCreateProfile_ECH_WritesAudit(t *testing.T) {
	pStore := newFakeProfileStore()
	audit := &recordingAudit{}
	svc := NewCertificateService(newFakeCertStore(), pStore, newFakeDeployStore(), audit, nil)

	ech := true
	req := &CreateTLSProfileRequest{
		Code:            "profile-ech",
		Name:            "ech profile",
		UTLSFingerprint: "chrome",
		ECHEnabled:      &ech,
	}
	profile, err := svc.CreateProfile(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateProfile failed: %v", err)
	}
	if !profile.ECHEnabled {
		t.Errorf("ECHEnabled = false, want true")
	}
	if len(audit.calls) != 1 {
		t.Fatalf("expected 1 audit call, got %d", len(audit.calls))
	}
	if audit.calls[0].action != "enable_ech" {
		t.Errorf("audit action = %q, want enable_ech", audit.calls[0].action)
	}
}

func TestDeleteProfile_InUse_ReturnsProfileInUse(t *testing.T) {
	pStore := newFakeProfileStore()
	pStore.usageCount = 2
	existing := &TLSProfile{ID: uuid.New(), Code: "p-inuse", Name: "n"}
	pStore.byID[existing.ID] = existing
	pStore.byCode["p-inuse"] = existing

	svc := NewCertificateService(newFakeCertStore(), pStore, newFakeDeployStore(), nil, nil)
	err := svc.DeleteProfile(context.Background(), existing.ID)
	if !errors.Is(err, ErrProfileInUse) {
		t.Fatalf("expected ErrProfileInUse, got %v", err)
	}
}
