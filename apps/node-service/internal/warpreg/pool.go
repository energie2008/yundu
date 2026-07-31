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

// RegisterOptions 控制注册行为的可选参数（对齐 3x-ui 一键注册体验）。
//   - LicenseKey: WARP+ License（团队零信任 Key 或个人 WARP+ Key），非空时注册后自动调用
//     Cloudflare API 绑定 License，享受 WARP+ 带宽加成。一个 License 可应用到多个 device_id。
//   - Endpoint: 优选 Endpoint（如 162.159.193.1:2408），非空时覆盖 Cloudflare 返回的默认
//     engage.cloudflareclient.com:2408，用于接入优选 IP 提升延迟和带宽。
//   - 两字段均可独立使用：仅填 LicenseKey → 注册并升级 WARP+；仅填 Endpoint → 注册并接入优选 IP。
type RegisterOptions struct {
	LicenseKey string // 可选：WARP+ License Key
	Endpoint   string // 可选：优选 Endpoint（host:port）
}

// RegisterForNode 为指定 VPS 注册一个新 WARP 账户并绑定。
// opts 可为 nil（使用默认行为：不应用 License，使用 Cloudflare 返回的 Endpoint）。
// 注册流程（对齐 3x-ui）：
//  1. 生成 curve25519 密钥对
//  2. 调用 Cloudflare API 注册账户（返回 device_id/access_token/private_key/address/client_id）
//  3. 若 opts.LicenseKey 非空 → 调用 SetLicense 应用 WARP+ License
//  4. 若 opts.Endpoint 非空 → 覆盖 Endpoint（优选 IP）
//  5. 写入 warp_profiles 表（自动填满 Private Key/Address/ClientID 等字段）
func (p *Pool) RegisterForNode(ctx context.Context, nodeID uuid.UUID, nodeCode string, opts *RegisterOptions) (*WarpProfileOutput, error) {
	// 1. 注册新账户
	result, err := p.registrar.Register(ctx)
	if err != nil {
		return nil, err
	}

	// 2. 若提供 WARP+ License Key，注册后自动应用（对齐 3x-ui "填 License 后一键注册"）
	if opts != nil && opts.LicenseKey != "" {
		if err := p.registrar.client.SetLicense(ctx, result.DeviceID, result.AccessToken, opts.LicenseKey); err != nil {
			p.logger.Warn("apply warp+ license failed (non-blocking, account still usable as free WARP)",
				"device_id", result.DeviceID, "license", opts.LicenseKey, "error", err)
			// License 应用失败不阻断注册流程，账户仍可用为免费 WARP
		} else {
			result.LicenseKey = opts.LicenseKey
			p.logger.Info("warp+ license applied", "device_id", result.DeviceID, "license", opts.LicenseKey)
		}
	}

	// 3. 若提供优选 Endpoint，覆盖 Cloudflare 返回的默认 Endpoint
	//    用于接入优选 IP（如 162.159.193.1:2408）提升延迟和带宽
	if opts != nil && opts.Endpoint != "" {
		result.Endpoint = opts.Endpoint
		p.logger.Info("warp endpoint overridden by preferred IP", "device_id", result.DeviceID, "endpoint", opts.Endpoint)
	}

	// 4. 计算当前节点的 WARP 账户数，用于生成 outbound_tag
	existing, _ := p.store.ListByNode(ctx, nodeID)
	tagIndex := len(existing) + 1
	outboundTag := fmt.Sprintf("warp-%d", tagIndex)
	code := fmt.Sprintf("warp-%s-%d", nodeCode, tagIndex)
	name := fmt.Sprintf("WARP %s #%d", nodeCode, tagIndex)

	// 5. 写入 warp_profiles 表（自动填满 Private Key/Address/ClientID/DeviceID/AccessToken 等字段）
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

	// 6. 重新查询返回完整记录
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

// ApplyLicense 为 WARP 账户应用 WARP+ License
// 调用 Cloudflare API 绑定 License 到设备，成功后更新 warp_profiles.license_key
func (p *Pool) ApplyLicense(ctx context.Context, deviceID, accessToken, license string) error {
	if err := p.registrar.client.SetLicense(ctx, deviceID, accessToken, license); err != nil {
		return fmt.Errorf("set license: %w", err)
	}
	p.logger.Info("warp license applied", "device_id", deviceID, "license", license)
	return nil
}
