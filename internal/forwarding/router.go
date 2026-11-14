package forwarding

import (
	"slices"
	"strings"
	"unicode"

	"github.com/ShitBot-Team/ShitBotUltra/internal/config"
	"github.com/ShitBot-Team/ShitBotUltra/sdk/adapter"
)

type Router struct{ routes []config.Route }

func New(routes []config.Route) *Router { return &Router{routes: routes} }
func (r *Router) FromBot(e adapter.Event) []config.Route {
	result := []config.Route{}
	for _, route := range r.routes {
		if route.ConnectionID == e.ConnectionID && route.Conversation == e.Conversation {
			result = append(result, route)
		}
	}
	return result
}
func (r *Router) FromGame(nodeID string) []config.Route {
	result := []config.Route{}
	for _, route := range r.routes {
		if slices.Contains(route.NodeIDs, nodeID) {
			result = append(result, route)
		}
	}
	return result
}

func Clean(text string, maxRunes int) string {
	var b strings.Builder
	count := 0
	skipFormat := false
	for _, r := range text {
		if skipFormat {
			skipFormat = false
			continue
		}
		if r == '§' {
			skipFormat = true
			continue
		}
		if unicode.IsControl(r) {
			if r == '\n' || r == '\t' {
				r = ' '
			} else {
				continue
			}
		}
		if count >= maxRunes {
			b.WriteRune('…')
			break
		}
		b.WriteRune(r)
		count++
	}
	return b.String()
}

func Text(message adapter.Message) string {
	var b strings.Builder
	for _, seg := range message.Segments {
		switch seg.Type {
		case "text":
			b.WriteString(seg.Text)
		case "image":
			b.WriteString("[图片]")
		case "mention":
			b.WriteString("[@用户]")
		case "reply":
		default:
			b.WriteString("[附件]")
		}
	}
	return Clean(b.String(), 2000)
}
