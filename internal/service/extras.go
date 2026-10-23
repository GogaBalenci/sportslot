package service

import (
	"context"
	"log"
	"strings"
	"time"
	"unicode/utf8"

	"sportslot/internal/model"
)

// ClientPresenter — сообщения клиенту о друзьях, оплатах и абонементах (пакет bot).
type ClientPresenter interface {
	FriendInvite(ctx context.Context, hostMaxID string, guest *model.BookingView, link string) error
	FriendJoined(ctx context.Context, hostMaxID, friendName string, v *model.BookingView) error
	PaymentReceived(ctx context.Context, p *model.Payment, v *model.BookingView, up *model.UserPass) error
	PassActivated(ctx context.Context, up *model.UserPass) error
	PassDeclined(ctx context.Context, up *model.UserPass) error
	FriendPlaceReleased(ctx context.Context, hostMaxID string, guest *model.BookingView) error
}

// SetClientPresenter подключает сообщения о друзьях, оплатах и абонементах.
func (s *BookingService) SetClientPresenter(p ClientPresenter) { s.client = p }

// SetDeepLink — ссылка, открывающая мини-приложение бота с параметром запуска.
func (s *BookingService) SetDeepLink(f func(param string) string) { s.deepLink = f }

func (s *BookingService) link(param string) string {
	if s.deepLink == nil {
		return param
	}
	return s.deepLink(param)
}

// Review — пара слов к оценке. Отзыв увидят другие, если студия включила показ отзывов.
func (s *BookingService) Review(ctx context.Context, bookingID, maxUserID, text string) error {
	text = strings.TrimSpace(text)
	if text == "" || utf8.RuneCountInString(text) > 500 {
		return ErrInvalidRequest
	}
	if _, err := s.owned(ctx, bookingID, maxUserID); err != nil {
		return err
	}
	if err := s.store.SetReview(ctx, bookingID, text); err != nil {
		return err
	}
	s.event(ctx, maxUserID, "review", map[string]interface{}{"booking_id": bookingID})
	return nil
}

// SeriesResult — запись на то же занятие на несколько недель вперёд.
type SeriesResult struct {
	Bookings []model.BookingView `json:"bookings"`
	// Skipped — недели, на которые записаться не получилось (нет мест, закрыто).
	Skipped []time.Time `json:"skipped"`
}

// BookSeries записывает на занятие и на то же время в следующие недели (всего weeks).
func (s *BookingService) BookSeries(ctx context.Context, maxUserID, name, slotID string, weeks int, channel model.SourceChannel) (*SeriesResult, error) {
	if weeks < 1 || weeks > 4 {
		return nil, ErrInvalidRequest
	}
	first, _, err := s.Book(ctx, maxUserID, name, slotID, channel)
	if err != nil {
		return nil, err
	}
	res := &SeriesResult{Bookings: []model.BookingView{*first}, Skipped: []time.Time{}}
	if weeks == 1 {
		return res, nil
	}
	next, err := s.store.SeriesSlots(ctx, &first.Slot, weeks-1)
	if err != nil {
		return res, nil
	}
	for i := range next {
		v, _, err := s.Book(ctx, maxUserID, name, next[i].ID, channel)
		if err != nil {
			res.Skipped = append(res.Skipped, next[i].StartAt)
			continue
		}
		res.Bookings = append(res.Bookings, *v)
	}
	return res, nil
}

// FriendPlace — второе место для друга и ссылка, по которой он его примет.
type FriendPlace struct {
	Booking model.BookingView `json:"booking"`
	Link    string            `json:"link,omitempty"`
	// Claimed — друг уже открыл ссылку; тогда ссылки нет, есть имя.
	Claimed    bool   `json:"claimed"`
	FriendName string `json:"friend_name,omitempty"`
}

// AddFriend бронирует второе место на то же занятие. Повторный вызов
// возвращает уже забронированное место.
func (s *BookingService) AddFriend(ctx context.Context, bookingID, maxUserID string) (*FriendPlace, error) {
	host, err := s.owned(ctx, bookingID, maxUserID)
	if err != nil {
		return nil, err
	}
	if host.IsGuest || host.HostBookingID != "" {
		return nil, ErrInvalidRequest
	}
	if existing, err := s.store.GuestOf(ctx, bookingID); err == nil && existing != nil {
		return s.friendPlace(ctx, existing.ID)
	}
	now := time.Now()
	if host.Slot.StartAt.Sub(now) <= friendReleaseBefore {
		return nil, limit("Место для друга можно взять не позже чем за 2 часа до начала — друг может записаться сам.")
	}
	if n, err := s.store.CountOpenGuests(ctx, host.UserID, now); err != nil {
		return nil, err
	} else if n >= maxOpenFriendPlaces {
		return nil, limit("Сейчас уже два места ждут твоих друзей. Когда они примут приглашения, можно будет позвать ещё.")
	}
	if n, err := s.store.CountBookingsSince(ctx, host.UserID, now.Add(-time.Hour)); err != nil {
		return nil, err
	} else if n >= maxBookingsPerHour {
		return nil, limit("Слишком много записей за последний час. Попробуй чуть позже.")
	}
	guest, err := s.store.AddGuest(ctx, bookingID)
	if err != nil {
		return nil, err
	}
	place, err := s.friendPlace(ctx, guest.ID)
	if err == nil && guest.CreatedAt.After(time.Now().Add(-time.Minute)) {
		s.event(ctx, maxUserID, "friend_place", map[string]interface{}{"booking_id": bookingID})
		s.notifyStudio("friend", &place.Booking)
	}
	return place, err
}

// Friend — место для друга при записи пригласившего (nil — места нет).
func (s *BookingService) Friend(ctx context.Context, bookingID, maxUserID string) (*FriendPlace, error) {
	if _, err := s.owned(ctx, bookingID, maxUserID); err != nil {
		return nil, err
	}
	guest, err := s.store.GuestOf(ctx, bookingID)
	if err != nil || guest == nil {
		return nil, err
	}
	return s.friendPlace(ctx, guest.ID)
}

func (s *BookingService) friendPlace(ctx context.Context, guestID string) (*FriendPlace, error) {
	view, err := s.store.GetBookingView(ctx, guestID)
	if err != nil {
		return nil, err
	}
	place := &FriendPlace{Booking: *view}
	if view.GuestToken != "" {
		place.Link = s.link("friend_" + view.GuestToken)
	} else {
		place.Claimed, place.FriendName = true, firstName(view.UserName)
	}
	return place, nil
}

// SendFriendInvite присылает пригласившему сообщение со ссылкой, которое удобно переслать другу.
func (s *BookingService) SendFriendInvite(ctx context.Context, place *FriendPlace, hostMaxID string) error {
	if s.client == nil || place.Link == "" {
		return nil
	}
	return s.client.FriendInvite(ctx, hostMaxID, &place.Booking, place.Link)
}

// ClaimFriend — друг открыл ссылку: место переходит к нему со своим QR и напоминаниями.
func (s *BookingService) ClaimFriend(ctx context.Context, token, maxUserID, name string) (*model.BookingView, error) {
	token = strings.TrimPrefix(strings.TrimSpace(token), "friend_")
	if len(token) != 32 {
		return nil, ErrNotFound
	}
	user, err := s.store.EnsureUser(ctx, maxUserID, name)
	if err != nil {
		return nil, err
	}
	b, err := s.store.ClaimGuest(ctx, token, user.ID)
	if err != nil {
		return nil, err
	}
	view, err := s.store.GetBookingView(ctx, b.ID)
	if err != nil {
		return nil, err
	}
	s.event(ctx, maxUserID, "friend_joined", map[string]interface{}{"booking_id": b.ID})
	if s.client != nil && b.HostBookingID != "" {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			host, err := s.store.GetBookingView(ctx, b.HostBookingID)
			if err != nil {
				return
			}
			if err := s.client.FriendJoined(ctx, host.MaxUserID, name, view); err != nil {
				log.Printf("friend joined message: %v", err)
			}
		}()
	}
	return view, nil
}

func firstName(name string) string {
	if f := strings.Fields(name); len(f) > 0 {
		return f[0]
	}
	return "Друг"
}
