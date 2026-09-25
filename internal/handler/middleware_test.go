package handler_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"sportslot/internal/handler"
)

func TestRequireMaxUserIDMiddleware(t *testing.T) {
	dummyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mw := handler.RequireMaxUserID(dummyHandler)

	// 1. Without header -> 401
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without X-MAX-User-ID header, got %d", rr.Code)
	}

	// 2. With header -> 200
	req = httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-MAX-User-ID", "test-user-123")
	rr = httptest.NewRecorder()
	mw.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 with X-MAX-User-ID header, got %d", rr.Code)
	}
}
