package app

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/airport-panel/config"
	"github.com/airport-panel/config/db"
	"github.com/airport-panel/config/events"
	"github.com/airport-panel/config/redis"
	"github.com/airport-panel/config/server"
	"github.com/gin-gonic/gin"

	svccfg "github.com/airport-panel/identity-service/internal/config"
	"github.com/airport-panel/identity-service/internal/handler"
	"github.com/airport-panel/identity-service/internal/middleware"
	"github.com/airport-panel/identity-service/internal/pkg"
	"github.com/airport-panel/identity-service/internal/repo"
	"github.com/airport-panel/identity-service/internal/service"
)

func Run() {
	cfg := svccfg.Load()
	logger := config.NewLogger(svccfg.ServiceName, cfg.LogLevel)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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

	jwtManager := pkg.NewJWTManager(cfg.JWT.Secret, cfg.JWT.AccessTTLSeconds, cfg.JWT.RefreshTTLSeconds)

	userRepo := repo.NewUserRepo(pool)
	adminRepo := repo.NewAdminRepo(pool)
	authRepo := repo.NewAuthRepo(pool)
	roleRepo := repo.NewRoleRepo(pool)
	auditRepo := repo.NewAuditRepo(pool)
	settingRepo := repo.NewSettingRepo(pool)
	planRepo := repo.NewPlanRepo(pool)
	userProfileRepo := repo.NewUserProfileRepo(pool)
	subscriptionRepo := repo.NewSubscriptionRepo(pool)
	paymentOrderRepo := repo.NewPaymentOrderRepo(pool)
	subscriptionTokenRepo := repo.NewSubscriptionTokenRepo(pool)
	couponRepo := repo.NewCouponRepo(pool)
	commissionLogRepo := repo.NewCommissionLogRepo(pool)
	inviteCodeRepo := repo.NewInviteCodeRepo(pool)
	commissionWithdrawRepo := repo.NewCommissionWithdrawRepo(pool)
	// Phase 6: 工单 / 公告 / 通知
	ticketRepo := repo.NewTicketRepo(pool)
	announcementRepo := repo.NewAnnouncementRepo(pool)
	notificationRepo := repo.NewNotificationRepo(pool)
	notificationTemplateRepo := repo.NewNotificationTemplateRepo(pool)
	knowledgeRepo := repo.NewKnowledgeRepo(pool)
	mailTemplateRepo := repo.NewMailTemplateRepo(pool)

	authService := service.NewAuthService(userRepo, adminRepo, authRepo, jwtManager)
	rbacService := service.NewRBACService(roleRepo)
	auditService := service.NewAuditService(auditRepo)
	settingService := service.NewSettingService(settingRepo)
	adminService := service.NewAdminService(userRepo, adminRepo)
	mailService := service.NewMailService(mailTemplateRepo, logger)
	// 通知服务提前创建，便于注入到支付/工单/公告等业务 service
	notificationService := service.NewNotificationService(notificationRepo, notificationTemplateRepo)
	notificationService.SetLogger(logger)
	// 佣金服务提前创建并注入支付订单仓库，用于结算时校验订单状态；
	// 同时作为支付定时任务佣金结算的统一入口（CheckPendingCommissions）。
	commissionService := service.NewCommissionService(commissionWithdrawRepo, userRepo, commissionLogRepo, inviteCodeRepo, settingRepo, paymentOrderRepo)
	commissionService.SetLogger(logger)
	paymentService := service.NewPaymentService(
		paymentOrderRepo, planRepo, userRepo, subscriptionRepo, subscriptionTokenRepo, settingRepo,
		couponRepo, commissionLogRepo, mailService, auditService, notificationService, commissionService, logger,
	)
	planService := service.NewPlanService(planRepo, logger)
	couponService := service.NewCouponService(couponRepo, logger)
	// 注入支付订单仓库，使 CouponService.ValidateCoupon 能执行 NewUserOnly 校验
	couponService.SetPaymentOrderRepo(paymentOrderRepo)
	userService := service.NewUserService(
		userRepo, userProfileRepo, subscriptionRepo, planRepo, subscriptionTokenRepo,
		paymentOrderRepo, settingRepo, inviteCodeRepo, auditService, mailService, redisClient, logger,
	)

	var eventBus *events.Bus
	if redisClient != nil {
		eventBus = events.NewBus(redisClient, logger)
	} else {
		eventBus = events.NewNopBus(logger)
	}
	userService.SetEventPublisher(func(ctx context.Context, topic string, payload interface{}) {
		_ = eventBus.Publish(ctx, topic, payload)
	})
	paymentService.SetEventPublisher(func(ctx context.Context, topic string, payload interface{}) {
		_ = eventBus.Publish(ctx, topic, payload)
	})
	// 启动时加载 SMTP 配置、站点信息和邮件模板缓存
	userService.RefreshMailConfig(ctx)
	// Phase 6
	ticketService := service.NewTicketService(ticketRepo, userRepo, notificationService)
	announcementService := service.NewAnnouncementService(announcementRepo, notificationService)
	// 流量/到期提醒服务（依赖通知服务发送站内信）
	trafficReminderService := service.NewTrafficReminderService(subscriptionRepo, userRepo, notificationService, logger)
	knowledgeService := service.NewKnowledgeService(knowledgeRepo)

	authMiddleware := middleware.NewAuthMiddleware(jwtManager, authService)
	rbacMiddleware := middleware.NewRBACMiddleware(rbacService, adminRepo)

	healthHandler := handler.NewHealthHandler()
	authHandler := handler.NewAuthHandler(authService)
	adminAuthHandler := handler.NewAdminAuthHandler(authService)
	adminHandler := handler.NewAdminHandler(adminService, userService)
	userHandler := handler.NewUserHandler(userService, paymentService)
	auditHandler := handler.NewAuditHandler(auditService)
	settingHandler := handler.NewSettingHandler(settingService)
	// Phase 6
	ticketHandler := handler.NewTicketHandler(ticketService)
	announcementHandler := handler.NewAnnouncementHandler(announcementService)
	notificationHandler := handler.NewNotificationHandler(notificationService)
	knowledgeHandler := handler.NewKnowledgeHandler(knowledgeService)
	adminPlanHandler := handler.NewAdminPlanHandler(planService)
	adminCouponHandler := handler.NewAdminCouponHandler(couponService)
	couponValidateHandler := handler.NewCouponValidateHandler(couponService)
	adminOrderHandler := handler.NewAdminOrderHandler(paymentOrderRepo, userService)
	adminPaymentHandler := handler.NewAdminPaymentHandler(settingRepo, paymentService)
	adminCommissionHandler := handler.NewAdminCommissionHandler(commissionService)
	userExtrasHandler := handler.NewUserExtrasHandler(userService, ticketService, notificationService, commissionService)
	adminMailHandler := handler.NewAdminMailHandler(mailService)
	verifyHandler := handler.NewVerifyHandler(userService)

	opts := server.DefaultOptions(svccfg.ServiceName, cfg.Port)
	opts.Logger = logger
	opts.RegisterRoutes = func(api *gin.RouterGroup) {
		healthHandler.RegisterRoutes(api)

		// 公开套餐接口
		plans := api.Group("/plans")
		{
			plans.GET("", userHandler.ListPlans)
			plans.GET("/:id", userHandler.GetPlan)
			plans.GET("/:id/nodes", userExtrasHandler.ListPlanNodes)
		}

		// 用户端优惠券校验（需认证，用于下单前预校验折扣金额）
		coupons := api.Group("/coupons")
		coupons.Use(authMiddleware.UserAuth())
		{
			coupons.POST("/validate", couponValidateHandler.Validate)
		}

		// 用户认证接口（公开）
		userAuth := api.Group("/user/auth")
		{
			userAuth.POST("/register", userHandler.Register)
			userAuth.POST("/login", authHandler.Login)
			userAuth.GET("/verify-email", userHandler.VerifyEmail)
			userAuth.POST("/forgot-password", userHandler.ForgotPassword)
			userAuth.POST("/reset-password", userHandler.ResetPassword)
		}

		// 原有auth路由（兼容）
		auth := api.Group("/auth")
		{
			auth.POST("/register", userHandler.Register)
			auth.POST("/login", authHandler.Login)
			auth.POST("/refresh", authHandler.Refresh)
			// 邮箱验证相关（POST 接口，使用邮件模板系统）
			auth.POST("/verify-email", verifyHandler.VerifyEmail)
			auth.POST("/forgot-password", verifyHandler.ForgotPassword)
			auth.POST("/reset-password", verifyHandler.ResetPassword)
			auth.Use(authMiddleware.UserAuth())
			auth.POST("/logout", authHandler.Logout)
		}

		api.GET("/me", authMiddleware.UserAuth(), authHandler.GetMe)

		// 用户自助接口（需认证）
		user := api.Group("/user")
		user.Use(authMiddleware.UserAuth())
		{
			user.GET("/me", userHandler.GetMe)
			user.PATCH("/me", userHandler.UpdateMe)

			userSub := user.Group("/subscription")
			{
				userSub.GET("", userHandler.GetSubscription)
				userSub.GET("/tokens", userHandler.ListSubscriptionTokens)
				userSub.POST("/tokens", userHandler.CreateSubscriptionToken)
				userSub.DELETE("/tokens/:id", userHandler.RevokeSubscriptionToken)
				userSub.POST("/tokens/:id/reset", userHandler.ResetSubscriptionToken)
				userSub.POST("/reset", userHandler.ResetAllSubscriptionTokens)
				userSub.POST("/token/ensure", userExtrasHandler.EnsureSubscriptionToken)
			}

			userOrders := user.Group("/orders")
			{
				userOrders.GET("", userHandler.ListOrders)
				userOrders.POST("", userHandler.CreateOrder)
				userOrders.GET("/:id", userHandler.GetOrder)
			}

			// 用户端支付方式列表
			user.GET("/payment-methods", userHandler.ListPaymentMethods)

			// 用户可见节点列表（基于活跃订阅套餐）
			user.GET("/nodes", userExtrasHandler.ListMyNodes)

			userTickets := user.Group("/tickets")
			{
				userTickets.GET("/:id", userExtrasHandler.GetMyTicket)
				userTickets.GET("/:id/replies", userExtrasHandler.ListMyTicketReplies)
				userTickets.POST("/:id/replies", userExtrasHandler.AddMyTicketReply)
			}

			user.GET("/preferences", userExtrasHandler.GetPreferences)
			user.PUT("/preferences", userExtrasHandler.UpdatePreferences)

			userCommissions := user.Group("/commissions")
			{
				userCommissions.POST("/withdraw", userExtrasHandler.RequestWithdraw)
				userCommissions.GET("/withdrawals", userExtrasHandler.ListMyWithdrawals)
				userCommissions.GET("/summary", userExtrasHandler.CommissionSummary)
				userCommissions.GET("/details", userExtrasHandler.ListMyCommissionDetails)
			}

			// 邀请明细：被邀请用户列表
			user.GET("/invitations", userExtrasHandler.ListMyInvitations)
		}

		// 管理员认证
		adminAuth := api.Group("/admin/auth")
		{
			adminAuth.POST("/login", adminAuthHandler.Login)
			adminAuth.Use(authMiddleware.AdminAuth())
			adminAuth.POST("/logout", adminAuthHandler.Logout)
		}

		// 管理员接口
		admin := api.Group("/admin")
		admin.Use(authMiddleware.AdminAuth())
		{
			admin.GET("/me", adminAuthHandler.GetMe)

			// 用户管理
			adminUsers := admin.Group("/users")
			{
				adminUsers.GET("", rbacMiddleware.RequirePermission("users.read"), adminHandler.AdminListUsers)
				adminUsers.POST("", rbacMiddleware.RequirePermission("users.write"), adminHandler.AdminCreateUser)
				adminUsers.POST("/batch/ban", rbacMiddleware.RequirePermission("users.write"), adminHandler.AdminBatchBan)
				adminUsers.POST("/batch/unban", rbacMiddleware.RequirePermission("users.write"), adminHandler.AdminBatchUnban)
				adminUsers.POST("/batch/reset-traffic", rbacMiddleware.RequirePermission("users.write"), adminHandler.AdminBatchResetTraffic)
				adminUsers.POST("/batch/delete", rbacMiddleware.RequirePermission("users.write"), adminHandler.AdminBatchDelete)
				adminUsers.GET("/:id", rbacMiddleware.RequirePermission("users.read"), adminHandler.AdminGetUser)
				adminUsers.PATCH("/:id", rbacMiddleware.RequirePermission("users.write"), adminHandler.AdminUpdateUser)
				adminUsers.POST("/:id/ban", rbacMiddleware.RequirePermission("users.write"), adminHandler.AdminBanUser)
				adminUsers.POST("/:id/unban", rbacMiddleware.RequirePermission("users.write"), adminHandler.AdminUnbanUser)
				adminUsers.POST("/:id/reset-password", rbacMiddleware.RequirePermission("users.write"), adminHandler.AdminResetPassword)
				adminUsers.POST("/:id/reset-traffic", rbacMiddleware.RequirePermission("users.write"), adminHandler.AdminResetTraffic)
				adminUsers.POST("/:id/add-traffic", rbacMiddleware.RequirePermission("users.write"), adminHandler.AdminAddTraffic)
				adminUsers.POST("/:id/extend", rbacMiddleware.RequirePermission("users.write"), adminHandler.AdminExtendSubscription)
				adminUsers.POST("/:id/change-plan", rbacMiddleware.RequirePermission("users.write"), adminHandler.AdminChangePlan)
				adminUsers.POST("/:id/subscription/reset", rbacMiddleware.RequirePermission("users.write"), adminHandler.AdminResetSubscriptionTokens)
				adminUsers.DELETE("/:id", rbacMiddleware.RequirePermission("users.write"), adminHandler.AdminDeleteUser)
				adminUsers.POST("/:id/impersonate", rbacMiddleware.RequirePermission("users.write"), adminHandler.AdminImpersonate)
			}

			adminPlanHandler.RegisterRoutesWithGroup(admin, rbacMiddleware)
			adminCouponHandler.RegisterRoutesWithGroup(admin, rbacMiddleware)
			adminOrderHandler.RegisterRoutesWithGroup(admin)
			adminPaymentHandler.RegisterRoutesWithGroup(admin)
			adminCommissionHandler.RegisterRoutesWithGroup(admin, rbacMiddleware)
			knowledgeHandler.RegisterRoutesWithGroup(admin, rbacMiddleware)

			admin.POST("/admins", rbacMiddleware.RequireSuperAdmin(), adminHandler.CreateAdmin)
			admin.GET("/admins", rbacMiddleware.RequirePermission("admins.read"), adminHandler.ListAdmins)
			admin.GET("/audit-logs", rbacMiddleware.RequirePermission("audit.read"), auditHandler.ListAuditLogs)

			adminSettings := admin.Group("/system/settings")
			{
				adminSettings.GET("", rbacMiddleware.RequirePermission("system.read"), settingHandler.GetSettings)
				adminSettings.PUT("/:group/:key", rbacMiddleware.RequirePermission("system.write"), settingHandler.UpdateSetting)
			}

			// ===== Phase 6: 工单 / 公告 / 通知 =====
			// 工单
			adminTickets := admin.Group("/tickets")
			{
				adminTickets.GET("", rbacMiddleware.RequirePermission("tickets.read"), ticketHandler.ListTickets)
				adminTickets.GET("/stats", rbacMiddleware.RequirePermission("tickets.read"), ticketHandler.Stats)
				adminTickets.POST("", rbacMiddleware.RequirePermission("tickets.write"), ticketHandler.AdminCreateTicket)
				adminTickets.GET("/:id", rbacMiddleware.RequirePermission("tickets.read"), ticketHandler.GetTicket)
				adminTickets.PATCH("/:id", rbacMiddleware.RequirePermission("tickets.write"), ticketHandler.UpdateTicket)
				adminTickets.POST("/:id/assign", rbacMiddleware.RequirePermission("tickets.write"), ticketHandler.AssignTicket)
				adminTickets.POST("/:id/replies", rbacMiddleware.RequirePermission("tickets.write"), ticketHandler.AddReply)
				adminTickets.GET("/:id/replies", rbacMiddleware.RequirePermission("tickets.read"), ticketHandler.ListReplies)
			}

			// 公告
			adminAnnouncements := admin.Group("/announcements")
			{
				adminAnnouncements.GET("", rbacMiddleware.RequirePermission("announcements.read"), announcementHandler.List)
				adminAnnouncements.GET("/stats", rbacMiddleware.RequirePermission("announcements.read"), announcementHandler.Stats)
				adminAnnouncements.POST("", rbacMiddleware.RequirePermission("announcements.write"), announcementHandler.Create)
				adminAnnouncements.GET("/:id", rbacMiddleware.RequirePermission("announcements.read"), announcementHandler.Get)
				adminAnnouncements.PATCH("/:id", rbacMiddleware.RequirePermission("announcements.write"), announcementHandler.Update)
				adminAnnouncements.POST("/:id/publish", rbacMiddleware.RequirePermission("announcements.write"), announcementHandler.Publish)
				adminAnnouncements.POST("/:id/archive", rbacMiddleware.RequirePermission("announcements.write"), announcementHandler.Archive)
				adminAnnouncements.DELETE("/:id", rbacMiddleware.RequirePermission("announcements.write"), announcementHandler.Delete)
				adminAnnouncements.POST("/:id/read", rbacMiddleware.RequirePermission("announcements.read"), announcementHandler.MarkRead)
			}

			// 通知
			adminNotifications := admin.Group("/notifications")
			{
				adminNotifications.GET("", rbacMiddleware.RequirePermission("notifications.read"), notificationHandler.List)
				adminNotifications.POST("", rbacMiddleware.RequirePermission("notifications.write"), notificationHandler.Create)
				adminNotifications.GET("/:id", rbacMiddleware.RequirePermission("notifications.read"), notificationHandler.GetByID)
				adminNotifications.DELETE("/:id", rbacMiddleware.RequirePermission("notifications.write"), notificationHandler.Delete)
				adminNotifications.POST("/:id/read", rbacMiddleware.RequirePermission("notifications.write"), notificationHandler.AdminMarkRead)
				adminNotifications.POST("/:id/archive", rbacMiddleware.RequirePermission("notifications.write"), notificationHandler.AdminArchive)
			}

			// 通知模板
			adminNotifyTemplates := admin.Group("/notification-templates")
			{
				adminNotifyTemplates.GET("", rbacMiddleware.RequirePermission("notifications.read"), notificationHandler.ListTemplates)
				adminNotifyTemplates.GET("/:code", rbacMiddleware.RequirePermission("notifications.read"), notificationHandler.GetTemplate)
				adminNotifyTemplates.PUT("/:code", rbacMiddleware.RequirePermission("notifications.write"), notificationHandler.UpsertTemplate)
				adminNotifyTemplates.PATCH("/:code/enabled", rbacMiddleware.RequirePermission("notifications.write"), notificationHandler.SetTemplateEnabled)
				adminNotifyTemplates.DELETE("/:code", rbacMiddleware.RequirePermission("notifications.write"), notificationHandler.DeleteTemplate)
			}

			// 邮件模板管理
			adminMail := admin.Group("/mail")
			{
				adminMail.GET("/templates", rbacMiddleware.RequirePermission("system.read"), adminMailHandler.ListTemplates)
				adminMail.PUT("/templates/:id", rbacMiddleware.RequirePermission("system.write"), adminMailHandler.UpdateTemplate)
				adminMail.POST("/templates/reload", rbacMiddleware.RequirePermission("system.write"), adminMailHandler.ReloadCache)
				adminMail.POST("/test", rbacMiddleware.RequirePermission("system.write"), adminMailHandler.SendTestMail)
				adminMail.POST("/send", rbacMiddleware.RequirePermission("system.write"), adminMailHandler.SendMail)
			}
		}

		// ===== 用户端路由（/me/...）兼容原有 =====
		me := api.Group("/me")
		me.Use(authMiddleware.UserAuth())
		{
			me.GET("/tickets", ticketHandler.ListMyTickets)
			me.POST("/tickets", ticketHandler.CreateMyTicket)
			me.GET("/notifications", notificationHandler.ListMyNotifications)
			me.GET("/notifications/unread-count", notificationHandler.UnreadCount)
			me.POST("/notifications/read-all", notificationHandler.MarkAllMyRead)
			me.POST("/notifications/:id/read", notificationHandler.MarkMyRead)
			me.POST("/notifications/:id/archive", notificationHandler.ArchiveMy)
			// 用户端公告（参考 XBoard /api/v1/user/notice/fetch）
			me.GET("/announcements", announcementHandler.ListForUser)
			me.GET("/announcements/:id", announcementHandler.GetForUser)
			me.POST("/announcements/:id/read", announcementHandler.MarkRead)
			knowledgeHandler.RegisterUserRoutes(me)
			me.GET("/invite-code", userExtrasHandler.GetMyInviteCode)
			me.GET("/traffic-logs", userExtrasHandler.GetMyTrafficLogs)
			// 用户可见节点列表（基于活跃订阅套餐）——前端调用 /api/v1/me/nodes
			me.GET("/nodes", userExtrasHandler.ListMyNodes)
			// 邀请明细列表（被邀请用户列表，分页）——对齐 xboard /user/invite/details
			me.GET("/invitations", userExtrasHandler.ListMyInvitations)
		}
	}

	srv := server.New(opts)

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		logger.Info("shutting down services...")
		paymentService.Stop()
		eventBus.Stop()
		cancel()
	}()

	go paymentService.StartPolling(ctx)
	if err := eventBus.Start(ctx); err != nil {
		logger.Error("failed to start event bus", "error", err)
	}

	// ===== 定时任务调度注册 =====
	// 1. 佣金结算：每小时一次。由 PaymentService.StartPolling 内的 commissionSettleLoop 调用
	//    CommissionService.CheckPendingCommissions（已统一佣金结算入口，见 processDailyCommissionSettle）。
	// 2. 通知调度：每 5 分钟处理已到调度时间的 scheduled 通知。
	runScheduled(ctx, logger, 5*time.Minute, "scheduled notifications", notificationService.ProcessScheduledNotifications)
	// 3. 套餐即将到期提醒：每小时扫描到期前 3 天内的订阅（模板 plan_expiry）。
	runScheduled(ctx, logger, time.Hour, "plan expiry reminder", trafficReminderService.CheckPlanExpiry)
	// 4. 流量即将耗尽提醒：每小时扫描已用流量 > 80% 的订阅（模板 traffic_warning）。
	runScheduled(ctx, logger, time.Hour, "traffic warning reminder", trafficReminderService.CheckTrafficWarning)

	srv.Start()
}

// runScheduled 周期性执行一个返回 error 的定时任务。
// 立即执行一次，随后按 interval 周期执行；任务 panic 会被 recover，不影响后续调度。
// 当 ctx 取消时退出。
func runScheduled(ctx context.Context, logger *slog.Logger, interval time.Duration, name string, fn func(context.Context) error) {
	go func() {
		// 启动后立即执行一次，再进入周期调度
		execOnce(ctx, logger, name, fn)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				logger.Info("scheduled task stopped", "name", name)
				return
			case <-ticker.C:
				execOnce(ctx, logger, name, fn)
			}
		}
	}()
}

func execOnce(ctx context.Context, logger *slog.Logger, name string, fn func(context.Context) error) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("scheduled task panic", "name", name, "panic", r)
		}
	}()
	if err := fn(ctx); err != nil {
		logger.Error("scheduled task failed", "name", name, "error", err)
	}
}
