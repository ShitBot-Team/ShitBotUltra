package minecraft

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	bridge "github.com/ShitBot-Team/ShitBotUltra/api/bridge/v1"
	"github.com/ShitBot-Team/ShitBotUltra/internal/config"
	"github.com/ShitBot-Team/ShitBotUltra/internal/fault"
	"github.com/ShitBot-Team/ShitBotUltra/internal/securefs"
	"github.com/ShitBot-Team/ShitBotUltra/internal/storage"
	"github.com/ShitBot-Team/ShitBotUltra/sdk/adapter"
	"github.com/coder/websocket"
)

const maxMessage = 3 << 20

type Handler interface {
	HandleGame(context.Context, config.Node, bridge.Envelope) (any, error)
	GameConnected(context.Context, config.Node, string)
	GameDisconnected(context.Context, config.Node, string)
	GameHeartbeat(context.Context, config.Node, string)
}

type NodeStatus struct {
	ID           string    `json:"id"`
	NetworkID    string    `json:"network_id"`
	ServerID     string    `json:"server_id"`
	Connected    bool      `json:"connected"`
	BootID       string    `json:"boot_id,omitempty"`
	Platform     string    `json:"platform,omitempty"`
	Capabilities []string  `json:"capabilities"`
	Since        time.Time `json:"since,omitempty"`
}

type Hub struct {
	ctx        context.Context
	db         storage.Database
	nodes      map[string]config.Node
	tokens     map[string]string
	handler    Handler
	mu         sync.RWMutex
	clients    map[string]*session
	connecting map[string]bool
	closed     bool
	wg         sync.WaitGroup
}

type session struct {
	hub              *Hub
	node             config.Node
	hello            bridge.Hello
	conn             *websocket.Conn
	ctx              context.Context
	cancel           context.CancelFunc
	since            time.Time
	sequence         atomic.Uint64
	writeMu          sync.Mutex
	mu               sync.Mutex
	pending          map[string]chan bridge.Envelope
	queue            chan bridge.Envelope
	cursor           uint64
	snapshotRequired bool
}

func New(ctx context.Context, db storage.Database, nodes []config.Node, tokens map[string]string, handler Handler) *Hub {
	h := &Hub{ctx: ctx, db: db, nodes: map[string]config.Node{}, tokens: tokens, handler: handler, clients: map[string]*session{}, connecting: map[string]bool{}}
	for _, n := range nodes {
		h.nodes[n.ID] = n
	}
	return h
}

func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || r.Header.Get("Origin") != "" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	node, ok := h.nodes[r.Header.Get("X-SBU-Node-ID")]
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") || subtle.ConstantTimeCompare([]byte(token), []byte(h.tokens[node.ID])) != 1 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	h.mu.Lock()
	if h.closed || h.clients[node.ID] != nil || h.connecting[node.ID] {
		h.mu.Unlock()
		http.Error(w, "node already connected or shutting down", http.StatusConflict)
		return
	}
	h.connecting[node.ID] = true
	h.wg.Add(1)
	h.mu.Unlock()
	defer h.wg.Done()
	defer func() { h.mu.Lock(); delete(h.connecting, node.ID); h.mu.Unlock() }()
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{CompressionMode: websocket.CompressionDisabled})
	if err != nil {
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(maxMessage)
	ctx, cancel := context.WithCancel(h.ctx)
	defer cancel()
	stop := context.AfterFunc(ctx, func() { _ = conn.CloseNow() })
	defer stop()
	handshakeCtx, handshakeCancel := context.WithTimeout(ctx, 10*time.Second)
	_, data, err := conn.Read(handshakeCtx)
	handshakeCancel()
	if err != nil {
		return
	}
	var first bridge.Envelope
	var hello bridge.Hello
	if json.Unmarshal(data, &first) != nil || json.Unmarshal(first.Payload, &hello) != nil || first.ProtocolVersion != bridge.Version || first.Kind != "control" || first.Type != "hello" || first.NodeID != node.ID || first.BootID != hello.BootID || hello.ProtocolVersion != bridge.Version || hello.NodeID != node.ID || hello.NetworkID != node.NetworkID || hello.ServerID != node.ServerID || !adapter.ValidID(hello.BootID) || len(hello.Capabilities) > 64 {
		_ = conn.Close(websocket.StatusPolicyViolation, "invalid handshake")
		return
	}
	if !adapter.ValidID(first.MessageID) || first.Sequence != 0 || first.OccurredAt.IsZero() || !adapter.ValidID(hello.Platform) || !adapter.ValidID(hello.AdapterVersion) || !adapter.ValidID(hello.MinecraftVersion) {
		_ = conn.Close(websocket.StatusPolicyViolation, "invalid node metadata")
		return
	}
	for _, capability := range hello.Capabilities {
		if !adapter.ValidID(capability) {
			_ = conn.Close(websocket.StatusPolicyViolation, "invalid capability")
			return
		}
	}
	s := &session{hub: h, node: node, hello: hello, conn: conn, ctx: ctx, cancel: cancel, since: time.Now().UTC(), pending: map[string]chan bridge.Envelope{}, queue: make(chan bridge.Envelope, 64), snapshotRequired: node.SessionAuthority}
	err = h.db.QueryRowContext(ctx, "SELECT sequence FROM bridge_cursors WHERE node_id=? AND boot_id=?", node.ID, hello.BootID).Scan(&s.cursor)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return
	}
	welcome := bridge.Welcome{ProtocolVersion: bridge.Version, SessionID: securefs.ID(), ServerTime: time.Now().UTC(), HeartbeatSeconds: 20, MaxMessageBytes: maxMessage, ResumeSequence: s.cursor, SnapshotRequired: s.snapshotRequired}
	if err := s.write(ctx, "control", "welcome", "", welcome, nil); err != nil {
		return
	}
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.clients[node.ID] = s
	h.mu.Unlock()
	h.handler.GameConnected(ctx, node, hello.BootID)
	defer func() {
		cancel()
		_ = conn.CloseNow()
		h.mu.Lock()
		if h.clients[node.ID] == s {
			delete(h.clients, node.ID)
		}
		h.mu.Unlock()
		cleanupCtx, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		h.handler.GameDisconnected(cleanupCtx, node, hello.BootID)
	}()
	var workers sync.WaitGroup
	workers.Add(2)
	go func() { defer workers.Done(); s.process() }()
	go func() { defer workers.Done(); s.heartbeat() }()
	s.read()
	cancel()
	_ = conn.CloseNow()
	workers.Wait()
}

func (s *session) write(ctx context.Context, kind, typ, requestID string, payload any, remoteErr *bridge.Error) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	e := bridge.Envelope{ProtocolVersion: bridge.Version, Kind: kind, Type: typ, MessageID: securefs.ID(), RequestID: requestID, NodeID: s.node.ID, BootID: s.hello.BootID, OccurredAt: time.Now().UTC(), Payload: data, Error: remoteErr}
	return s.writeEnvelope(ctx, e)
}

func (s *session) writeEnvelope(ctx context.Context, e bridge.Envelope) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	e.Sequence = s.sequence.Add(1)
	if e.Sequence > bridge.MaxSequence {
		return fault.New("SESSION_EXPIRED", "连接序号已耗尽")
	}
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if len(data) > maxMessage {
		return fault.New("INVALID_INPUT", "Bridge 消息过大")
	}
	writeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return s.conn.Write(writeCtx, websocket.MessageText, data)
}

func (s *session) read() {
	var last uint64
	for {
		kind, data, err := s.conn.Read(s.ctx)
		if err != nil {
			return
		}
		if kind != websocket.MessageText {
			return
		}
		var e bridge.Envelope
		if json.Unmarshal(data, &e) != nil || e.ProtocolVersion != bridge.Version || e.NodeID != s.node.ID || e.BootID != s.hello.BootID || !adapter.ValidID(e.MessageID) || !adapter.ValidID(e.Type) || len(e.Type) > 128 || e.OccurredAt.IsZero() || e.Sequence == 0 || e.Sequence > bridge.MaxSequence || e.Sequence <= last {
			return
		}
		last = e.Sequence
		if e.Kind == "response" {
			s.mu.Lock()
			ch := s.pending[e.RequestID]
			s.mu.Unlock()
			if ch != nil {
				select {
				case ch <- e:
				default:
				}
			}
			continue
		}
		if e.Kind != "event" && e.Kind != "request" {
			return
		}
		if e.Kind == "request" && (!adapter.ValidID(e.RequestID) || e.Deadline == nil || e.TimeoutMS < 1 || e.TimeoutMS > 30000) {
			s.respondError(e, fault.New("INVALID_INPUT", "请求缺少有效期限"))
			continue
		}
		select {
		case s.queue <- e:
		case <-s.ctx.Done():
			return
		default:
			if e.Kind == "request" {
				s.respondError(e, fault.New("BUSY", "节点请求队列已满"))
			} else {
				return
			}
		}
	}
}

func (s *session) respondError(e bridge.Envelope, err error) {
	public := fault.Public(err)
	_ = s.write(s.ctx, "response", e.Type, e.RequestID, nil, &bridge.Error{Code: public.Code, Message: public.Message})
}

func (s *session) process() {
	for {
		select {
		case <-s.ctx.Done():
			return
		case e := <-s.queue:
			if e.Kind == "event" && e.Sequence <= s.cursor {
				_ = s.write(s.ctx, "control", "ack", "", map[string]uint64{"sequence": s.cursor}, nil)
				continue
			}
			if e.Kind == "event" && s.snapshotRequired && e.Type != "players.snapshot" {
				_ = s.write(s.ctx, "control", "snapshot.required", "", nil, nil)
				s.cancel()
				return
			}
			timeout := 15 * time.Second
			if e.Kind == "request" {
				// A bounded skew allowance is additional to, not a replacement for,
				// the receiver's monotonic timeout budget.
				remaining := time.Until(*e.Deadline) + 30*time.Second
				if remaining <= 0 {
					s.respondError(e, fault.New("DEADLINE_EXCEEDED", "请求已过期"))
					continue
				}
				timeout = min(time.Duration(e.TimeoutMS)*time.Millisecond, remaining)
			}
			ctx, cancel := context.WithTimeout(s.ctx, timeout)
			value, err := s.hub.handler.HandleGame(ctx, s.node, e)
			cancel()
			if e.Kind == "request" {
				if err != nil {
					s.respondError(e, err)
				} else {
					_ = s.write(s.ctx, "response", e.Type, e.RequestID, value, nil)
				}
				continue
			}
			if err != nil {
				_ = s.write(s.ctx, "control", "event.rejected", "", map[string]string{"message_id": e.MessageID, "code": fault.Public(err).Code}, nil)
				s.cancel()
				return
			}
			_, err = s.hub.db.ExecContext(s.ctx, `INSERT INTO bridge_cursors(node_id,boot_id,sequence,updated_at) VALUES(?,?,?,?) ON CONFLICT(node_id,boot_id) DO UPDATE SET sequence=MAX(sequence,excluded.sequence),updated_at=excluded.updated_at`, s.node.ID, s.hello.BootID, e.Sequence, time.Now().UnixMilli())
			if err != nil {
				s.cancel()
				return
			}
			s.cursor = e.Sequence
			if e.Type == "players.snapshot" {
				s.snapshotRequired = false
			}
			if err := s.write(s.ctx, "control", "ack", "", map[string]uint64{"sequence": s.cursor}, nil); err != nil {
				s.cancel()
				return
			}
		}
	}
}

func (s *session) heartbeat() {
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
			err := s.conn.Ping(ctx)
			if err == nil {
				s.hub.handler.GameHeartbeat(ctx, s.node, s.hello.BootID)
			}
			cancel()
			if err != nil {
				s.cancel()
				return
			}
		}
	}
}

// Request never retries side effects after write, timeout or disconnect. A lost
// response has an unknown outcome and must not be presented as a safe retry.
func (h *Hub) Request(ctx context.Context, nodeID, capability string, payload any) (json.RawMessage, error) {
	h.mu.RLock()
	s := h.clients[nodeID]
	h.mu.RUnlock()
	if s == nil {
		return nil, fault.New("NODE_OFFLINE", "游戏节点离线")
	}
	if !slices.Contains(s.node.Actions, capability) {
		return nil, fault.New("FORBIDDEN", "节点配置未允许该动作")
	}
	if !slices.Contains(s.hello.Capabilities, capability) {
		return nil, fault.New("UNSUPPORTED_CAPABILITY", "游戏节点不支持该动作")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	id := securefs.ID()
	ch := make(chan bridge.Envelope, 1)
	s.mu.Lock()
	if len(s.pending) >= 64 {
		s.mu.Unlock()
		return nil, fault.New("BUSY", "游戏动作队列已满")
	}
	s.pending[id] = ch
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.pending, id); s.mu.Unlock() }()
	deadline, _ := ctx.Deadline()
	e := bridge.Envelope{ProtocolVersion: bridge.Version, Kind: "request", Type: capability, MessageID: securefs.ID(), RequestID: id, NodeID: nodeID, BootID: s.hello.BootID, OccurredAt: time.Now().UTC(), Deadline: &deadline, TimeoutMS: max(1, time.Until(deadline).Milliseconds()), IdempotencyKey: id, Payload: data}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err := s.writeEnvelope(ctx, e); err != nil {
		return nil, fault.New("RESULT_UNKNOWN", "游戏动作可能已执行，未自动重试")
	}
	select {
	case response := <-ch:
		if response.Type != capability {
			return nil, fault.New("RESULT_UNKNOWN", "游戏动作响应类型不匹配")
		}
		if response.Error != nil {
			return nil, fault.New(response.Error.Code, response.Error.Message)
		}
		return response.Payload, nil
	case <-ctx.Done():
		return nil, fault.New("RESULT_UNKNOWN", "游戏动作超时，执行结果未知")
	case <-s.ctx.Done():
		return nil, fault.New("RESULT_UNKNOWN", "游戏节点断开，执行结果未知")
	}
}

func (h *Hub) Status() []NodeStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()
	result := make([]NodeStatus, 0, len(h.nodes))
	for _, n := range h.nodes {
		status := NodeStatus{ID: n.ID, NetworkID: n.NetworkID, ServerID: n.ServerID, Capabilities: []string{}}
		if s := h.clients[n.ID]; s != nil {
			status.Connected = true
			status.BootID = s.hello.BootID
			status.Platform = s.hello.Platform
			status.Capabilities = append([]string{}, s.hello.Capabilities...)
			status.Since = s.since
		}
		result = append(result, status)
	}
	slices.SortFunc(result, func(a, b NodeStatus) int { return strings.Compare(a.ID, b.ID) })
	return result
}

func (h *Hub) Close() {
	h.mu.Lock()
	h.closed = true
	for _, s := range h.clients {
		s.cancel()
	}
	h.mu.Unlock()
	h.wg.Wait()
}
