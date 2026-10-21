package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"sportslot/internal/auth"
	"sportslot/internal/bot"
	"sportslot/internal/geocode"
	"sportslot/internal/handler"
	"sportslot/internal/maxclient"
	"sportslot/internal/routing"
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
			t.Logf("bot → MAX: %.300s", body)
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
	dgis := fake2GIS(t)
	search.SetRouter(routing.NewDGIS(dgis, "dgis-key"))
	geocoder := geocode.New(dgis, "dgis-key")
	bookings := service.NewBookingService(store)
	partner := service.NewPartnerService(store, "partner-code")
	presenter := bot.NewPresenter(client)
	bookings.SetPresenter(presenter)
	partner.SetPresenter(presenter)
	chatBot := bot.New(client, store, search, bookings, presenter)
	chatBot.SetGeocoder(geocoder)
	studio := service.NewStudioService(store, partner, []string{"999"})
	studio.SetPresenter(presenter)
	studio.SetGeocoder(geocoder)
	bookings.SetStudioNotifier(studio)
	chatBot.SetStudio(studio)
	router := handler.NewRouter(handler.Dependencies{
		Search: search, Bookings: bookings, Partner: partner, Geocoder: geocoder, Studio: studio,
		Bot:         chatBot,
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
	max.next(t, "Как будешь добираться")
	webhook(t, srv, callback("tm:walking:15"))
	max.next(t, "до 15 мин пешком")
	card := max.next(t, "Стойка")
	if !strings.Contains(card, "5 мин пешком") || strings.Contains(card, "≈") {
		t.Fatalf("card must show the route time from 2GIS: %s", card)
	}
	start := strings.Index(card, `"bk:`)
	if start < 0 {
		t.Fatalf("card has no booking button: %s", card)
	}
	slotID := card[start+4 : start+40]

	webhook(t, srv, callback("bk:"+slotID))
	confirmation := max.next(t, "Ты записан на пробное занятие")
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

	status, me := call(t, http.MethodGet, srv.URL+"/api/v1/studio/me", partner, nil)
	if status != http.StatusOK || me["demo"] != true || len(me["studios"].([]interface{})) == 0 {
		t.Fatalf("studio me: %d %v", status, me)
	}
	status, training := call(t, http.MethodPost, srv.URL+"/api/v1/studio/checkin", partner, map[string]string{"code": "SPORTSLOT:000000"})
	if status != http.StatusOK || training["training"] != true {
		t.Fatalf("training check-in: %d %v", status, training)
	}
}

// fake2GIS отвечает на матрицу расстояний (до любого зала 5 минут) и на
// геокодер (любой адрес с «Малиновского» — у клуба «Стойка»).
func fake2GIS(t *testing.T) string {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/3.0/items/geocode" {
			if strings.Contains(r.URL.Query().Get("q"), "Малиновского") {
				_, _ = w.Write([]byte(`{"meta":{"code":200},"result":{"items":[{"address_name":"Малиновского, 16","point":{"lat":47.2336,"lon":39.6179}}]}}`))
				return
			}
			_, _ = w.Write([]byte(`{"meta":{"code":404}}`))
			return
		}
		var req struct {
			Targets []int `json:"targets"`
		}
		if r.URL.Query().Get("key") != "dgis-key" || json.NewDecoder(r.Body).Decode(&req) != nil {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		routes := []map[string]interface{}{}
		for _, target := range req.Targets {
			routes = append(routes, map[string]interface{}{"status": "OK", "source_id": 0, "target_id": target, "distance": 400, "duration": 300})
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"routes": routes})
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestVenuesByTravelTime(t *testing.T) {
	srv, _ := newServer(t)
	status, body := call(t, http.MethodGet, srv.URL+"/api/v1/venues?sport=boxing&lat=47.2336&lon=39.6179&mode=transit&max_minutes=20", nil, nil)
	venues, _ := body["venues"].([]interface{})
	if status != http.StatusOK || len(venues) == 0 {
		t.Fatalf("status %d, %d venues", status, len(venues))
	}
	travel := venues[0].(map[string]interface{})["travel"].(map[string]interface{})
	if travel["mode"] != "transit" || travel["minutes"].(float64) != 5 || travel["estimated"] != false {
		t.Fatalf("unexpected travel: %v", travel)
	}
	if status, _ := call(t, http.MethodGet, srv.URL+"/api/v1/venues?mode=bike", nil, nil); status != http.StatusBadRequest {
		t.Fatalf("invalid mode: want 400, got %d", status)
	}
}

func TestBotAcceptsTypedAddress(t *testing.T) {
	srv, max := newServer(t)
	webhook(t, srv, callback("sp:boxing"))
	webhook(t, srv, callback("w:any"))
	max.next(t, "Отправь геолокацию")

	text := func(s string) string {
		return `{"update_type":"message_created","message":{"sender":{"user_id":777},"body":{"text":"` + s + `"}}}`
	}
	webhook(t, srv, text("где-то на Луне"))
	max.next(t, "Не нашёл такой адрес")
	webhook(t, srv, text("Малиновского 16"))
	max.next(t, "Считаю от адреса")
	max.next(t, "Как будешь добираться")
	webhook(t, srv, callback("tm:transit:20"))
	card := max.next(t, "Стойка")
	if !strings.Contains(card, "5 мин на транспорте") || !strings.Contains(card, "rtext=47.233600,39.617900~") || !strings.Contains(card, "rtt=mt") {
		t.Fatalf("card must show travel time and route from the user's point: %s", card)
	}

	// Мини-приложение получает эту точку как последнюю известную.
	status, me := call(t, http.MethodGet, srv.URL+"/api/v1/me", map[string]string{"Authorization": "tma " + initData(777)}, nil)
	loc, _ := me["last_location"].(map[string]interface{})
	if status != http.StatusOK || loc == nil || loc["lat"].(float64) != 47.2336 || loc["address"] != "Малиновского, 16" {
		t.Fatalf("last_location missing: %d %v", status, me)
	}
}

func TestGeocodeEndpoint(t *testing.T) {
	srv, _ := newServer(t)
	status, body := call(t, http.MethodGet, srv.URL+"/api/v1/geocode?q="+url.QueryEscape("ул. Малиновского, 16"), nil, nil)
	if status != http.StatusOK || body["lat"].(float64) != 47.2336 || body["address"] != "Малиновского, 16" {
		t.Fatalf("geocode: %d %v", status, body)
	}
	if status, _ := call(t, http.MethodGet, srv.URL+"/api/v1/geocode?q="+url.QueryEscape("Луна"), nil, nil); status != http.StatusNotFound {
		t.Fatalf("unknown address: want 404, got %d", status)
	}
	if status, _ := call(t, http.MethodGet, srv.URL+"/api/v1/geocode?q=a", nil, nil); status != http.StatusBadRequest {
		t.Fatalf("short query: want 400, got %d", status)
	}
}

func tma(userID int64) map[string]string {
	return map[string]string{"Authorization": "tma " + initData(userID)}
}

func TestStudioCabinetAPI(t *testing.T) {
	srv, max := newServer(t)
	owner, admin := tma(999), tma(555)

	if status, body := call(t, http.MethodGet, srv.URL+"/api/v1/studio/me", admin, nil); status != http.StatusOK || len(body["studios"].([]interface{})) != 0 {
		t.Fatalf("not an admin yet: %d %v", status, body)
	}
	if status, _ := call(t, http.MethodGet, srv.URL+"/api/v1/platform/studios", admin, nil); status != http.StatusForbidden {
		t.Fatalf("platform for a stranger: want 403, got %d", status)
	}

	_, found := call(t, http.MethodGet, srv.URL+"/api/v1/platform/catalog?q="+url.QueryEscape("фитнес"), owner, nil)
	venueID := found["venues"].([]interface{})[0].(map[string]interface{})["id"].(string)
	if status, v := call(t, http.MethodPost, srv.URL+"/api/v1/platform/studios", owner, map[string]string{"venue_id": venueID}); status != http.StatusCreated || v["booking_mode"] != "instant" {
		t.Fatalf("connect studio: %d %v", status, v)
	}
	status, inv := call(t, http.MethodPost, srv.URL+"/api/v1/platform/studios/"+venueID+"/invites", owner, nil)
	if status != http.StatusCreated || !strings.Contains(inv["link"].(string), "studio_") {
		t.Fatalf("invite: %d %v", status, inv)
	}
	if status, _ := call(t, http.MethodPost, srv.URL+"/api/v1/studio/invites/accept", admin, map[string]string{"token": inv["token"].(string)}); status != http.StatusOK {
		t.Fatalf("accept invite: %d", status)
	}
	if _, me := call(t, http.MethodGet, srv.URL+"/api/v1/studio/me", admin, nil); len(me["studios"].([]interface{})) != 1 || me["platform"] != false {
		t.Fatalf("studio/me after invite: %v", me)
	}

	tomorrow := time.Now().Add(24 * time.Hour)
	tpl := map[string]interface{}{"weekday": int(tomorrow.Weekday()), "start_minute": 19 * 60, "duration_min": 60,
		"title": "Круговая тренировка", "sport_type": "fitness", "quota": 8}
	if status, body := call(t, http.MethodPost, srv.URL+"/api/v1/studio/"+venueID+"/templates", admin, tpl); status != http.StatusOK {
		t.Fatalf("template: %d %v", status, body)
	}
	_, slots := call(t, http.MethodGet, srv.URL+"/api/v1/studio/"+venueID+"/slots", admin, nil)
	slotID := slots["slots"].([]interface{})[0].(map[string]interface{})["id"].(string)

	_, booking := call(t, http.MethodPost, srv.URL+"/api/v1/bookings", tma(888), map[string]string{"slot_id": slotID})
	note := max.next(t, "Новая запись")
	if !strings.Contains(note, "max://user/888") || !strings.Contains(note, "sb_"+booking["id"].(string)) {
		t.Fatalf("admin notification must link the client and the booking: %s", note)
	}
	status, card := call(t, http.MethodGet, srv.URL+"/api/v1/studio/bookings/"+booking["id"].(string), admin, nil)
	if status != http.StatusOK || card["max_user_id"] != "888" {
		t.Fatalf("client card: %d %v", status, card)
	}
	if status, _ := call(t, http.MethodGet, srv.URL+"/api/v1/studio/"+seed.FlagshipVenueID+"/bookings", admin, nil); status != http.StatusForbidden {
		t.Fatalf("other studio: want 403, got %d", status)
	}

	if status, _ := call(t, http.MethodPost, srv.URL+"/api/v1/messages", tma(888), map[string]string{"venue_id": venueID, "text": "Есть душ?"}); status != http.StatusCreated {
		t.Fatalf("client message: %d", status)
	}
	max.next(t, "Есть душ?")
	_, threads := call(t, http.MethodGet, srv.URL+"/api/v1/studio/"+venueID+"/threads", admin, nil)
	userID := threads["threads"].([]interface{})[0].(map[string]interface{})["user_id"].(string)
	if status, _ := call(t, http.MethodPost, srv.URL+"/api/v1/studio/"+venueID+"/threads/"+userID, admin, map[string]string{"text": "Да, есть"}); status != http.StatusCreated {
		t.Fatalf("reply: %d", status)
	}
	max.next(t, "Да, есть")

	if status, _ := call(t, http.MethodPost, srv.URL+"/api/v1/studio/"+venueID+"/slots/"+slotID+"/cancel", admin, nil); status != http.StatusOK {
		t.Fatalf("cancel slot: %d", status)
	}
	max.next(t, "Занятие отменено студией")

	if status, body := call(t, http.MethodPost, srv.URL+"/api/v1/studio/checkin", admin, map[string]string{"code": "SPORTSLOT:000000"}); status != http.StatusOK || body["training"] != true {
		t.Fatalf("training check-in: %d %v", status, body)
	}
	if status, stats := call(t, http.MethodGet, srv.URL+"/api/v1/studio/"+venueID+"/stats", admin, nil); status != http.StatusOK || stats["clients"] == nil {
		t.Fatalf("stats: %d %v", status, stats)
	}

	// Общий демо-код открывает только демо-студии.
	_, login := call(t, http.MethodPost, srv.URL+"/api/v1/partner/login", nil, map[string]string{"code": "partner-code"})
	demo := map[string]string{"Authorization": "Partner " + login["token"].(string)}
	if _, me := call(t, http.MethodGet, srv.URL+"/api/v1/studio/me", demo, nil); me["demo"] != true || len(me["studios"].([]interface{})) != 8 {
		t.Fatalf("demo access: %v", me)
	}
	if status, _ := call(t, http.MethodGet, srv.URL+"/api/v1/studio/"+venueID+"/bookings", demo, nil); status != http.StatusForbidden {
		t.Fatalf("demo code must not open real studios: %d", status)
	}
}

func TestLocationFromMiniAppContinuesChat(t *testing.T) {
	srv, max := newServer(t)
	webhook(t, srv, callback("sp:boxing"))
	max.next(t, "Когда удобно")
	webhook(t, srv, callback("w:any"))
	max.next(t, "Определить через приложение")

	status, body := call(t, http.MethodPost, srv.URL+"/api/v1/me/location", tma(777), map[string]float64{"lat": 47.2336, "lon": 39.6179})
	if status != http.StatusOK || body["sport"] != "boxing" || body["when"] != "any" {
		t.Fatalf("location from app: %d %v", status, body)
	}
	max.next(t, "Геолокацию получил из приложения")
	max.next(t, "Как будешь добираться")
	if status, _ := call(t, http.MethodPost, srv.URL+"/api/v1/me/location", tma(777), map[string]float64{"lat": 55.75, "lon": 37.61}); status != http.StatusBadRequest {
		t.Fatalf("point outside Rostov: want 400, got %d", status)
	}
}
