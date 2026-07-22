package transport

import (
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	pb "github.com/airport-panel/proto/agent/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
)

type GRPCChannel struct {
	mu        sync.Mutex
	addr      string
	token     string
	conn      *grpc.ClientConn
	stream    pb.AgentChannel_StreamClient
	recvCh    chan *pb.PanelMessage
	sendMu    sync.Mutex
	connected bool
	stopCh    chan struct{}
	connEpoch uint64
}

func NewGRPCChannel(addr, token string) *GRPCChannel {
	return &GRPCChannel{
		addr:   addr,
		token:  token,
		recvCh: make(chan *pb.PanelMessage, 64),
	}
}

func (g *GRPCChannel) Name() string  { return "grpc" }
func (g *GRPCChannel) Priority() int { return 0 }
func (g *GRPCChannel) IsConnected() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.connected
}

func (g *GRPCChannel) Connect(ctx context.Context) error {
	g.mu.Lock()
	if g.stopCh != nil {
		select {
		case <-g.stopCh:
		default:
			close(g.stopCh)
		}
	}
	g.connected = false
	g.stopCh = make(chan struct{})
	epoch := atomic.AddUint64(&g.connEpoch, 1)
	g.mu.Unlock()

	kacp := keepalive.ClientParameters{
		Time:                20 * time.Second,
		Timeout:             5 * time.Second,
		PermitWithoutStream: true,
	}

	conn, err := grpc.DialContext(ctx, g.addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(kacp),
	)
	if err != nil {
		return fmt.Errorf("grpc dial: %w", err)
	}

	client := pb.NewAgentChannelClient(conn)
	md := metadata.New(map[string]string{
		"authorization": "Bearer " + g.token,
	})
	streamCtx := metadata.NewOutgoingContext(context.Background(), md)
	stream, err := client.Stream(streamCtx)
	if err != nil {
		conn.Close()
		return fmt.Errorf("grpc stream: %w", err)
	}

	g.mu.Lock()
	g.conn = conn
	g.stream = stream
	g.connected = true
	stopCh := g.stopCh
	recvCh := g.recvCh
	g.mu.Unlock()

	go g.recvLoop(epoch, stream, stopCh, recvCh)

	return nil
}

func (g *GRPCChannel) Send(msg *pb.AgentMessage) error {
	g.sendMu.Lock()
	defer g.sendMu.Unlock()
	g.mu.Lock()
	stream := g.stream
	connected := g.connected
	g.mu.Unlock()
	if !connected || stream == nil {
		return ErrNotConnected
	}
	if err := stream.Send(msg); err != nil {
		return fmt.Errorf("grpc send: %w", err)
	}
	return nil
}

func (g *GRPCChannel) Recv() (<-chan *pb.PanelMessage, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.connected {
		return nil, ErrNotConnected
	}
	return g.recvCh, nil
}

func (g *GRPCChannel) HealthCheck(ctx context.Context, timeout time.Duration) error {
	g.mu.Lock()
	stream := g.stream
	connected := g.connected
	g.mu.Unlock()
	if !connected || stream == nil {
		return ErrHealthCheckFailed
	}

	pingSeq := time.Now().UnixNano()
	ping := &pb.AgentMessage{
		Seq:       pingSeq,
		Timestamp: time.Now().UnixMilli(),
		Payload:   &pb.AgentMessage_Ping{Ping: &pb.Ping{Timestamp: time.Now().UnixMilli()}},
	}

	sendCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	g.sendMu.Lock()
	err := stream.Send(ping)
	g.sendMu.Unlock()
	if err != nil {
		return fmt.Errorf("%w: ping send: %v", ErrHealthCheckFailed, err)
	}

	type recvResult struct {
		msg *pb.PanelMessage
		err error
	}
	resultCh := make(chan recvResult, 1)

	go func() {
		msg, err := stream.Recv()
		resultCh <- recvResult{msg, err}
	}()

	select {
	case result := <-resultCh:
		if result.err != nil {
			return fmt.Errorf("%w: pong recv: %v", ErrHealthCheckFailed, result.err)
		}
		if result.msg.GetPong() == nil {
			return nil
		}
		pong := result.msg.GetPong()
		if pong.PingTimestamp == 0 {
			return nil
		}
		now := time.Now().UnixMilli()
		rtt := now - pong.PingTimestamp
		if rtt < 0 || rtt > timeout.Milliseconds()*2 {
			return fmt.Errorf("%w: rtt %dms out of range", ErrHealthCheckFailed, rtt)
		}
		return nil
	case <-sendCtx.Done():
		return fmt.Errorf("%w: %v", ErrHealthCheckFailed, sendCtx.Err())
	}
}

func (g *GRPCChannel) Close() error {
	g.mu.Lock()
	if !g.connected {
		g.mu.Unlock()
		return nil
	}
	g.connected = false
	if g.stopCh != nil {
		select {
		case <-g.stopCh:
		default:
			close(g.stopCh)
		}
	}
	stream := g.stream
	conn := g.conn
	g.mu.Unlock()

	if stream != nil {
		stream.CloseSend()
	}
	if conn != nil {
		conn.Close()
	}
	return nil
}

func (g *GRPCChannel) markDisconnected(epoch uint64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.connEpoch != epoch {
		return
	}
	g.connected = false
}

func (g *GRPCChannel) recvLoop(epoch uint64, stream pb.AgentChannel_StreamClient, stopCh chan struct{}, recvCh chan *pb.PanelMessage) {
	defer func() {
		g.markDisconnected(epoch)
	}()
	for {
		select {
		case <-stopCh:
			return
		default:
		}
		msg, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				return
			}
			select {
			case <-stopCh:
				return
			default:
				time.Sleep(100 * time.Millisecond)
				// Check if epoch changed (reconnected) before continuing
				g.mu.Lock()
				curEpoch := g.connEpoch
				g.mu.Unlock()
				if curEpoch != epoch {
					return
				}
				continue
			}
		}
		select {
		case recvCh <- msg:
		case <-stopCh:
			return
		default:
		}
	}
}

var _ Channel = (*GRPCChannel)(nil)
