package service_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"sportslot/internal/model"
	"sportslot/internal/repository/postgres"
	"sportslot/internal/seed"
	"sportslot/internal/service"
	"sportslot/internal/testdb"
)

type recorder struct {
	mu    sync.Mutex
	calls []string
	done  chan string
}

func newRecorder() *recorder { return &recorder{done: make(chan string, 16)} }

func (r *recorder) add(kind string, v *model.BookingView) error {
	r.mu.Lock()
	r.calls = append(r.calls, kind+":"+v.MaxUserID)
	r.mu.Unlock()
	r.done <- kind + ":" + v.MaxUserID
	return nil
}
func (r *recorder) Reminder24h(_ context.Context, v *model.BookingView) error { return r.add("24h", v) }
func (r *recorder) Reminder2h(_ context.Context, v *model.BookingView) error  { return r.add("2h", v) }
func (r *recorder) AskFeedback(_ context.Context, v *model.BookingView) error {
	return r.add("feedback", v)
}
func (r *recorder) Promoted(_ context.Context, v *model.BookingView) error {
	return r.add("promoted", v)
}

func (r *recorder) wait(t *testing.T, want string) {
	t.Helper()
	select {
	case got := <-r.done:
		if got != want {
			t.Fatalf("want message %q, got %q", want, got)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("message %q was not sent", want)
	}
}

// oneSeatSlot создаёт занятие с одним местом у флагманского демо-партнёра.
func oneSeatSlot(t *testing.T, store *postgres.Store, id string, start time.Time) {
	t.Helper()
	err := store.InsertSlotIfAbsent(context.Background(), &model.Slot{
		ID: id, VenueID: seed.FlagshipVenueID, Title: "Тест", SportType: "boxing", Level: "beginner",
		StartAt: start, EndAt: start.Add(time.Hour), QuotaTotal: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestBookIsIdempotentAndCancelReleasesSeat(t *testing.T) {
	store := testdb.New(t)
	ctx := context.Background()
	svc := service.NewBookingService(store)

	view, created, err := svc.Book(ctx, "100", "Анна", seed.ReferenceSlotOne, model.SourceChannelBot)
	if err != nil || !created {
		t.Fatalf("book: created=%v err=%v", created, err)
	}
	if len(view.CheckinCode) != 6 || view.Venue.ID != seed.FlagshipVenueID {
		t.Fatalf("unexpected view: %+v", view.Booking)
	}
	again, created, err := svc.Book(ctx, "100", "Анна", seed.ReferenceSlotOne, model.SourceChannelMiniApp)
	if err != nil || created || again.ID != view.ID {
		t.Fatalf("second booking must return the first one: created=%v err=%v", created, err)
	}
	slot, _ := store.GetSlot(ctx, seed.ReferenceSlotOne)
	if slot.QuotaBooked != 1 {
		t.Fatalf("quota booked = %d, want 1", slot.QuotaBooked)
	}

	if _, err := svc.Cancel(ctx, view.ID, "999"); !errors.Is(err, service.ErrForbidden) {
		t.Fatalf("foreign cancel: want ErrForbidden, got %v", err)
	}
	if _, err := svc.Cancel(ctx, view.ID, "100"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Cancel(ctx, view.ID, "100"); !errors.Is(err, service.ErrNotActive) {
		t.Fatalf("double cancel: want ErrNotActive, got %v", err)
	}
	slot, _ = store.GetSlot(ctx, seed.ReferenceSlotOne)
	if slot.QuotaBooked != 0 {
		t.Fatalf("quota booked after cancel = %d, want 0", slot.QuotaBooked)
	}
}

func TestLastSeatGoesToExactlyOnePerson(t *testing.T) {
	store := testdb.New(t)
	svc := service.NewBookingService(store)
	slotID := "d0000000-0000-0000-0000-000000000001"
	oneSeatSlot(t, store, slotID, time.Now().Add(48*time.Hour))

	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _, err := svc.Book(context.Background(), fmt.Sprintf("user-%d", i), "", slotID, model.SourceChannelBot)
			results <- err
		}(i)
	}
	wg.Wait()
	close(results)
	ok, full := 0, 0
	for err := range results {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, service.ErrQuotaExceeded):
			full++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if ok != 1 || full != 7 {
		t.Fatalf("ok=%d full=%d, want 1 and 7", ok, full)
	}
}

func TestCancelPromotesFirstFromWaitlist(t *testing.T) {
	store := testdb.New(t)
	ctx := context.Background()
	rec := newRecorder()
	svc := service.NewBookingService(store)
	svc.SetPresenter(rec)
	slotID := "d0000000-0000-0000-0000-000000000002"
	oneSeatSlot(t, store, slotID, time.Now().Add(48*time.Hour))

	first, _, err := svc.Book(ctx, "alice", "Алиса", slotID, model.SourceChannelBot)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.Book(ctx, "bob", "Борис", slotID, model.SourceChannelBot); !errors.Is(err, service.ErrQuotaExceeded) {
		t.Fatalf("want ErrQuotaExceeded, got %v", err)
	}
	for i, user := range []string{"bob", "carol"} {
		pos, err := svc.JoinWaitlist(ctx, user, "", slotID)
		if err != nil || pos != i+1 {
			t.Fatalf("waitlist %s: pos=%d err=%v", user, pos, err)
		}
	}
	if _, err := svc.Cancel(ctx, first.ID, "alice"); err != nil {
		t.Fatal(err)
	}
	rec.wait(t, "promoted:bob")

	_, views, err := svc.MyBookings(ctx, "bob", "")
	if err != nil || len(views) != 1 || views[0].Status != model.BookingStatusConfirmed {
		t.Fatalf("bob must be booked: %+v err=%v", views, err)
	}
	slot, _ := store.GetSlot(ctx, slotID)
	if slot.QuotaBooked != 1 {
		t.Fatalf("seat must stay taken, quota booked = %d", slot.QuotaBooked)
	}
}

func TestCheckInAsksFeedbackOnce(t *testing.T) {
	store := testdb.New(t)
	ctx := context.Background()
	rec := newRecorder()
	bookings := service.NewBookingService(store)
	partner := service.NewPartnerService(store, "demo-code-2026")
	partner.SetPresenter(rec)

	if _, _, err := partner.Login(ctx, "wrong"); !errors.Is(err, service.ErrPartnerCode) {
		t.Fatalf("want ErrPartnerCode, got %v", err)
	}
	token, _, err := partner.Login(ctx, "demo-code-2026")
	if err != nil || partner.Authorize(ctx, token) != nil {
		t.Fatalf("login failed: %v", err)
	}

	view, _, err := bookings.Book(ctx, "200", "Ира", seed.ReferenceSlotTwo, model.SourceChannelMiniApp)
	if err != nil {
		t.Fatal(err)
	}
	checked, err := partner.CheckIn(ctx, "SPORTSLOT:"+view.CheckinCode)
	if err != nil || checked.Status != model.BookingStatusAttended {
		t.Fatalf("check-in: %+v err=%v", checked, err)
	}
	rec.wait(t, "feedback:200")
	if _, err := partner.CheckIn(ctx, view.CheckinCode); !errors.Is(err, service.ErrNotActive) {
		t.Fatalf("second check-in: want ErrNotActive, got %v", err)
	}

	notifier := service.NewNotifier(store, rec, time.Minute, false)
	notifier.Tick(ctx, time.Now())
	select {
	case msg := <-rec.done:
		t.Fatalf("feedback must not be asked twice, got %q", msg)
	case <-time.After(200 * time.Millisecond):
	}

	if _, err := bookings.Rate(ctx, view.ID, "200", 5); err != nil {
		t.Fatal(err)
	}
}

func TestRemindersAreSentOnce(t *testing.T) {
	store := testdb.New(t)
	ctx := context.Background()
	rec := newRecorder()
	svc := service.NewBookingService(store)
	slotID := "d0000000-0000-0000-0000-000000000003"
	start := time.Now().Add(20 * time.Hour)
	oneSeatSlot(t, store, slotID, start)

	view, _, err := svc.Book(ctx, "300", "", slotID, model.SourceChannelBot)
	if err != nil {
		t.Fatal(err)
	}
	// Запись сделана заранее — за двое суток до занятия.
	testdb.Exec(t, `UPDATE bookings SET created_at = now() - interval '2 days' WHERE id = $1`, view.ID)

	notifier := service.NewNotifier(store, rec, time.Minute, false)
	notifier.Tick(ctx, time.Now())
	rec.wait(t, "24h:300")
	notifier.Tick(ctx, time.Now())
	notifier.Tick(ctx, start.Add(-90*time.Minute))
	rec.wait(t, "2h:300")
	notifier.Tick(ctx, start.Add(-60*time.Minute))
	select {
	case msg := <-rec.done:
		t.Fatalf("unexpected extra message %q", msg)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestRescheduleKeepsCheckinCode(t *testing.T) {
	store := testdb.New(t)
	ctx := context.Background()
	svc := service.NewBookingService(store)
	view, _, err := svc.Book(ctx, "400", "", seed.ReferenceSlotOne, model.SourceChannelBot)
	if err != nil {
		t.Fatal(err)
	}
	moved, err := svc.Reschedule(ctx, view.ID, seed.ReferenceSlotTwo, "400")
	if err != nil {
		t.Fatal(err)
	}
	if moved.SlotID != seed.ReferenceSlotTwo || moved.CheckinCode != view.CheckinCode {
		t.Fatalf("unexpected reschedule result: %+v", moved.Booking)
	}
	one, _ := store.GetSlot(ctx, seed.ReferenceSlotOne)
	two, _ := store.GetSlot(ctx, seed.ReferenceSlotTwo)
	if one.QuotaBooked != 0 || two.QuotaBooked != 1 {
		t.Fatalf("quota not moved: %d/%d", one.QuotaBooked, two.QuotaBooked)
	}
}

func TestSearchPutsBookableStudiosFirst(t *testing.T) {
	store := testdb.New(t)
	ctx := context.Background()
	if _, err := seed.LoadCatalog(ctx, store, "../../seed-data/rostov_catalog.json"); err != nil {
		t.Fatal(err)
	}
	search := service.NewSearchService(store, true, time.Now())
	cards, err := search.FindVenues(ctx, service.VenueSearch{Sport: "boxing", District: "sovetsky", When: "any"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) < 3 {
		t.Fatalf("want demo studios and catalog venues, got %d cards", len(cards))
	}
	if cards[0].Venue.Name != "Клуб единоборств «Стойка»" {
		t.Fatalf("nearest bookable studio must be first, got %s", cards[0].Venue.Name)
	}
	sawCatalog := false
	for _, c := range cards {
		if c.Venue.Source == model.VenueSourceOSM {
			sawCatalog = true
		} else if sawCatalog && c.AvailableSlots > 0 {
			t.Fatal("bookable studio after catalog venue")
		}
	}
	if !sawCatalog {
		t.Fatal("catalog venues are missing in results")
	}

	off := service.NewSearchService(store, false, time.Now())
	cards, _ = off.FindVenues(ctx, service.VenueSearch{Sport: "boxing", When: "any"}, time.Now())
	for _, c := range cards {
		if c.Venue.IsDemo() {
			t.Fatal("demo venue shown with DEMO_DATA=off")
		}
	}
}
