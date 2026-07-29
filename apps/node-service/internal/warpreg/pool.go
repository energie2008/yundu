package warpreg

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

// WarpProfileStore 抽象 warp_profiles 数据访问
type WarpProfileStore interface {
	Create(ctx context.Context, w *WarpProfileInput) error
	GetByID(ctx context.Context, id uuid.UUID) (*WarpProfileOutput, error)
	ListByNode(ctx context.Context, nodeID uuid.UUID) ([]*WarpProfileOutput, error)
	Update(ctx context.Context, w *WarpProfileInput) error
}

// WarpProfileInput 用于创建/更新 warp_profiles 记录
type WarpProfileInput struct {
	ID               *uuid.UUID
	Code             string
	Name             string
	WarpMode         string
	Endpoint         string
	LicenseKey       string
	PrivateKey       string
	PublicKey        string
	LocalAddress     string
	MTU              int
	DeviceID         string
	AccessToken      string
	ClientID         string
	IPv4Address      string
	IPv6Address      string
	Status           string
	NodeID           *uuid.UUID
	OutboundTag      string
	LastRotatedAt    *time.Time
}

// WarpProfileOutput 是查询返回的 warp_profiles 记录
type WarpProfileOutput struct {
	ID            uuid.UUID
	Code          string
	Name          string
	WarpMode      string
	Endpoint      *string
	LicenseKey    *string
	PrivateKey    *string
	PublicKey     *string
	LocalAddress  *string
	MTU           int
	DeviceID      *string
	AccessToken   *string
	ClientID      *string
	IPv4Address   *string
	IPv6Address   *string
	Status        string
	NodeID        *uuid.UUID
	OutboundTag   *string
	LastRotatedAt *time.Time
}

// Pool 管理 WARP 账户池
type Pool struct {
	registrar *Registrar
	store     WarpProfileStore
	logger    *slog.Logger
}

func NewPool(registrar *Registrar, store WarpProfileStore, logger *slog.Logger) *Pool {
	return &Pool{
		registrar: registrar,
		store:     store,
		logger:    logger,
	}
}

// RegisterForNode 为指定 VPS 注册一个新 WARP 账户并绑定
func (p *Pool) RegisterForNode(ctx context.Context, nodeID uuid.UUID, nodeCode string) (*WarpProfileOutput, error) {
	// 1. 注册新账户
	result, err := p.registrar.Register(ctx)
	if err != nil {
		return nil, err
	}

	// 2. 计算当前节点的 WARP 账户数，用于生成 outbound_tag
	existing, _ := p.store.ListByNode(ctx, nodeID)
	tagIndex := len(existing) + 1
	outboundTag := fmt.Sprintf("warp-%d", tagIndex)
	code := fmt.Sprintf("warp-%s-%d", nodeCode, tagIndex)
	name := fmt.Sprintf("WARP %s #%d", nodeCode, tagIndex)

	// 3. 写入 warp_profiles 表
	localAddr := result.IPv4Address
	if result.IPv6Address != "" {
		localAddr = fmt.Sprintf("%s, %s", result.IPv4Address, result.IPv6Address)
	}

	input := &WarpProfileInput{
		Code:         code,
		Name:         name,
		WarpMode:     "wireguard",
		Endpoint:     result.Endpoint,
		LicenseKey:   result.LicenseKey,
		PrivateKey:   result.PrivateKey,
		PublicKey:    result.PublicKey,
		LocalAddress: localAddr,
		MTU:          1280,
		DeviceID:     result.DeviceID,
		AccessToken:  result.AccessToken,
		ClientID:     result.ClientID,
		IPv4Address:  result.IPv4Address,
		IPv6Address:  result.IPv6Address,
		Status:       "active",
		NodeID:       &nodeID,
		OutboundTag:  outboundTag,
	}

	if err := p.store.Create(ctx, input); err != nil {
		return nil, fmt.Errorf("create warp profile: %w", err)
	}

	// 4. 重新查询返回完整记录
	profiles, _ := p.store.ListByNode(ctx, nodeID)
	for _, prof := range profiles {
		if prof.OutboundTag != nil && *prof.OutboundTag == outboundTag {
			return prof, nil
		}
	}
	return nil, fmt.Errorf("profile created but not found")
}

// ImportExisting 导入已有 WARP 账户（如 VPS81 现有账户）
func (p *Pool) ImportExisting(ctx context.Context, nodeID uuid.UUID, nodeCode, privateKey, localAddress string) (*WarpProfileOutput, error) {
	publicKey := "bmXOC+F1FxEMF9dyiK2H5/1SUtzH0JuVo51h2wPfgyo="
	endpoint := "engage.cloudflareclient.com:2408"

	existing, _ := p.store.ListByNode(ctx, nodeID)
	tagIndex := len(existing) + 1
	outboundTag := fmt.Sprintf("warp-%d", tagIndex)
	code := fmt.Sprintf("warp-%s-%d", nodeCode, tagIndex)
	name := fmt.Sprintf("WARP %s #%d (imported)", nodeCode, tagIndex)

	// 解析 local_address 获取 v4/v6
	v4 := localAddress
	v6 := ""
	if idx := indexOf(localAddress, ','); idx >= 0 {
		v4 = trim(localAddress[:idx])
		v6 = trim(localAddress[idx+1:])
	}

	input := &WarpProfileInput{
		Code:         code,
		Name:         name,
		WarpMode:     "wireguard",
		Endpoint:     endpoint,
		PrivateKey:   privateKey,
		PublicKey:    publicKey,
		LocalAddress: localAddress,
		MTU:          1280,
		IPv4Address:  v4,
		IPv6Address:  v6,
		Status:       "active",
		NodeID:       &nodeID,
		OutboundTag:  outboundTag,
	}

	if err := p.store.Create(ctx, input); err != nil {
		return nil, err
	}
	profiles, _ := p.store.ListByNode(ctx, nodeID)
	for _, prof := range profiles {
		if prof.OutboundTag != nil && *prof.OutboundTag == outboundTag {
			return prof, nil
		}
	}
	return nil, fmt.Errorf("profile imported but not found")
}

func indexOf(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

func trim(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}
