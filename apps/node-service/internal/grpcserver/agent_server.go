package grpcserver

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/airport-panel/node-service/internal/metrics"
	"github.com/airport-panel/node-service/internal/pkg"
	pb "github.com/airport-panel/proto/agent/v1"
)

type AgentServer struct {
	pb.UnimplementedAgentChannelServer
	mu         sync.RWMutex
	sessions   map[string]*agentSession
	logger     *slog.Logger
	tokenSalt  string
	onMessage  func(ctx context.Context, machineID string, msg *pb.AgentMessage) (*pb.PanelMessage, error)
	logStore   *LogStore
	nonceCache *pkg.NonceCache
}

type agentSession struct {
	machineID  string
	stream     pb.AgentChannel_StreamServer
	sendMu     sync.Mutex
	connected  time.Time
	lastActive time.Time
}

func NewAgentServer(logger *slog.Logger, tokenSalt string, onMessage func(ctx context.Context, machineID string, msg *pb.AgentMessage) (*pb.PanelMessage, error), logStore *LogStore, nonceCache *pkg.NonceCache) *AgentServer {
	if logStore == nil {
		logStore = NewLogStore()
	}
	return &AgentServer{
		sessions:   make(map[string]*agentSession),
		logger:     logger.With("component", "grpc-agent"),
		tokenSalt:  tokenSalt,
		onMessage:  onMessage,
		logStore:   logStore,
		nonceCache: nonceCache,
	}
}

func (s *AgentServer) LogStore() *LogStore {
	return s.logStore
}

func (s *AgentServer) Stream(stream pb.AgentChannel_StreamServer) error {
	ctx := stream.Context()

	authMsg, err := stream.Recv()
	if err != nil {
		return fmt.Errorf("recv auth: %w", err)
	}
	auth := authMsg.GetAuth()
	if auth == nil {
		return fmt.Errorf("first message must be AuthRequest")
	}

	if !s.verifyAuth(auth) {
		sendErr := stream.Send(&pb.PanelMessage{
			Seq:       1,
			Timestamp: time.Now().UnixMilli(),
			Payload: &pb.PanelMessage_AuthAck{AuthAck: &pb.AuthAck{
				Ok:    false,
				Error: "invalid token or signature",
			}},
		})
		if sendErr != nil {
			return sendErr
		}
		return fmt.Errorf("auth failed for machine %s", auth.MachineId)
	}

	session := &agentSession{
		machineID:  auth.MachineId,
		stream:     stream,
		connected:  time.Now(),
		lastActive: time.Now(),
	}

	s.mu.Lock()
	s.sessions[auth.MachineId] = session
	s.mu.Unlock()
	metrics.GRPCAgentConnections.Inc()

	s.logger.Info("agent connected via gRPC", "machine_id", auth.MachineId, "version", auth.AgentVersion)

	authAck := &pb.PanelMessage{
		Seq:       1,
		Timestamp: time.Now().UnixMilli(),
		Payload: &pb.PanelMessage_AuthAck{AuthAck: &pb.AuthAck{
			Ok:               true,
			SessionToken:     s.generateSessionToken(auth.MachineId),
			SessionExpiresAt: time.Now().Add(1 * time.Hour).Unix(),
			ServerId:         auth.MachineId,
		}},
	}
	if err := s.sendToSession(session, authAck); err != nil {
		return err
	}

	defer func() {
		s.mu.Lock()
		delete(s.sessions, auth.MachineId)
		s.mu.Unlock()
		metrics.GRPCAgentConnections.Dec()
		s.logger.Info("agent disconnected from gRPC", "machine_id", auth.MachineId)
	}()

	go s.pingLoop(session)

	for {
		msg, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		session.lastActive = time.Now()

		if ping := msg.GetPing(); ping != nil {
			metrics.GRPCMessagesReceived.WithLabelValues("ping").Inc()
			pong := &pb.PanelMessage{
				Seq:       msg.Seq,
				Timestamp: time.Now().UnixMilli(),
				Payload: &pb.PanelMessage_Pong{Pong: &pb.Pong{
					Timestamp:     time.Now().UnixMilli(),
					PingTimestamp: ping.Timestamp,
				}},
			}
			if err := s.sendToSession(session, pong); err != nil {
				return err
			}
			continue
		}

		if logChunk := msg.GetLogChunk(); logChunk != nil {
			metrics.GRPCMessagesReceived.WithLabelValues("log_chunk").Inc()
			if entries := logChunk.GetEntries(); len(entries) > 0 {
				s.logStore.Append(auth.MachineId, entries...)
				s.logger.Debug("received log chunk", "machine_id", auth.MachineId, "entries", len(entries))
			}
			continue
		}

		metrics.GRPCMessagesReceived.WithLabelValues(agentMessageType(msg)).Inc()

		if s.onMessage != nil {
			resp, err := s.onMessage(ctx, auth.MachineId, msg)
			if err != nil {
				s.logger.Error("handler error", "machine_id", auth.MachineId, "error", err)
				continue
			}
			if resp != nil {
				if resp.Seq == 0 {
					resp.Seq = msg.Seq
				}
				if resp.Timestamp == 0 {
					resp.Timestamp = time.Now().UnixMilli()
				}
				if err := s.sendToSession(session, resp); err != nil {
					return err
				}
			}
		}
	}
}

func (s *AgentServer) PushToMachine(machineID string, msg *pb.PanelMessage) error {
	s.mu.RLock()
	session, ok := s.sessions[machineID]
	s.mu.RUnlock()
	if !ok {
		return fmt.Errorf("machine %s not connected", machineID)
	}
	if msg.Timestamp == 0 {
		msg.Timestamp = time.Now().UnixMilli()
	}
	metrics.GRPCMessagesPushed.WithLabelValues(panelMessageType(msg)).Inc()
	return s.sendToSession(session, msg)
}

// agentMessageType 返回 AgentMessage 的消息类型字符串（用于 Prometheus 标签）
func agentMessageType(msg *pb.AgentMessage) string {
	switch msg.Payload.(type) {
	case *pb.AgentMessage_Auth:
		return "auth"
	case *pb.AgentMessage_Heartbeat:
		return "heartbeat"
	case *pb.AgentMessage_ConfigResult:
		return "config_result"
	case *pb.AgentMessage_Ping:
		return "ping"
	case *pb.AgentMessage_LogChunk:
		return "log_chunk"
	case *pb.AgentMessage_TrafficReport:
		return "traffic_report"
	default:
		return "unknown"
	}
}

// panelMessageType 返回 PanelMessage 的消息类型字符串（用于 Prometheus 标签）
func panelMessageType(msg *pb.PanelMessage) string {
	switch msg.Payload.(type) {
	case *pb.PanelMessage_AuthAck:
		return "auth_ack"
	case *pb.PanelMessage_HeartbeatAck:
		return "heartbeat_ack"
	case *pb.PanelMessage_ConfigPush:
		return "config_push"
	case *pb.PanelMessage_UserBan:
		return "user_ban"
	case *pb.PanelMessage_CertRenew:
		return "cert_renew"
	case *pb.PanelMessage_Pong:
		return "pong"
	case *pb.PanelMessage_Maintenance:
		return "maintenance"
	case *pb.PanelMessage_DeltaSync:
		return "delta_sync"
	default:
		return "unknown"
	}
}

func (s *AgentServer) sendToSession(sess *agentSession, msg *pb.PanelMessage) error {
	sess.sendMu.Lock()
	defer sess.sendMu.Unlock()
	return sess.stream.Send(msg)
}

func (s *AgentServer) pingLoop(sess *agentSession) {
	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()
	seq := int64(100)
	for {
		select {
		case <-sess.stream.Context().Done():
			return
		case <-ticker.C:
			seq++
			ping := &pb.PanelMessage{
				Seq:       seq,
				Timestamp: time.Now().UnixMilli(),
				Payload: &pb.PanelMessage_Pong{Pong: &pb.Pong{
					Timestamp: time.Now().UnixMilli(),
				}},
			}
			_ = s.sendToSession(sess, ping)
		}
	}
}

func (s *AgentServer) verifyAuth(auth *pb.AuthRequest) bool {
	if auth.MachineToken == "" || auth.MachineId == "" || auth.Nonce == "" {
		return false
	}
	expectedToken := s.computeAgentToken(auth.MachineId)
	if !hmac.Equal([]byte(expectedToken), []byte(auth.MachineToken)) {
		return false
	}
	payload := fmt.Sprintf("%s%s%d", auth.MachineId, auth.Nonce, auth.Timestamp)
	expectedSig := s.hmacSHA256(payload, auth.MachineToken)
	if !hmac.Equal([]byte(expectedSig), []byte(auth.Signature)) {
		s.logger.Warn("signature mismatch", "machine_id", auth.MachineId)
		return false
	}
	now := time.Now().Unix()
	if auth.Timestamp < now-300 || auth.Timestamp > now+300 {
		s.logger.Warn("timestamp out of range", "machine_id", auth.MachineId, "ts", auth.Timestamp, "now", now)
		return false
	}
	if s.nonceCache != nil {
		if err := s.nonceCache.CheckAndStore(auth.MachineId, auth.Nonce, auth.Timestamp); err != nil {
			s.logger.Warn("nonce replay or cache error", "machine_id", auth.MachineId, "nonce", auth.Nonce, "error", err)
			return false
		}
	}
	return true
}

func (s *AgentServer) computeAgentToken(machineID string) string {
	return s.hmacSHA256(machineID, s.tokenSalt)
}

func (s *AgentServer) hmacSHA256(msg, key string) string {
	h := hmac.New(sha256.New, []byte(key))
	h.Write([]byte(msg))
	return hex.EncodeToString(h.Sum(nil))
}

func (s *AgentServer) generateSessionToken(machineID string) string {
	ts := fmt.Sprintf("%d", time.Now().Unix())
	return s.hmacSHA256(machineID+":"+ts, s.tokenSalt)
}
