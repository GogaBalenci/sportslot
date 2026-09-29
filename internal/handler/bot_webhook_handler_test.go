package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"sportslot/internal/maxclient"
	"sportslot/internal/service"
)

type capturedCall struct {
	Method, Path, Query, Auth string
	Body                      map[string]interface{}
}

func fakeMaxAPI(t *testing.T) (*httptest.Server, func() []capturedCall) {
	t.Helper()
	var mu sync.Mutex
	var calls []capturedCall
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]interface{}
		_ = json.Unmarshal(raw, &body)
		mu.Lock()
		calls = append(calls, capturedCall{r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Get("Authorization"), body})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	t.Cleanup(srv.Close)
	return srv, func() []capturedCall { mu.Lock(); defer mu.Unlock(); return append([]capturedCall(nil), calls...) }
}

func TestBotStartedSendsWelcomeWithCallbackKeyboard(t *testing.T) {
	api, calls := fakeMaxAPI(t)
	client := maxclient.NewClient(api.URL, "test-token")
	h := NewBotWebhookHandler(service.NewDialogService(nil, nil, ""), client, "secret-123", false)

	update := `{"update_type":"bot_started","timestamp":1,"chat_id":42,"user":{"user_id":987654321,"first_name":"Иван","is_bot":false}}`
	req := httptest.NewRequest(http.MethodPost, "/bot/webhook", strings.NewReader(update))
	req.Header.Set("X-Max-Bot-Api-Secret", "secret-123")
	rec := httptest.NewRecorder()
	h.HandleWebhook(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	got := calls()
	if len(got) != 1 {
		t.Fatalf("expected 1 call to MAX API, got %d", len(got))
	}
	c := got[0]
	if c.Method != http.MethodPost || c.Path != "/messages" || c.Query != "user_id=987654321" {
		t.Fatalf("unexpected request: %s %s?%s", c.Method, c.Path, c.Query)
	}
	if c.Auth != "test-token" {
		t.Fatalf("Authorization must be the raw token, got %q", c.Auth)
	}
	atts, _ := c.Body["attachments"].([]interface{})
	if len(atts) != 1 {
		t.Fatalf("expected inline keyboard attachment, body=%v", c.Body)
	}
	att := atts[0].(map[string]interface{})
	if att["type"] != "inline_keyboard" {
		t.Fatalf("unexpected attachment type %v", att["type"])
	}
	rows := att["payload"].(map[string]interface{})["buttons"].([]interface{})
	first := rows[0].([]interface{})[0].(map[string]interface{})
	if first["type"] != "callback" || first["payload"] != "sport:boxing" {
		t.Fatalf("unexpected first button %v", first)
	}
}

func TestCallbackIsAnswered(t *testing.T) {
	api, calls := fakeMaxAPI(t)
	client := maxclient.NewClient(api.URL, "test-token")
	h := NewBotWebhookHandler(service.NewDialogService(nil, nil, ""), client, "secret-123", false)

	update := `{"update_type":"message_callback","callback":{"callback_id":"cb-1","payload":"sport:yoga","user":{"user_id":5}}}`
	req := httptest.NewRequest(http.MethodPost, "/bot/webhook", strings.NewReader(update))
	req.Header.Set("X-Max-Bot-Api-Secret", "secret-123")
	rec := httptest.NewRecorder()
	h.HandleWebhook(rec, req)

	got := calls()
	if rec.Code != http.StatusOK || len(got) != 2 {
		t.Fatalf("code=%d calls=%d", rec.Code, len(got))
	}
	if got[0].Path != "/answers" || got[0].Query != "callback_id=cb-1" {
		t.Fatalf("callback must be answered first, got %s?%s", got[0].Path, got[0].Query)
	}
	if got[1].Path != "/messages" || got[1].Query != "user_id=5" {
		t.Fatalf("expected reply message, got %s?%s", got[1].Path, got[1].Query)
	}
}

func TestWebhookRejectsWrongSecretAndIgnoresUnknownUpdates(t *testing.T) {
	api, calls := fakeMaxAPI(t)
	h := NewBotWebhookHandler(service.NewDialogService(nil, nil, ""), maxclient.NewClient(api.URL, "t"), "secret-123", false)

	req := httptest.NewRequest(http.MethodPost, "/bot/webhook", strings.NewReader(`{}`))
	req.Header.Set("X-Max-Bot-Api-Secret", "wrong")
	rec := httptest.NewRecorder()
	h.HandleWebhook(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/bot/webhook", strings.NewReader(`{"update_type":"message_edited"}`))
	req.Header.Set("X-Max-Bot-Api-Secret", "secret-123")
	rec = httptest.NewRecorder()
	h.HandleWebhook(rec, req)
	if rec.Code != http.StatusOK || len(calls()) != 0 {
		t.Fatalf("unknown update must be acknowledged with 200 and no API calls")
	}
}
