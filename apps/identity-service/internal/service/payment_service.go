package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/big"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/airport-panel/config/events"
	"github.com/airport-panel/identity-service/internal/model"
	"github.com/airport-panel/identity-service/internal/pkg"
	"github.com/airport-panel/identity-service/internal/repo"
	"github.com/google/uuid"
)

type TRC20Config struct {
	Enabled          bool    `json:"enabled"`
	Address          string  `json:"address"`
	USDTContract     string  `json:"usdt_contract"`
	TronGridAPI      string  `json:"trongrid_api"`
	TronGridAPIKey   string  `json:"trongrid_api_key"`
	MinConfirmations int     `json:"min_confirmations"`
	OrderExpiryHours int     `json:"order_expiry_hours"`
	AmountTolerance  float64 `json:"amount_tolerance"`
	AutoActivate     bool    `json:"auto_activate"`
	PollInterval     int     `json:"poll_interval_seconds"`
}

type ERC20Config struct {
	Enabled          bool     `json:"enabled"`
	Address          string   `json:"address"`
	USDTContract     string   `json:"usdt_contract"`
	EtherscanAPI     string   `json:"etherscan_api"`
	EtherscanAPIKey  string   `json:"etherscan_api_key"`
	ChainID          int      `json:"chain_id"`
	MinConfirmations int      `json:"min_confirmations"`
	OrderExpiryHours int      `json:"order_expiry_hours"`
	AmountTolerance  float64  `json:"amount_tolerance"`
	AutoActivate     bool     `json:"auto_activate"`
	PollInterval     int      `json:"poll_interval_seconds"`
	Network          string   `json:"network"`
	Networks         []string `json:"networks"`
}

type BEP20Config struct {
	Enabled          bool     `json:"enabled"`
	Address          string   `json:"address"`
	USDTContract     string   `json:"usdt_contract"`
	BscRPC           []string `json:"bsc_rpc"`
	MinConfirmations int      `json:"min_confirmations"`
	OrderExpiryHours int      `json:"order_expiry_hours"`
	AmountTolerance  float64  `json:"amount_tolerance"`
	AutoActivate     bool     `json:"auto_activate"`
	PollInterval     int      `json:"poll_interval_seconds"`
}

type evmNetworkMeta struct {
	Key          string
	Label        string
	ChainID      int
	USDTContract string
	Decimals     int
}

// evmNetworks 支持的低手续费 EVM 网络（Ethereum 主网手续费过高，已下线）
var evmNetworks = map[string]evmNetworkMeta{
	"polygon": {
		Key:          "polygon",
		Label:        "Polygon",
		ChainID:      137,
		USDTContract: "0xc2132D05D31c914a87C6611C10748AEb04B58e8F",
		Decimals:     6,
	},
	"arbitrum": {
		Key:          "arbitrum",
		Label:        "Arbitrum One",
		ChainID:      42161,
		USDTContract: "0xFd086bC7CD5C481DCC9C85ebE478A1C0b69FCbb9",
		Decimals:     6,
	},
	"bsc": {
		Key:          "bsc",
		Label:        "BEP20",
		ChainID:      56,
		USDTContract: "0x55d398326f99059fF775485246999027B3197955",
		Decimals:     18,
	},
}

// EnabledNetworks 返回当前启用的 EVM 网络列表，兼容旧的单 network 配置。
func (c ERC20Config) EnabledNetworks() []string {
	if len(c.Networks) > 0 {
		var out []string
		for _, n := range c.Networks {
			n = strings.ToLower(strings.TrimSpace(n))
			if _, ok := evmNetworks[n]; ok {
				out = append(out, n)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	n := strings.ToLower(strings.TrimSpace(c.Network))
	if _, ok := evmNetworks[n]; ok {
		return []string{n}
	}
	// Ethereum 已下线，未显式配置时默认 Polygon
	return []string{"polygon"}
}

func evmNetworkKeyFromPayCurrency(payCurrency string) string {
	label := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(payCurrency, "USDT-")))
	if strings.Contains(label, "bep") || strings.Contains(label, "bsc") {
		return "bsc"
	}
	for key, meta := range evmNetworks {
		if strings.ToLower(meta.Label) == label {
			return key
		}
	}
	if strings.Contains(label, "polygon") {
		return "polygon"
	}
	return "polygon"
}

// WechatConfig 微信支付配置（框架预留，暂不对接真实接口）
type WechatConfig struct {
	Enabled          bool `json:"enabled"`
	OrderExpiryHours int  `json:"order_expiry_hours"`
	AutoActivate     bool `json:"auto_activate"`
	// 以下字段为后续对接真实接口预留，当前框架模式不使用
	MchID     string     `json:"mch_id,omitempty"`
	APIKey    string     `json:"api_key,omitempty"`
	AppID     string     `json:"app_id,omitempty"`
	NotifyURL string     `json:"notify_url,omitempty"`
	Epay      EpayConfig `json:"epay,omitempty"`
}

// AlipayConfig 支付宝支付配置（框架预留，暂不对接真实接口）
type AlipayConfig struct {
	Enabled          bool `json:"enabled"`
	OrderExpiryHours int  `json:"order_expiry_hours"`
	AutoActivate     bool `json:"auto_activate"`
	// 以下字段为后续对接真实接口预留，当前框架模式不使用
	AppID      string     `json:"app_id,omitempty"`
	PrivateKey string     `json:"private_key,omitempty"`
	NotifyURL  string     `json:"notify_url,omitempty"`
	Epay       EpayConfig `json:"epay,omitempty"`
}

type PaymentService struct {
	paymentOrderRepo  *repo.PaymentOrderRepo
	planRepo          *repo.PlanRepo
	userRepo          *repo.UserRepo
	subRepo           *repo.SubscriptionRepo
	subTokenRepo      *repo.SubscriptionTokenRepo
	settingRepo       *repo.SettingRepo
	couponRepo        *repo.CouponRepo
	commissionLogRepo *repo.CommissionLogRepo
	mailSvc           *MailService
	auditSvc          *AuditService
	notifySvc         *NotificationService
	commissionSvc     *CommissionService
	log               *slog.Logger
	httpClient        *http.Client
	stopPoll          chan struct{}
	pollWg            sync.WaitGroup
	trc20Cfg          TRC20Config
	erc20Cfg          ERC20Config
	bep20Cfg          BEP20Config
	wechatCfg         WechatConfig
	alipayCfg         AlipayConfig
	cfgMu             sync.RWMutex
	lastCommissionRun time.Time
	exchangeRate      float64
	onEvent           func(ctx context.Context, topic string, payload interface{})
}

func (s *PaymentService) SetEventPublisher(fn func(ctx context.Context, topic string, payload interface{})) {
	if fn != nil {
		s.onEvent = fn
	}
}

func NewPaymentService(
	paymentOrderRepo *repo.PaymentOrderRepo,
	planRepo *repo.PlanRepo,
	userRepo *repo.UserRepo,
	subRepo *repo.SubscriptionRepo,
	subTokenRepo *repo.SubscriptionTokenRepo,
	settingRepo *repo.SettingRepo,
	couponRepo *repo.CouponRepo,
	commissionLogRepo *repo.CommissionLogRepo,
	mailSvc *MailService,
	auditSvc *AuditService,
	notifySvc *NotificationService,
	commissionSvc *CommissionService,
	log *slog.Logger,
) *PaymentService {
	svc := &PaymentService{
		paymentOrderRepo:  paymentOrderRepo,
		planRepo:          planRepo,
		userRepo:          userRepo,
		subRepo:           subRepo,
		subTokenRepo:      subTokenRepo,
		settingRepo:       settingRepo,
		couponRepo:        couponRepo,
		commissionLogRepo: commissionLogRepo,
		mailSvc:           mailSvc,
		auditSvc:          auditSvc,
		notifySvc:         notifySvc,
		commissionSvc:     commissionSvc,
		log:               log,
		httpClient:        &http.Client{Timeout: 15 * time.Second},
		stopPoll:          make(chan struct{}),
		onEvent:           func(ctx context.Context, topic string, payload interface{}) {},
	}
	svc.trc20Cfg = svc.loadTRC20Config()
	svc.erc20Cfg = svc.loadERC20Config()
	svc.bep20Cfg = svc.loadBEP20Config()
	svc.wechatCfg = svc.loadWechatConfig()
	svc.alipayCfg = svc.loadAlipayConfig()
	svc.exchangeRate = svc.loadExchangeRate()
	return svc
}

func (s *PaymentService) StartPolling(ctx context.Context) {
	s.pollWg.Add(4)
	go s.pollPaymentsLoop()
	go s.orderExpiryLoop()
	go s.commissionSettleLoop()
	go s.overQuotaCheckLoop()
	s.log.Info("Payment service scheduled jobs started (payment polling, order expiry, commission settle, over-quota check)")
}

func (s *PaymentService) orderExpiryLoop() {
	defer s.pollWg.Done()
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	s.markExpiredOrders()
	for {
		select {
		case <-s.stopPoll:
			return
		case <-ticker.C:
			s.markExpiredOrders()
		}
	}
}

func (s *PaymentService) markExpiredOrders() {
	defer func() {
		if r := recover(); r != nil {
			s.log.Error("markExpiredOrders panic", "error", r)
		}
	}()
	ctx := context.Background()
	count, err := s.paymentOrderRepo.MarkExpired(ctx, time.Now())
	if err != nil {
		s.log.Error("scheduled: mark expired orders failed", "error", err)
		return
	}
	if count > 0 {
		s.log.Info("scheduled: orders marked as expired", "count", count)
	}
}

func (s *PaymentService) commissionSettleLoop() {
	defer s.pollWg.Done()
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	s.processDailyCommissionSettle()
	for {
		select {
		case <-s.stopPoll:
			return
		case <-ticker.C:
			s.processDailyCommissionSettle()
		}
	}
}

func (s *PaymentService) processDailyCommissionSettle() {
	defer func() {
		if r := recover(); r != nil {
			s.log.Error("processDailyCommissionSettle panic", "error", r)
		}
	}()
	ctx := context.Background()
	// 统一委托给 CommissionService.CheckPendingCommissions，避免 PaymentService 直接操作 repo。
	// 该方法为幂等操作，每小时执行一次（由 commissionSettleLoop 调度）。
	if s.commissionSvc != nil {
		if err := s.commissionSvc.CheckPendingCommissions(ctx); err != nil {
			s.log.Error("scheduled: commission settle failed", "error", err)
			return
		}
		s.log.Info("scheduled: commission settlement completed (delegated to CommissionService)")
	} else {
		// 退化兼容：未注入 CommissionService 时回退到本地实现
		if err := s.ProcessSettledCommissions(ctx); err != nil {
			s.log.Error("scheduled: commission settle (legacy) failed", "error", err)
		}
	}
}

func (s *PaymentService) overQuotaCheckLoop() {
	defer s.pollWg.Done()
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopPoll:
			return
		case <-ticker.C:
		}
	}
}

func (s *PaymentService) loadTRC20Config() TRC20Config {
	cfg := TRC20Config{
		Enabled:          false,
		USDTContract:     "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		TronGridAPI:      "https://api.trongrid.io",
		MinConfirmations: 6,
		OrderExpiryHours: 2,
		AmountTolerance:  0.01,
		AutoActivate:     true,
		PollInterval:     60,
	}
	data, err := s.settingRepo.GetJSON(context.Background(), "payment", "trc20")
	if err != nil {
		s.log.Warn("loadTRC20Config fallback to defaults", "error", err)
		return cfg
	}
	_ = json.Unmarshal(data, &cfg)
	cfg.Address = strings.TrimSpace(cfg.Address)
	return cfg
}

func (s *PaymentService) loadERC20Config() ERC20Config {
	cfg := ERC20Config{
		Enabled:          false,
		USDTContract:     "0xdAC17F958D2ee523a2206206994597C13D831ec7",
		EtherscanAPI:     "https://api.etherscan.io/v2/api",
		ChainID:          1,
		MinConfirmations: 3,
		OrderExpiryHours: 6,
		AmountTolerance:  0.01,
		AutoActivate:     true,
		PollInterval:     60,
		Network:          "Ethereum(ERC20)",
	}
	data, err := s.settingRepo.GetJSON(context.Background(), "payment", "erc20")
	if err != nil {
		s.log.Warn("loadERC20Config fallback to defaults", "error", err)
		return cfg
	}
	_ = json.Unmarshal(data, &cfg)
	cfg.Address = strings.TrimSpace(cfg.Address)
	if cfg.EtherscanAPI == "" || strings.Contains(cfg.EtherscanAPI, "api.polygonscan.com") {
		cfg.EtherscanAPI = "https://api.etherscan.io/v2/api"
	}
	return cfg
}

func (s *PaymentService) loadBEP20Config() BEP20Config {
	cfg := BEP20Config{
		Enabled:          false,
		USDTContract:     "0x55d398326f99059fF775485246999027B3197955",
		BscRPC:           []string{"https://bsc-mainnet.nodereal.io/v1/64a9df0874fb4a93b9d0a3849de012d3", "https://bsc-dataseed.binance.org", "https://bsc-dataseed1.bnbchain.org"},
		MinConfirmations: 3,
		OrderExpiryHours: 6,
		AmountTolerance:  0.01,
		AutoActivate:     true,
		PollInterval:     60,
	}
	data, err := s.settingRepo.GetJSON(context.Background(), "payment", "bep20")
	if err != nil {
		s.log.Warn("loadBEP20Config fallback to defaults", "error", err)
		return cfg
	}
	_ = json.Unmarshal(data, &cfg)
	cfg.Address = strings.TrimSpace(cfg.Address)
	if len(cfg.BscRPC) == 0 {
		cfg.BscRPC = []string{"https://bsc-mainnet.nodereal.io/v1/64a9df0874fb4a93b9d0a3849de012d3", "https://bsc-dataseed.binance.org", "https://bsc-dataseed1.bnbchain.org"}
	}
	return cfg
}

func (s *PaymentService) loadWechatConfig() WechatConfig {
	cfg := WechatConfig{
		Enabled:          false,
		OrderExpiryHours: 2,
		AutoActivate:     true,
	}
	data, err := s.settingRepo.GetJSON(context.Background(), "payment", "wechat")
	if err != nil {
		return cfg
	}
	_ = json.Unmarshal(data, &cfg)
	return cfg
}

func (s *PaymentService) loadAlipayConfig() AlipayConfig {
	cfg := AlipayConfig{
		Enabled:          false,
		OrderExpiryHours: 2,
		AutoActivate:     true,
	}
	data, err := s.settingRepo.GetJSON(context.Background(), "payment", "alipay")
	if err != nil {
		return cfg
	}
	_ = json.Unmarshal(data, &cfg)
	return cfg
}

func (s *PaymentService) GetTRC20Config() TRC20Config {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	return s.trc20Cfg
}

func (s *PaymentService) GetERC20Config() ERC20Config {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	return s.erc20Cfg
}

func (s *PaymentService) GetBEP20Config() BEP20Config {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	return s.bep20Cfg
}

func (s *PaymentService) GetWechatConfig() WechatConfig {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	return s.wechatCfg
}

func (s *PaymentService) GetAlipayConfig() AlipayConfig {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	return s.alipayCfg
}

func (s *PaymentService) ReloadConfigs() {
	s.cfgMu.Lock()
	s.trc20Cfg = s.loadTRC20Config()
	s.erc20Cfg = s.loadERC20Config()
	s.bep20Cfg = s.loadBEP20Config()
	s.wechatCfg = s.loadWechatConfig()
	s.alipayCfg = s.loadAlipayConfig()
	s.exchangeRate = s.loadExchangeRate()
	s.cfgMu.Unlock()
}

// loadExchangeRate 从 system_settings 读取 USDT 到 CNY 汇率，默认 7.2
func (s *PaymentService) loadExchangeRate() float64 {
	const defaultRate = 7.2
	data, err := s.settingRepo.GetJSON(context.Background(), "payment", "exchange_rate")
	if err != nil {
		s.log.Warn("loadExchangeRate fallback to default", "error", err, "rate", defaultRate)
		return defaultRate
	}
	var cfg struct {
		USDTToCNY float64 `json:"usdt_to_cny"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil || cfg.USDTToCNY <= 0 {
		return defaultRate
	}
	return cfg.USDTToCNY
}

// GetExchangeRate 返回当前缓存的 USDT 到 CNY 汇率
func (s *PaymentService) GetExchangeRate() float64 {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	if s.exchangeRate <= 0 {
		return 7.2
	}
	return s.exchangeRate
}

func (s *PaymentService) CreateOrder(ctx context.Context, userID uuid.UUID, req model.CreateOrderRequest) (*model.PaymentOrder, error) {
	plan, err := s.planRepo.GetByID(ctx, req.PlanID)
	if err != nil {
		return nil, fmt.Errorf("fetch plan: %w", err)
	}
	if plan == nil {
		return nil, ErrPlanNotFound
	}
	days, ok := model.PeriodDaysMap[req.PeriodCode]
	if !ok {
		return nil, ErrInvalidPeriodCode
	}
	_ = days

	prices, err := s.planRepo.GetPrices(ctx, plan.ID)
	if err != nil {
		return nil, fmt.Errorf("fetch prices: %w", err)
	}
	entry, ok := prices[req.PeriodCode]
	if !ok {
		return nil, fmt.Errorf("price not set for period")
	}
	basePrice := entry.USDT

	// 读取并锁定汇率（提前到优惠码校验之前，保证 CNY 金额可用于校验）
	rate := s.GetExchangeRate()
	// 优先使用数据库中存储的 CNY 价格（管理员录入），避免汇率换算精度丢失
	var amountCNY float64
	if entry.CNY > 0 {
		amountCNY = entry.CNY
	} else {
		amountCNY = math.Round(basePrice*rate*100) / 100
	}

	// 以 CNY 为基准计算折扣和最终金额
	discountCNY := 0.0
	finalCNY := amountCNY
	couponCode := ""
	if req.CouponCode != "" {
		// 优惠码校验统一使用 CNY 金额（与用户端预校验 coupon_validate_handler 一致）
		coupon, err := s.ValidateAndApplyCoupon(ctx, userID, req.CouponCode, amountCNY, req.PlanID, req.PeriodCode)
		if err != nil {
			return nil, err
		}
		discountCNY = coupon.Discount // CNY
		finalCNY = amountCNY - discountCNY
		if finalCNY < 0 {
			finalCNY = 0
		}
		couponCode = req.CouponCode
	}

	// 支付方式选择
	// CNY 为主结算货币：微信/支付宝按 CNY 结算，USDT 按 CNY/汇率 换算
	paymentMethod := req.PaymentMethod
	if finalCNY > 0 {
		if paymentMethod == "" {
			// 默认选择优先级：支付宝 > 微信 > USDT-TRC20 > USDT-BEP20 > USDT-ERC20
			alipay := s.GetAlipayConfig()
			wechat := s.GetWechatConfig()
			trc := s.GetTRC20Config()
			bep := s.GetBEP20Config()
			erc := s.GetERC20Config()
			if alipay.Enabled {
				paymentMethod = model.PaymentMethodAlipay
			} else if wechat.Enabled {
				paymentMethod = model.PaymentMethodWechat
			} else if trc.Enabled && trc.Address != "" {
				paymentMethod = model.PaymentMethodUSDTTRC20
			} else if bep.Enabled && bep.Address != "" {
				paymentMethod = model.PaymentMethodUSDTBEP20
			} else if erc.Enabled && erc.Address != "" {
				paymentMethod = model.PaymentMethodUSDTERC20
			} else if trc.Enabled {
				paymentMethod = model.PaymentMethodUSDTTRC20
			} else {
				paymentMethod = model.PaymentMethodUSDTBEP20
			}
		}
	} else {
		paymentMethod = model.PaymentMethodZero
	}

	var payAddress, payCurrency string
	var expiryHours int
	var finalAmount float64    // 实付金额（币种由 payCurrency 决定）
	var discountAmount float64 // 折扣金额（币种与 finalAmount 一致）

	if finalCNY > 0 {
		switch paymentMethod {
		case model.PaymentMethodWechat:
			cfg := s.GetWechatConfig()
			if !cfg.Enabled {
				return nil, ErrWechatDisabled
			}
			payAddress = "" // 法币支付不需要链上地址
			payCurrency = "CNY"
			expiryHours = cfg.OrderExpiryHours
			if expiryHours <= 0 {
				expiryHours = 2
			}
			finalAmount = math.Round(finalCNY*100) / 100
			discountAmount = math.Round(discountCNY*100) / 100

		case model.PaymentMethodAlipay:
			cfg := s.GetAlipayConfig()
			if !cfg.Enabled {
				return nil, ErrAlipayDisabled
			}
			payAddress = ""
			payCurrency = "CNY"
			expiryHours = cfg.OrderExpiryHours
			if expiryHours <= 0 {
				expiryHours = 2
			}
			finalAmount = math.Round(finalCNY*100) / 100
			discountAmount = math.Round(discountCNY*100) / 100

		case model.PaymentMethodUSDTTRC20:
			cfg := s.GetTRC20Config()
			if !cfg.Enabled {
				return nil, ErrTRC20Disabled
			}
			payAddress = cfg.Address
			payCurrency = "USDT-TRC20"
			expiryHours = cfg.OrderExpiryHours
			if expiryHours <= 0 {
				expiryHours = 2
			}
			if payAddress == "" {
				return nil, fmt.Errorf("USDT-TRC20 receiving address not configured by admin")
			}
			// CNY → USDT 换算
			finalAmount = math.Round(finalCNY/rate*100) / 100
			discountAmount = math.Round(discountCNY/rate*100) / 100

		case model.PaymentMethodUSDTBEP20:
			cfg := s.GetBEP20Config()
			if !cfg.Enabled {
				return nil, fmt.Errorf("USDT BEP20 payment not enabled")
			}
			payAddress = cfg.Address
			payCurrency = "USDT-BEP20"
			expiryHours = cfg.OrderExpiryHours
			if expiryHours <= 0 {
				expiryHours = 6
			}
			if payAddress == "" {
				return nil, fmt.Errorf("USDT-BEP20 receiving address not configured by admin")
			}
			finalAmount = math.Round(finalCNY/rate*100) / 100
			discountAmount = math.Round(discountCNY/rate*100) / 100

		case model.PaymentMethodUSDTERC20:
			cfg := s.GetERC20Config()
			if !cfg.Enabled {
				return nil, fmt.Errorf("USDT EVM payment not enabled")
			}
			nets := cfg.EnabledNetworks()
			netKey := strings.ToLower(strings.TrimSpace(req.Network))
			if netKey == "" {
				netKey = nets[0]
			}
			meta, ok := evmNetworks[netKey]
			if !ok || !slices.Contains(nets, netKey) {
				return nil, fmt.Errorf("%w: %s", ErrUnsupportedNetwork, netKey)
			}
			payAddress = cfg.Address
			payCurrency = "USDT-" + meta.Label
			expiryHours = cfg.OrderExpiryHours
			if expiryHours <= 0 {
				expiryHours = 6
			}
			if payAddress == "" {
				return nil, fmt.Errorf("USDT EVM receiving address not configured by admin")
			}
			finalAmount = math.Round(finalCNY/rate*100) / 100
			discountAmount = math.Round(discountCNY/rate*100) / 100

		default:
			return nil, fmt.Errorf("unsupported payment method: %s", paymentMethod)
		}
	} else {
		payCurrency = "ZERO"
		expiryHours = 1
		finalAmount = 0
		discountAmount = math.Round(discountCNY*100) / 100
	}

	orderNo := fmt.Sprintf("P%s%d", time.Now().Format("20060102150405"), rand.Intn(9000)+1000)
	order := &model.PaymentOrder{
		ID:             uuid.New(),
		OrderNo:        orderNo,
		UserID:         userID,
		PlanID:         plan.ID,
		PlanName:       plan.Name,
		PeriodCode:     req.PeriodCode,
		AmountUSDT:     math.Round(basePrice*100) / 100,
		AmountCNY:      amountCNY,
		ExchangeRate:   rate,
		DiscountAmount: discountAmount,
		FinalAmount:    finalAmount,
		CouponCode:     couponCode,
		PayAddress:     payAddress,
		PayCurrency:    payCurrency,
		PaymentMethod:  paymentMethod,
		Status:         model.PaymentStatusPending,
		ExpiresAt:      time.Now().Add(time.Duration(expiryHours) * time.Hour),
	}
	if err := s.paymentOrderRepo.Create(ctx, order); err != nil {
		return nil, fmt.Errorf("create order: %w", err)
	}
	if model.IsFiatPayment(order.PaymentMethod) {
		if err := s.createEpayPayment(ctx, order); err != nil {
			// 网关不可用时取消订单，避免产生无支付入口的死单
			_, _ = s.paymentOrderRepo.UpdateStatus(ctx, order.ID, model.PaymentStatusCanceled, nil, nil, nil)
			return nil, err
		}
	} else {
		order.PaymentURI = s.GetPaymentURI(order)
	}
	if req.CouponCode != "" {
		coupon, _ := s.couponRepo.GetByCode(ctx, req.CouponCode)
		if coupon != nil {
			_ = s.couponRepo.IncrementUsed(ctx, coupon.ID)
			usage := model.CouponUsage{
				ID:              uuid.New(),
				CouponID:        coupon.ID,
				UserID:          userID,
				OrderID:         &order.ID,
				DiscountApplied: discountAmount,
			}
			_ = s.couponRepo.CreateUsage(ctx, &usage)
		}
	}
	s.log.Info("Order created", "order_no", orderNo, "user", userID, "amount", finalAmount, "method", paymentMethod)

	// 0元订单（全额优惠券）自动标记 paid 并激活订阅，跳过链上支付流程
	if finalAmount <= 0 {
		now := time.Now()
		zeroPaid := 0.0
		zeroTxHash := "ZERO:" + order.OrderNo
		if updated, err := s.paymentOrderRepo.UpdateStatus(ctx, order.ID, model.PaymentStatusPaid, &zeroTxHash, &zeroPaid, &now); err != nil || !updated {
			s.log.Error("auto-mark 0-yuan order paid failed", "order", orderNo, "error", err)
		} else {
			order.Status = model.PaymentStatusPaid
			order.TxHash = &zeroTxHash
			order.PaidAmount = &zeroPaid
			order.PaidAt = &now
			s.activateOrder(ctx, order, 0)
			s.log.Info("0-yuan order auto-activated", "order", orderNo, "user", userID)
		}
	}
	return order, nil
}

func (s *PaymentService) GetPaymentURI(order *model.PaymentOrder) string {
	switch order.PaymentMethod {
	case model.PaymentMethodUSDTBEP20:
		meta := evmNetworks["bsc"]
		return fmt.Sprintf("%s:%s?value=%.2f&contract=%s", meta.Key, order.PayAddress, order.FinalAmount, meta.USDTContract)
	case model.PaymentMethodUSDTERC20:
		meta, ok := evmNetworks[evmNetworkKeyFromPayCurrency(order.PayCurrency)]
		if !ok {
			meta = evmNetworks["polygon"]
		}
		return fmt.Sprintf("%s:%s?value=%.2f&contract=%s", meta.Key, order.PayAddress, order.FinalAmount, meta.USDTContract)
	case model.PaymentMethodUSDTTRC20:
		cfg := s.GetTRC20Config()
		amount := strconv.FormatFloat(order.FinalAmount*1000000, 'f', 0, 64)
		return fmt.Sprintf("tron:%s?amount=%s&contract=%s", order.PayAddress, amount, cfg.USDTContract)
	case model.PaymentMethodWechat, model.PaymentMethodAlipay:
		if order.PaymentURI != "" {
			return order.PaymentURI
		}
		// 兼容旧订单：未走网关时返回占位 URI
		return fmt.Sprintf("pending:%s?amount=%.2f&currency=CNY&method=%s", order.OrderNo, order.FinalAmount, order.PaymentMethod)
	default:
		return ""
	}
}

// defaultEpayNotifyURL 易支付异步回调地址：未配置时按面板公网地址自动补齐，
// 避免支付成功却无法回调导致订阅不自动激活。
func defaultEpayNotifyURL(cur, method string) string {
	if cur != "" {
		return cur
	}
	base := strings.TrimRight(os.Getenv("AGENT_PANEL_ENDPOINT"), "/")
	if base == "" {
		base = "https://6.tiktokplay.na.am"
	}
	return base + "/api/v1/payment/notify/" + method
}

// defaultEpayReturnURL 易支付同步跳转地址：未配置时回退到用户订单列表。
func defaultEpayReturnURL(cur string) string {
	if cur != "" {
		return cur
	}
	base := strings.TrimRight(os.Getenv("AGENT_PANEL_ENDPOINT"), "/")
	if base == "" {
		base = "https://6.tiktokplay.na.am"
	}
	return base + "/api/v1/user/orders"
}

func (s *PaymentService) epayGatewayFor(method string) (*EpayGateway, error) {
	switch method {
	case model.PaymentMethodWechat:
		cfg := s.GetWechatConfig()
		if !cfg.Epay.Configured() {
			return nil, fmt.Errorf("epay gateway not configured for wechat")
		}
		payType := cfg.Epay.PayType
		if payType == "" {
			payType = "wxpay"
		}
		epay := cfg.Epay
		epay.NotifyURL = defaultEpayNotifyURL(epay.NotifyURL, "wechat")
		epay.ReturnURL = defaultEpayReturnURL(epay.ReturnURL)
		return NewEpayGateway(s.log, s.httpClient, epay, payType), nil
	case model.PaymentMethodAlipay:
		cfg := s.GetAlipayConfig()
		if !cfg.Epay.Configured() {
			return nil, fmt.Errorf("epay gateway not configured for alipay")
		}
		payType := cfg.Epay.PayType
		if payType == "" {
			payType = "alipay"
		}
		epay := cfg.Epay
		epay.NotifyURL = defaultEpayNotifyURL(epay.NotifyURL, "alipay")
		epay.ReturnURL = defaultEpayReturnURL(epay.ReturnURL)
		return NewEpayGateway(s.log, s.httpClient, epay, payType), nil
	default:
		return nil, fmt.Errorf("unsupported epay method: %s", method)
	}
}

func (s *PaymentService) epayAutoActivate(method string) bool {
	switch method {
	case model.PaymentMethodWechat:
		return s.GetWechatConfig().AutoActivate
	case model.PaymentMethodAlipay:
		return s.GetAlipayConfig().AutoActivate
	default:
		return true
	}
}

// ErrEpayGatewayError 易支付网关调用失败（通道未配置/平台无可用收款账号等）
var ErrEpayGatewayError = errors.New("epay gateway error")

func (s *PaymentService) createEpayPayment(ctx context.Context, order *model.PaymentOrder) error {
	gw, err := s.epayGatewayFor(order.PaymentMethod)
	if err != nil {
		return err
	}
	pay, err := gw.CreatePayment(ctx, order)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrEpayGatewayError, err.Error())
	}
	order.Gateway = gw.Name()
	order.GatewayTradeNo = pay.TradeNo
	order.PaymentURI = pay.URL
	order.PayAddress = pay.QRCode
	if err := s.paymentOrderRepo.UpdateGatewayInfo(ctx, order.ID, order.Gateway, pay.TradeNo, pay.URL, pay.QRCode); err != nil {
		return fmt.Errorf("save epay gateway info: %w", err)
	}
	return nil
}

// HandleEpayNotify 处理易支付异步回调：验签、金额校验、幂等激活。
func (s *PaymentService) HandleEpayNotify(ctx context.Context, method string, params map[string]string) (string, error) {
	gw, err := s.epayGatewayFor(method)
	if err != nil {
		return "", err
	}
	notify, err := gw.VerifyNotify(params)
	if err != nil {
		return "", err
	}
	if notify.OutTradeNo == "" {
		return "", errors.New("missing out_trade_no")
	}
	if notify.Status != "" && notify.Status != "TRADE_SUCCESS" && notify.Status != "SUCCESS" {
		return "", fmt.Errorf("trade not success: %s", notify.Status)
	}
	order, err := s.paymentOrderRepo.GetByOrderNo(ctx, notify.OutTradeNo)
	if err != nil {
		return "", err
	}
	if order == nil {
		return "", errors.New("order not found")
	}
	if order.PaymentMethod != method {
		return "", fmt.Errorf("payment method mismatch: %s", order.PaymentMethod)
	}
	if math.Abs(notify.Amount-order.FinalAmount) > 0.01 {
		return "", fmt.Errorf("amount mismatch: notify=%v order=%v", notify.Amount, order.FinalAmount)
	}
	tradeNo := notify.TradeNo
	if tradeNo == "" {
		tradeNo = "EPAY:" + order.OrderNo
	}
	now := time.Now()
	updated, err := s.paymentOrderRepo.MarkPaidIfPending(ctx, order.ID, tradeNo, order.FinalAmount, now)
	if err != nil {
		return "", err
	}
	if !updated {
		return "", errors.New("order not pending")
	}
	_ = s.paymentOrderRepo.UpdateGatewayInfo(ctx, order.ID, gw.Name(), tradeNo, order.PaymentURI, order.PayAddress)
	order.Status = model.PaymentStatusPaid
	order.TxHash = &tradeNo
	order.PaidAmount = &order.FinalAmount
	order.PaidAt = &now
	s.log.Info("epay order paid via notify", "order_no", order.OrderNo, "method", method, "trade_no", tradeNo, "amount", notify.Amount)
	if s.epayAutoActivate(method) {
		s.activateOrder(ctx, order, order.FinalAmount)
	}
	return "success", nil
}

func (s *PaymentService) pollEpayOrder(ctx context.Context, o *model.PaymentOrder) {
	if time.Since(o.CreatedAt) < time.Minute {
		return
	}
	gw, err := s.epayGatewayFor(o.PaymentMethod)
	if err != nil {
		return
	}
	tradeNo, paid, err := gw.QueryOrder(ctx, o)
	if err != nil {
		s.log.Warn("epay query order error", "order_no", o.OrderNo, "method", o.PaymentMethod, "error", err)
		return
	}
	if !paid {
		return
	}
	if tradeNo == "" {
		tradeNo = "EPAY:" + o.OrderNo
	}
	now := time.Now()
	updated, err := s.paymentOrderRepo.MarkPaidIfPending(ctx, o.ID, tradeNo, o.FinalAmount, now)
	if err != nil {
		s.log.Error("mark epay order paid", "error", err)
		return
	}
	if !updated {
		return
	}
	_ = s.paymentOrderRepo.UpdateGatewayInfo(ctx, o.ID, gw.Name(), tradeNo, o.PaymentURI, o.PayAddress)
	o.Status = model.PaymentStatusPaid
	o.TxHash = &tradeNo
	o.PaidAmount = &o.FinalAmount
	o.PaidAt = &now
	s.log.Info("epay order paid via query", "order_no", o.OrderNo, "method", o.PaymentMethod, "trade_no", tradeNo)
	if s.epayAutoActivate(o.PaymentMethod) {
		s.activateOrder(ctx, o, o.FinalAmount)
	}
}

type tronTransfer struct {
	TransactionID string  `json:"transaction_id"`
	From          string  `json:"from"`
	To            string  `json:"to"`
	Value         float64 `json:"value"`
	Timestamp     int64   `json:"timestamp"`
	BlockNumber   int64   `json:"block_number"`
	Confirmed     bool    `json:"confirmed"`
}

type tronTxInfo struct {
	ID              string `json:"id"`
	BlockNumber     int64  `json:"blockNumber"`
	Confirmed       bool   `json:"confirmed"`
	ContractAddress string `json:"contract_address"`
}

type tronBlock struct {
	BlockHeader struct {
		Number int64 `json:"number"`
	} `json:"block_header"`
}

type ethTransaction struct {
	Hash          string `json:"hash"`
	BlockNumber   string `json:"blockNumber"`
	From          string `json:"from"`
	To            string `json:"to"`
	Value         string `json:"value"`
	TimeStamp     string `json:"timeStamp"`
	Confirmations string `json:"confirmations"`
}

func (s *PaymentService) fetchTRC20Transfers(cfg TRC20Config) ([]tronTransfer, error) {
	if cfg.Address == "" {
		return nil, nil
	}
	apiURL := fmt.Sprintf("%s/v1/accounts/%s/transactions/trc20?limit=50&contract_address=%s&only_to=true",
		strings.TrimRight(cfg.TronGridAPI, "/"), cfg.Address, cfg.USDTContract)
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	if cfg.TronGridAPIKey != "" {
		req.Header.Set("TRON-PRO-API-KEY", cfg.TronGridAPIKey)
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result struct {
		Data []struct {
			TransactionID string `json:"transaction_id"`
			TokenInfo     struct {
				Address  string `json:"address"`
				Decimals int    `json:"decimals"`
			} `json:"token_info"`
			From  string `json:"from"`
			To    string `json:"to"`
			Type  string `json:"type"`
			Value string `json:"value"`
		} `json:"data"`
		Meta struct {
			At          int64  `json:"at"`
			Fingerprint string `json:"fingerprint"`
		} `json:"meta"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	var transfers []tronTransfer
	for _, d := range result.Data {
		if d.Type != "Transfer" {
			continue
		}
		decimals := d.TokenInfo.Decimals
		if decimals == 0 {
			decimals = 6
		}
		val, err := strconv.ParseFloat(d.Value, 64)
		if err != nil {
			continue
		}
		amount := val / math.Pow(10, float64(decimals))
		transfers = append(transfers, tronTransfer{
			TransactionID: d.TransactionID,
			From:          d.From,
			To:            d.To,
			Value:         amount,
		})
	}
	latestBlockNum, _ := s.fetchTronLatestBlock(cfg)
	for i := range transfers {
		txInfo, err := s.fetchTronTxInfo(cfg, transfers[i].TransactionID)
		if err == nil {
			transfers[i].BlockNumber = txInfo.BlockNumber
			transfers[i].Confirmed = txInfo.Confirmed
			if latestBlockNum > 0 && txInfo.BlockNumber > 0 {
				confirmations := latestBlockNum - txInfo.BlockNumber
				if confirmations >= int64(cfg.MinConfirmations) {
					transfers[i].Confirmed = true
				}
			}
		}
	}
	return transfers, nil
}

func (s *PaymentService) fetchTronLatestBlock(cfg TRC20Config) (int64, error) {
	apiURL := fmt.Sprintf("%s/wallet/getnowblock", strings.TrimRight(cfg.TronGridAPI, "/"))
	resp, err := s.httpClient.Get(apiURL)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	var block tronBlock
	raw := json.NewDecoder(resp.Body)
	if err := raw.Decode(&block); err != nil {
		return 0, err
	}
	return block.BlockHeader.Number, nil
}

func (s *PaymentService) fetchTronTxInfo(cfg TRC20Config, txid string) (*tronTxInfo, error) {
	apiURL := fmt.Sprintf("%s/wallet/gettransactioninfobyid?value=%s", strings.TrimRight(cfg.TronGridAPI, "/"), txid)
	resp, err := s.httpClient.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var info tronTxInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, err
	}
	return &info, nil
}

func (s *PaymentService) fetchERC20Transfers(cfg ERC20Config, netKey string) ([]ethTransaction, error) {
	meta, ok := evmNetworks[netKey]
	if !ok {
		return nil, fmt.Errorf("unsupported EVM network: %s", netKey)
	}
	// BEP20 使用独立 RPC 查询方式
	if netKey == "bsc" {
		return s.fetchBEP20TransfersViaRPC(meta)
	}
	if cfg.Address == "" {
		return nil, nil
	}
	params := url.Values{}
	params.Set("module", "account")
	params.Set("action", "tokentx")
	params.Set("contractaddress", meta.USDTContract)
	params.Set("address", cfg.Address)
	params.Set("page", "1")
	params.Set("offset", "50")
	params.Set("sort", "desc")
	if cfg.EtherscanAPIKey != "" {
		params.Set("apikey", cfg.EtherscanAPIKey)
	}
	apiBase := strings.TrimRight(cfg.EtherscanAPI, "/")
	if strings.Contains(apiBase, "/v2/") || strings.HasSuffix(apiBase, "/v2/api") {
		// Etherscan V2 通过 chainid 指定链，不再使用 explorer 专属域名
		params.Set("chainid", strconv.Itoa(meta.ChainID))
	}
	apiURL := fmt.Sprintf("%s?%s", apiBase, params.Encode())
	resp, err := s.httpClient.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result struct {
		Status  string          `json:"status"`
		Message string          `json:"message"`
		Result  json.RawMessage `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if len(result.Result) == 0 || string(result.Result) == "null" {
		return nil, nil
	}
	if result.Result[0] == '"' {
		// Etherscan/Polygonscan 无 key 或限流时 result 是字符串告警
		var apiErr string
		_ = json.Unmarshal(result.Result, &apiErr)
		return nil, fmt.Errorf("explorer api error: %s", apiErr)
	}
	var transfers []ethTransaction
	var txs []ethTransaction
	if err := json.Unmarshal(result.Result, &txs); err != nil {
		return nil, err
	}
	for _, tx := range txs {
		if strings.EqualFold(tx.To, cfg.Address) {
			transfers = append(transfers, tx)
		}
	}
	return transfers, nil
}

type rpcLogEntry struct {
	Address          string   `json:"address"`
	Topics           []string `json:"topics"`
	Data             string   `json:"data"`
	BlockNumber      string   `json:"blockNumber"`
	TransactionHash  string   `json:"transactionHash"`
	TransactionIndex string   `json:"transactionIndex"`
	BlockHash        string   `json:"blockHash"`
	LogIndex         string   `json:"logIndex"`
	Removed          bool     `json:"removed"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type rpcBlockResponse struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Result  string `json:"result"`
}

// fetchBEP20TransfersViaRPC 使用 BSC 公共 RPC 通过 eth_getLogs 查询 BEP20 USDT 转账事件（免费无需 API Key）
func (s *PaymentService) fetchBEP20TransfersViaRPC(meta evmNetworkMeta) ([]ethTransaction, error) {
	bepCfg := s.GetBEP20Config()
	if bepCfg.Address == "" {
		return nil, nil
	}
	// Transfer 事件签名：Transfer(address indexed from, address indexed to, uint256 value)
	transferTopic := "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"
	// 将收款地址转换为 topic（32字节，左侧补零）
	toAddrPadded := strings.ToLower(bepCfg.Address)
	if strings.HasPrefix(toAddrPadded, "0x") {
		toAddrPadded = toAddrPadded[2:]
	}
	toTopic := "0x000000000000000000000000" + toAddrPadded

	// 获取当前区块号，查询最近 5000 个区块（约 2-3 小时）
	latestBlock, err := s.getBSCLatestBlock(bepCfg.BscRPC)
	if err != nil {
		return nil, fmt.Errorf("get latest block: %w", err)
	}
	fromBlock := latestBlock - 5000
	if fromBlock < 0 {
		fromBlock = 0
	}

	// 构造 eth_getLogs 请求
	reqBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "eth_getLogs",
		"params": []interface{}{
			map[string]interface{}{
				"fromBlock": "0x" + strconv.FormatInt(fromBlock, 16),
				"toBlock":   "0x" + strconv.FormatInt(latestBlock, 16),
				"address":   meta.USDTContract,
				"topics":    []interface{}{transferTopic, nil, toTopic},
			},
		},
		"id": 1,
	}

	var logs []rpcLogEntry
	for _, rpcURL := range bepCfg.BscRPC {
		logs, err = s.doRPCLogsQuery(rpcURL, reqBody)
		if err == nil {
			break
		}
		s.log.Warn("BSC RPC query failed, trying next", "rpc", rpcURL, "error", err)
	}
	if err != nil {
		return nil, err
	}

	var transfers []ethTransaction
	decimals := meta.Decimals
	if decimals == 0 {
		decimals = 18
	}
	for _, log := range logs {
		if log.Removed {
			continue
		}
		if len(log.Topics) < 3 {
			continue
		}
		// 解析 to 地址
		toHex := log.Topics[2]
		if len(toHex) >= 42 {
			toHex = "0x" + toHex[26:]
		}
		if !strings.EqualFold(toHex, bepCfg.Address) {
			continue
		}
		// 解析 from 地址
		fromHex := log.Topics[1]
		if len(fromHex) >= 42 {
			fromHex = "0x" + fromHex[26:]
		}
		// 解析 value (十六进制)
		valueHex := log.Data
		valueWei := new(big.Int)
		if strings.HasPrefix(valueHex, "0x") {
			valueWei.SetString(valueHex[2:], 16)
		} else {
			valueWei.SetString(valueHex, 16)
		}
		// 转换为 big.Float 并按小数位数转换
		valueFloat := new(big.Float).SetInt(valueWei)
		divisor := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil))
		amount, _ := new(big.Float).Quo(valueFloat, divisor).Float64()

		// 计算确认数
		blockNumHex := log.BlockNumber
		if strings.HasPrefix(blockNumHex, "0x") {
			blockNumHex = blockNumHex[2:]
		}
		blockNum, _ := strconv.ParseInt(blockNumHex, 16, 64)
		confirmations := latestBlock - blockNum

		transfers = append(transfers, ethTransaction{
			Hash:          log.TransactionHash,
			BlockNumber:   log.BlockNumber,
			From:          fromHex,
			To:            toHex,
			Value:         strconv.FormatFloat(amount, 'f', -1, 64),
			TimeStamp:     strconv.FormatInt(time.Now().Unix(), 10),
			Confirmations: strconv.FormatInt(confirmations, 10),
		})
	}
	return transfers, nil
}

func (s *PaymentService) doRPCLogsQuery(rpcURL string, reqBody map[string]interface{}) ([]rpcLogEntry, error) {
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", rpcURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var rpcResp rpcResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return nil, err
	}
	if rpcResp.Error != nil {
		return nil, fmt.Errorf("rpc error: %d - %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}
	var logs []rpcLogEntry
	if err := json.Unmarshal(rpcResp.Result, &logs); err != nil {
		return nil, err
	}
	return logs, nil
}

func (s *PaymentService) getBSCLatestBlock(rpcURLs []string) (int64, error) {
	reqBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "eth_blockNumber",
		"params":  []interface{}{},
		"id":      1,
	}
	bodyBytes, _ := json.Marshal(reqBody)
	for _, rpcURL := range rpcURLs {
		req, err := http.NewRequest("POST", rpcURL, bytes.NewReader(bodyBytes))
		if err != nil {
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := s.httpClient.Do(req)
		if err != nil {
			continue
		}
		var rpcResp rpcBlockResponse
		if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
			resp.Body.Close()
			continue
		}
		resp.Body.Close()
		blockHex := rpcResp.Result
		if strings.HasPrefix(blockHex, "0x") {
			blockHex = blockHex[2:]
		}
		blockNum, err := strconv.ParseInt(blockHex, 16, 64)
		if err == nil {
			return blockNum, nil
		}
	}
	return 0, fmt.Errorf("all BSC RPC endpoints failed")
}

func (s *PaymentService) Stop() {
	close(s.stopPoll)
	s.pollWg.Wait()
}

func (s *PaymentService) pollPaymentsLoop() {
	defer s.pollWg.Done()
	s.log.Info("Payment poll loop started")
	pollInterval := 60 * time.Second
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopPoll:
			s.log.Info("Payment poll loop stopped")
			return
		case <-ticker.C:
			s.pollPendingOrders()
		}
	}
}

func (s *PaymentService) pollPendingOrders() {
	defer func() {
		if r := recover(); r != nil {
			s.log.Error("pollPayments panic", "error", r)
		}
	}()
	ctx := context.Background()
	s.cfgMu.RLock()
	trcCfg := s.trc20Cfg
	ercCfg := s.erc20Cfg
	bepCfg := s.bep20Cfg
	s.cfgMu.RUnlock()

	const pageSize = 100
	orders := []*model.PaymentOrder{}
	page := 1
	for {
		batch, total, err := s.paymentOrderRepo.ListPending(ctx, page, pageSize)
		if err != nil {
			s.log.Error("list pending orders", "error", err, "page", page)
			return
		}
		orders = append(orders, batch...)
		if page*pageSize >= total {
			break
		}
		page++
	}
	if len(orders) == 0 {
		return
	}

	needTRC := false
	needEVM := map[string]bool{}
	needBEP20 := false
	for _, o := range orders {
		switch o.PaymentMethod {
		case model.PaymentMethodUSDTTRC20:
			needTRC = true
		case model.PaymentMethodUSDTERC20:
			needEVM[evmNetworkKeyFromPayCurrency(o.PayCurrency)] = true
		case model.PaymentMethodUSDTBEP20:
			needBEP20 = true
			needEVM["bsc"] = true
		}
	}

	trcTransfers := map[string]*tronTransfer{}
	if needTRC && trcCfg.Enabled && trcCfg.Address != "" {
		if t, err := s.fetchTRC20Transfers(trcCfg); err == nil {
			for i := range t {
				trcTransfers[t[i].TransactionID] = &t[i]
			}
		} else {
			s.log.Warn("fetch TRC20 transfers error", "error", err)
		}
	}

	ercByNetwork := map[string]map[string]*ethTransaction{}
	// 查询 BEP20 网络（使用 BSC 公共 RPC）
	if needBEP20 && bepCfg.Enabled && bepCfg.Address != "" {
		meta := evmNetworks["bsc"]
		et, err := s.fetchBEP20TransfersViaRPC(meta)
		if err != nil {
			s.log.Warn("fetch BEP20 transfers error", "error", err)
		} else {
			m := map[string]*ethTransaction{}
			for i := range et {
				m[strings.ToLower(et[i].Hash)] = &et[i]
			}
			ercByNetwork["bsc"] = m
		}
	}
	if ercCfg.Enabled && ercCfg.Address != "" {
		for _, netKey := range ercCfg.EnabledNetworks() {
			if !needEVM[netKey] || netKey == "bsc" {
				continue
			}
			var et []ethTransaction
			var err error
			et, err = s.fetchERC20Transfers(ercCfg, netKey)
			if err != nil {
				s.log.Warn("fetch EVM transfers error", "network", netKey, "error", err)
				continue
			}
			m := map[string]*ethTransaction{}
			for i := range et {
				m[strings.ToLower(et[i].Hash)] = &et[i]
			}
			ercByNetwork[netKey] = m
		}
	}

	for _, o := range orders {
		switch o.PaymentMethod {
		case model.PaymentMethodUSDTTRC20:
			s.matchTRC20(ctx, o, trcTransfers, trcCfg)
		case model.PaymentMethodUSDTERC20:
			s.matchERC20(ctx, o, ercByNetwork, ercCfg)
		case model.PaymentMethodUSDTBEP20:
			s.matchERC20(ctx, o, ercByNetwork, ercCfg)
		case model.PaymentMethodWechat, model.PaymentMethodAlipay:
			s.pollEpayOrder(ctx, o)
		}
	}
}

func (s *PaymentService) matchTRC20(ctx context.Context, o *model.PaymentOrder, transfers map[string]*tronTransfer, cfg TRC20Config) {
	for _, t := range transfers {
		if !strings.EqualFold(t.To, cfg.Address) {
			continue
		}
		if math.Abs(t.Value-o.FinalAmount) > cfg.AmountTolerance && t.Value < o.FinalAmount {
			continue
		}
		blockDiff := int64(0)
		if t.BlockNumber > 0 {
			latest, _ := s.fetchTronLatestBlock(cfg)
			if latest > t.BlockNumber {
				blockDiff = latest - t.BlockNumber
			}
		}
		needConf := int64(cfg.MinConfirmations)
		if blockDiff < needConf && !t.Confirmed {
			continue
		}
		paid := t.Value
		hash := t.TransactionID
		// 一笔链上交易只能认领一个订单，防止同金额订单被重复激活
		if existing, err := s.paymentOrderRepo.GetByTxHash(ctx, hash); err == nil && existing != nil && existing.ID != o.ID {
			continue
		}
		paidAt := time.Now()
		var blockNum *int64
		if t.BlockNumber > 0 {
			blockNum = &t.BlockNumber
		}
		updated, err := s.paymentOrderRepo.MarkPaidIfPending(ctx, o.ID, hash, paid, paidAt)
		if err != nil {
			s.log.Error("mark order paid", "error", err)
			continue
		}
		if !updated {
			continue
		}
		_ = s.paymentOrderRepo.UpdateBlockNumber(ctx, o.ID, blockNum)
		s.log.Info("TRC20 order paid", "order_no", o.OrderNo, "tx", hash, "amount", paid)
		if cfg.AutoActivate {
			s.activateOrder(ctx, o, paid)
		}
	}
}

func (s *PaymentService) matchERC20(ctx context.Context, o *model.PaymentOrder, byNetwork map[string]map[string]*ethTransaction, cfg ERC20Config) {
	netKey := evmNetworkKeyFromPayCurrency(o.PayCurrency)
	transfers := byNetwork[netKey]
	if len(transfers) == 0 {
		return
	}
	meta, ok := evmNetworks[netKey]
	if !ok {
		meta = evmNetworks["polygon"]
	}
	decimals := meta.Decimals
	if decimals == 0 {
		decimals = 6
	}
	for _, t := range transfers {
		if !strings.EqualFold(t.To, cfg.Address) && netKey != "bsc" {
			// BEP20使用独立配置的地址
			bepCfg := s.GetBEP20Config()
			if netKey == "bsc" && !strings.EqualFold(t.To, bepCfg.Address) {
				continue
			}
			if netKey != "bsc" {
				continue
			}
		}
		valueWei, err := strconv.ParseFloat(t.Value, 64)
		if err != nil {
			continue
		}
		amount := valueWei / math.Pow10(decimals)
		toleranceCfg := cfg.AmountTolerance
		if netKey == "bsc" {
			bepCfg := s.GetBEP20Config()
			toleranceCfg = bepCfg.AmountTolerance
		}
		if math.Abs(amount-o.FinalAmount) > toleranceCfg && amount < o.FinalAmount {
			continue
		}
		confirms, _ := strconv.Atoi(t.Confirmations)
		minConf := cfg.MinConfirmations
		autoActivate := cfg.AutoActivate
		if netKey == "bsc" {
			bepCfg := s.GetBEP20Config()
			minConf = bepCfg.MinConfirmations
			autoActivate = bepCfg.AutoActivate
		}
		if confirms < minConf {
			continue
		}
		paid := amount
		hash := t.Hash
		// 一笔链上交易只能认领同网络订单；不同网络交易哈希互不占用
		if existing, err := s.paymentOrderRepo.GetByTxHash(ctx, hash); err == nil && existing != nil && existing.ID != o.ID && existing.PayCurrency == o.PayCurrency {
			continue
		}
		paidAt := time.Now()
		var blockNum *int64
		bnStr := strings.TrimPrefix(t.BlockNumber, "0x")
		if bn, err := strconv.ParseInt(bnStr, 16, 64); err == nil {
			blockNum = &bn
		} else if bn, err := strconv.ParseInt(t.BlockNumber, 10, 64); err == nil {
			blockNum = &bn
		}
		updated, err := s.paymentOrderRepo.MarkPaidIfPending(ctx, o.ID, hash, paid, paidAt)
		if err != nil {
			s.log.Error("mark order paid", "error", err)
			continue
		}
		if !updated {
			continue
		}
		if blockNum != nil {
			_ = s.paymentOrderRepo.UpdateBlockNumber(ctx, o.ID, blockNum)
		}
		s.log.Info("EVM order paid", "network", netKey, "order_no", o.OrderNo, "tx", hash, "amount", paid)
		if autoActivate {
			s.activateOrder(ctx, o, paid)
		}
	}
}

func (s *PaymentService) activateOrder(ctx context.Context, o *model.PaymentOrder, paidAmount float64) {
	plan, err := s.planRepo.GetByID(ctx, o.PlanID)
	if err != nil || plan == nil {
		s.log.Error("activate: plan not found", "error", err)
		return
	}
	days := model.PeriodDaysMap[o.PeriodCode]

	// 订阅/续费统一按新周期起算：续费不延长原到期时间，完全按新套餐重新计算（新周期 + 流量重置）
	existingSub, _ := s.subRepo.GetActiveByUserID(ctx, o.UserID)
	if existingSub != nil {
		_ = s.subRepo.MarkReplaced(ctx, existingSub.ID)
	}
	now := time.Now()
	expiresAt := now.AddDate(0, 0, days)
	sub := &model.UserPlanSubscription{
		ID:                uuid.New(),
		UserID:            o.UserID,
		PlanID:            plan.ID,
		Status:            model.SubscriptionStatusActive,
		StartedAt:         &now,
		ExpiresAt:         &expiresAt,
		RenewalMode:       model.RenewalModeManual,
		TrafficQuotaBytes: plan.TrafficBytes,
		TrafficUsedBytes:  0,
		SpeedLimitMbps:    plan.SpeedLimitMbps,
		DeviceLimit:       plan.DeviceLimit,
		IPLimit:           plan.IPLimit,
		Source:            "purchase",
	}
	if err := s.subRepo.Create(ctx, sub); err != nil {
		s.log.Error("activate subscription", "error", err)
		return
	}

	s.ensureDefaultSubscriptionToken(ctx, o.UserID)

	if plan.GroupID != nil {
		if err := s.userRepo.UpdateGroupID(ctx, o.UserID, plan.GroupID); err != nil {
			s.log.Error("update user group_id failed", "user", o.UserID, "group_id", *plan.GroupID, "error", err)
		} else {
			s.log.Info("user group_id updated", "user", o.UserID, "group_id", *plan.GroupID, "plan", plan.Code)
		}
	}

	user, _ := s.userRepo.GetByID(ctx, o.UserID)
	if user != nil && s.mailSvc != nil {
		_ = s.mailSvc.SendPaymentReceived(ctx, user.Email, o.OrderNo, paidAmount)
	}

	// 支付成功站内信通知（异步，不阻塞主流程）
	if s.notifySvc != nil {
		s.notifySvc.NotifyUserAsync(o.UserID, "payment_success", map[string]interface{}{
			"order_id":  o.ID.String(),
			"order_no":  o.OrderNo,
			"plan_name": o.PlanName,
			"amount":    paidAmount,
			"user_id":   o.UserID.String(),
		})
	}

	s.log.Info("Subscription activated", "order", o.OrderNo, "user", o.UserID, "days", days)
	go s.processCommission(context.Background(), o, paidAmount)

	// 发布事件通知 node-service 实时同步用户到节点（新周期：重新计算套餐与流量）
	s.onEvent(ctx, events.TopicUserUnbanned, events.UserEvent{
		UserID: o.UserID.String(),
		Reason: "purchase_activated",
	})
}

func (s *PaymentService) ensureDefaultSubscriptionToken(ctx context.Context, userID uuid.UUID) {
	if s.subTokenRepo == nil {
		return
	}
	tokens, err := s.subTokenRepo.ListByUser(ctx, userID)
	if err != nil {
		s.log.Warn("list subscription tokens failed during activation", "user", userID, "error", err)
		return
	}
	if len(tokens) > 0 {
		return
	}
	rawToken, tokenHash := pkg.GenerateSubscriptionToken()
	preview := rawToken[:16]
	token := &model.SubscriptionToken{
		ID:           uuid.New(),
		UserID:       userID,
		TokenHash:    tokenHash,
		TokenPreview: preview,
		Status:       model.SubscriptionTokenStatusActive,
		AllowIPBind:  true,
	}
	if err := s.subTokenRepo.Create(ctx, token); err != nil {
		s.log.Error("auto-create subscription token failed", "user", userID, "error", err)
		return
	}
	s.log.Info("auto-created default subscription token for new subscriber", "user", userID)
}

func (s *PaymentService) CheckOrderAndActivate(ctx context.Context, userID uuid.UUID, orderID uuid.UUID, txHash string) (*model.PaymentOrder, error) {
	order, err := s.paymentOrderRepo.GetByID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if order == nil || order.UserID != userID {
		return nil, ErrOrderNotFound
	}
	if order.Status == model.PaymentStatusPaid {
		return order, nil
	}
	if txHash != "" {
		existing, _ := s.paymentOrderRepo.GetByTxHash(ctx, txHash)
		if existing != nil && existing.ID != order.ID {
			return nil, fmt.Errorf("tx already used")
		}
		paid := order.FinalAmount
		now := time.Now()
		updated, err := s.paymentOrderRepo.UpdateStatus(ctx, order.ID, model.PaymentStatusPaid, &txHash, &paid, &now)
		if err != nil {
			return nil, fmt.Errorf("mark order paid: %w", err)
		}
		if !updated {
			return nil, fmt.Errorf("order is not pending and cannot be marked paid")
		}
		order.Status = model.PaymentStatusPaid
		order.TxHash = &txHash
		order.PaidAmount = &paid
		order.PaidAt = &now
		trcCfg := s.GetTRC20Config()
		ercCfg := s.GetERC20Config()
		autoActivate := trcCfg.AutoActivate || ercCfg.AutoActivate
		if autoActivate {
			s.activateOrder(ctx, order, paid)
		}
	}
	return order, nil
}

func (s *PaymentService) ValidateAndApplyCoupon(ctx context.Context, userID uuid.UUID, code string, basePrice float64, planID uuid.UUID, period string) (*model.Coupon, error) {
	coupon, err := s.couponRepo.GetByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	if coupon == nil {
		return nil, ErrCouponNotFound
	}
	if !coupon.IsActive {
		return nil, ErrCouponInvalid
	}
	now := time.Now()
	if coupon.StartsAt != nil && now.Before(*coupon.StartsAt) {
		return nil, ErrCouponNotStarted
	}
	if coupon.ExpiresAt != nil && now.After(*coupon.ExpiresAt) {
		return nil, ErrCouponExpired
	}
	if coupon.MaxUses > 0 && coupon.UsedCount >= coupon.MaxUses {
		return nil, ErrCouponUsedUp
	}
	// 一次性券：不可重复使用，全局已用过即拒绝
	if !coupon.IsRepeatable && coupon.UsedCount > 0 {
		return nil, ErrCouponNotRepeatable
	}
	if basePrice < coupon.MinOrderAmount {
		return nil, ErrCouponMinAmount
	}
	if coupon.PlanID != nil && *coupon.PlanID != planID {
		if len(coupon.LimitPlanIDs) > 0 {
			found := false
			for _, pid := range coupon.LimitPlanIDs {
				if pid == planID {
					found = true
					break
				}
			}
			if !found {
				return nil, ErrCouponPlanLimit
			}
		} else {
			return nil, ErrCouponPlanLimit
		}
	}
	// 限制可用周期（limit_period 空=不限制）
	if period != "" && len(coupon.LimitPeriod) > 0 {
		found := false
		for _, p := range coupon.LimitPeriod {
			if p == period {
				found = true
				break
			}
		}
		if !found {
			return nil, ErrCouponPeriodLimit
		}
	}
	if coupon.LimitUseByUser > 0 {
		count, err := s.couponRepo.CountUsageByUser(ctx, coupon.ID, userID)
		if err == nil && count >= coupon.LimitUseByUser {
			return nil, ErrCouponUsedUp
		}
	}
	if coupon.NewUserOnly {
		orders, _, err := s.paymentOrderRepo.ListByUser(ctx, userID, 1, 1, "")
		if err == nil && len(orders) > 0 {
			return nil, ErrCouponNewUserOnly
		}
	}
	discount := 0.0
	switch coupon.DiscountType {
	case "percentage":
		discount = basePrice * coupon.DiscountValue / 100.0
	case "fixed":
		discount = coupon.DiscountValue
	default:
		discount = basePrice * coupon.DiscountValue / 100.0
	}
	// max_discount 上限（0=不限）
	if coupon.MaxDiscount > 0 && discount > coupon.MaxDiscount {
		discount = coupon.MaxDiscount
	}
	if discount > basePrice {
		discount = basePrice
	}
	if discount < 0 {
		discount = 0
	}
	coupon.Discount = math.Round(discount*100) / 100
	return coupon, nil
}

var (
	ErrCouponNotStarted = fmt.Errorf("coupon not started yet")
)

func (s *PaymentService) processCommission(ctx context.Context, order *model.PaymentOrder, paidAmount float64) {
	invCfg := s.loadCommissionConfig()
	if !invCfg.Enabled {
		return
	}
	user, err := s.userRepo.GetByID(ctx, order.UserID)
	if err != nil || user == nil || user.InviterID == nil {
		return
	}
	inviterID := *user.InviterID
	commissionRate := invCfg.Rate / 100.0
	commissionAmount := math.Round(paidAmount*commissionRate*100) / 100
	if commissionAmount < 0.01 {
		return
	}
	inviter, err := s.userRepo.GetByID(ctx, inviterID)
	if err != nil || inviter == nil {
		return
	}
	log := &model.CommissionLog{
		ID:                uuid.New(),
		InviterID:         inviterID,
		InviteeID:         user.ID,
		OrderID:           &order.ID,
		OrderAmount:       paidAmount,
		GetAmount:         commissionAmount,
		CommissionBalance: inviter.CommissionBalance,
		Status:            0,
	}
	_ = s.commissionLogRepo.Create(ctx, log)
	s.log.Info("Commission log created (pending confirm)", "inviter", inviterID, "amount", commissionAmount)
}

type commissionConfig struct {
	Enabled        bool    `json:"enabled"`
	Rate           float64 `json:"rate"`
	FirstPullback  float64 `json:"first_pullback"`
	RegisterReward float64 `json:"register_reward"`
	InviteReward   float64 `json:"invite_reward"`
	ConfirmDays    int     `json:"confirm_days"`
	WithdrawEnable bool    `json:"withdraw_enable"`
	MinWithdraw    float64 `json:"min_withdraw"`
}

func (s *PaymentService) loadCommissionConfig() commissionConfig {
	cfg := commissionConfig{
		Enabled:     false,
		Rate:        20,
		ConfirmDays: 3,
		MinWithdraw: 10,
	}
	data, err := s.settingRepo.GetJSON(context.Background(), "invite", "commission")
	if err != nil {
		return cfg
	}
	_ = json.Unmarshal(data, &cfg)
	return cfg
}

// ProcessSettledCommissions 直接操作 repo 进行佣金结算（已废弃）。
//
// Deprecated: 佣金结算逻辑已统一到 CommissionService.CheckPendingCommissions，
// 由 processDailyCommissionSettle 委托调用。本方法仅作为退化兼容保留，
// 新代码请使用 CommissionService.CheckPendingCommissions / SettleCommission。
func (s *PaymentService) ProcessSettledCommissions(ctx context.Context) error {
	cfg := s.loadCommissionConfig()
	if !cfg.Enabled {
		return nil
	}
	cutoff := time.Now().Add(-time.Duration(cfg.ConfirmDays) * 24 * time.Hour)
	pendingLogs, err := s.commissionLogRepo.ListPendingBefore(ctx, cutoff)
	if err != nil {
		return err
	}
	for _, cl := range pendingLogs {
		inviter, err := s.userRepo.GetByID(ctx, cl.InviterID)
		if err != nil || inviter == nil {
			_ = s.commissionLogRepo.UpdateStatus(ctx, cl.ID, 2)
			continue
		}
		order, err := s.paymentOrderRepo.GetByID(ctx, *cl.OrderID)
		if err != nil || order == nil || order.Status != model.PaymentStatusPaid {
			_ = s.commissionLogRepo.UpdateStatus(ctx, cl.ID, 2)
			continue
		}
		newBalance := math.Round((inviter.CommissionBalance+cl.GetAmount)*100) / 100
		newTotal := math.Round((inviter.CommissionTotal+cl.GetAmount)*100) / 100
		if err := s.userRepo.UpdateCommission(ctx, inviter.ID, newBalance, newTotal); err != nil {
			s.log.Error("update commission balance", "error", err)
			continue
		}
		cl.CommissionBalance = newBalance
		_ = s.commissionLogRepo.UpdateStatus(ctx, cl.ID, 1)
		s.log.Info("Commission settled", "inviter", inviter.ID, "amount", cl.GetAmount)
	}
	return nil
}

func (s *PaymentService) ListUserOrders(ctx context.Context, userID uuid.UUID, page, pageSize int, statusFilter string) ([]*model.PaymentOrder, int, error) {
	return s.paymentOrderRepo.ListByUser(ctx, userID, page, pageSize, statusFilter)
}

func (s *PaymentService) GetOrder(ctx context.Context, userID uuid.UUID, orderID uuid.UUID) (*model.PaymentOrder, error) {
	order, err := s.paymentOrderRepo.GetByID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if order == nil || order.UserID != userID {
		return nil, ErrOrderNotFound
	}
	return order, nil
}
