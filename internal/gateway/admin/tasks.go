package admin

import (
	"net/http"

	"github.com/ShitBot-Team/ShitBotUltra/internal/fault"
)

func (h *Handler) registerTasks() {
	h.mux.HandleFunc("GET /api/v1/tasks", func(w http.ResponseWriter, r *http.Request) {
		respond(w, http.StatusOK, h.engine.Tasks.Snapshot())
	})
	h.mux.HandleFunc("POST /api/v1/tasks/{id}/run", h.runTask)
	h.mux.HandleFunc("GET /api/v1/task-runs", h.taskRuns)
	h.mux.HandleFunc("GET /api/v1/task-runs/{id}", h.taskRun)
}

func (h *Handler) runTask(w http.ResponseWriter, r *http.Request) {
	var request struct {
		RequestID string `json:"request_id"`
		Revision  string `json:"revision"`
	}
	if !decode(w, r, &request) {
		return
	}
	if request.Revision == "" {
		writeError(w, fault.New("INVALID_INPUT", "请提供正在运行的任务快照 revision"))
		return
	}
	run, err := h.engine.Tasks.Manual(r.Context(), r.PathValue("id"), request.RequestID, request.Revision)
	if err != nil {
		writeError(w, err)
		return
	}
	status := http.StatusOK
	if run.Status == "queued" || run.Status == "running" {
		status = http.StatusAccepted
	}
	w.Header().Set("Location", "/api/v1/task-runs/"+run.ID)
	respond(w, status, run)
}

func (h *Handler) taskRuns(w http.ResponseWriter, r *http.Request) {
	limit, offset, err := page(r)
	if err != nil {
		writeError(w, err)
		return
	}
	items, err := h.engine.Tasks.List(r.Context(), r.URL.Query().Get("task_id"), limit, offset)
	if err != nil {
		writeError(w, err)
		return
	}
	respond(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) taskRun(w http.ResponseWriter, r *http.Request) {
	run, err := h.engine.Tasks.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	respond(w, http.StatusOK, run)
}
