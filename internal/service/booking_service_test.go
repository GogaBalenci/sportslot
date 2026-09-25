package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"sportslot/internal/model"
	"sportslot/internal/repository"
	"sportslot/internal/service"
)

type mockSlotRepo struct {
	getByIDFunc func(ctx context.Context, id string) (*model.Slot, error)
}

func (m *mockSlotRepo) Create(ctx context.Context, s *model.Slot) error { return nil }
func (m *mockSlotRepo) GetByID(ctx context.Context, id string) (*model.Slot, error) {
	if m.getByIDFunc != nil {
		return m.getByIDFunc(ctx, id)
	}
	return nil, repository.ErrNotFound
}
func (m *mockSlotRepo) Search(ctx context.Context, params repository.SlotSearchParams) ([]repository.SlotSearchRow, error) {
	return nil, nil
}
func (m *mockSlotRepo) ListFutureByVenueID(ctx context.Context, venueID string) ([]model.Slot, error) {
	return nil, nil
}
func (m *mockSlotRepo) IncrementQuota(ctx context.Context, slotID string) error { return nil }
func (m *mockSlotRepo) DecrementQuota(ctx context.Context, slotID string) error { return nil }

type mockBookingRepo struct {
	createConfirmedFunc func(ctx context.Context, b *model.Booking) error
	getByIDFunc         func(ctx context.Context, id string) (*model.Booking, error)
	cancelFunc          func(ctx context.Context, bookingID string) error
	rescheduleFunc      func(ctx context.Context, bookingID, newSlotID string) (*model.Booking, error)
	listByUserIDFunc    func(ctx context.Context, userID string) ([]model.Booking, error)
}

func (m *mockBookingRepo) CreateConfirmed(ctx context.Context, b *model.Booking) error {
	if m.createConfirmedFunc != nil {
		return m.createConfirmedFunc(ctx, b)
	}
	return nil
}
func (m *mockBookingRepo) GetByID(ctx context.Context, id string) (*model.Booking, error) {
	if m.getByIDFunc != nil {
		return m.getByIDFunc(ctx, id)
	}
	return nil, repository.ErrNotFound
}
func (m *mockBookingRepo) ListByUserID(ctx context.Context, userID string) ([]model.Booking, error) {
	if m.listByUserIDFunc != nil {
		return m.listByUserIDFunc(ctx, userID)
	}
	return nil, nil
}
func (m *mockBookingRepo) CancelAndReleaseQuota(ctx context.Context, bookingID string) error {
	if m.cancelFunc != nil {
		return m.cancelFunc(ctx, bookingID)
	}
	return nil
}
func (m *mockBookingRepo) RescheduleWithQuota(ctx context.Context, bookingID, newSlotID string) (*model.Booking, error) {
	if m.rescheduleFunc != nil {
		return m.rescheduleFunc(ctx, bookingID, newSlotID)
	}
	return nil, nil
}
func (m *mockBookingRepo) FindUpcomingForReminder(ctx context.Context, windowStart, windowEnd time.Time) ([]model.UpcomingReminder, error) {
	return nil, nil
}
func (m *mockBookingRepo) MarkReminderSent(ctx context.Context, id string) error { return nil }

type mockUserRepo struct {
	getOrCreateFunc func(ctx context.Context, maxUserID string, name string) (*model.User, error)
	getByIDFunc     func(ctx context.Context, id string) (*model.User, error)
}

func (m *mockUserRepo) GetOrCreateByMaxUserID(ctx context.Context, maxUserID string, name string) (*model.User, error) {
	if m.getOrCreateFunc != nil {
		return m.getOrCreateFunc(ctx, maxUserID, name)
	}
	return &model.User{ID: "user-1", MaxUserID: maxUserID, Name: name}, nil
}
func (m *mockUserRepo) GetByID(ctx context.Context, id string) (*model.User, error) {
	if m.getByIDFunc != nil {
		return m.getByIDFunc(ctx, id)
	}
	return nil, repository.ErrNotFound
}

func TestCreateBooking_Validation(t *testing.T) {
	svc := service.NewBookingService(&mockBookingRepo{}, &mockSlotRepo{}, &mockUserRepo{})

	// 1. Missing slot_id
	_, err := svc.CreateBooking(context.Background(), service.CreateBookingInput{
		MaxUserID:     "user-1",
		SlotID:        "",
		SourceChannel: model.SourceChannelMiniApp,
	})
	if err == nil || err.Error() != "slot_id is required" {
		t.Fatalf("expected slot_id is required, got: %v", err)
	}

	// 2. Invalid source_channel
	_, err = svc.CreateBooking(context.Background(), service.CreateBookingInput{
		MaxUserID:     "user-1",
		SlotID:        "slot-1",
		SourceChannel: "telegram",
	})
	if err == nil {
		t.Fatalf("expected error for invalid channel, got nil")
	}
}

func TestCreateBooking_SlotChecks(t *testing.T) {
	ctx := context.Background()

	// 1. Slot not found
	slotRepo := &mockSlotRepo{
		getByIDFunc: func(ctx context.Context, id string) (*model.Slot, error) {
			return nil, repository.ErrNotFound
		},
	}
	svc := service.NewBookingService(&mockBookingRepo{}, slotRepo, &mockUserRepo{})

	_, err := svc.CreateBooking(ctx, service.CreateBookingInput{
		MaxUserID:     "u1",
		SlotID:        "slot-unknown",
		SourceChannel: model.SourceChannelBot,
	})
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}

	// 2. Slot is in the past
	pastSlot := &model.Slot{
		ID:          "slot-past",
		StartAt:     time.Now().Add(-2 * time.Hour),
		EndAt:       time.Now().Add(-1 * time.Hour),
		QuotaTotal:  3,
		QuotaBooked: 0,
	}
	slotRepo.getByIDFunc = func(ctx context.Context, id string) (*model.Slot, error) {
		return pastSlot, nil
	}

	_, err = svc.CreateBooking(ctx, service.CreateBookingInput{
		MaxUserID:     "u1",
		SlotID:        "slot-past",
		SourceChannel: model.SourceChannelBot,
	})
	if err == nil || err.Error() != "slot is in the past" {
		t.Fatalf("expected 'slot is in the past' error, got: %v", err)
	}
}

func TestCreateBooking_SuccessAndQuotaExceeded(t *testing.T) {
	ctx := context.Background()

	futureSlot := &model.Slot{
		ID:          "slot-future",
		StartAt:     time.Now().Add(24 * time.Hour),
		EndAt:       time.Now().Add(25 * time.Hour),
		QuotaTotal:  3,
		QuotaBooked: 1,
	}
	slotRepo := &mockSlotRepo{
		getByIDFunc: func(ctx context.Context, id string) (*model.Slot, error) {
			return futureSlot, nil
		},
	}

	// 1. Success
	bookingRepo := &mockBookingRepo{
		createConfirmedFunc: func(ctx context.Context, b *model.Booking) error {
			return nil
		},
	}
	userRepo := &mockUserRepo{
		getOrCreateFunc: func(ctx context.Context, maxUserID, name string) (*model.User, error) {
			return &model.User{ID: "uuid-user-1", MaxUserID: maxUserID}, nil
		},
	}
	svc := service.NewBookingService(bookingRepo, slotRepo, userRepo)

	b, err := svc.CreateBooking(ctx, service.CreateBookingInput{
		MaxUserID:     "max-u1",
		SlotID:        "slot-future",
		SourceChannel: model.SourceChannelMiniApp,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b.Status != model.BookingStatusConfirmed {
		t.Errorf("expected confirmed status, got %s", b.Status)
	}
	if b.UserID != "uuid-user-1" {
		t.Errorf("expected user uuid-user-1, got %s", b.UserID)
	}

	// 2. Quota exceeded
	bookingRepo.createConfirmedFunc = func(ctx context.Context, b *model.Booking) error {
		return repository.ErrQuotaExceeded
	}

	_, err = svc.CreateBooking(ctx, service.CreateBookingInput{
		MaxUserID:     "max-u1",
		SlotID:        "slot-future",
		SourceChannel: model.SourceChannelMiniApp,
	})
	if !errors.Is(err, service.ErrQuotaExceeded) {
		t.Fatalf("expected ErrQuotaExceeded, got: %v", err)
	}
}

func TestCancelBooking(t *testing.T) {
	ctx := context.Background()

	bookingRepo := &mockBookingRepo{
		getByIDFunc: func(ctx context.Context, id string) (*model.Booking, error) {
			return &model.Booking{ID: id, UserID: "user-owner-uuid"}, nil
		},
		cancelFunc: func(ctx context.Context, bookingID string) error {
			return nil
		},
	}
	userRepo := &mockUserRepo{
		getOrCreateFunc: func(ctx context.Context, maxUserID, name string) (*model.User, error) {
			if maxUserID == "owner" {
				return &model.User{ID: "user-owner-uuid", MaxUserID: "owner"}, nil
			}
			return &model.User{ID: "other-user-uuid", MaxUserID: "stranger"}, nil
		},
	}
	svc := service.NewBookingService(bookingRepo, &mockSlotRepo{}, userRepo)

	// 1. Success by owner
	if err := svc.CancelBooking(ctx, "b-1", "owner"); err != nil {
		t.Fatalf("expected success, got: %v", err)
	}

	// 2. Forbidden by stranger
	err := svc.CancelBooking(ctx, "b-1", "stranger")
	if !errors.Is(err, service.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got: %v", err)
	}
}

func TestRescheduleBooking(t *testing.T) {
	ctx := context.Background()

	slotFuture := &model.Slot{
		ID:      "slot-new",
		StartAt: time.Now().Add(48 * time.Hour),
		EndAt:   time.Now().Add(49 * time.Hour),
	}
	slotRepo := &mockSlotRepo{
		getByIDFunc: func(ctx context.Context, id string) (*model.Slot, error) {
			if id == "slot-new" {
				return slotFuture, nil
			}
			return nil, repository.ErrNotFound
		},
	}

	bookingRepo := &mockBookingRepo{
		getByIDFunc: func(ctx context.Context, id string) (*model.Booking, error) {
			return &model.Booking{ID: id, UserID: "user-owner-uuid", SlotID: "slot-old"}, nil
		},
		rescheduleFunc: func(ctx context.Context, bookingID, newSlotID string) (*model.Booking, error) {
			return &model.Booking{ID: bookingID, UserID: "user-owner-uuid", SlotID: newSlotID, Status: model.BookingStatusConfirmed}, nil
		},
	}
	userRepo := &mockUserRepo{
		getOrCreateFunc: func(ctx context.Context, maxUserID, name string) (*model.User, error) {
			return &model.User{ID: "user-owner-uuid", MaxUserID: maxUserID}, nil
		},
	}
	svc := service.NewBookingService(bookingRepo, slotRepo, userRepo)

	// 1. Reschedule success
	b, err := svc.RescheduleBooking(ctx, "b-1", "slot-new", "owner")
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if b.SlotID != "slot-new" {
		t.Errorf("expected slot-new, got: %s", b.SlotID)
	}

	// 2. Reschedule to unknown slot
	_, err = svc.RescheduleBooking(ctx, "b-1", "slot-unknown", "owner")
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for missing slot, got: %v", err)
	}
}
