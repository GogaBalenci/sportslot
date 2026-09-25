package handler_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"sportslot/internal/handler"
)

func TestHealthHandler(t *testing.T) {
	h := handler.NewHealthHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rr := httptest.NewRecorder()

	h.Health(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got: %d", rr.Code)
	}

	expected := "{\"status\":\"ok\"}\n"
	if rr.Body.String() != expected {
		t.Errorf("expected %q, got: %q", expected, rr.Body.String())
	}
}
