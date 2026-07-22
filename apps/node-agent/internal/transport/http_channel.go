package transport

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/airport-panel/config/auth"
	pb "github.com/airport-panel/proto/agent/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

type HTTPChannel struct {
	mu         sync.Mutex
	baseURL    string
	token      string
	hmacSecret string
	client     *http.Client
	connected  bool
	recvCh     chan *pb.PanelMessage
	stopCh     chan struct{}
	lastSeq    int64
}

func NewHTTPChannel(baseURL, token, hmacSecret string) *HTTPChannel {
	return &HTTPChannel{
		baseURL:    baseURL,
		token:      token,
		hmacSecret: hmacSecret,
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:       10,
				IdleConnTimeout:    90 * time.Second,
				DisableCompression: false,
			},
		},
		recvCh: make(chan *pb.PanelMessage, 32),
		stopCh: make(chan struct{}),
	}
}

func (h *HTTPChannel) Name() string      { return "http" }
func (h *HTTPChannel) Priority() int     { return 2 }
func (h *HTTPChannel) IsConnected() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.connected
}

func (h *HTTPChannel) Connect(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.connected = true
	h.stopCh = make(chan struct{})
	return nil
}

func (h *HTTPChannel) generateNonce() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (h *HTTPChannel) Send(msg *pb.AgentMessage) error {
	h.mu.Lock()
	if !h.connected {
		h.mu.Unlock()
		return ErrNotConnected
	}
	h.mu.Unlock()

	data, err := protojson.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	path := "/api/v1/agent/messages"
	url := fmt.Sprintf("%s%s", h.baseURL, path)
	timestamp := time.Now().Unix()
	nonce := h.generateNonce()
	signature := auth.Sign("POST", path, string(data), timestamp, nonce, h.hmacSecret)

	req, err := http.NewRequest("POST", url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+h.token)
	req.Header.Set(auth.HeaderTimestamp, fmt.Sprintf("%d", timestamp))
	req.Header.Set(auth.HeaderNonce, nonce)
	req.Header.Set(auth.HeaderSignature, signature)

	resp, err := h.client.Do(req)
	if err != nil {
		return fmt.Errorf("http post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("http %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	var pm pb.PanelMessage
	if err := protojson.Unmarshal(body, &pm); err != nil {
		return fmt.Errorf("unmarshal response: %w", err)
	}

	if pm.Seq > 0 {
		select {
		case h.recvCh <- &pm:
		default:
		}
	}
	return nil
}

func (h *HTTPChannel) Recv() (<-chan *pb.PanelMessage, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.connected {
		return nil, ErrNotConnected
	}
	return h.recvCh, nil
}

func (h *HTTPChannel) HealthCheck(ctx context.Context, timeout time.Duration) error {
	path := "/api/v1/agent/health"
	url := fmt.Sprintf("%s%s", h.baseURL, path)
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	timestamp := time.Now().Unix()
	nonce := h.generateNonce()
	signature := auth.Sign("GET", path, "", timestamp, nonce, h.hmacSecret)

	req, err := http.NewRequestWithContext(reqCtx, "GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+h.token)
	req.Header.Set(auth.HeaderTimestamp, fmt.Sprintf("%d", timestamp))
	req.Header.Set(auth.HeaderNonce, nonce)
	req.Header.Set(auth.HeaderSignature, signature)

	resp, err := h.client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrHealthCheckFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: status %d", ErrHealthCheckFailed, resp.StatusCode)
	}
	return nil
}

func (h *HTTPChannel) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.connected {
		return nil
	}
	h.connected = false
	close(h.stopCh)
	return nil
}

var _ = json.Marshal
