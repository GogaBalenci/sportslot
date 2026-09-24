package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"sportslot/internal/model"
	"sportslot/internal/repository"

	"github.com/google/uuid"
)

var (
	ErrQuotaExceeded = repository.ErrQuotaExceeded
	ErrNotFound      = repository.ErrNotFound
	ErrForbidden     = errors.New("booking does not belong to user")
)

type BookingService struct {
	bookingRepo repository.BookingRepository
	slotRepo    repository.SlotRepository
	userRepo    repository.UserRepository
}

func NewBookingService(
	bookingRepo repository.BookingRepository,
	slotRepo repository.SlotRepository,
	userRepo repository.UserRepository,
) *BookingService {
	return &BookingService{bookingRepo: bookingRepo, slotRepo: slotRepo, userRepo: userRepo}
}

type CreateBookingInput struct {
	MaxUserID     string
	UserName      string
	SlotID        string
	SourceChannel model.SourceChannel
}

// CreateBooking резолвит пользователя по MaxUserID (создавая при первом
// обращении) и атомарно бронирует слот в рамках гарантированной квоты.
func (s *BookingService) CreateBooking(ctx context.Context, in CreateBookingInput) (*model.Booking, error) {
	if in.SlotID == "" {
		return nil, fmt.Errorf("slot_id is required")
	}
	if in.SourceChannel != model.SourceChannelBot && in.SourceChannel != model.SourceChannelMiniApp {
		return nil, fmt.Errorf("invalid source_channel: %s", in.SourceChannel)
	}

	slot, err := s.slotRepo.GetByID(ctx, in.SlotID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, fmt.Errorf("slot not found: %w", ErrNotFound)
		}
		return nil, fmt.Errorf("get slot: %w", err)
	}

	if slot.StartAt.Before(time.Now()) {
		return nil, fmt.Errorf("slot is in the past")
	}

	user, err := s.userRepo.GetOrCreateByMaxUserID(ctx, in.MaxUserID, in.UserName)
	if err != nil {
		return nil, fmt.Errorf("resolve user: %w", err)
	}

	booking := &model.Booking{
		ID:            uuid.NewString(),
		UserID:        user.ID,
		SlotID:        in.SlotID,
		Status:        model.BookingStatusConfirmed,
		SourceChannel: in.SourceChannel,
		ReminderSent:  false,
		CreatedAt:     time.Now(),
	}

	if err := s.bookingRepo.CreateConfirmed(ctx, booking); err != nil {
		if errors.Is(err, repository.ErrQuotaExceeded) {
			return nil, ErrQuotaExceeded
		}
		return nil, fmt.Errorf("create booking: %w", err)
	}

	return booking, nil
}

func (s *BookingService) CancelBooking(ctx context.Context, bookingID, maxUserID string) error {
	if err := s.ensureBookingOwner(ctx, bookingID, maxUserID); err != nil {
		return err
	}
	if err := s.bookingRepo.CancelAndReleaseQuota(ctx, bookingID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrNotFound
		}
		return fmt.Errorf("cancel booking: %w", err)
	}
	return nil
}

func (s *BookingService) RescheduleBooking(ctx context.Context, bookingID, newSlotID, maxUserID string) (*model.Booking, error) {
	if err := s.ensureBookingOwner(ctx, bookingID, maxUserID); err != nil {
		return nil, err
	}
	if newSlotID == "" {
		return nil, fmt.Errorf("new_slot_id is required")
	}

	newSlot, err := s.slotRepo.GetByID(ctx, newSlotID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, fmt.Errorf("new slot not found: %w", ErrNotFound)
		}
		return nil, fmt.Errorf("get new slot: %w", err)
	}

	if newSlot.StartAt.Before(time.Now()) {
		return nil, fmt.Errorf("new slot is in the past")
	}

	booking, err := s.bookingRepo.RescheduleWithQuota(ctx, bookingID, newSlotID)
	if err != nil {
		if errors.Is(err, repository.ErrQuotaExceeded) {
			return nil, ErrQuotaExceeded
		}
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("reschedule booking: %w", err)
	}

	return booking, nil
}

func (s *BookingService) ListUserBookings(ctx context.Context, userID, maxUserID string) ([]model.Booking, error) {
	user, err := s.userRepo.GetOrCreateByMaxUserID(ctx, maxUserID, "")
	if err != nil {
		return nil, fmt.Errorf("resolve user: %w", err)
	}
	if user.ID != userID {
		return nil, ErrForbidden
	}

	bookings, err := s.bookingRepo.ListByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list bookings: %w", err)
	}
	return bookings, nil
}

func (s *BookingService) ensureBookingOwner(ctx context.Context, bookingID, maxUserID string) error {
	booking, err := s.bookingRepo.GetByID(ctx, bookingID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrNotFound
		}
		return fmt.Errorf("get booking: %w", err)
	}
	user, err := s.userRepo.GetOrCreateByMaxUserID(ctx, maxUserID, "")
	if err != nil {
		return fmt.Errorf("resolve user: %w", err)
	}
	if booking.UserID != user.ID {
		return ErrForbidden
	}
	return nil
}

func (s *BookingService) GetBooking(ctx context.Context, id string) (*model.Booking, error) {
	b, err := s.bookingRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get booking: %w", err)
	}
	return b, nil
}
