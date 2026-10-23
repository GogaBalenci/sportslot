package service_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"sportslot/internal/model"
	"sportslot/internal/repository/postgres"
	"sportslot/internal/seed"
	"sportslot/internal/service"
	"sportslot/internal/testdb"
)

// studioRecorder записывает сообщения кабинета студии.
type studioRecorder struct {
	mu   sync.Mutex
	done chan string
}

func newStudioRecorder() *studioRecorder { return &studioRecorder{done: make(chan string, 32)} }

func (r *studioRecorder) push(s string) error {
	r.done <- s
	return nil
}
func (r *studioRecorder) StudioEvent(_ context.Context, admin, kind string, v *model.BookingView) error {
	return r.push("event:" + kind + ":" + admin + ":" + v.MaxUserID)
}
func (r *studioRecorder) StudioMessage(_ context.Context, admin string, _ *model.Venue, c *model.User, text string, _ int64) error {
	return r.push("message:" + admin + ":" + c.MaxUserID + ":" + text)
}
func (r *studioRecorder) StudioReply(_ context.Context, client string, _ *model.Venue, text string) error {
	return r.push("reply:" + client + ":" + text)
}
func (r *studioRecorder) SlotCancelled(_ context.Context, v *model.BookingView) error {
	return r.push("cancelled:" + v.MaxUserID)
}

func (r *studioRecorder) StudioPassRequest(_ context.Context, admin string, up *model.UserPass) error {
	return r.push("passreq:" + admin + ":" + up.MaxUserID)
}
func (r *studioRecorder) StudioPayment(_ context.Context, admin string, p *model.Payment, _ *model.Venue) error {
	return r.push("paid:" + admin + ":" + p.Kind)
}
func (r *studioRecorder) StudioWaiting(_ context.Context, admin string, _ *model.Venue, _ string, _ time.Time, _ int64) error {
	return r.push("waiting:" + admin)
}
func (r *studioRecorder) ClientWaiting(_ context.Context, client string, _ *model.Venue) error {
	return r.push("client-waiting:" + client)
}
func (r *studioRecorder) StudioDigest(_ context.Context, admin string, _ time.Time, studios []service.DigestStudio) error {
	n := 0
	for _, s := range studios {
		n += len(s.Today)
	}
	return r.push(fmt.Sprintf("digest:%s:%d", admin, n))
}
func (r *studioRecorder) FriendInvite(_ context.Context, host string, _ *model.BookingView, _ string) error {
	return r.push("friend-invite:" + host)
}
func (r *studioRecorder) FriendJoined(_ context.Context, host, _ string, _ *model.BookingView) error {
	return r.push("friend-joined:" + host)
}
func (r *studioRecorder) PaymentReceived(_ context.Context, p *model.Payment, _ *model.BookingView, _ *model.UserPass) error {
	return r.push("payment-received:" + p.MaxUserID + ":" + p.Kind)
}
func (r *studioRecorder) PassActivated(_ context.Context, up *model.UserPass) error {
	return r.push("pass-active:" + up.MaxUserID)
}
func (r *studioRecorder) FriendPlaceReleased(_ context.Context, host string, _ *model.BookingView) error {
	return r.push("friend-released:" + host)
}
func (r *studioRecorder) PassDeclined(_ context.Context, up *model.UserPass) error {
	return r.push("pass-declined:" + up.MaxUserID)
}

func (r *studioRecorder) expect(t *testing.T, want string) {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case got := <-r.done:
			if got == want {
				return
			}
		case <-deadline:
			t.Fatalf("message %q was not sent", want)
		}
	}
}

type studioEnv struct {
	store    *postgres.Store
	bookings *service.BookingService
	studio   *service.StudioService
	rec      *studioRecorder
	platform service.Access
}

func newStudioEnv(t *testing.T) *studioEnv {
	store := testdb.New(t)
	ctx := context.Background()
	if _, err := seed.LoadCatalog(ctx, store, "../../seed-data/rostov_catalog.json"); err != nil {
		t.Fatal(err)
	}
	rec := newStudioRecorder()
	bookings := service.NewBookingService(store)
	partner := service.NewPartnerService(store, "demo-code")
	studio := service.NewStudioService(store, partner, []string{"owner"})
	studio.SetPresenter(rec)
	bookings.SetStudioNotifier(studio)
	platform, err := studio.ResolveUser(ctx, "owner", "Владелец")
	if err != nil || !platform.Platform {
		t.Fatalf("platform access: %+v %v", platform, err)
	}
	return &studioEnv{store: store, bookings: bookings, studio: studio, rec: rec, platform: platform}
}

// adminOf выпускает приглашение и принимает его от имени пользователя.
func (e *studioEnv) adminOf(t *testing.T, venueID, maxUserID string) service.Access {
	t.Helper()
	ctx := context.Background()
	inv, err := e.studio.CreateInvite(ctx, e.platform, venueID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(inv.Link, inv.Token) {
		t.Fatalf("invite link must contain the token: %s", inv.Link)
	}
	if _, err := e.studio.AcceptInvite(ctx, maxUserID, "Админ "+maxUserID, "studio_"+inv.Token); err != nil {
		t.Fatal(err)
	}
	// Повторное открытие тем же человеком — не ошибка, другим — ошибка.
	if _, err := e.studio.AcceptInvite(ctx, maxUserID, "", inv.Token); err != nil {
		t.Fatalf("reopening own invite: %v", err)
	}
	if _, err := e.studio.AcceptInvite(ctx, "stranger", "", inv.Token); !errors.Is(err, service.ErrInvalidInvite) {
		t.Fatalf("used invite must be rejected for others, got %v", err)
	}
	acc, err := e.studio.ResolveUser(ctx, maxUserID, "")
	if err != nil || !acc.Can(venueID) || acc.Platform {
		t.Fatalf("admin access: %+v %v", acc, err)
	}
	return acc
}

func TestConnectStudioAndScheduleTemplates(t *testing.T) {
	e := newStudioEnv(t)
	ctx := context.Background()

	found, err := e.studio.SearchCatalog(ctx, e.platform, "фитнес")
	if err != nil || len(found) == 0 {
		t.Fatalf("catalog search: %d %v", len(found), err)
	}
	venue, err := e.studio.Connect(ctx, e.platform, found[0].ID)
	if err != nil || venue.Source != model.VenueSourcePartner || venue.BookingMode != model.BookingModeInstant {
		t.Fatalf("connect: %+v %v", venue, err)
	}
	// Перезагрузка каталога OpenStreetMap не возвращает студию обратно в каталог.
	if _, err := seed.LoadCatalog(ctx, e.store, "../../seed-data/rostov_catalog.json"); err != nil {
		t.Fatal(err)
	}
	if v, _ := e.store.GetVenue(ctx, venue.ID); v.Source != model.VenueSourcePartner {
		t.Fatalf("catalog reload reverted the studio: %s", v.Source)
	}

	admin := e.adminOf(t, venue.ID, "500")
	if _, err := e.studio.CreateInvite(ctx, admin, venue.ID); !errors.Is(err, service.ErrNotPlatform) {
		t.Fatalf("studio admin must not issue invites, got %v", err)
	}
	if _, err := e.studio.Bookings(ctx, admin, seed.FlagshipVenueID); !errors.Is(err, service.ErrForbidden) {
		t.Fatalf("admin must not see another studio, got %v", err)
	}

	tomorrow := time.Now().In(time.Local).Add(24 * time.Hour)
	tpl := &model.Template{VenueID: venue.ID, Weekday: int(tomorrow.Weekday()), StartMinute: 19 * 60,
		DurationMin: 60, Title: "Функциональный тренинг", SportType: "fitness", Quota: 2}
	if _, err := e.studio.SaveTemplate(ctx, admin, tpl); err != nil {
		t.Fatal(err)
	}
	slots, err := e.studio.Slots(ctx, admin, venue.ID)
	if err != nil || len(slots) < 2 {
		t.Fatalf("template must give two weeks of lessons, got %d (%v)", len(slots), err)
	}
	first := slots[0].Slot

	if _, _, err := e.bookings.Book(ctx, "client-1", "Анна", first.ID, model.SourceChannelBot); err != nil {
		t.Fatal(err)
	}
	e.rec.expect(t, "event:booked:500:client-1")

	// Перенос времени в шаблоне: пустые занятия переезжают, занятие с записью остаётся.
	tpl.StartMinute = 20 * 60
	kept, err := e.studio.SaveTemplate(ctx, admin, tpl)
	if err != nil || kept != 1 {
		t.Fatalf("kept = %d, err = %v", kept, err)
	}
	if sl, err := e.store.GetSlot(ctx, first.ID); err != nil || sl.StartAt.Hour() != first.StartAt.Hour() {
		t.Fatalf("booked lesson must keep its time: %+v %v", sl, err)
	}

	// Закрыть запись — нельзя записаться; отменить занятие — клиент получает сообщение.
	if err := e.studio.CloseSlot(ctx, admin, venue.ID, first.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.bookings.Book(ctx, "client-2", "", first.ID, model.SourceChannelBot); !errors.Is(err, service.ErrBookingClosed) {
		t.Fatalf("closed lesson: want ErrBookingClosed, got %v", err)
	}
	n, err := e.studio.CancelSlot(ctx, admin, venue.ID, first.ID)
	if err != nil || n != 1 {
		t.Fatalf("cancel slot: %d %v", n, err)
	}
	e.rec.expect(t, "cancelled:client-1")
	_, views, _ := e.bookings.MyBookings(ctx, "client-1", "")
	if len(views) != 0 {
		t.Fatalf("cancelled booking must disappear from active list: %+v", views)
	}

	if _, err := e.studio.DeleteTemplate(ctx, admin, venue.ID, tpl.ID); err != nil {
		t.Fatal(err)
	}
}

func TestStudioNotificationsMessagesAndStats(t *testing.T) {
	e := newStudioEnv(t)
	ctx := context.Background()
	admin := e.adminOf(t, seed.FlagshipVenueID, "700")

	view, _, err := e.bookings.Book(ctx, "client-9", "Борис", seed.ReferenceSlotOne, model.SourceChannelMiniApp)
	if err != nil {
		t.Fatal(err)
	}
	e.rec.expect(t, "event:booked:700:client-9")
	if err := e.bookings.MarkGoing(ctx, view.ID, "client-9"); err != nil {
		t.Fatal(err)
	}
	e.rec.expect(t, "event:going:700:client-9")

	card, err := e.studio.Booking(ctx, admin, view.ID)
	if err != nil || card.MaxUserID != "client-9" || len(card.History) != 1 {
		t.Fatalf("client card: %+v %v", card, err)
	}

	if _, err := e.studio.SendClientMessage(ctx, "client-9", "Борис", seed.FlagshipVenueID, "Нужна ли своя форма?"); err != nil {
		t.Fatal(err)
	}
	e.rec.expect(t, "message:700:client-9:Нужна ли своя форма?")
	threads, err := e.studio.Threads(ctx, admin, seed.FlagshipVenueID)
	if err != nil || len(threads) != 1 || threads[0].Unread != 1 {
		t.Fatalf("threads: %+v %v", threads, err)
	}
	if _, err := e.studio.Reply(ctx, admin, seed.FlagshipVenueID, threads[0].UserID, "Форму дадим"); err != nil {
		t.Fatal(err)
	}
	e.rec.expect(t, "reply:client-9:Форму дадим")
	msgs, _ := e.studio.ClientThread(ctx, "client-9", seed.FlagshipVenueID)
	if len(msgs) != 2 || msgs[1].Author != "studio" {
		t.Fatalf("client thread: %+v", msgs)
	}
	threads, _ = e.studio.Threads(ctx, admin, seed.FlagshipVenueID)
	if threads[0].Unread != 0 {
		t.Fatal("reply must mark the thread read")
	}

	if _, err := e.studio.CheckIn(ctx, admin, "SPORTSLOT:"+view.CheckinCode); err != nil {
		t.Fatal(err)
	}
	if _, err := e.studio.CheckIn(ctx, admin, "SPORTSLOT:000000"); !errors.Is(err, service.ErrTrainingCheckin) {
		t.Fatalf("training code: want ErrTrainingCheckin, got %v", err)
	}
	stats, err := e.studio.Stats(ctx, admin, seed.FlagshipVenueID)
	if err != nil || stats.Bookings != 1 || stats.Attended != 1 || stats.Clients != 1 {
		t.Fatalf("stats: %+v %v", stats, err)
	}

	// Общий демо-код видит демо-студии, но не управляет платформой и профилем.
	demo, _ := e.studio.ResolveDemo(ctx)
	if !demo.Can(seed.FlagshipVenueID) {
		t.Fatal("demo access must include demo studios")
	}
	if _, err := e.studio.PlatformStudios(ctx, demo); !errors.Is(err, service.ErrNotPlatform) {
		t.Fatalf("demo must not see the platform, got %v", err)
	}
	if _, err := e.studio.UpdateProfile(ctx, demo, seed.FlagshipVenueID, service.VenueProfile{Phone: "1"}); !errors.Is(err, service.ErrDemoReadOnly) {
		t.Fatalf("demo must not edit profiles, got %v", err)
	}
}
