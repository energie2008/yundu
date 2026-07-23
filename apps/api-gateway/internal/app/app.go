package app

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/airport-panel/config"
	pkgmw "github.com/airport-panel/config/middleware"
	"github.com/airport-panel/config/observability"
	configredis "github.com/airport-panel/config/redis"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	svccfg "github.com/airport-panel/api-gateway/internal/config"
	gwmiddleware "github.com/airport-panel/api-gateway/internal/middleware"
	"github.com/airport-panel/api-gateway/internal/proxy"
)

const version = "dev"

func Run() {
	cfg := svccfg.Load()
	logger := config.NewLogger(svccfg.ServiceName, cfg.LogLevel)

	gin.SetMode(gin.ReleaseMode)
	if os.Getenv("APP_ENV") == "development" {
		gin.SetMode(gin.DebugMode)
	}

	identityTarget := "http://" + cfg.IdentityAddr
	nodeTarget := "http://" + cfg.NodeAddr
	subscriptionTarget := "http://" + cfg.SubscriptionAddr
	trafficTarget := "http://" + cfg.TrafficAddr

	identityProxy, err := proxy.NewSingleHostReverseProxy(identityTarget, logger)
	if err != nil {
		logger.Error("failed to create identity proxy", "error", err)
		panic(err)
	}
	nodeProxy, err := proxy.NewSingleHostReverseProxy(nodeTarget, logger)
	if err != nil {
		logger.Error("failed to create node proxy", "error", err)
		panic(err)
	}
	subscriptionProxy, err := proxy.NewSingleHostReverseProxy(subscriptionTarget, logger)
	if err != nil {
		logger.Error("failed to create subscription proxy", "error", err)
		panic(err)
	}
	trafficProxy, err := proxy.NewSingleHostReverseProxy(trafficTarget, logger)
	if err != nil {
		logger.Error("failed to create traffic proxy", "error", err)
		panic(err)
	}

	authMW := gwmiddleware.NewAuthMiddleware(cfg.JWTSecret)
	// rate limit: 600 req/min per IP (10 req/s)，足够 admin dashboard 多面板同时刷新
	rateLimiter := gwmiddleware.NewRateLimiter(600, time.Minute)

	// 安全中间件所需的 Redis 客户端（限流 / 防暴力破解）。连接失败时降级为直通，不阻断业务。
	redisClient, err := configredis.NewClient(cfg.Redis)
	if err != nil {
		logger.Warn("failed to connect to redis, security middleware will degrade to pass-through", "error", err)
		redisClient = nil
	} else {
		defer redisClient.Close()
	}

	r := gin.New()
	// 信任 nginx 反代，正确读取 X-Forwarded-For / X-Real-IP（避免所有用户共享 127.0.0.1 限流桶）
	_ = r.SetTrustedProxies([]string{"127.0.0.0/8", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"})
	r.Use(func(c *gin.Context) {
		c.Set("service_name", svccfg.ServiceName)
		c.Next()
	})
	r.Use(pkgmw.RequestID())
	r.Use(pkgmw.Recovery(logger))
	r.Use(observability.PrometheusMiddleware(svccfg.ServiceName))
	r.Use(pkgmw.AccessLog(logger, svccfg.ServiceName))
	r.Use(pkgmw.CORS(pkgmw.DefaultCORSConfig()))
	// 全局 30s 超时；aidiag 单独 120s（DeepSeek 调用 + 指标采集需要时间），
	// /api/v1/agent/* 豁免（node-agent WebSocket 长连接）
	r.Use(pkgmw.TimeoutByPath(30*time.Second, map[string]time.Duration{
		"/api/v1/admin/diagnosis/": 120 * time.Second,
		"/api/v1/agent/":           0,
	}))
	r.Use(rateLimiter.Middleware())
	// 全局拦截恶意 User-Agent（空 UA / SQL 注入 / XSS 特征），无外部依赖
	r.Use(gwmiddleware.BlockMaliciousUA())

	observability.SetBuildInfo(svccfg.ServiceName, version, "unknown")

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": svccfg.ServiceName, "version": version})
	})
	r.GET("/readyz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ready", "service": svccfg.ServiceName})
	})
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	r.Any("/sub/*action", subscriptionProxy)

	public := r.Group("/api/v1")
	// 公开接口：登录 5 次/分钟、注册 3 次/小时（按 IP），登录失败累计触发防暴力破解锁定
	public.Use(gwmiddleware.RateLimitMiddleware(redisClient))
	public.Use(gwmiddleware.AntiBruteForce(redisClient))
	{
		public.POST("/auth/register", identityProxy)
		public.POST("/auth/login", identityProxy)
		public.POST("/auth/refresh", identityProxy)
		public.POST("/admin/auth/login", identityProxy)

		public.POST("/user/auth/register", identityProxy)
		public.POST("/user/auth/login", identityProxy)
		public.POST("/user/auth/refresh", identityProxy)
		public.GET("/user/auth/verify-email", identityProxy)
		public.POST("/user/auth/forgot-password", identityProxy)
		public.POST("/user/auth/reset-password", identityProxy)

		public.GET("/plans", identityProxy)
		public.GET("/plans/:id", identityProxy)
		public.GET("/plans/:id/nodes", identityProxy)

		public.GET("/guest/config", identityProxy)

		public.GET("/public/protocol-presets", nodeProxy)
		public.GET("/protocol-presets", nodeProxy)
	}

	userAPI := r.Group("/api/v1")
	userAPI.Use(authMW.UserAuth())
	// 已登录 API 接口：每用户每秒 10 次（redis 不可用时降级直通）
	userAPI.Use(gwmiddleware.RateLimitMiddleware(redisClient))
	{
		userAPI.POST("/auth/logout", identityProxy)
		userAPI.GET("/me", identityProxy)
		userAPI.Any("/me/*action", identityProxy)
		userAPI.POST("/user/auth/logout", identityProxy)
		userAPI.GET("/user/me", identityProxy)
		userAPI.PATCH("/user/me", identityProxy)
		userAPI.Any("/user/subscription", identityProxy)
		userAPI.Any("/user/subscription/*action", identityProxy)
		userAPI.Any("/user/orders", identityProxy)
		userAPI.Any("/user/orders/*action", identityProxy)
		userAPI.Any("/user/nodes", identityProxy)
		userAPI.Any("/user/tickets", identityProxy)
		userAPI.Any("/user/tickets/*action", identityProxy)
		userAPI.Any("/user/notifications", identityProxy)
		userAPI.Any("/user/notifications/*action", identityProxy)
		userAPI.Any("/user/preferences", identityProxy)
		userAPI.Any("/user/commissions", identityProxy)
		userAPI.Any("/user/commissions/*action", identityProxy)
		userAPI.GET("/user/payment-methods", identityProxy)
		userAPI.POST("/coupons/validate", identityProxy)
		userAPI.GET("/user/traffic", trafficProxy)
	}

	adminAPI := r.Group("/api/v1/admin")
	adminAPI.Use(authMW.AdminAuth())
	{
		adminAPI.POST("/auth/logout", identityProxy)
		adminAPI.GET("/me", identityProxy)

		adminAPI.Any("/users", gwmiddleware.RequirePermission("users.read"), identityProxy)
		adminAPI.Any("/users/*action", gwmiddleware.RequirePermission("users.read"), identityProxy)
		adminAPI.Any("/admins", identityProxy)
		adminAPI.Any("/admins/*action", identityProxy)
		adminAPI.Any("/audit-logs", gwmiddleware.RequirePermission("audit.read"), identityProxy)
		adminAPI.Any("/audit-logs/*action", gwmiddleware.RequirePermission("audit.read"), identityProxy)
		adminAPI.Any("/plans", gwmiddleware.RequirePermission("plans.read"), identityProxy)
		adminAPI.Any("/plans/*action", gwmiddleware.RequirePermission("plans.read"), identityProxy)
		// 优惠券管理（转发到 identity-service）
		adminAPI.Any("/coupons", gwmiddleware.RequirePermission("coupons.read"), identityProxy)
		adminAPI.Any("/coupons/*action", gwmiddleware.RequirePermission("coupons.read"), identityProxy)
		adminAPI.Any("/orders", identityProxy)
		adminAPI.Any("/orders/*action", identityProxy)
		adminAPI.Any("/payment-methods", identityProxy)
		adminAPI.Any("/payment-methods/*action", identityProxy)
		adminAPI.Any("/system/settings", identityProxy)
		adminAPI.Any("/system/settings/*action", identityProxy)

		// 邮件模板与 SMTP 配置管理（转发到 identity-service，RBAC 由 identity-service 处理）
		adminAPI.Any("/mail", identityProxy)
		adminAPI.Any("/mail/*action", identityProxy)

		// Phase 6: 工单 / 公告 / 通知（转发到 identity-service）
		adminAPI.Any("/tickets", gwmiddleware.RequirePermission("tickets.read"), identityProxy)
		adminAPI.Any("/tickets/*action", gwmiddleware.RequirePermission("tickets.read"), identityProxy)
		adminAPI.Any("/announcements", gwmiddleware.RequirePermission("announcements.read"), identityProxy)
		adminAPI.Any("/announcements/*action", gwmiddleware.RequirePermission("announcements.read"), identityProxy)
		adminAPI.Any("/notifications", gwmiddleware.RequirePermission("notifications.read"), identityProxy)
		adminAPI.Any("/notifications/*action", gwmiddleware.RequirePermission("notifications.read"), identityProxy)
		adminAPI.Any("/notification-templates", gwmiddleware.RequirePermission("notifications.read"), identityProxy)
		adminAPI.Any("/notification-templates/*action", gwmiddleware.RequirePermission("notifications.read"), identityProxy)

		// 知识库管理（转发到 identity-service）
		adminAPI.Any("/knowledge", gwmiddleware.RequirePermission("knowledge.read"), identityProxy)
		adminAPI.Any("/knowledge/*action", gwmiddleware.RequirePermission("knowledge.read"), identityProxy)

		// 返利/提现管理（转发到 identity-service）
		adminAPI.Any("/commissions", identityProxy)
		adminAPI.Any("/commissions/*action", identityProxy)

		adminAPI.Any("/servers", nodeProxy)
		adminAPI.Any("/servers/*action", nodeProxy)
		adminAPI.Any("/nodes", nodeProxy)
		adminAPI.Any("/nodes/*action", nodeProxy)
		adminAPI.Any("/proxy-chains", nodeProxy)
		adminAPI.Any("/proxy-chains/*action", nodeProxy)
		adminAPI.Any("/deployments", nodeProxy)
		adminAPI.Any("/deployments/*action", nodeProxy)
		adminAPI.Any("/health/*action", nodeProxy)
		adminAPI.Any("/runtimes", nodeProxy)
		adminAPI.Any("/runtimes/*action", nodeProxy)

		// node-service 新增路由（tasks 21/25/28-32 暴露的接口）
		adminAPI.Any("/tls-certificates", nodeProxy)
		adminAPI.Any("/tls-certificates/*action", nodeProxy)
		adminAPI.Any("/tls-profiles", nodeProxy)
		adminAPI.Any("/tls-profiles/*action", nodeProxy)
		adminAPI.Any("/config-imports", nodeProxy)
		adminAPI.Any("/config-imports/*action", nodeProxy)
		adminAPI.Any("/config-import", nodeProxy)
		adminAPI.Any("/config-import/*action", nodeProxy)
		adminAPI.Any("/protocol-registry", nodeProxy)
		adminAPI.Any("/protocol-registry/*action", nodeProxy)
		adminAPI.Any("/config-templates", nodeProxy)
		adminAPI.Any("/config-templates/*action", nodeProxy)
		adminAPI.Any("/protocol-presets", nodeProxy)
		adminAPI.Any("/protocol-presets/*action", nodeProxy)
		adminAPI.Any("/public/protocol-presets", nodeProxy)
		adminAPI.Any("/route-rule-sets", nodeProxy)
		adminAPI.Any("/route-rule-sets/*action", nodeProxy)
		adminAPI.Any("/route-policies", nodeProxy)
		adminAPI.Any("/route-policies/*action", nodeProxy)
		adminAPI.Any("/route-policy-rules", nodeProxy)
		adminAPI.Any("/route-policy-rules/*action", nodeProxy)
		adminAPI.Any("/node-groups", nodeProxy)
		adminAPI.Any("/node-groups/*action", nodeProxy)
		adminAPI.Any("/runtime-upgrades", nodeProxy)
		adminAPI.Any("/runtime-upgrades/*action", nodeProxy)
		adminAPI.Any("/warp-profiles", nodeProxy)
		adminAPI.Any("/warp-profiles/*action", nodeProxy)

		// node-service 新增路由（aidiag / channelhealth / experience）
		adminAPI.Any("/diagnosis", nodeProxy)
		adminAPI.Any("/diagnosis/*action", nodeProxy)
		adminAPI.Any("/channels", nodeProxy)
		adminAPI.Any("/channels/*action", nodeProxy)
		adminAPI.Any("/experience", nodeProxy)
		adminAPI.Any("/experience/*action", nodeProxy)

		// subscription-service 新增路由（tasks 28-32 客户端兼容矩阵）
		adminAPI.Any("/client-profiles", subscriptionProxy)
		adminAPI.Any("/client-compat-matrix", subscriptionProxy)
		adminAPI.Any("/client-compat-matrix/*action", subscriptionProxy)

		adminAPI.Any("/subscription-tokens", subscriptionProxy)
		adminAPI.Any("/subscription-tokens/*action", subscriptionProxy)
		adminAPI.Any("/subscription/templates", subscriptionProxy)
		adminAPI.Any("/subscription/templates/*action", subscriptionProxy)
		adminAPI.Any("/subscription/access-overview", subscriptionProxy)
		adminAPI.Any("/subscription/access-logs", subscriptionProxy)
		adminAPI.Any("/subscribe/templates", subscriptionProxy)
		adminAPI.Any("/subscribe/templates/*action", subscriptionProxy)
		adminAPI.Any("/subscribe", subscriptionProxy)

		adminAPI.Any("/traffic/*action", trafficProxy)
	}

	// agent 路由：流量上报路由到 traffic-service，其余路由到 node-service
	// Gin 不允许同一层级同时存在静态段和 catch-all，因此使用单一 catch-all + 前缀分发
	r.Any("/api/v1/agent/*action", func(c *gin.Context) {
		action := c.Param("action")
		if len(action) >= 9 && action[:9] == "/traffic/" {
			trafficProxy(c)
		} else {
			nodeProxy(c)
		}
	})

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		logger.Info(fmt.Sprintf("starting %s", svccfg.ServiceName), "addr", srv.Addr)
		logger.Info("downstream services",
			"identity", identityTarget,
			"node", nodeTarget,
			"subscription", subscriptionTarget,
			"traffic", trafficTarget,
		)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server failed to start", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info("shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("server forced to shutdown", "error", err)
	}
	logger.Info("server exited")
}
