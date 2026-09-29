package service

import (
	"context"
	"log"
	"time"

	"sportslot/internal/model"
	"sportslot/internal/repository/postgres"
)

type BookingService struct {
	store     *postgres.Store
	presenter Presenter
}

func NewBookingService(store *postgres.Store) *BookingService {
	return &BookingService{store: store}
}

// SetPresenter подключает отправку сообщений; без него сервис работает молча (тесты, локальный запуск).
func (s *BookingService) SetPresenter(p Presenter) { s.presenter = p }

func (s *BookingService) Store() *postgres.Store { return s.store }

// Book записывает пользователя на занятие. created=false — запись уже была.
func (s *BookingService) Book(ctx context.Context, maxUserID, userName, slotID string, channel model.SourceChannel) (*model.BookingView, bool, error) {
	slot, err := s.store.GetSlot(ctx, slotID)
	if err != nil {
		return nil, false, err
	}
	venue, err := s.store.GetVenue(ctx, slot.VenueID)
	if err != nil {
		return nil, false, err
	}
	if venue.BookingMode != model.BookingModeInstant {
		return nil, false, ErrExternalVenue
	}
	user, err := s.store.EnsureUser(ctx, maxUserID, userName)
	if err != nil {
		return nil, false, err
	}
	b, created, err := s.store.CreateBooking(ctx, user.ID, slotID, channel)
	if err != nil {
		return nil, false, err
	}
	if created {
		s.event(ctx, maxUserID, "booking_created", map[string]interface{}{"slot_id": slotID, "channel": channel})
	}
	view, err := s.store.GetBookingView(ctx, b.ID)
	return view, created, err
}

func (s *BookingService) owned(ctx context.Context, bookingID, maxUserID string) (*model.BookingView, error) {
	view, err := s.store.GetBookingView(ctx, bookingID)
	if err != nil {
		return nil, err
	}
	if view.MaxUserID != maxUserID {
		return nil, ErrForbidden
	}
	return view, nil
}

// Cancel отменяет запись. Освободившееся место сразу получает первый из листа ожидания.
func (s *BookingService) Cancel(ctx context.Context, bookingID, maxUserID string) (*model.BookingView, error) {
	view, err := s.owned(ctx, bookingID, maxUserID)
	if err != nil {
		return nil, err
	}
	promotion, err := s.store.CancelBooking(ctx, bookingID)
	if err != nil {
		return nil, err
	}
	view.Status = model.BookingStatusCancelled
	s.event(ctx, maxUserID, "booking_cancelled", map[string]interface{}{"booking_id": bookingID})
	s.notifyPromotion(promotion)
	return view, nil
}

func (s *BookingService) Reschedule(ctx context.Context, bookingID, newSlotID, maxUserID string) (*model.BookingView, error) {
	if _, err := s.owned(ctx, bookingID, maxUserID); err != nil {
		return nil, err
	}
	newSlot, err := s.store.GetSlot(ctx, newSlotID)
	if err != nil {
		return nil, err
	}
	venue, err := s.store.GetVenue(ctx, newSlot.VenueID)
	if err != nil {
		return nil, err
	}
	if venue.BookingMode != model.BookingModeInstant {
		return nil, ErrExternalVenue
	}
	b, promotion, err := s.store.RescheduleBooking(ctx, bookingID, newSlotID)
	if err != nil {
		return nil, err
	}
	s.event(ctx, maxUserID, "booking_rescheduled", map[string]interface{}{"booking_id": bookingID})
	s.notifyPromotion(promotion)
	return s.store.GetBookingView(ctx, b.ID)
}

// Get возвращает запись пользователя (например, для экрана QR-пропуска).
func (s *BookingService) Get(ctx context.Context, bookingID, maxUserID string) (*model.BookingView, error) {
	return s.owned(ctx, bookingID, maxUserID)
}

func (s *BookingService) MyBookings(ctx context.Context, maxUserID, name string) (*model.User, []model.BookingView, error) {
	user, err := s.store.EnsureUser(ctx, maxUserID, name)
	if err != nil {
		return nil, nil, err
	}
	views, err := s.store.ListUserBookingViews(ctx, user.ID)
	if views == nil {
		views = []model.BookingView{}
	}
	return user, views, err
}

// ListByUserID — исторический метод GET /bookings/user/{user_id}: только свои брони.
func (s *BookingService) ListByUserID(ctx context.Context, userID, maxUserID string) ([]model.Booking, error) {
	user, err := s.store.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user.MaxUserID != maxUserID {
		return nil, ErrForbidden
	}
	return s.store.ListUserBookings(ctx, userID)
}

func (s *BookingService) JoinWaitlist(ctx context.Context, maxUserID, name, slotID string) (int, error) {
	slot, err := s.store.GetSlot(ctx, slotID)
	if err != nil {
		return 0, err
	}
	if !slot.StartAt.After(time.Now()) {
		return 0, ErrSlotStarted
	}
	user, err := s.store.EnsureUser(ctx, maxUserID, name)
	if err != nil {
		return 0, err
	}
	pos, err := s.store.JoinWaitlist(ctx, user.ID, slotID)
	if err == nil {
		s.event(ctx, maxUserID, "waitlist_joined", map[string]interface{}{"slot_id": slotID})
	}
	return pos, err
}

func (s *BookingService) Rate(ctx context.Context, bookingID, maxUserID string, rating int) (*model.BookingView, error) {
	if rating < 1 || rating > 5 {
		return nil, ErrInvalidRequest
	}
	view, err := s.owned(ctx, bookingID, maxUserID)
	if err != nil {
		return nil, err
	}
	if err := s.store.SetRating(ctx, bookingID, rating); err != nil {
		return nil, err
	}
	s.event(ctx, maxUserID, "feedback", map[string]interface{}{"booking_id": bookingID, "rating": rating})
	return view, nil
}

// NextWeek ищет то же занятие через неделю для повторной записи.
func (s *BookingService) NextWeek(ctx context.Context, view *model.BookingView) (*model.Slot, error) {
	return s.store.FindSlotNextWeek(ctx, &view.Slot)
}

func (s *BookingService) notifyPromotion(p *postgres.Promotion) {
	if p == nil || s.presenter == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		view, err := s.store.GetBookingView(ctx, p.BookingID)
		if err != nil {
			log.Printf("waitlist promotion %s: %v", p.BookingID, err)
			return
		}
		if err := s.presenter.Promoted(ctx, view); err != nil {
			log.Printf("waitlist promotion message %s: %v", p.BookingID, err)
		}
		s.event(ctx, p.MaxUserID, "waitlist_promoted", map[string]interface{}{"booking_id": p.BookingID})
	}()
}

func (s *BookingService) event(ctx context.Context, maxUserID, kind string, payload map[string]interface{}) {
	if err := s.store.LogEvent(ctx, maxUserID, kind, payload); err != nil {
		log.Printf("event %s: %v", kind, err)
	}
}

// LogEvent пишет шаг воронки, который происходит вне записи (старт, подбор, поиск).
func (s *BookingService) LogEvent(ctx context.Context, maxUserID, kind string, payload map[string]interface{}) {
	s.event(ctx, maxUserID, kind, payload)
}
