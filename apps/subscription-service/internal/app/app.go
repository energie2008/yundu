package app

import (
	"context"
	"encoding/json"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/airport-panel/config"
	"github.com/airport-panel/config/db"
	"github.com/airport-panel/config/events"
	"github.com/airport-panel/config/server"
	"github.com/gin-gonic/gin"

	"github.com/airport-panel/config/redis"
	"github.com/airport-panel/subscription-service/internal/cache"
	"github.com/airport-panel/subscription-service/internal/compat"
	svccfg "github.com/airport-panel/subscription-service/internal/config"
	"github.com/airport-panel/subscription-service/internal/handler"
	"github.com/airport-panel/subscription-service/internal/lb"
	"github.com/airport-panel/subscription-service/internal/middleware"
	"github.com/airport-panel/subscription-service/internal/node"
	"github.com/airport-panel/subscription-service/internal/pkg"
	"github.com/airport-panel/subscription-service/internal/repo"
	"github.com/airport-panel/subscription-service/internal/renderer"
	"github.com/airport-panel/subscription-service/internal/service"
)

func Run() {
	cfg := svccfg.Load()
	logger := config.NewLogger(svccfg.ServiceName, cfg.LogLevel)

	ctx := context.Background()

	pool, err := db.NewPool(ctx, cfg.Database)
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		panic(err)
	}
	defer pool.Close()

	jwtManager := pkg.NewJWTManager(cfg.JWT.Secret)

	tokenRepo := repo.NewTokenRepo(pool)
	userRepo := repo.NewUserRepo(pool)
	templateRepo := repo.NewTemplateRepo(pool)
	accessLogRepo := repo.NewAccessLogRepo(pool)
	shortCodeRepo := repo.NewShortCodeRepo(pool)
	subscribeTemplateRepo := repo.NewSubscribeTemplateRepo(pool)

	nodeProvider := node.NewDBNodeProvider(pool)
	// P0-3: 缓存 TTL 从 60s 降至 10s，减少节点新建/更新后的可见延迟
	subCache := cache.NewMemoryCache(10 * time.Second)

	clientProfileRepo := compat.NewClientProfileRepo(pool)
	compatMatrixRepo := compat.NewCompatMatrixRepo(pool)
	advancedPatchRepo := compat.NewAdvancedPatchRepo(pool)
	compatService := compat.NewCompatService(clientProfileRepo, compatMatrixRepo, advancedPatchRepo)
	nodeRenderer := renderer.NewNodeRenderer(compatService)
	adminCompatHandler := compat.NewAdminCompatHandler(compatService)

	redisClient, err := redis.NewClient(cfg.Redis)
	var rateLimiter middleware.RateLimiter
	if err != nil {
		logger.Warn("failed to connect to redis, using in-memory rate limiter", "error", err)
		rateLimiter = middleware.NewMemoryRateLimiter()
		redisClient = nil
	} else {
		rateLimiter = middleware.NewRedisRateLimiter(redisClient)
	}
	rateLimitMiddleware := middleware.NewRateLimitMiddleware(rateLimiter, middleware.DefaultRateLimitConfig())

	lbPolicyRepo := lb.NewLBPolicyRepo(pool)
	lbNodeRepo := lb.NewNodeCandidateRepo(pool)
	lbDataReader := &lb.NodeDataReaderAdapter{
		PolicyRepo: lbPolicyRepo,
		NodeRepo:   lbNodeRepo,
		Redis:      redisClient,
	}
	lbEngine := lb.NewLBEngine(lbDataReader, logger)
	lbService := lb.NewLBService(lbEngine, lbPolicyRepo, logger)

	subscriptionService := service.NewSubscriptionService(
		tokenRepo, userRepo, templateRepo, accessLogRepo, shortCodeRepo, nodeProvider, subCache, logger,
		nodeRenderer, lbService,
	)

	// 订阅模板服务（按名称索引，对齐 Xboard subscribe_template helper）。
	// 启动时预加载缓存，渲染器可直接 GetTemplate('singbox')/'clash' 取模板内容。
	subscribeTemplateSvc := service.NewTemplateService(subscribeTemplateRepo)
	if err := subscribeTemplateSvc.ReloadCache(ctx); err != nil {
		logger.Warn("failed to preload subscribe template cache", "error", err)
	}
	// 注入订阅模板服务，渲染时按客户端类型从数据库加载基础模板
	subscriptionService.SetTemplateService(subscribeTemplateSvc)
	adminTemplateHandler := handler.NewAdminTemplateHandler(subscribeTemplateSvc)

	authMiddleware := middleware.NewAuthMiddleware(jwtManager)

	healthHandler := handler.NewHealthHandler()
	subscriptionHandler := handler.NewSubscriptionHandler(subscriptionService)

	opts := server.DefaultOptions(svccfg.ServiceName, cfg.Port)
	opts.Logger = logger
	opts.RegisterRoutes = func(api *gin.RouterGroup) {
		healthHandler.RegisterRoutes(api)

		user := api.Group("/user")
		user.Use(authMiddleware.UserAuth())
		{
			user.GET("/sub/:token/short", subscriptionHandler.GetShortCode)
			user.GET("/subscription/stats", subscriptionHandler.GetUserAccessStats)
		}

		admin := api.Group("/admin")
		admin.Use(authMiddleware.AdminAuth())
		{
			admin.GET("/subscription-tokens", subscriptionHandler.ListTokens)
			admin.POST("/subscription-tokens", subscriptionHandler.CreateToken)
			admin.DELETE("/subscription-tokens/:id", subscriptionHandler.RevokeToken)
			admin.GET("/subscription/templates", subscriptionHandler.ListTemplates)
			admin.POST("/subscription/templates", subscriptionHandler.CreateTemplate)
			admin.PUT("/subscription/templates/:id", subscriptionHandler.UpdateTemplate)
			admin.PUT("/subscription/templates/:id/default", subscriptionHandler.SetDefaultTemplate)
			admin.POST("/subscription/short-codes", subscriptionHandler.GenerateShortCode)
			admin.DELETE("/subscription/short-codes/:code", subscriptionHandler.RevokeShortCode)
			admin.GET("/subscription/access-overview", subscriptionHandler.GetAccessOverview)
			admin.GET("/subscription/access-logs", subscriptionHandler.GetAccessLogs)

			// 订阅模板管理（按名称索引，对齐 Xboard subscribe_template helper）
			adminTemplateHandler.RegisterAdminRoutes(admin)

			adminCompatHandler.RegisterAdminRoutes(admin)
		}
	}

	srv := server.New(opts)

	engine := srv.Engine()
	engine.Use(rateLimitMiddleware.GlobalRateLimit())

	publicSub := engine.Group("/sub")
	publicSub.Use(rateLimitMiddleware.SubscriptionRateLimit())
	{
		publicSub.GET("/:token", subscriptionHandler.GetSubscription)
		publicSub.HEAD("/:token", subscriptionHandler.GetSubscription)
		publicSub.GET("/:token/info", subscriptionHandler.GetSubscriptionInfo)
		publicSub.HEAD("/:token/info", subscriptionHandler.GetSubscriptionInfo)
		publicSub.GET("/:token/qr", subscriptionHandler.GetSubscriptionQR)
		publicSub.GET("/:token/qrcode", subscriptionHandler.GetSubscriptionQR)
	}

	publicShort := engine.Group("/s")
	publicShort.Use(rateLimitMiddleware.SubscriptionRateLimit())
	{
		publicShort.GET("/:code", subscriptionHandler.ResolveShortCode)
		publicShort.HEAD("/:code", subscriptionHandler.ResolveShortCode)
	}

	var eventBus *events.Bus
	if redisClient != nil {
		eventBus = events.NewBus(redisClient, logger)
	} else {
		eventBus = events.NewNopBus(logger)
	}

	type userPayload struct {
		UserID string `json:"user_id"`
	}
	invalidateHandler := func(evt events.Event) {
		var p userPayload
		_ = json.Unmarshal(evt.Data, &p)
		logger.Info("received user event, invalidating cache", "topic", evt.Topic, "user", p.UserID)
		subscriptionService.InvalidateUserCache()
	}
	eventBus.Subscribe(events.TopicUserBanned, invalidateHandler)
	eventBus.Subscribe(events.TopicUserUnbanned, invalidateHandler)
	eventBus.Subscribe(events.TopicTrafficReset, invalidateHandler)
	eventBus.Subscribe(events.TopicPlanChanged, invalidateHandler)
	eventBus.Subscribe(events.TopicTokenRevoked, invalidateHandler)
	// P0-3: 订阅节点配置变更事件，立即失效订阅缓存
	eventBus.Subscribe(events.TopicConfigChanged, func(evt events.Event) {
		logger.Info("received node config changed event, invalidating all subscription cache", "topic", evt.Topic)
		subscriptionService.InvalidateUserCache()
	})

	// P0-3: 内部缓存失效端点（供 node-service 在节点创建/更新后直接调用，不依赖 Redis）
	internal := engine.Group("/internal")
	{
		internal.POST("/invalidate-cache", func(c *gin.Context) {
			// 简单的共享密钥验证（INTERNAL_API_KEY 环境变量）
			expectedKey := os.Getenv("INTERNAL_API_KEY")
			if expectedKey != "" {
				authKey := c.GetHeader("X-Internal-Key")
				if authKey != expectedKey {
					c.Status(401)
					return
				}
			}
			subscriptionService.InvalidateUserCache()
			logger.Info("subscription cache invalidated via internal endpoint")
			c.JSON(200, gin.H{"invalidated": true})
		})
	}

	busCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := eventBus.Start(busCtx); err != nil {
		logger.Error("failed to start event bus", "error", err)
	}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		logger.Info("shutting down subscription-service...")
		eventBus.Stop()
		cancel()
	}()

	srv.Start()
}
