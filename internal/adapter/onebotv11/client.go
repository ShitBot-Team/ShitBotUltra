package onebotv11

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ShitBot-Team/ShitBotUltra/internal/securefs"
	"github.com/ShitBot-Team/ShitBotUltra/sdk/adapter"
	"github.com/coder/websocket"
)

type Client struct {
	cfg     adapter.Config
	mu      sync.RWMutex
	session *session
	state   adapter.Status
	running atomic.Bool
	dropped atomic.Uint64
}

type session struct {
	conn    *websocket.Conn
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.Mutex
	pending map[string]chan response
	ready   atomic.Bool
}

type response struct {
	Status  string          `json:"status"`
	Retcode int             `json:"retcode"`
	Data    json.RawMessage `json:"data"`
	Echo    string          `json:"echo"`
}

func New(cfg adapter.Config) (adapter.Adapter, error) {
	u, err := url.Parse(cfg.Endpoint)
	if err != nil || u.Hostname() == "" || (u.Scheme != "ws" && u.Scheme != "wss") || u.User != nil || u.RawQuery != "" {
		return nil, errors.New("invalid OneBot endpoint")
	}
	if cfg.ID == "" || cfg.AccountID == "" || cfg.Token == "" {
		return nil, errors.New("OneBot requires ID, account ID and token")
	}
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 64
	}
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = 15 * time.Second
	}
	return &Client{cfg: cfg, state: adapter.Status{ID: cfg.ID, Type: "onebot.v11", State: "stopped", Capabilities: []string{"message.text", "message.image", "message.mention", "message.reply", "member.events"}, Since: time.Now().UTC()}}, nil
}

func (c *Client) setState(state, code string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.state.State = state
	c.state.LastError = code
	c.state.Since = time.Now().UTC()
}
func (c *Client) Status() adapter.Status {
	c.mu.RLock()
	defer c.mu.RUnlock()
	s := c.state
	s.Capabilities = append([]string{}, s.Capabilities...)
	s.Dropped = c.dropped.Load()
	return s
}

func (c *Client) Run(ctx context.Context, sink adapter.Sink) error {
	if !c.running.CompareAndSwap(false, true) {
		return errors.New("adapter already running")
	}
	defer c.running.Store(false)
	defer c.setState("stopped", "")
	backoff := time.Second
	for ctx.Err() == nil {
		c.setState("connecting", "")
		started := time.Now()
		err := c.connect(ctx, sink)
		if ctx.Err() != nil {
			return nil
		}
		code := "CONNECTION_LOST"
		if errors.Is(err, errAccount) {
			code = "ACCOUNT_MISMATCH"
		}
		c.setState("reconnecting", code)
		if time.Since(started) > time.Minute {
			backoff = time.Second
		}
		delay := backoff + time.Duration(rand.Int64N(int64(backoff/2)+1))
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
		backoff = min(backoff*2, 60*time.Second)
	}
	return nil
}

var errAccount = errors.New("OneBot account mismatch")

func (c *Client) connect(ctx context.Context, sink adapter.Sink) error {
	dialCtx, dialCancel := context.WithTimeout(ctx, 15*time.Second)
	conn, resp, err := websocket.Dial(dialCtx, c.cfg.Endpoint, &websocket.DialOptions{HTTPHeader: http.Header{"Authorization": []string{"Bearer " + c.cfg.Token}}, HTTPClient: &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirect refused") }}, CompressionMode: websocket.CompressionDisabled})
	dialCancel()
	if err != nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		return err
	}
	conn.SetReadLimit(2 << 20)
	sctx, cancel := context.WithCancel(ctx)
	s := &session{conn: conn, ctx: sctx, cancel: cancel, pending: map[string]chan response{}}
	stop := context.AfterFunc(sctx, func() { _ = conn.CloseNow() })
	defer stop()
	c.mu.Lock()
	c.session = s
	c.mu.Unlock()
	defer func() {
		cancel()
		_ = conn.CloseNow()
		c.mu.Lock()
		if c.session == s {
			c.session = nil
		}
		c.mu.Unlock()
	}()
	done := make(chan error, 1)
	go func() { done <- c.read(s, sink); cancel() }()
	// Start the reader before requesting account information so response
	// correlation does not depend on the event consumer returning.
	data, err := c.action(sctx, s, "get_login_info", map[string]any{})
	if err == nil {
		var login struct {
			UserID opaqueID `json:"user_id"`
		}
		if json.Unmarshal(data, &login) != nil || string(login.UserID) != c.cfg.AccountID {
			err = errAccount
		}
	}
	if err != nil {
		cancel()
		_ = conn.CloseNow()
		<-done
		return err
	}
	s.ready.Store(true)
	c.setState("connected", "")
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case err := <-done:
			return err
		case <-ctx.Done():
			cancel()
			_ = conn.CloseNow()
			<-done
			return nil
		case <-ticker.C:
			pingCtx, pingCancel := context.WithTimeout(sctx, 10*time.Second)
			err := conn.Ping(pingCtx)
			pingCancel()
			if err != nil {
				cancel()
				_ = conn.CloseNow()
				<-done
				return err
			}
		}
	}
}

func (c *Client) read(s *session, sink adapter.Sink) error {
	for {
		typ, data, err := s.conn.Read(s.ctx)
		if err != nil {
			return err
		}
		if typ != websocket.MessageText {
			return errors.New("binary OneBot frame")
		}
		var header struct {
			PostType string          `json:"post_type"`
			Echo     json.RawMessage `json:"echo"`
		}
		if json.Unmarshal(data, &header) != nil {
			continue
		}
		if len(header.Echo) > 0 && header.PostType == "" {
			var result response
			if json.Unmarshal(data, &result) != nil {
				continue
			}
			s.mu.Lock()
			ch := s.pending[result.Echo]
			s.mu.Unlock()
			if ch != nil {
				select {
				case ch <- result:
				default:
				}
			}
			continue
		}
		if !s.ready.Load() {
			continue
		}
		e, ok := decodeEvent(c.cfg, data)
		if !ok {
			continue
		}
		if err := sink(s.ctx, e); err != nil {
			c.dropped.Add(1)
		}
	}
}

func (c *Client) action(ctx context.Context, s *session, action string, params any) (json.RawMessage, error) {
	if s.ctx.Err() != nil {
		return nil, adapter.ErrOffline
	}
	ctx, cancel := context.WithTimeout(ctx, c.cfg.RequestTimeout)
	defer cancel()
	id := securefs.ID()
	ch := make(chan response, 1)
	s.mu.Lock()
	if len(s.pending) >= c.cfg.QueueSize {
		s.mu.Unlock()
		return nil, adapter.ErrBusy
	}
	s.pending[id] = ch
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.pending, id); s.mu.Unlock() }()
	data, err := json.Marshal(map[string]any{"action": action, "params": params, "echo": id})
	if err != nil {
		return nil, err
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err := s.conn.Write(ctx, websocket.MessageText, data); err != nil {
		return nil, adapter.ErrUnknownResult
	}
	select {
	case result := <-ch:
		if result.Status != "ok" || result.Retcode != 0 {
			return nil, errors.New("OneBot rejected action")
		}
		return result.Data, nil
	case <-ctx.Done():
		return nil, adapter.ErrUnknownResult
	case <-s.ctx.Done():
		return nil, adapter.ErrUnknownResult
	}
}

func (c *Client) Send(ctx context.Context, target adapter.Conversation, message adapter.Message) (adapter.Receipt, error) {
	if err := target.Validate(); err != nil {
		return adapter.Receipt{}, err
	}
	if target.Platform != "qq.numeric" || target.Scope != "global" || (target.Kind != "group" && target.Kind != "private") {
		return adapter.Receipt{}, adapter.ErrUnsupported
	}
	if len(message.Segments) == 0 || len(message.Segments) > 64 {
		return adapter.Receipt{}, errors.New("invalid message segment count")
	}
	segments := make([]map[string]any, 0, len(message.Segments)+1)
	if message.ReplyTo != "" {
		segments = append(segments, map[string]any{"type": "reply", "data": map[string]string{"id": message.ReplyTo}})
	}
	size := 0
	mediaBytes := 0
	for _, seg := range message.Segments {
		typ := seg.Type
		data := map[string]string{}
		switch seg.Type {
		case "text":
			size += len(seg.Text)
			data["text"] = seg.Text
		case "image":
			if len(seg.Data) > 0 {
				mediaBytes += len(seg.Data)
				if mediaBytes > 16<<20 {
					return adapter.Receipt{}, errors.New("total image data too large")
				}
				data["file"] = "base64://" + base64.StdEncoding.EncodeToString(seg.Data)
				break
			}
			u, err := url.Parse(seg.URL)
			if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
				return adapter.Receipt{}, errors.New("invalid media URL")
			}
			data["file"] = seg.URL
		case "mention":
			if seg.Target == nil || seg.Target.Validate() != nil || seg.Target.Platform != "qq.numeric" || seg.Target.Scope != "global" {
				return adapter.Receipt{}, adapter.ErrUnsupported
			}
			typ = "at"
			data["qq"] = seg.Target.ID
		case "reply":
			data["id"] = seg.MessageID
		default:
			return adapter.Receipt{}, adapter.ErrUnsupported
		}
		segments = append(segments, map[string]any{"type": typ, "data": data})
	}
	if size > 16000 {
		return adapter.Receipt{}, errors.New("message text too long")
	}
	c.mu.RLock()
	s := c.session
	c.mu.RUnlock()
	if s == nil || !s.ready.Load() {
		return adapter.Receipt{}, adapter.ErrOffline
	}
	params := map[string]any{"message": segments, "auto_escape": true}
	action := "send_group_msg"
	if target.Kind == "group" {
		params["group_id"] = target.ID
	} else {
		action = "send_private_msg"
		params["user_id"] = target.ID
	}
	data, err := c.action(ctx, s, action, params)
	if err != nil {
		return adapter.Receipt{}, err
	}
	var result struct {
		MessageID opaqueID `json:"message_id"`
	}
	if json.Unmarshal(data, &result) != nil || result.MessageID == "" {
		return adapter.Receipt{}, adapter.ErrUnknownResult
	}
	return adapter.Receipt{MessageID: string(result.MessageID)}, nil
}

// A numeric wire ID is decoded without passing through float64.
type opaqueID string

func (id *opaqueID) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		return nil
	}
	if data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		*id = opaqueID(s)
		return nil
	}
	s := string(data)
	if strings.ContainsAny(s, ".eE+") {
		return errors.New("nonintegral identifier")
	}
	var n json.Number
	if err := json.Unmarshal(data, &n); err != nil {
		return err
	}
	*id = opaqueID(s)
	return nil
}
