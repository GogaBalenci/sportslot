package handler

import (
	"context"
	"net/http"
	"time"
)

// HealthHandler отвечает «ok», только если API видит базу: на это смотрит мониторинг.
type HealthHandler struct {
	ping func(ctx context.Context) error
}

func NewHealthHandler(ping func(ctx context.Context) error) *HealthHandler {
	return &HealthHandler{ping: ping}
}

func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	if h.ping != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := h.ping(ctx); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "degraded", "db": "unavailable"})
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
