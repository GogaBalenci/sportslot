package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"sportslot/internal/catalog"
	"sportslot/internal/model"
	"sportslot/internal/payments"
	"sportslot/internal/repository/postgres"
	"sportslot/internal/secret"
	"sportslot/internal/service"
)

const poolVenue = "a1111111-0000-0000-0000-000000000006" // бассейн, пробное 400 ₽

// fakeYooKassa — ЮKassa с проверкой Basic-авторизации; статус платежа задаёт тест.
type fakeYooKassa struct {
	mu     sync.Mutex
	status map[string]string
	n      int
}

func (f *fakeYooKassa) server(t *testing.T) *httptest.Server {
	f.status = map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, pass, ok := r.BasicAuth(); !ok || user != "shop-1" || pass != "live_secret" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"type":"error","code":"invalid_credentials"}`))
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v3/me":
			_, _ = w.Write([]byte(`{"account_id":"shop-1"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v3/payments":
			if r.Header.Get("Idempotence-Key") == "" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			var body struct {
				Amount struct{ Value string } `json:"amount"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.n++
			id := fmt.Sprintf("yk-%d", f.n)
			f.status[id] = "pending:" + body.Amount.Value
			_, _ = fmt.Fprintf(w, `{"id":%q,"status":"pending","amount":{"value":%q,"currency":"RUB"},
				"confirmation":{"type":"redirect","confirmation_url":"https://yoomoney.example/pay/%s"}}`, id, body.Amount.Value, id)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v3/payments/"):
			id := strings.TrimPrefix(r.URL.Path, "/v3/payments/")
			st, amount, _ := strings.Cut(f.status[id], ":")
			_, _ = fmt.Fprintf(w, `{"id":%q,"status":%q,"amount":{"value":%q,"currency":"RUB"}}`, id, st, amount)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func (f *fakeYooKassa) set(id, status string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, amount, _ := strings.Cut(f.status[id], ":")
	f.status[id] = status + ":" + amount
}

// fakeTBank — Т-Касса: проверяет подпись запросов паролем терминала.
func fakeTBank(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		dec := json.NewDecoder(strings.NewReader(string(raw)))
		dec.UseNumber()
		var params map[string]interface{}
		_ = dec.Decode(&params)
		if params["Token"] != payments.TBankToken(params, "tb-password") {
			_, _ = w.Write([]byte(`{"Success":false,"ErrorCode":"204","Message":"Неверный токен"}`))
			return
		}
		switch r.URL.Path {
		case "/v2/Init":
			_, _ = fmt.Fprintf(w, `{"Success":true,"ErrorCode":"0","Status":"NEW","PaymentId":777001,"OrderId":%q,
				"Amount":%s,"PaymentURL":"https://securepay.example/new/777001"}`, params["OrderId"], params["Amount"])
		case "/v2/GetState":
			_, _ = w.Write([]byte(`{"Success":true,"ErrorCode":"0","Status":"NEW","PaymentId":"777001"}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

type commerceEnv struct {
	*studioEnv
	commerce *service.CommerceService
	yoo      *fakeYooKassa
	admin    service.Access
}

func newCommerceEnv(t *testing.T) *commerceEnv {
	e := newStudioEnv(t)
	yoo := &fakeYooKassa{}
	ysrv := yoo.server(t)
	tsrv := fakeTBank(t)
	box, err := secret.New(strings.Repeat("ab", 32))
	if err != nil {
		t.Fatal(err)
	}
	c := service.NewCommerceService(e.store, payments.NewGateway(ysrv.URL, tsrv.URL), box, e.studio)
	c.SetClientPresenter(e.rec)
	c.SetPublicURL("https://sportslot.test")
	e.bookings.SetClientPresenter(e.rec)
	e.bookings.SetDeepLink(func(p string) string { return "https://max.ru/bot?startapp=" + p })
	return &commerceEnv{studioEnv: e, commerce: c, yoo: yoo, admin: e.adminOf(t, poolVenue, "700")}
}

func boolp(b bool) *bool    { return &b }
func strp(s string) *string { return &s }
func repeatingSlot(t *testing.T, store *postgres.Store, venueID string, free int) *model.Slot {
	t.Helper()
	now := time.Now()
	slots, err := store.ListSlots(context.Background(), postgres.SlotQuery{VenueIDs: []string{venueID},
		From: now.Add(3 * time.Hour), To: now.Add(7 * 24 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	for i := range slots {
		if slots[i].TemplateID != "" && slots[i].QuotaAvailable() >= free {
			return &slots[i]
		}
	}
	t.Fatalf("no repeating slot with %d free places", free)
	return nil
}

func TestReviewsVisibleOnlyWhenStudioEnablesThem(t *testing.T) {
	e := newCommerceEnv(t)
	ctx := context.Background()
	slot := repeatingSlot(t, e.store, poolVenue, 1)
	view, _, err := e.bookings.Book(ctx, "client-1", "Мария Иванова", slot.ID, model.SourceChannelMiniApp)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.bookings.Review(ctx, view.ID, "client-1", "Отлично"); err == nil {
		t.Fatal("review before the visit must be rejected")
	}
	if _, err := e.studio.CheckIn(ctx, e.admin, view.CheckinCode); err != nil {
		t.Fatal(err)
	}
	if _, err := e.bookings.Rate(ctx, view.ID, "client-1", 5); err != nil {
		t.Fatal(err)
	}
	if err := e.bookings.Review(ctx, view.ID, "client-1", "  Тренер объяснил всё с нуля  "); err != nil {
		t.Fatal(err)
	}

	search := service.NewSearchService(e.store, true, time.Now())
	details, err := search.VenueDetails(ctx, poolVenue, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if details.Reviews != nil {
		t.Fatal("reviews must stay hidden until the studio enables them")
	}
	if _, err := e.commerce.SaveSettings(ctx, e.admin, poolVenue, service.SettingsInput{ShowReviews: boolp(true)}); err != nil {
		t.Fatal(err)
	}
	details, _ = search.VenueDetails(ctx, poolVenue, time.Now())
	if details.Reviews == nil || details.Reviews.Count != 1 || details.Reviews.Items[0].Author != "Мария" ||
		details.Reviews.Items[0].Text != "Тренер объяснил всё с нуля" {
		t.Fatalf("public reviews: %+v", details.Reviews)
	}
	cards, err := search.FindVenues(ctx, service.VenueSearch{Sport: "swimming", When: "any"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cards {
		if c.Venue.ID == poolVenue && (c.Rating == nil || c.Rating.Average != 5) {
			t.Fatalf("search card rating: %+v", c.Rating)
		}
	}
	// Студия в кабинете видит полное имя.
	all, err := e.commerce.Reviews(ctx, e.admin, poolVenue)
	if err != nil || all.Items[0].Author != "Мария Иванова" {
		t.Fatalf("studio reviews: %+v %v", all, err)
	}
}

func TestPassRequestedConfirmedAndUsedForSeries(t *testing.T) {
	e := newCommerceEnv(t)
	ctx := context.Background()
	if _, err := e.commerce.SavePass(ctx, e.admin, poolVenue, model.Pass{Title: "x", Visits: 4, ValidDays: 30, Price: 2000}); !errors.Is(err, service.ErrInvalidRequest) {
		t.Fatalf("short title must be rejected, got %v", err)
	}
	pass, err := e.commerce.SavePass(ctx, e.admin, poolVenue, model.Pass{Title: "4 занятия", Visits: 4, ValidDays: 30, Price: 2000, Active: true})
	if err != nil {
		t.Fatal(err)
	}
	// Оплата не подключена: заявка уходит студии.
	order, err := e.commerce.RequestPass(ctx, "client-2", "Олег", pass.ID)
	if err != nil || order.Payment != nil || order.UserPass.Status != model.UserPassRequested {
		t.Fatalf("pass request: %+v %v", order, err)
	}
	e.rec.expect(t, "passreq:700:client-2")
	if _, err := e.commerce.ConfirmPass(ctx, e.admin, poolVenue, order.UserPass.ID); err != nil {
		t.Fatal(err)
	}
	e.rec.expect(t, "pass-active:client-2")
	if _, err := e.commerce.ConfirmPass(ctx, e.admin, poolVenue, order.UserPass.ID); !errors.Is(err, service.ErrNotActive) {
		t.Fatalf("second confirmation must fail, got %v", err)
	}

	slot := repeatingSlot(t, e.store, poolVenue, 1)
	series, err := e.bookings.BookSeries(ctx, "client-2", "Олег", slot.ID, 3, model.SourceChannelMiniApp)
	if err != nil {
		t.Fatal(err)
	}
	booked := len(series.Bookings)
	if booked < 2 || booked+len(series.Skipped) != 3 {
		t.Fatalf("series: %d booked, %d skipped", booked, len(series.Skipped))
	}
	for i, b := range series.Bookings {
		if b.UserPassID != order.UserPass.ID {
			t.Fatalf("booking %d is not paid by the pass", i)
		}
		if i > 0 && b.Slot.StartAt.Sub(series.Bookings[i-1].Slot.StartAt) < 6*24*time.Hour {
			t.Fatalf("series must go week by week")
		}
	}
	up, _ := e.store.GetUserPass(ctx, order.UserPass.ID)
	if up.VisitsLeft != 4-booked {
		t.Fatalf("visits left %d, want %d", up.VisitsLeft, 4-booked)
	}
	// Отмена возвращает визит.
	if _, err := e.bookings.Cancel(ctx, series.Bookings[0].ID, "client-2"); err != nil {
		t.Fatal(err)
	}
	up, _ = e.store.GetUserPass(ctx, order.UserPass.ID)
	if up.VisitsLeft != 4-booked+1 {
		t.Fatalf("visit was not refunded: %d", up.VisitsLeft)
	}
}

func TestYooKassaTrialPayment(t *testing.T) {
	e := newCommerceEnv(t)
	ctx := context.Background()
	slot := repeatingSlot(t, e.store, poolVenue, 1)
	view, _, err := e.bookings.Book(ctx, "client-3", "Ира", slot.ID, model.SourceChannelBot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.commerce.PayTrial(ctx, "client-3", view.ID); !errors.Is(err, service.ErrPaymentsOff) {
		t.Fatalf("payments are off, got %v", err)
	}
	// Включить оплату без ключа нельзя.
	if _, err := e.commerce.SaveSettings(ctx, e.admin, poolVenue, service.SettingsInput{PaymentsEnabled: boolp(true)}); !errors.Is(err, service.ErrInvalidRequest) {
		t.Fatalf("enabling without keys must fail, got %v", err)
	}
	// Демо-доступ не меняет настройки демо-студии.
	demo, _ := e.studio.ResolveDemo(ctx)
	if _, err := e.commerce.SaveSettings(ctx, demo, poolVenue, service.SettingsInput{ShowReviews: boolp(true)}); !errors.Is(err, service.ErrDemoReadOnly) {
		t.Fatalf("demo access must be read-only, got %v", err)
	}
	settings, err := e.commerce.SaveSettings(ctx, e.admin, poolVenue, service.SettingsInput{
		Provider: strp("yookassa"), Account: strp("shop-1"), Secret: strp("wrong"), PaymentsEnabled: boolp(true), TrialOnline: boolp(true)})
	if err != nil {
		t.Fatal(err)
	}
	if !settings.SecretSaved || settings.PaymentSecretHint != "••••" || !strings.HasSuffix(settings.YooKassaWebhook, "/api/v1/payments/yookassa/"+poolVenue) {
		t.Fatalf("settings view: %+v", settings)
	}
	if raw, _ := json.Marshal(settings); strings.Contains(string(raw), "wrong") {
		t.Fatal("secret must never be returned")
	}
	if err := e.commerce.CheckPayments(ctx, e.admin, poolVenue); !errors.Is(err, service.ErrPaymentFailed) {
		t.Fatalf("wrong key must fail the check, got %v", err)
	}
	if _, err := e.commerce.SaveSettings(ctx, e.admin, poolVenue, service.SettingsInput{Secret: strp("live_secret")}); err != nil {
		t.Fatal(err)
	}
	if err := e.commerce.CheckPayments(ctx, e.admin, poolVenue); err != nil {
		t.Fatalf("check: %v", err)
	}

	pay, err := e.commerce.PayTrial(ctx, "client-3", view.ID)
	if err != nil {
		t.Fatal(err)
	}
	if pay.Amount != 40000 || !strings.HasPrefix(pay.ConfirmationURL, "https://yoomoney.example/") {
		t.Fatalf("payment: %+v", pay)
	}
	again, err := e.commerce.PayTrial(ctx, "client-3", view.ID)
	if err != nil || again.ID != pay.ID {
		t.Fatalf("an open payment must be reused: %v", err)
	}
	if _, err := e.commerce.PayTrial(ctx, "stranger", view.ID); !errors.Is(err, service.ErrForbidden) {
		t.Fatalf("someone else's booking, got %v", err)
	}

	// Уведомление приходит, пока платёж ещё не прошёл — ничего не меняется.
	note := []byte(`{"type":"notification","event":"payment.succeeded","object":{"id":"yk-1","status":"succeeded"}}`)
	if err := e.commerce.YooKassaNotification(ctx, poolVenue, note); err != nil {
		t.Fatal(err)
	}
	if p, _ := e.store.GetPayment(ctx, pay.ID); p.Status != model.PaymentPending {
		t.Fatalf("unconfirmed notification must not mark payment paid: %s", p.Status)
	}
	e.yoo.set("yk-1", "succeeded")
	for i := 0; i < 2; i++ { // повторное уведомление не дублирует сообщения
		if err := e.commerce.YooKassaNotification(ctx, poolVenue, note); err != nil {
			t.Fatal(err)
		}
	}
	e.rec.expect(t, "payment-received:client-3:trial")
	e.rec.expect(t, "paid:700:trial")
	b, _ := e.store.GetBookingView(ctx, view.ID)
	if b.PaidAt == nil {
		t.Fatal("booking must be marked paid")
	}
	if _, err := e.commerce.PayTrial(ctx, "client-3", view.ID); !errors.Is(err, service.ErrNothingToPay) {
		t.Fatalf("paid booking, got %v", err)
	}
	// Уведомление для чужой студии игнорируется.
	if err := e.commerce.YooKassaNotification(ctx, seedFlagship, note); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("foreign venue, got %v", err)
	}
	totals, _ := e.commerce.StudioCommerce(ctx, e.admin, poolVenue)
	if totals.Month.Count != 1 || totals.Month.Amount != 400 {
		t.Fatalf("month totals: %+v", totals.Month)
	}
}

const seedFlagship = "a1111111-0000-0000-0000-000000000001"

func TestTBankPassPaymentNotification(t *testing.T) {
	e := newCommerceEnv(t)
	ctx := context.Background()
	if _, err := e.commerce.SaveSettings(ctx, e.admin, poolVenue, service.SettingsInput{
		Provider: strp("tbank"), Account: strp("TerminalDEMO"), Secret: strp("tb-password"), PaymentsEnabled: boolp(true)}); err != nil {
		t.Fatal(err)
	}
	pass, _ := e.commerce.SavePass(ctx, e.admin, poolVenue, model.Pass{Title: "Месяц плавания", Visits: 8, ValidDays: 30, Price: 3500, Active: true})
	order, err := e.commerce.RequestPass(ctx, "client-4", "Пётр", pass.ID)
	if err != nil || order.Payment == nil || order.Payment.Amount != 350000 {
		t.Fatalf("online pass order: %+v %v", order, err)
	}

	notification := func(status, password string) []byte {
		params := map[string]interface{}{"TerminalKey": "TerminalDEMO", "OrderId": order.Payment.ID, "Success": true,
			"Status": status, "PaymentId": json.Number("777001"), "ErrorCode": "0", "Amount": json.Number("350000"),
			"Pan": "430000******0777", "ExpDate": "1130", "Data": map[string]string{"x": "y"}}
		params["Token"] = payments.TBankToken(params, password)
		raw, _ := json.Marshal(params)
		return raw
	}
	if err := e.commerce.TBankNotification(ctx, poolVenue, notification("CONFIRMED", "forged")); !errors.Is(err, payments.ErrBadSignature) {
		t.Fatalf("forged notification, got %v", err)
	}
	if err := e.commerce.TBankNotification(ctx, poolVenue, notification("AUTHORIZED", "tb-password")); err != nil {
		t.Fatal(err)
	}
	if up, _ := e.store.GetUserPass(ctx, order.UserPass.ID); up.Status != model.UserPassRequested {
		t.Fatalf("authorized is not paid yet: %s", up.Status)
	}
	if err := e.commerce.TBankNotification(ctx, poolVenue, notification("CONFIRMED", "tb-password")); err != nil {
		t.Fatal(err)
	}
	e.rec.expect(t, "payment-received:client-4:pass")
	up, _ := e.store.GetUserPass(ctx, order.UserPass.ID)
	if up.Status != model.UserPassActive || up.PaidBy != "online" || up.VisitsLeft != 8 || up.ValidUntil == nil {
		t.Fatalf("pass after payment: %+v", up)
	}
}

func TestFriendPlaceClaimAndCancel(t *testing.T) {
	e := newCommerceEnv(t)
	ctx := context.Background()
	slot := repeatingSlot(t, e.store, poolVenue, 2)
	host, _, err := e.bookings.Book(ctx, "host-1", "Анна", slot.ID, model.SourceChannelMiniApp)
	if err != nil {
		t.Fatal(err)
	}
	place, err := e.bookings.AddFriend(ctx, host.ID, "host-1")
	if err != nil {
		t.Fatal(err)
	}
	if !place.Booking.IsGuest || !strings.Contains(place.Link, "startapp=friend_") || place.Claimed {
		t.Fatalf("friend place: %+v", place)
	}
	again, _ := e.bookings.AddFriend(ctx, host.ID, "host-1")
	if again.Booking.ID != place.Booking.ID {
		t.Fatal("second call must return the same place")
	}
	if s, _ := e.store.GetSlot(ctx, slot.ID); s.QuotaBooked != slot.QuotaBooked+2 {
		t.Fatalf("two places must be taken: %d -> %d", slot.QuotaBooked, s.QuotaBooked)
	}
	token := place.Link[strings.Index(place.Link, "friend_"):]
	if _, err := e.bookings.ClaimFriend(ctx, token, "host-1", "Анна"); !errors.Is(err, service.ErrOwnInvite) {
		t.Fatalf("host cannot claim own link, got %v", err)
	}
	friend, err := e.bookings.ClaimFriend(ctx, token, "friend-1", "Вера Петрова")
	if err != nil {
		t.Fatal(err)
	}
	if friend.MaxUserID != "friend-1" || friend.IsGuest || friend.CheckinCode == host.CheckinCode {
		t.Fatalf("claimed place: %+v", friend)
	}
	e.rec.expect(t, "friend-joined:host-1")
	if _, err := e.bookings.ClaimFriend(ctx, token, "friend-2", ""); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("link is single-use, got %v", err)
	}
	if p, _ := e.bookings.Friend(ctx, host.ID, "host-1"); p == nil || !p.Claimed || p.FriendName != "Вера" {
		t.Fatalf("host sees the friend: %+v", p)
	}

	// Непринятое место отменяется вместе с записью пригласившего.
	slot2 := repeatingSlot(t, e.store, seedFlagship, 2)
	host2, _, _ := e.bookings.Book(ctx, "host-2", "Илья", slot2.ID, model.SourceChannelMiniApp)
	guest2, err := e.bookings.AddFriend(ctx, host2.ID, "host-2")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.bookings.Cancel(ctx, host2.ID, "host-2"); err != nil {
		t.Fatal(err)
	}
	if g, _ := e.store.GetBooking(ctx, guest2.Booking.ID); g.Status != model.BookingStatusCancelled {
		t.Fatalf("unclaimed guest place must be cancelled: %s", g.Status)
	}
	if s, _ := e.store.GetSlot(ctx, slot2.ID); s.QuotaBooked != slot2.QuotaBooked {
		t.Fatalf("both places must be released: %d -> %d", slot2.QuotaBooked, s.QuotaBooked)
	}
}

func TestDigestAndUnansweredMessages(t *testing.T) {
	e := newCommerceEnv(t)
	ctx := context.Background()
	slot := repeatingSlot(t, e.store, poolVenue, 1)
	if _, _, err := e.bookings.Book(ctx, "client-5", "Нина", slot.ID, model.SourceChannelMiniApp); err != nil {
		t.Fatal(err)
	}
	e.rec.expect(t, "event:booked:700:client-5")

	// Сводка — в рабочее время в день занятия, один раз.
	local := slot.StartAt.In(catalog.Moscow)
	morning := time.Date(local.Year(), local.Month(), local.Day(), 9, 5, 0, 0, catalog.Moscow)
	e.studio.StudioTick(ctx, morning)
	e.rec.expect(t, "digest:700:1")
	e.studio.StudioTick(ctx, morning.Add(time.Hour))
	night := morning.Add(-6 * time.Hour)
	e.studio.StudioTick(ctx, night)

	if _, err := e.studio.SendClientMessage(ctx, "client-5", "Нина", poolVenue, "Есть ли фен?"); err != nil {
		t.Fatal(err)
	}
	e.rec.expect(t, "message:700:client-5:Есть ли фен?")
	now := time.Now().In(catalog.Moscow)
	noon := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, catalog.Moscow).AddDate(0, 0, 1)
	e.studio.StudioTick(ctx, noon)
	e.rec.expect(t, "waiting:700")
	// Клиенту контакты — только если у студии есть телефон.
	if _, err := e.studio.UpdateProfile(ctx, e.admin, poolVenue, service.VenueProfile{Phone: "+7 863 000-00-00"}); err != nil {
		t.Fatal(err)
	}
	e.studio.StudioTick(ctx, noon.Add(time.Minute))
	e.rec.expect(t, "client-waiting:client-5")

	stats, err := e.studio.Stats(ctx, e.admin, poolVenue)
	if err != nil || stats.Response.Waiting != 1 || stats.Response.Answered != 0 {
		t.Fatalf("response stats before reply: %+v %v", stats.Response, err)
	}
	thread, err := e.studio.Threads(ctx, e.admin, poolVenue)
	if err != nil || len(thread) != 1 {
		t.Fatalf("threads: %v %v", thread, err)
	}
	if _, err := e.studio.Reply(ctx, e.admin, poolVenue, thread[0].UserID, "Да, есть"); err != nil {
		t.Fatal(err)
	}
	stats, _ = e.studio.Stats(ctx, e.admin, poolVenue)
	if stats.Response.Answered != 1 || stats.Response.WithinHour != 1 || stats.Response.Waiting != 0 {
		t.Fatalf("response stats after reply: %+v", stats.Response)
	}
}
