package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"net/http"
	"net/url"
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
	Enabled          bool    `json:"enabled"`
	Address          string  `json:"address"`
	USDTContract     string  `json:"usdt_contract"`
	EtherscanAPI     string  `json:"etherscan_api"`
	EtherscanAPIKey  string  `json:"etherscan_api_key"`
	MinConfirmations int     `json:"min_confirmations"`
	OrderExpiryHours int     `json:"order_expiry_hours"`
	AmountTolerance  float64 `json:"amount_tolerance"`
	AutoActivate     bool    `json:"auto_activate"`
	PollInterval     int     `json:"poll_interval_seconds"`
	Network          string  `json:"network"`
}

// WechatConfig 微信支付配置（框架预留，暂不对接真实接口）
type WechatConfig struct {
	Enabled          bool   `json:"enabled"`
	OrderExpiryHours int    `json:"order_expiry_hours"`
	AutoActivate     bool   `json:"auto_activate"`
	// 以下字段为后续对接真实接口预留，当前框架模式不使用
	MchID            string `json:"mch_id,omitempty"`
	APIKey           string `json:"api_key,omitempty"`
	AppID            string `json:"app_id,omitempty"`
	NotifyURL        string `json:"notify_url,omitempty"`
}

// AlipayConfig 支付宝支付配置（框架预留，暂不对接真实接口）
type AlipayConfig struct {
	Enabled          bool   `json:"enabled"`
	OrderExpiryHours int    `json:"order_expiry_hours"`
	AutoActivate     bool   `json:"auto_activate"`
	// 以下字段为后续对接真实接口预留，当前框架模式不使用
	AppID            string `json:"app_id,omitempty"`
	PrivateKey       string `json:"private_key,omitempty"`
	NotifyURL        string `json:"notify_url,omitempty"`
}

type PaymentService struct {
	paymentOrderRepo    *repo.PaymentOrderRepo
	planRepo            *repo.PlanRepo
	userRepo            *repo.UserRepo
	subRepo             *repo.SubscriptionRepo
	subTokenRepo        *repo.SubscriptionTokenRepo
	settingRepo         *repo.SettingRepo
	couponRepo          *repo.CouponRepo
	commissionLogRepo   *repo.CommissionLogRepo
	mailSvc             *MailService
	auditSvc            *AuditService
	notifySvc           *NotificationService
	commissionSvc       *CommissionService
	log                 *slog.Logger
	httpClient          *http.Client
	stopPoll            chan struct{}
	pollWg              sync.WaitGroup
	trc20Cfg            TRC20Config
	erc20Cfg            ERC20Config
	wechatCfg           WechatConfig
	alipayCfg           AlipayConfig
	cfgMu               sync.RWMutex
	lastCommissionRun   time.Time
	exchangeRate        float64
	onEvent             func(ctx context.Context, topic string, payload interface{})
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
	return cfg
}

func (s *PaymentService) loadERC20Config() ERC20Config {
	cfg := ERC20Config{
		Enabled:          false,
		USDTContract:     "0xdAC17F958D2ee523a2206206994597C13D831ec7",
		EtherscanAPI:     "https://api.etherscan.io/api",
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
			// 默认选择优先级：支付宝 > 微信 > USDT-TRC20 > USDT-ERC20
			alipay := s.GetAlipayConfig()
			wechat := s.GetWechatConfig()
			trc := s.GetTRC20Config()
			erc := s.GetERC20Config()
			if alipay.Enabled {
				paymentMethod = model.PaymentMethodAlipay
			} else if wechat.Enabled {
				paymentMethod = model.PaymentMethodWechat
			} else if trc.Enabled && trc.Address != "" {
				paymentMethod = model.PaymentMethodUSDTTRC20
			} else if erc.Enabled && erc.Address != "" {
				paymentMethod = model.PaymentMethodUSDTERC20
			} else if trc.Enabled {
				paymentMethod = model.PaymentMethodUSDTTRC20
			} else {
				paymentMethod = model.PaymentMethodUSDTERC20
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

		case model.PaymentMethodUSDTERC20:
			cfg := s.GetERC20Config()
			if !cfg.Enabled {
				return nil, fmt.Errorf("USDT-ERC20 payment not enabled")
			}
			payAddress = cfg.Address
			payCurrency = "USDT-ERC20"
			expiryHours = cfg.OrderExpiryHours
			if expiryHours <= 0 {
				expiryHours = 6
			}
			if payAddress == "" {
				return nil, fmt.Errorf("USDT-ERC20 receiving address not configured by admin")
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
	order.PaymentURI = s.GetPaymentURI(order)
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
	if finalAmount == 0 {
		now := time.Now()
		zeroPaid := 0.0
		zeroTxHash := "ZERO_YUAN_COUPON"
		if err := s.paymentOrderRepo.UpdateStatus(ctx, order.ID, model.PaymentStatusPaid, &zeroTxHash, &zeroPaid, &now); err != nil {
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
	case model.PaymentMethodUSDTERC20:
		return fmt.Sprintf("ethereum:%s?value=%.2f&contract=%s", order.PayAddress, order.FinalAmount, s.GetERC20Config().USDTContract)
	case model.PaymentMethodUSDTTRC20:
		cfg := s.GetTRC20Config()
		amount := strconv.FormatFloat(order.FinalAmount*1000000, 'f', 0, 64)
		return fmt.Sprintf("tron:%s?amount=%s&contract=%s", order.PayAddress, amount, cfg.USDTContract)
	case model.PaymentMethodWechat, model.PaymentMethodAlipay:
		// 法币支付框架预留：返回占位 URI，后续对接真实接口时替换为支付链接/二维码 URL
		return fmt.Sprintf("pending:%s?amount=%.2f&currency=CNY&method=%s", order.OrderNo, order.FinalAmount, order.PaymentMethod)
	default:
		return ""
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

func (s *PaymentService) fetchERC20Transfers(cfg ERC20Config) ([]ethTransaction, error) {
	if cfg.Address == "" {
		return nil, nil
	}
	params := url.Values{}
	params.Set("module", "account")
	params.Set("action", "tokentx")
	params.Set("contractaddress", cfg.USDTContract)
	params.Set("address", cfg.Address)
	params.Set("page", "1")
	params.Set("offset", "50")
	params.Set("sort", "desc")
	if cfg.EtherscanAPIKey != "" {
		params.Set("apikey", cfg.EtherscanAPIKey)
	}
	apiURL := fmt.Sprintf("%s?%s", strings.TrimRight(cfg.EtherscanAPI, "/"), params.Encode())
	resp, err := s.httpClient.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result struct {
		Status  string           `json:"status"`
		Message string           `json:"message"`
		Result  []ethTransaction `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	var transfers []ethTransaction
	for _, tx := range result.Result {
		if strings.EqualFold(tx.To, cfg.Address) {
			transfers = append(transfers, tx)
		}
	}
	return transfers, nil
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
	s.cfgMu.RUnlock()

	trcTransfers := map[string]*tronTransfer{}
	if trcCfg.Enabled && trcCfg.Address != "" {
		if t, err := s.fetchTRC20Transfers(trcCfg); err == nil {
			for i := range t {
				trcTransfers[t[i].TransactionID] = &t[i]
			}
		} else {
			s.log.Warn("fetch TRC20 transfers error", "error", err)
		}
	}

	ercTransfers := map[string]*ethTransaction{}
	if ercCfg.Enabled && ercCfg.Address != "" {
		if et, err := s.fetchERC20Transfers(ercCfg); err == nil {
			for i := range et {
				ercTransfers[strings.ToLower(et[i].Hash)] = &et[i]
			}
		} else {
			s.log.Warn("fetch ERC20 transfers error", "error", err)
		}
	}

	const pageSize = 100
	page := 1
	for {
		orders, total, err := s.paymentOrderRepo.ListPending(ctx, page, pageSize)
		if err != nil {
			s.log.Error("list pending orders", "error", err, "page", page)
			return
		}
		for _, o := range orders {
			switch o.PaymentMethod {
			case model.PaymentMethodUSDTTRC20:
				s.matchTRC20(ctx, o, trcTransfers, trcCfg)
			case model.PaymentMethodUSDTERC20:
				s.matchERC20(ctx, o, ercTransfers, ercCfg)
			}
		}
		if page*pageSize >= total {
			break
		}
		page++
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
		paidAt := time.Now()
		var blockNum *int64
		if t.BlockNumber > 0 {
			blockNum = &t.BlockNumber
		}
		if err := s.paymentOrderRepo.UpdateStatus(ctx, o.ID, model.PaymentStatusPaid, &hash, &paid, &paidAt); err != nil {
			s.log.Error("update order paid", "error", err)
			continue
		}
		_ = s.paymentOrderRepo.UpdateBlockNumber(ctx, o.ID, blockNum)
		s.log.Info("TRC20 order paid", "order_no", o.OrderNo, "tx", hash, "amount", paid)
		if cfg.AutoActivate {
			s.activateOrder(ctx, o, paid)
		}
	}
}

func (s *PaymentService) matchERC20(ctx context.Context, o *model.PaymentOrder, transfers map[string]*ethTransaction, cfg ERC20Config) {
	for _, t := range transfers {
		if !strings.EqualFold(t.To, cfg.Address) {
			continue
		}
		valueWei, err := strconv.ParseFloat(t.Value, 64)
		if err != nil {
			continue
		}
		amount := valueWei / math.Pow10(6)
		if math.Abs(amount-o.FinalAmount) > cfg.AmountTolerance && amount < o.FinalAmount {
			continue
		}
		confirms, _ := strconv.Atoi(t.Confirmations)
		if confirms < cfg.MinConfirmations {
			continue
		}
		paid := amount
		hash := t.Hash
		paidAt := time.Now()
		var blockNum *int64
		if bn, err := strconv.ParseInt(strings.TrimPrefix(t.BlockNumber, "0x"), 16, 64); err == nil {
			blockNum = &bn
		}
		if err := s.paymentOrderRepo.UpdateStatus(ctx, o.ID, model.PaymentStatusPaid, &hash, &paid, &paidAt); err != nil {
			s.log.Error("update order paid", "error", err)
			continue
		}
		if blockNum != nil {
			_ = s.paymentOrderRepo.UpdateBlockNumber(ctx, o.ID, blockNum)
		}
		s.log.Info("ERC20 order paid", "order_no", o.OrderNo, "tx", hash, "amount", paid)
		if cfg.AutoActivate {
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

	isNewPurchase := false
	existingSub, _ := s.subRepo.GetActiveByUserID(ctx, o.UserID)
	if existingSub != nil && existingSub.PlanID == plan.ID {
		// 同套餐续费：延长有效期
		_ = s.subRepo.ExtendByDays(ctx, existingSub.ID, days)
	} else {
		// 新订阅或套餐升级：替换旧订阅（如有），创建新订阅
		isNewPurchase = true
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
			"order_id":   o.ID.String(),
			"order_no":   o.OrderNo,
			"plan_name":  o.PlanName,
			"amount":     paidAmount,
			"user_id":    o.UserID.String(),
		})
	}

	s.log.Info("Subscription activated", "order", o.OrderNo, "user", o.UserID, "days", days)
	go s.processCommission(context.Background(), o, paidAmount)

	// 发布事件通知 node-service 实时同步用户到节点
	if isNewPurchase {
		// 新购/升级：用户可能刚注册或之前无有效订阅，发 Unbanned 添加用户到所有节点
		s.onEvent(ctx, events.TopicUserUnbanned, events.UserEvent{
			UserID: o.UserID.String(),
			Reason: "purchase_activated",
		})
	} else {
		// 续费：延长时间，发 PlanChanged 事件更新用户状态（若之前因过期被ban则恢复）
		s.onEvent(ctx, events.TopicPlanChanged, struct {
			UserID   string `json:"user_id"`
			PlanID   string `json:"plan_id"`
			Operator string `json:"operator,omitempty"`
		}{
			UserID: o.UserID.String(),
			PlanID: plan.ID.String(),
		})
	}
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
		_ = s.paymentOrderRepo.UpdateStatus(ctx, order.ID, model.PaymentStatusPaid, &txHash, &paid, &now)
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
