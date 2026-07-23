package handler

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/airport-panel/node-service/internal/grpcserver"
	"github.com/airport-panel/node-service/internal/middleware"
	"github.com/airport-panel/node-service/internal/model"
	"github.com/airport-panel/node-service/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	pb "github.com/airport-panel/proto/agent/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
	HandshakeTimeout: 10 * time.Second,
	ReadBufferSize:   1024,
	WriteBufferSize:  1024,
}

type WSHandler struct {
	logger            *slog.Logger
	logStore          *grpcserver.LogStore
	runtimeService    *service.RuntimeService
	healthService     *service.HealthService
	serverService     *service.ServerService
	deploymentService *service.DeploymentService
	mu                sync.RWMutex
	sessions          map[string]*wsSession
}

type wsSession struct {
	serverCode string
	conn       *websocket.Conn
	sendMu     sync.Mutex
	lastActive time.Time
}

func NewWSHandler(logger *slog.Logger, logStore *grpcserver.LogStore, runtimeService *service.RuntimeService, healthService *service.HealthService, serverService *service.ServerService, deploymentService *service.DeploymentService) *WSHandler {
	return &WSHandler{
		logger:            logger.With("component", "ws-handler"),
		logStore:          logStore,
		runtimeService:    runtimeService,
		healthService:     healthService,
		serverService:     serverService,
		deploymentService: deploymentService,
		sessions:          make(map[string]*wsSession),
	}
}

func (h *WSHandler) HandleWebSocket(c *gin.Context) {
	serverCode := middleware.GetAgentServerCode(c)
	if serverCode == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.logger.Error("websocket upgrade failed", "server_code", serverCode, "error", err)
		return
	}

	session := &wsSession{
		serverCode: serverCode,
		conn:       conn,
		lastActive: time.Now(),
	}

	h.mu.Lock()
	if oldSess, ok := h.sessions[serverCode]; ok {
		_ = oldSess.conn.Close()
	}
	h.sessions[serverCode] = session
	h.mu.Unlock()

	h.logger.Info("agent connected via WebSocket", "server_code", serverCode)

	defer func() {
		h.mu.Lock()
		if currentSess, ok := h.sessions[serverCode]; ok && currentSess == session {
			delete(h.sessions, serverCode)
		}
		h.mu.Unlock()
		_ = conn.Close()
		h.logger.Info("agent disconnected from WebSocket", "server_code", serverCode)
	}()

	conn.SetPongHandler(func(string) error {
		session.lastActive = time.Now()
		return nil
	})

	go h.pingLoop(session)

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				h.logger.Debug("websocket read error", "server_code", serverCode, "error", err)
			}
			break
		}

		session.lastActive = time.Now()

		var msg pb.AgentMessage
		if err := protojson.Unmarshal(data, &msg); err != nil {
			h.logger.Warn("invalid websocket message", "server_code", serverCode, "error", err)
			continue
		}

		if ping := msg.GetPing(); ping != nil {
			pong := &pb.PanelMessage{
				Seq:       msg.Seq,
				Timestamp: time.Now().UnixMilli(),
				Payload: &pb.PanelMessage_Pong{Pong: &pb.Pong{
					Timestamp:     time.Now().UnixMilli(),
					PingTimestamp: ping.Timestamp,
				}},
			}
			h.sendToSession(session, pong)
			continue
		}

		// 处理 Heartbeat 消息：复用与 HTTP heartbeat 相同的逻辑，
		// 比较 config version 并返回 HeartbeatAck（含 reload 指令）
		if hb := msg.GetHeartbeat(); hb != nil {
			h.handleWSHeartbeat(session, msg.Seq, hb)
			continue
		}

		// 处理 LogChunk 消息：将日志条目存入共享 LogStore，
		// 供 admin API GET /admin/servers/:id/logs 查询
		if logChunk := msg.GetLogChunk(); logChunk != nil {
			if entries := logChunk.GetEntries(); len(entries) > 0 && h.logStore != nil {
				h.logStore.Append(serverCode, entries...)
				h.logger.Debug("received log chunk via websocket",
					"server_code", serverCode, "entries", len(entries))
			}
			continue
		}
	}
}

// handleWSHeartbeat processes a Heartbeat message received via WebSocket,
// replicating the HTTP heartbeat handler logic: report health, compare config
// versions, and return a HeartbeatAck with the appropriate action.
func (h *WSHandler) handleWSHeartbeat(sess *wsSession, seq int64, hb *pb.Heartbeat) {
	serverCode := sess.serverCode

	currentVersion := strconv.FormatInt(hb.GetConfigVersion(), 10)
	hbReq := &model.AgentHeartbeatRequest{
		ServerCode:    serverCode,
		Timestamp:     time.Now(),
		ConfigVersion: currentVersion,
	}

	// 提取 ServerLoad (CPU/内存/磁盘/网络) 指标
	if load := hb.GetLoad(); load != nil {
		cpu := float64(load.GetCpuPercent())
		mem := float64(load.GetMemPercent())
		disk := float64(load.GetDiskPercent())
		netIn := float64(load.GetNetworkInKbps())
		netOut := float64(load.GetNetworkOutKbps())
		memTotal := load.GetMemTotalMb()
		memUsed := load.GetMemUsedMb()
		diskTotal := load.GetDiskTotalGb()
		diskUsed := load.GetDiskUsedGb()
		uptime := load.GetUptimeSeconds()
		hbReq.CPUPercent = &cpu
		hbReq.MemPercent = &mem
		hbReq.DiskPercent = &disk
		hbReq.Metrics = map[string]interface{}{
			"cpu_percent":       cpu,
			"mem_percent":       mem,
			"mem_total_mb":      memTotal,
			"mem_used_mb":       memUsed,
			"disk_percent":      disk,
			"disk_total_gb":     diskTotal,
			"disk_used_gb":      diskUsed,
			"network_in_kbps":   netIn,
			"network_out_kbps":  netOut,
			"uptime_seconds":    uptime,
			"load_1":            load.GetLoad_1(),
			"load_5":            load.GetLoad_5(),
			"load_15":           load.GetLoad_15(),
			"tcp_connections":   load.GetTcpConnections(),
			"active_streams":    load.GetActiveStreams(),
			"goroutines":        load.GetGoroutines(),
		}
	}

	// 提取 KernelInfo（版本、运行状态）
	if kernel := hb.GetKernel(); kernel != nil {
		hbReq.RuntimeVersion = kernel.GetVersion()
		if kernel.GetRunning() {
			hbReq.RuntimeStatus = "active"
		} else {
			hbReq.RuntimeStatus = "inactive"
		}
	}

	if ch := hb.GetChannel(); ch != nil {
		chState := "healthy"
		switch ch.GetState() {
		case pb.ChannelState_CHANNEL_STATE_DEGRADED:
			chState = "degraded"
		case pb.ChannelState_CHANNEL_STATE_UNHEALTHY:
			chState = "unhealthy"
		case pb.ChannelState_CHANNEL_STATE_UNKNOWN:
			chState = "unknown"
		}
		rttMs := int(ch.GetRttMs())
		lastErr := ch.GetLastError()
		hbReq.ChannelHealth = &model.ChannelHealthReport{
			ActiveChannel: "ws",
			ChannelState:  chState,
			RTTMs:         &rttMs,
			LastError:     &lastErr,
		}
	}

	if h.healthService != nil {
		_ = h.healthService.ReportHeartbeat(context.Background(), serverCode, nil, hbReq)
	}

	action := pb.HeartbeatAction_HEARTBEAT_ACTION_NONE
	latestVersion := int64(0)

	// Check if config needs reload
	if h.serverService != nil && h.runtimeService != nil && h.deploymentService != nil {
		serverSrv, err := h.serverService.GetServerByCode(context.Background(), serverCode)
		if err == nil && serverSrv != nil {
			providerType := model.RuntimeProviderNodeAgent
			rt, err := h.runtimeService.GetRuntimeByServerAndProvider(context.Background(), serverSrv.ID, providerType, nil)
			if err == nil && rt != nil {
				targetVersion, err := h.deploymentService.GetRuntimeConfig(context.Background(), rt.ID, "")
				if err == nil && targetVersion != nil {
					latestVersion = targetVersion.VersionNo
					if hb.GetConfigVersion() < targetVersion.VersionNo {
						action = pb.HeartbeatAction_HEARTBEAT_ACTION_RELOAD
					}
				}
			}
		}
	}

	ack := &pb.PanelMessage{
		Seq:       seq,
		Timestamp: time.Now().UnixMilli(),
		Payload: &pb.PanelMessage_HeartbeatAck{
			HeartbeatAck: &pb.HeartbeatAck{
				Action:              action,
				LatestConfigVersion: latestVersion,
				ServerTime:          time.Now().Unix(),
			},
		},
	}

	if err := h.sendToSession(sess, ack); err != nil {
		h.logger.Warn("failed to send heartbeat ack via ws", "server_code", serverCode, "error", err)
	}
}

func (h *WSHandler) sendToSession(sess *wsSession, msg *pb.PanelMessage) error {
	data, err := protojson.Marshal(msg)
	if err != nil {
		return err
	}

	sess.sendMu.Lock()
	defer sess.sendMu.Unlock()

	_ = sess.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	return sess.conn.WriteMessage(websocket.TextMessage, data)
}

func (h *WSHandler) pingLoop(sess *wsSession) {
	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if time.Since(sess.lastActive) > 60*time.Second {
				_ = sess.conn.Close()
				return
			}
			sess.sendMu.Lock()
			_ = sess.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(3*time.Second))
			sess.sendMu.Unlock()
		}
	}
}

func (h *WSHandler) PushToMachine(serverCode string, msg *pb.PanelMessage) error {
	h.mu.RLock()
	sess, ok := h.sessions[serverCode]
	h.mu.RUnlock()
	if !ok {
		// agent 未连接 WebSocket 时必须返回 error，
		// 否则 CompositeConfigPusher 会误认为推送成功，导致消息丢失。
		return fmt.Errorf("machine %s not connected via websocket", serverCode)
	}
	if msg.Timestamp == 0 {
		msg.Timestamp = time.Now().UnixMilli()
	}
	return h.sendToSession(sess, msg)
}
