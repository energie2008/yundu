package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/airport-panel/config"
	"github.com/airport-panel/config/middleware"
	"github.com/airport-panel/config/observability"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Server struct {
	engine      *gin.Engine
	httpServer  *http.Server
	logger      *slog.Logger
	serviceName string
}

type Options struct {
	ServiceName    string
	Port           string
	Version        string
	Logger         *slog.Logger
	Timeout        time.Duration
	CORS           middleware.CORSConfig
	RegisterRoutes func(r *gin.RouterGroup)
}

func DefaultOptions(serviceName, port string) Options {
	return Options{
		ServiceName: serviceName,
		Port:        port,
		Version:     "dev",
		Logger:      nil,
		Timeout:     30 * time.Second,
		CORS:        middleware.DefaultCORSConfig(),
	}
}

func New(opts Options) *Server {
	if opts.Logger == nil {
		opts.Logger = config.NewLogger(opts.ServiceName, "info")
	}
	if opts.Timeout == 0 {
		opts.Timeout = 30 * time.Second
	}
	if opts.Version == "" {
		opts.Version = "dev"
	}

	gin.SetMode(gin.ReleaseMode)
	if os.Getenv("APP_ENV") == "development" {
		gin.SetMode(gin.DebugMode)
	}

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("service_name", opts.ServiceName)
		c.Next()
	})
	r.Use(middleware.RequestID())
	r.Use(middleware.Recovery(opts.Logger))
	r.Use(observability.PrometheusMiddleware(opts.ServiceName))
	r.Use(middleware.AccessLog(opts.Logger, opts.ServiceName))
	r.Use(middleware.CORS(opts.CORS))
	r.Use(middleware.Timeout(opts.Timeout))

	observability.SetBuildInfo(opts.ServiceName, opts.Version, "unknown")

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": opts.ServiceName, "version": opts.Version})
	})
	r.GET("/readyz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ready", "service": opts.ServiceName})
	})
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	if opts.RegisterRoutes != nil {
		api := r.Group("/api/v1")
		opts.RegisterRoutes(api)
	}

	srv := &http.Server{
		Addr:              ":" + opts.Port,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}

	return &Server{
		engine:      r,
		httpServer:  srv,
		logger:      opts.Logger,
		serviceName: opts.ServiceName,
	}
}

func (s *Server) Engine() *gin.Engine {
	return s.engine
}

func (s *Server) Start() {
	// DUMP_ROUTES=1 时输出所有已注册路由到 JSON 文件并退出，不做网络监听
	if os.Getenv("DUMP_ROUTES") == "1" {
		s.dumpRoutes()
		return
	}

	go func() {
		s.logger.Info("starting server", "addr", s.httpServer.Addr)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("server failed to start", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	s.logger.Info("shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.httpServer.Shutdown(ctx); err != nil {
		s.logger.Error("server forced to shutdown", "error", err)
	}
	s.logger.Info("server exited")
}

// dumpRoutes 将所有已注册路由序列化为 JSON 写入 tmp/{serviceName}-routes.json
func (s *Server) dumpRoutes() {
	type routeInfo struct {
		Method string `json:"method"`
		Path   string `json:"path"`
	}
	var routes []routeInfo
	for _, info := range s.engine.Routes() {
		routes = append(routes, routeInfo{Method: info.Method, Path: info.Path})
	}
	data, err := json.MarshalIndent(routes, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshal routes failed: %v\n", err)
		os.Exit(1)
	}
	outDir := "tmp"
	_ = os.MkdirAll(outDir, 0755)
	outPath := fmt.Sprintf("%s/%s-routes.json", outDir, s.serviceName)
	if err := os.WriteFile(outPath, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "write routes file failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("dumped %d routes to %s\n", len(routes), outPath)
}
