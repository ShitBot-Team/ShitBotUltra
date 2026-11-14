package onebotv11

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"time"

	"github.com/ShitBot-Team/ShitBotUltra/sdk/adapter"
)

func qq(id string) adapter.Identity {
	return adapter.Identity{Platform: "qq.numeric", Scope: "global", ID: id}
}

func decodeEvent(cfg adapter.Config, data []byte) (adapter.Event, bool) {
	var wire struct {
		Time        int64           `json:"time"`
		SelfID      opaqueID        `json:"self_id"`
		PostType    string          `json:"post_type"`
		MessageType string          `json:"message_type"`
		NoticeType  string          `json:"notice_type"`
		MessageID   opaqueID        `json:"message_id"`
		UserID      opaqueID        `json:"user_id"`
		GroupID     opaqueID        `json:"group_id"`
		Message     json.RawMessage `json:"message"`
		Sender      struct {
			Nickname string `json:"nickname"`
			Card     string `json:"card"`
			Role     string `json:"role"`
		} `json:"sender"`
	}
	if json.Unmarshal(data, &wire) != nil || string(wire.SelfID) != cfg.AccountID || wire.UserID == "" || string(wire.UserID) == cfg.AccountID {
		return adapter.Event{}, false
	}
	e := adapter.Event{ConnectionID: cfg.ID, ID: string(wire.MessageID), Sender: adapter.Actor{Identity: qq(string(wire.UserID)), DisplayName: wire.Sender.Nickname}, OccurredAt: time.Unix(wire.Time, 0).UTC()}
	if wire.Sender.Card != "" {
		e.Sender.DisplayName = wire.Sender.Card
	}
	switch wire.Sender.Role {
	case "owner":
		e.Sender.Roles = []string{"owner", "moderator", "member"}
	case "admin":
		e.Sender.Roles = []string{"moderator", "member"}
	default:
		e.Sender.Roles = []string{"member"}
	}
	if wire.GroupID != "" {
		e.Conversation = adapter.Conversation{Identity: qq(string(wire.GroupID)), Kind: "group"}
	} else if wire.MessageType == "private" {
		e.Conversation = adapter.Conversation{Identity: qq(string(wire.UserID)), Kind: "private"}
	} else {
		return e, false
	}
	switch wire.PostType {
	case "message":
		if e.ID == "" {
			return e, false
		}
		e.Type = "message"
		var segments []struct {
			Type string                     `json:"type"`
			Data map[string]json.RawMessage `json:"data"`
		}
		if json.Unmarshal(wire.Message, &segments) != nil {
			var text string
			if json.Unmarshal(wire.Message, &text) != nil {
				return e, false
			}
			e.Message = adapter.Text(text)
		} else {
			if len(segments) > 64 {
				return e, false
			}
			for _, seg := range segments {
				value := func(key string) string {
					var s string
					if json.Unmarshal(seg.Data[key], &s) == nil {
						return s
					}
					var id opaqueID
					if json.Unmarshal(seg.Data[key], &id) == nil {
						return string(id)
					}
					return ""
				}
				switch seg.Type {
				case "text":
					e.Message.Segments = append(e.Message.Segments, adapter.Segment{Type: "text", Text: value("text")})
				case "image":
					e.Message.Segments = append(e.Message.Segments, adapter.Segment{Type: "image", URL: value("url")})
				case "at":
					target := qq(value("qq"))
					e.Message.Segments = append(e.Message.Segments, adapter.Segment{Type: "mention", Target: &target})
				case "reply":
					e.Message.Segments = append(e.Message.Segments, adapter.Segment{Type: "reply", MessageID: value("id")})
				default:
					e.Message.Segments = append(e.Message.Segments, adapter.Segment{Type: "text", Text: "[" + seg.Type + "]"})
				}
			}
		}
	case "notice":
		switch wire.NoticeType {
		case "group_increase":
			e.Type = "member.joined"
		case "group_decrease":
			e.Type = "member.left"
		default:
			return e, false
		}
		// OneBot notices have no message_id; use their stable identifying fields.
		sum := sha256.Sum256([]byte(wire.NoticeType + ":" + string(wire.GroupID) + ":" + string(wire.UserID) + ":" + strconv.FormatInt(wire.Time, 10)))
		e.ID = "notice:" + hex.EncodeToString(sum[:])
	default:
		return e, false
	}
	if e.Sender.Validate() != nil || e.Conversation.Validate() != nil || len(e.Message.PlainText()) > 16000 {
		return e, false
	}
	return e, true
}
