package transport

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/airport-panel/config/auth"
	"github.com/gorilla/websocket"
	pb "github.com/airport-panel/proto/agent/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

type wsChannel struct {
	mu         sync.RWMutex
	baseURL    string
	nodeID     string
	token      string
	hmacSecret string
	conn       *websocket.Conn
	connected  bool
	recvCh     chan *pb.PanelMessage
	sendMu     sync.Mutex
	stopCh     chan struct{}
	pongCh     chan struct{}
	lastPong   time.Time
	connEpoch  uint64
}

func NewWSChannel(baseURL, nodeID, token, hmacSecret string) *wsChannel {
	return &wsChannel{
		baseURL:    baseURL,
		nodeID:     nodeID,
		token:      token,
		hmacSecret: hmacSecret,
	}
}

func (w *wsChannel) generateNonce() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (w *wsChannel) Name() string       { return "ws" }
func (w *wsChannel) Priority() int      { return 1 }
func (w *wsChannel) IsConnected() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.connected
}

func (w *wsChannel) Connect(ctx context.Context) error {
	w.mu.Lock()
	if w.stopCh != nil {
		select {
		case <-w.stopCh:
		default:
			close(w.stopCh)
		}
	}
	w.connected = false
	w.stopCh = make(chan struct{})
	w.pongCh = make(chan struct{}, 1)
	w.recvCh = make(chan *pb.PanelMessage, 64)
	w.lastPong = time.Now()
	epoch := atomic.AddUint64(&w.connEpoch, 1)
	w.mu.Unlock()

	wsURL := w.baseURL
	path := "/api/v1/agent/ws"
	if !strings.HasSuffix(wsURL, path) {
		wsURL = strings.TrimRight(wsURL, "/") + path
	}

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	timestamp := time.Now().Unix()
	nonce := w.generateNonce()
	signature := auth.Sign("GET", path, "", timestamp, nonce, w.hmacSecret)

	header := http.Header{}
	header.Set("Authorization", "Bearer "+w.token)
	header.Set("X-Server-Code", w.nodeID)
	header.Set("X-Agent-Token", w.token)
	header.Set(auth.HeaderTimestamp, fmt.Sprintf("%d", timestamp))
	header.Set(auth.HeaderNonce, nonce)
	header.Set(auth.HeaderSignature, signature)

	conn, _, err := dialer.DialContext(ctx, wsURL, header)
	if err != nil {
		return fmt.Errorf("ws dial: %w", err)
	}

	w.mu.Lock()
	w.conn = conn
	w.connected = true
	stopCh := w.stopCh
	pongCh := w.pongCh
	recvCh := w.recvCh
	w.mu.Unlock()

	conn.SetPongHandler(func(string) error {
		w.mu.Lock()
		w.lastPong = time.Now()
		w.mu.Unlock()
		select {
		case pongCh <- struct{}{}:
		default:
		}
		return nil
	})

	go w.readLoop(epoch, conn, stopCh, pongCh, recvCh)
	go w.pingLoop(epoch, conn, stopCh)

	return nil
}

func (w *wsChannel) Send(msg *pb.AgentMessage) error {
	w.sendMu.Lock()
	defer w.sendMu.Unlock()

	w.mu.RLock()
	conn := w.conn
	connected := w.connected
	w.mu.RUnlock()

	if !connected || conn == nil {
		return ErrNotConnected
	}

	data, err := protojson.Marshal(msg)
	if err != nil {
		return fmt.Errorf("ws marshal: %w", err)
	}

	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return fmt.Errorf("ws send: %w", err)
	}
	return nil
}

func (w *wsChannel) Recv() (<-chan *pb.PanelMessage, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if !w.connected {
		return nil, ErrNotConnected
	}
	return w.recvCh, nil
}

func (w *wsChannel) HealthCheck(ctx context.Context, timeout time.Duration) error {
	w.mu.RLock()
	conn := w.conn
	connected := w.connected
	pongCh := w.pongCh
	w.mu.RUnlock()

	if !connected || conn == nil {
		return ErrHealthCheckFailed
	}

	pingSeq := time.Now().UnixNano()
	ping := &pb.AgentMessage{
		Seq:       pingSeq,
		Timestamp: time.Now().UnixMilli(),
		Payload:   &pb.AgentMessage_Ping{Ping: &pb.Ping{Timestamp: time.Now().UnixMilli()}},
	}

	if err := w.Send(ping); err != nil {
		return fmt.Errorf("%w: ping send: %v", ErrHealthCheckFailed, err)
	}

	select {
	case <-pongCh:
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("%w: pong timeout", ErrHealthCheckFailed)
	case <-ctx.Done():
		return fmt.Errorf("%w: %v", ErrHealthCheckFailed, ctx.Err())
	}
}

func (w *wsChannel) Close() error {
	w.mu.Lock()
	if !w.connected {
		w.mu.Unlock()
		return nil
	}
	w.connected = false
	if w.stopCh != nil {
		select {
		case <-w.stopCh:
		default:
			close(w.stopCh)
		}
	}
	conn := w.conn
	w.mu.Unlock()

	if conn != nil {
		deadline := time.Now().Add(3 * time.Second)
		_ = conn.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
			deadline)
		conn.Close()
	}
	return nil
}

func (w *wsChannel) markDisconnected(epoch uint64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.connEpoch != epoch {
		return
	}
	w.connected = false
}

func (w *wsChannel) readLoop(epoch uint64, conn *websocket.Conn, stopCh chan struct{}, pongCh chan struct{}, recvCh chan *pb.PanelMessage) {
	defer func() {
		w.markDisconnected(epoch)
	}()

	for {
		select {
		case <-stopCh:
			return
		default:
		}

		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}

		var msg pb.PanelMessage
		if err := protojson.Unmarshal(data, &msg); err != nil {
			continue
		}

		if msg.GetPong() != nil {
			select {
			case pongCh <- struct{}{}:
			default:
			}
			continue
		}

		select {
		case recvCh <- &msg:
		case <-stopCh:
			return
		default:
		}
	}
}

func (w *wsChannel) pingLoop(epoch uint64, conn *websocket.Conn, stopCh chan struct{}) {
	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			w.mu.RLock()
			lastPong := w.lastPong
			curEpoch := w.connEpoch
			w.mu.RUnlock()

			if curEpoch != epoch {
				return
			}

			if time.Since(lastPong) > 30*time.Second {
				w.markDisconnected(epoch)
				return
			}

			w.sendMu.Lock()
			_ = conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(3*time.Second))
			w.sendMu.Unlock()
		}
	}
}

var _ Channel = (*wsChannel)(nil)
