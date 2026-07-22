package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/airport-panel/config"
	"github.com/airport-panel/config/auth"
	"github.com/airport-panel/config/db"
	"github.com/airport-panel/config/events"
	"github.com/airport-panel/config/redis"
	"github.com/airport-panel/config/server"
	"github.com/gin-gonic/gin"

	svccfg "github.com/airport-panel/traffic-service/internal/config"
	"github.com/airport-panel/traffic-service/internal/handler"
	"github.com/airport-panel/traffic-service/internal/middleware"
	"github.com/airport-panel/traffic-service/internal/pkg"
	"github.com/airport-panel/traffic-service/internal/repo"
	"github.com/airport-panel/traffic-service/internal/service"
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

	redisClient, err := redis.NewClient(cfg.Redis)
	if err != nil {
		logger.Warn("failed to connect to redis, continuing without redis", "error", err)
		redisClient = nil
	} else {
		defer redisClient.Close()
	}

	jwtManager := pkg.NewJWTManager(cfg.JWT.Secret)

	trafficRepo := repo.NewTrafficRepo(pool)
	sessionRepo := repo.NewSessionRepo(pool)
	credentialRepo := repo.NewUserNodeCredentialRepo(pool)

	trafficService := service.NewTrafficService(trafficRepo, sessionRepo, credentialRepo, redisClient)
	deviceStateService := service.NewDeviceStateService(redisClient, logger)

	authMiddleware := middleware.NewAuthMiddleware(jwtManager)
	agentAuthMiddleware := middleware.NewAgentAuthMiddleware(cfg.AgentAPITokenSalt)
	nonceStore := auth.NewMemoryNonceStore()
	defer nonceStore.Stop()
	hmacMiddleware := auth.SignatureGinMiddleware(cfg.HMACSecret, nonceStore)

	healthHandler := handler.NewHealthHandler()
	agentHandler := handler.NewAgentHandler(trafficService)
	trafficHandler := handler.NewTrafficHandler(trafficService)
	deviceStateHandler := handler.NewDeviceStateHandler(deviceStateService)

	opts := server.DefaultOptions(svccfg.ServiceName, cfg.Port)
	opts.Logger = logger
	opts.RegisterRoutes = func(api *gin.RouterGroup) {
		healthHandler.RegisterRoutes(api)

		agentRoutes := api.Group("/agent")
		agentRoutes.Use(hmacMiddleware)
		agentRoutes.Use(agentAuthMiddleware.AgentAuth())
		{
			agentRoutes.POST("/traffic/report", agentHandler.ReportTraffic)
			// 设备态聚合：Agent 上报/查询/清除跨节点在线设备
			deviceStateHandler.RegisterRoutes(agentRoutes)
		}

		userRoutes := api.Group("/user")
		userRoutes.Use(authMiddleware.UserAuth())
		{
			userRoutes.GET("/traffic", trafficHandler.GetMyTraffic)
		}

		adminRoutes := api.Group("/admin")
		adminRoutes.Use(authMiddleware.AdminAuth())
		{
			adminRoutes.GET("/traffic/overview", trafficHandler.GetOverview)
			adminRoutes.GET("/traffic/user/:id", trafficHandler.GetUserTraffic)
			adminRoutes.GET("/traffic/user/:id/quota", trafficHandler.CheckUserQuota)
			adminRoutes.POST("/traffic/user/:id/reset", trafficHandler.ResetUserTraffic)
		}
	}

	srv := server.New(opts)

	var eventBus *events.Bus
	if redisClient != nil {
		eventBus = events.NewBus(redisClient, logger)
	} else {
		eventBus = events.NewNopBus(logger)
	}

	// 注入 logger 与 eventBus 到 trafficService（供定时任务记录日志与发布超额事件）
	trafficService.SetLogger(logger)
	trafficService.SetEventBus(eventBus)

	type userPayload struct {
		UserID string `json:"user_id"`
	}
	eventBus.Subscribe(events.TopicUserBanned, func(evt events.Event) {
		var p userPayload
		_ = json.Unmarshal(evt.Data, &p)
		logger.Info("user banned, disconnecting sessions", "user", p.UserID)
		if redisClient != nil && p.UserID != "" {
			onlineKey := fmt.Sprintf("online:user:%s", p.UserID)
			_ = redisClient.Del(context.Background(), onlineKey).Err()
		}
	})

	busCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := eventBus.Start(busCtx); err != nil {
		logger.Error("failed to start event bus", "error", err)
	}

	// 启动流量/订阅自动化定时任务（检查超额、过期、日汇总、月度重置）
	go trafficService.StartScheduledJobs(busCtx)

	// 流量提醒邮件定时任务：每天 11:30 检查流量使用超过 80% 的用户并发送告警邮件
	trafficReminderService := service.NewTrafficReminderService(trafficRepo, nil, logger)
	trafficReminderService.StartScheduledJobs(busCtx)

	// 每日流量统计汇总：每天 23:55 归档当日数据到 traffic_statistics_daily 表
	statisticsService := service.NewStatisticsService(trafficRepo, logger)
	statisticsService.StartScheduledJobs(busCtx)

	// P3-N: 节点级流量限额检查定时任务：每 5 分钟检查节点累计流量是否超过 transfer_enable_bytes
	nodeTrafficQuotaService := service.NewNodeTrafficQuotaService(pool, logger)
	nodeTrafficQuotaService.StartScheduledJobs(busCtx)

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		logger.Info("shutting down traffic-service...")
		eventBus.Stop()
		cancel()
	}()

	srv.Start()
}
