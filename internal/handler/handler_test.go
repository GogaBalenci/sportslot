package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"sportslot/internal/auth"
	"sportslot/internal/bot"
	"sportslot/internal/handler"
	"sportslot/internal/maxclient"
	"sportslot/internal/seed"
	"sportslot/internal/service"
	"sportslot/internal/testdb"
)

const botToken = "test-token"

// fakeMax записывает сообщения, которые бот отправляет через MAX Bot API.
type fakeMax struct {
	server   *httptest.Server
	messages chan string
}

func newFakeMax(t *testing.T) *fakeMax {
	f := &fakeMax{messages: make(chan string, 64)}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != botToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		body, _ := io.ReadAll(r.Body)
		switch r.URL.Path {
		case "/me":
			_, _ = w.Write([]byte(`{"user_id":1,"username":"sportslot_bot","name":"СпортСлот"}`))
		case "/messages":
			f.messages <- string(body)
			_, _ = w.Write([]byte(`{"message":{}}`))
		default:
			_, _ = w.Write([]byte(`{"success":true}`))
		}
	}))
	t.Cleanup(f.server.Close)
	return f
}

// next ждёт сообщение, в тексте которого есть substr, пропуская остальные.
func (f *fakeMax) next(t *testing.T, substr string) string {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case msg := <-f.messages:
			if strings.Contains(msg, substr) {
				return msg
			}
		case <-deadline:
			t.Fatalf("bot did not send a message containing %q", substr)
		}
	}
}

func newServer(t *testing.T) (*httptest.Server, *fakeMax) {
	store := testdb.New(t)
	if _, err := seed.LoadCatalog(context.Background(), store, "../../seed-data/rostov_catalog.json"); err != nil {
		t.Fatal(err)
	}
	max := newFakeMax(t)
	client := maxclient.NewClient(max.server.URL, botToken)
	search := service.NewSearchService(store, true, time.Now())
	bookings := service.NewBookingService(store)
	partner := service.NewPartnerService(store, "partner-code")
	presenter := bot.NewPresenter(client)
	bookings.SetPresenter(presenter)
	partner.SetPresenter(presenter)
	router := handler.NewRouter(handler.Dependencies{
		Search: search, Bookings: bookings, Partner: partner,
		Bot:         bot.New(client, store, search, bookings, presenter),
		Auth:        handler.UserAuth{BotToken: botToken, MaxAge: time.Hour, TestUsers: map[string]bool{seed.TestMaxUserID: true}},
		CORSOrigins: []string{"*"}, WebhookSecret: "hook-secret",
	})
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return srv, max
}

func call(t *testing.T, method, url string, headers map[string]string, body interface{}) (int, map[string]interface{}) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	}
	req, _ := http.NewRequest(method, url, reader)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]interface{}
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

func webhook(t *testing.T, srv *httptest.Server, update string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/bot/webhook", strings.NewReader(update))
	req.Header.Set("X-Max-Bot-Api-Secret", "hook-secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("webhook: status=%v err=%v", resp, err)
	}
	resp.Body.Close()
}

func callback(payload string) string {
	return `{"update_type":"message_callback","callback":{"callback_id":"cb","payload":"` + payload +
		`","user":{"user_id":777,"first_name":"Олег"}}}`
}

func TestBotScenarioFromStartToBooking(t *testing.T) {
	srv, max := newServer(t)

	webhook(t, srv, `{"update_type":"bot_started","user":{"user_id":777,"first_name":"Олег"}}`)
	max.next(t, "Привет, Олег!")

	webhook(t, srv, callback("sp:boxing"))
	max.next(t, "Когда удобно")
	webhook(t, srv, callback("w:any"))
	max.next(t, "Отправь геолокацию")

	// Геолокация рядом с клубом «Стойка» в Советском районе.
	webhook(t, srv, `{"update_type":"message_created","message":{"sender":{"user_id":777},"body":{"attachments":[{"type":"location","latitude":47.2336,"longitude":39.6179}]}}}`)
	max.next(t, "рядом с тобой")
	card := max.next(t, "Стойка")
	start := strings.Index(card, `"bk:`)
	if start < 0 {
		t.Fatalf("card has no booking button: %s", card)
	}
	slotID := card[start+4 : start+40]

	webhook(t, srv, callback("bk:"+slotID))
	confirmation := max.next(t, "Записал тебя")
	for _, want := range []string{"Код входа", "ticket_", "rs:", "cx:"} {
		if !strings.Contains(confirmation, want) {
			t.Fatalf("confirmation misses %q: %s", want, confirmation)
		}
	}
	max.next(t, `"type":"location"`)

	webhook(t, srv, callback("my"))
	max.next(t, "Код входа")
}

func TestQuizInBot(t *testing.T) {
	srv, max := newServer(t)
	webhook(t, srv, callback("quiz"))
	max.next(t, "Вопрос 1 из 5")
	for _, a := range []string{"goal:stress", "water:no", "format:group", "pace:calm", "contact:no"} {
		webhook(t, srv, callback("qa:"+a))
	}
	result := max.next(t, "может подойти")
	if !strings.Contains(result, "sp:yoga") {
		t.Fatalf("yoga expected in quiz result: %s", result)
	}
}

func TestWebhookRejectsWrongSecret(t *testing.T) {
	srv, _ := newServer(t)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/bot/webhook", strings.NewReader(`{}`))
	req.Header.Set("X-Max-Bot-Api-Secret", "wrong")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

func initData(userID int64) string {
	return auth.BuildInitData(map[string]string{
		"auth_date": strconv.FormatInt(time.Now().Unix(), 10),
		"query_id":  "q",
		"user":      `{"id":` + strconv.FormatInt(userID, 10) + `,"first_name":"Мария","last_name":""}`,
	}, botToken)
}

func TestMiniAppAuth(t *testing.T) {
	srv, _ := newServer(t)
	status, body := call(t, http.MethodGet, srv.URL+"/api/v1/me", map[string]string{"Authorization": "tma " + initData(555)}, nil)
	if status != http.StatusOK || body["user"] == nil {
		t.Fatalf("valid initData: status %d body %v", status, body)
	}
	forged := strings.Replace(initData(555), "555", "556", 1)
	if status, _ := call(t, http.MethodGet, srv.URL+"/api/v1/me", map[string]string{"Authorization": "tma " + forged}, nil); status != http.StatusUnauthorized {
		t.Fatalf("forged initData: want 401, got %d", status)
	}
	if status, _ := call(t, http.MethodGet, srv.URL+"/api/v1/me", map[string]string{"X-MAX-User-ID": "555"}, nil); status != http.StatusUnauthorized {
		t.Fatalf("header auth for real user: want 401, got %d", status)
	}
}

// TestDataAPIContract повторяет проверки из openapi/DATA-API.yaml.
func TestDataAPIContract(t *testing.T) {
	srv, _ := newServer(t)
	user := map[string]string{"X-MAX-User-ID": seed.TestMaxUserID}

	status, body := call(t, http.MethodPost, srv.URL+"/api/v1/search", user, map[string]interface{}{
		"sport_type": "boxing", "lat": 47.2225, "lon": 39.7185, "radius_km": 20,
		"date_from": time.Now().Format("2006-01-02"), "date_to": time.Now().AddDate(0, 0, 14).Format("2006-01-02"),
		"time_from": "18:00", "time_to": "21:00", "level": "beginner",
	})
	results, _ := body["results"].([]interface{})
	if status != http.StatusOK || len(results) == 0 || body["meta"].(map[string]interface{})["data_source"] == nil {
		t.Fatalf("search: status %d, %d results", status, len(results))
	}

	status, body = call(t, http.MethodGet, srv.URL+"/api/v1/venues/"+seed.FlagshipVenueID, nil, nil)
	if status != http.StatusOK || body["venue_id"] == nil || body["all_slots"] == nil || body["address"] == nil {
		t.Fatalf("venue: status %d body %v", status, body)
	}

	req := map[string]string{"slot_id": seed.ReferenceSlotOne, "source_channel": "miniapp"}
	status, body = call(t, http.MethodPost, srv.URL+"/api/v1/bookings", user, req)
	if status != http.StatusCreated || body["id"] == nil || body["status"] != "confirmed" || body["source_channel"] != "miniapp" {
		t.Fatalf("create booking: status %d body %v", status, body)
	}
	bookingID := body["id"].(string)
	if status, _ = call(t, http.MethodPost, srv.URL+"/api/v1/bookings", user, req); status != http.StatusOK {
		t.Fatalf("repeated booking: want 200, got %d", status)
	}

	status, body = call(t, http.MethodPatch, srv.URL+"/api/v1/bookings/"+bookingID+"/reschedule", user,
		map[string]string{"new_slot_id": seed.ReferenceSlotTwo})
	if status != http.StatusOK || body["slot_id"] != seed.ReferenceSlotTwo {
		t.Fatalf("reschedule: status %d body %v", status, body)
	}
	if status, _ = call(t, http.MethodPatch, srv.URL+"/api/v1/bookings/test-booking-uuid/reschedule", user,
		map[string]string{"new_slot_id": seed.ReferenceSlotTwo}); status != http.StatusNotFound {
		t.Fatalf("reschedule unknown id: want 404, got %d", status)
	}

	status, _ = call(t, http.MethodGet, srv.URL+"/api/v1/bookings/user/"+seed.TestUserID, user, nil)
	if status != http.StatusOK {
		t.Fatalf("list by user: want 200, got %d", status)
	}
	if status, _ = call(t, http.MethodDelete, srv.URL+"/api/v1/bookings/"+bookingID, user, nil); status != http.StatusNoContent {
		t.Fatalf("cancel: want 204, got %d", status)
	}
	if status, _ = call(t, http.MethodDelete, srv.URL+"/api/v1/bookings/test-booking-uuid", user, nil); status != http.StatusNotFound {
		t.Fatalf("cancel unknown: want 404, got %d", status)
	}
	if status, body = call(t, http.MethodGet, srv.URL+"/api/v1/health", nil, nil); status != http.StatusOK || body["status"] != "ok" {
		t.Fatalf("health: %d %v", status, body)
	}
}

func TestPartnerCheckIn(t *testing.T) {
	srv, max := newServer(t)
	user := map[string]string{"Authorization": "tma " + initData(888)}
	_, booking := call(t, http.MethodPost, srv.URL+"/api/v1/bookings", user, map[string]string{"slot_id": seed.ReferenceSlotOne})
	code := booking["checkin_code"].(string)

	if status, _ := call(t, http.MethodPost, srv.URL+"/api/v1/partner/login", nil, map[string]string{"code": "nope"}); status != http.StatusUnauthorized {
		t.Fatalf("wrong code: want 401, got %d", status)
	}
	_, login := call(t, http.MethodPost, srv.URL+"/api/v1/partner/login", nil, map[string]string{"code": "partner-code"})
	partner := map[string]string{"Authorization": "Partner " + login["token"].(string)}

	status, list := call(t, http.MethodGet, srv.URL+"/api/v1/partner/bookings", partner, nil)
	if status != http.StatusOK || len(list["bookings"].([]interface{})) != 1 {
		t.Fatalf("partner bookings: %d %v", status, list)
	}
	status, checked := call(t, http.MethodPost, srv.URL+"/api/v1/partner/checkin", partner, map[string]string{"code": "SPORTSLOT:" + code})
	if status != http.StatusOK || checked["status"] != "attended" {
		t.Fatalf("check-in: %d %v", status, checked)
	}
	max.next(t, "Как прошло")
	if status, _ := call(t, http.MethodPost, srv.URL+"/api/v1/partner/checkin", partner, map[string]string{"code": code}); status != http.StatusConflict {
		t.Fatalf("second check-in: want 409, got %d", status)
	}
}
