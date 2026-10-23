package service

import (
	"context"
	"log"
	"time"
)

// Ограничения от злоупотреблений: один человек не должен занять все места,
// засыпать студию заявками или сообщениями.
const (
	maxUpcomingBookings   = 10            // своих активных записей на будущее
	maxBookingsPerHour    = 12            // новых записей за час, включая места для друзей
	maxOpenFriendPlaces   = 2             // непринятых мест для друзей одновременно
	friendReleaseBefore   = 2 * time.Hour // непринятое место освобождается за 2 часа до начала
	maxOpenPassRequests   = 3             // неоплаченных заявок на абонементы
	maxPassRequestsPerDay = 5
	maxMessagesToStudio   = 5 // сообщений одной студии за 10 минут
	maxMessagesPerHour    = 20
)

// LimitError — действие отклонено лимитом; Message показывается пользователю.
type LimitError struct{ Message string }

func (e *LimitError) Error() string { return "limit: " + e.Message }

func limit(msg string) error { return &LimitError{Message: msg} }

// checkBookingLimits проверяет лимиты перед новой записью пользователя.
func (s *BookingService) checkBookingLimits(ctx context.Context, userID string, now time.Time) error {
	upcoming, err := s.store.CountUpcomingBookings(ctx, userID, now)
	if err != nil {
		return err
	}
	if upcoming >= maxUpcomingBookings {
		return limit("У тебя уже 10 будущих записей. Отмени ненужные, чтобы записаться ещё.")
	}
	recent, err := s.store.CountBookingsSince(ctx, userID, now.Add(-time.Hour))
	if err != nil {
		return err
	}
	if recent >= maxBookingsPerHour {
		return limit("Слишком много записей за последний час. Попробуй чуть позже.")
	}
	return nil
}

// ReleaseUnclaimedFriends освобождает места для друзей, которые не приняли
// приглашение за 2 часа до начала, — их получают другие или лист ожидания.
func (s *BookingService) ReleaseUnclaimedFriends(ctx context.Context, now time.Time) {
	guests, err := s.store.UnclaimedGuests(ctx, now.Add(friendReleaseBefore))
	if err != nil {
		log.Printf("friends: unclaimed: %v", err)
		return
	}
	for i := range guests {
		g := &guests[i]
		promotion, err := s.store.CancelBooking(ctx, g.ID)
		if err != nil {
			continue
		}
		s.notifyPromotion(promotion)
		s.event(ctx, g.MaxUserID, "friend_place_released", map[string]interface{}{"booking_id": g.ID})
		if s.client != nil {
			if err := s.client.FriendPlaceReleased(ctx, g.MaxUserID, g); err != nil {
				log.Printf("friends: released message: %v", err)
			}
		}
	}
}
