package admin

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/ShitBot-Team/ShitBotUltra/internal/events"
	"github.com/ShitBot-Team/ShitBotUltra/internal/fault"
)

func (h *Handler) stream(w http.ResponseWriter, r *http.Request) {
	after := uint64(0)
	if id := r.Header.Get("Last-Event-ID"); id != "" {
		value, err := strconv.ParseUint(id, 10, 64)
		if err != nil {
			writeError(w, fault.New("INVALID_INPUT", "Last-Event-ID 无效"))
			return
		}
		after = value
	}
	backlog, ch, cancel, ok := h.engine.Events.Subscribe(after)
	if !ok {
		writeError(w, fault.New("BUSY", "事件订阅数已达到上限"))
		return
	}
	defer cancel()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	controller := http.NewResponseController(w)
	send := func(event events.Event) error {
		_ = controller.SetWriteDeadline(time.Now().Add(10 * time.Second))
		data, err := json.Marshal(event)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", event.ID, event.Type, data); err != nil {
			return err
		}
		return controller.Flush()
	}
	for _, event := range backlog {
		if send(event) != nil {
			return
		}
	}
	_ = controller.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if _, err := io.WriteString(w, ": connected\n\n"); err != nil {
		return
	}
	if controller.Flush() != nil {
		return
	}
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			if send(event) != nil {
				return
			}
		case <-ticker.C:
			_ = controller.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if _, err := io.WriteString(w, ": heartbeat\n\n"); err != nil {
				return
			}
			if controller.Flush() != nil {
				return
			}
		}
	}
}
