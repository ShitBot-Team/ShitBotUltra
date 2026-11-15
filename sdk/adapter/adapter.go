// Package adapter defines the platform-neutral bot adapter API.
package adapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const APIVersion = 1

var (
	ErrOffline       = errors.New("adapter offline")
	ErrUnsupported   = errors.New("unsupported capability")
	ErrBusy          = errors.New("adapter capacity exceeded")
	ErrUnknownResult = errors.New("action result unknown; do not retry automatically")
)

// Identity IDs are opaque. A numeric QQ ID and an official QQ openid are
// different platforms, even if their string representations happen to match.
type Identity struct {
	Platform string `json:"platform" yaml:"platform"`
	Scope    string `json:"scope" yaml:"scope"`
	ID       string `json:"id" yaml:"id"`
}

func ValidID(s string) bool {
	return s != "" && len(s) <= 256 && utf8.ValidString(s) && strings.TrimSpace(s) == s && !strings.ContainsFunc(s, unicode.IsControl)
}

func (i Identity) Validate() error {
	if !ValidID(i.Platform) || !ValidID(i.Scope) || !ValidID(i.ID) {
		return errors.New("identity requires a platform, scope and opaque ID")
	}
	return nil
}

func (i Identity) Key() string { return i.Platform + "\x1f" + i.Scope + "\x1f" + i.ID }

type Conversation struct {
	Identity `yaml:",inline"`
	Kind     string `json:"kind" yaml:"kind"` // private, group, channel or thread
}

func (c Conversation) Validate() error {
	if err := c.Identity.Validate(); err != nil {
		return err
	}
	switch c.Kind {
	case "private", "group", "channel", "thread":
		return nil
	default:
		return fmt.Errorf("unknown conversation kind %q", c.Kind)
	}
}

func (c Conversation) Key() string { return c.Kind + "\x1f" + c.Identity.Key() }

type Segment struct {
	Type       string    `json:"type"`
	Text       string    `json:"text,omitempty"`
	URL        string    `json:"url,omitempty"`
	Target     *Identity `json:"target,omitempty"`
	MessageID  string    `json:"message_id,omitempty"`
	ResourceID string    `json:"resource_id,omitempty"`
	Data       []byte    `json:"-"`
}

type Message struct {
	Segments []Segment `json:"segments"`
	ReplyTo  string    `json:"reply_to,omitempty"`
}

func Text(text string) Message { return Message{Segments: []Segment{{Type: "text", Text: text}}} }

func (m Message) PlainText() string {
	var b strings.Builder
	for _, s := range m.Segments {
		if s.Type == "text" {
			b.WriteString(s.Text)
		}
	}
	return b.String()
}

type Actor struct {
	Identity    `yaml:",inline"`
	DisplayName string   `json:"display_name"`
	Roles       []string `json:"roles,omitempty"`
}

type Event struct {
	ConnectionID string       `json:"connection_id"`
	ID           string       `json:"id"`
	Type         string       `json:"type"` // message, member.joined or member.left
	Conversation Conversation `json:"conversation"`
	Sender       Actor        `json:"sender"`
	Message      Message      `json:"message"`
	OccurredAt   time.Time    `json:"occurred_at"`
}

type Status struct {
	ID           string    `json:"id"`
	Type         string    `json:"type"`
	State        string    `json:"state"`
	Capabilities []string  `json:"capabilities"`
	Since        time.Time `json:"since"`
	LastError    string    `json:"last_error,omitempty"`
	Dropped      uint64    `json:"dropped"`
}

type Receipt struct {
	MessageID string `json:"message_id"`
}

// Sink must return promptly. ErrBusy means the event was not accepted.
type Sink func(context.Context, Event) error

// Run owns the connection and returns after cancellation and worker cleanup.
// Send never silently queues an action for delivery after a reconnect.
type Adapter interface {
	Run(context.Context, Sink) error
	Send(context.Context, Conversation, Message) (Receipt, error)
	Status() Status
}

type Config struct {
	ID             string
	Endpoint       string
	Token          string
	AccountID      string
	QueueSize      int
	RequestTimeout time.Duration
	Options        json.RawMessage
}

type Factory func(Config) (Adapter, error)
