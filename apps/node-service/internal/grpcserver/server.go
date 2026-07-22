package grpcserver

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/airport-panel/node-service/internal/pkg"
	pb "github.com/airport-panel/proto/agent/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

type ServerConfig struct {
	Port         string
	TLSCertFile  string
	TLSKeyFile   string
	TokenSalt    string
	Logger       *slog.Logger
	OnMessage    func(ctx context.Context, machineID string, msg *pb.AgentMessage) (*pb.PanelMessage, error)
	LogStore     *LogStore
	NonceCache   *pkg.NonceCache
}

// StartGRPCServer 启动 gRPC 服务并返回 *grpc.Server 与 *AgentServer
//
// 返回 *AgentServer 是为了让调用方（app.go）能通过 PushToMachine 下发
// MaintenanceCommand / ConfigPush 等面板→agent 指令（AI 诊断自动修复、通道切换等）。
func StartGRPCServer(cfg *ServerConfig) (*grpc.Server, *AgentServer, error) {
	opts := []grpc.ServerOption{
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle:     0,
			MaxConnectionAge:      0,
			MaxConnectionAgeGrace: 0,
			Time:                  20 * time.Second,
			Timeout:               5 * time.Second,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             10 * time.Second,
			PermitWithoutStream: true,
		}),
		grpc.MaxRecvMsgSize(16 * 1024 * 1024),
		grpc.MaxSendMsgSize(16 * 1024 * 1024),
	}

	if cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
		cert, err := tls.LoadX509KeyPair(cfg.TLSCertFile, cfg.TLSKeyFile)
		if err != nil {
			return nil, nil, fmt.Errorf("load TLS cert: %w", err)
		}
		opts = append(opts, grpc.Creds(credentials.NewTLS(&tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		})))
	} else {
		opts = append(opts, grpc.Creds(insecure.NewCredentials()))
	}

	srv := grpc.NewServer(opts...)
	logStore := cfg.LogStore
	if logStore == nil {
		logStore = NewLogStore()
	}
	agentSrv := NewAgentServer(cfg.Logger, cfg.TokenSalt, cfg.OnMessage, logStore, cfg.NonceCache)
	pb.RegisterAgentChannelServer(srv, agentSrv)

	addr := ":" + cfg.Port
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, nil, fmt.Errorf("listen gRPC %s: %w", addr, err)
	}

	go func() {
		cfg.Logger.Info("gRPC server starting", "addr", addr, "tls", cfg.TLSCertFile != "")
		if err := srv.Serve(lis); err != nil {
			cfg.Logger.Error("gRPC server error", "error", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cfg.Logger.Info("gRPC server shutting down")
		srv.GracefulStop()
	}()

	return srv, agentSrv, nil
}
