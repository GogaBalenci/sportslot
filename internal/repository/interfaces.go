package repository

import (
	"context"
	"time"

	"sportslot/internal/model"
)

type VenueRepository interface {
	GetByID(ctx context.Context, id string) (*model.Venue, error)
	Create(ctx context.Context, v *model.Venue) error
	List(ctx context.Context) ([]model.Venue, error)
}

type SlotSearchParams struct {
	SportType string
	Level     string
	DateFrom  time.Time
	DateTo    time.Time
	TimeFrom  string // "HH:MM", опционально
	TimeTo    string // "HH:MM", опционально
	Lat       float64
	Lon       float64
	RadiusKM  float64
}

type SlotSearchRow struct {
	Venue model.Venue
	Slot  model.Slot
}

type SlotRepository interface {
	Create(ctx context.Context, s *model.Slot) error
	GetByID(ctx context.Context, id string) (*model.Slot, error)
	Search(ctx context.Context, params SlotSearchParams) ([]SlotSearchRow, error)
	ListFutureByVenueID(ctx context.Context, venueID string) ([]model.Slot, error)
	IncrementQuota(ctx context.Context, slotID string) error
	DecrementQuota(ctx context.Context, slotID string) error
}

type BookingRepository interface {
	CreateConfirmed(ctx context.Context, b *model.Booking) error
	GetByID(ctx context.Context, id string) (*model.Booking, error)
	ListByUserID(ctx context.Context, userID string) ([]model.Booking, error)
	CancelAndReleaseQuota(ctx context.Context, bookingID string) error
	RescheduleWithQuota(ctx context.Context, bookingID, newSlotID string) (*model.Booking, error)
	FindUpcomingForReminder(ctx context.Context, windowStart, windowEnd time.Time) ([]model.UpcomingReminder, error)
	MarkReminderSent(ctx context.Context, id string) error
}

type UserRepository interface {
	GetOrCreateByMaxUserID(ctx context.Context, maxUserID string, name string) (*model.User, error)
	GetByID(ctx context.Context, id string) (*model.User, error)
}
