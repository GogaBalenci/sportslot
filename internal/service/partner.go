package service

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"log"
	"regexp"
	"time"

	"sportslot/internal/model"
	"sportslot/internal/repository/postgres"
)

var (
	ErrPartnerCode    = errors.New("invalid partner code")
	ErrPartnerSession = errors.New("partner session is invalid")
	ErrBadCheckinCode = errors.New("check-in code not recognized")
)

const partnerSessionTTL = 12 * time.Hour

// PartnerService — кабинет администратора студии. В пилоте один общий код
// доступа ко всем демо-партнёрам; в проде у каждой студии свои учётные записи.
type PartnerService struct {
	store     *postgres.Store
	code      string
	presenter Presenter
}

func NewPartnerService(store *postgres.Store, code string) *PartnerService {
	return &PartnerService{store: store, code: code}
}

func (s *PartnerService) SetPresenter(p Presenter) { s.presenter = p }

func (s *PartnerService) Enabled() bool { return s.code != "" }

func (s *PartnerService) Login(ctx context.Context, code string) (string, time.Time, error) {
	if s.code == "" || subtle.ConstantTimeCompare([]byte(code), []byte(s.code)) != 1 {
		return "", time.Time{}, ErrPartnerCode
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", time.Time{}, err
	}
	token := hex.EncodeToString(buf)
	expires, err := s.store.CreatePartnerSession(ctx, token, partnerSessionTTL)
	return token, expires, err
}

func (s *PartnerService) Authorize(ctx context.Context, token string) error {
	if len(token) != 64 {
		return ErrPartnerSession
	}
	ok, err := s.store.PartnerSessionValid(ctx, token)
	if err != nil {
		return err
	}
	if !ok {
		return ErrPartnerSession
	}
	return nil
}

// Bookings — записи на ближайшую неделю, включая сегодняшние.
func (s *PartnerService) Bookings(ctx context.Context, now time.Time) ([]model.BookingView, error) {
	start := now.Add(-12 * time.Hour)
	views, err := s.store.ListVenueBookings(ctx, start, now.Add(8*24*time.Hour), true)
	if views == nil {
		views = []model.BookingView{}
	}
	return views, err
}

var codePattern = regexp.MustCompile(`(\d{6})`)

// ParseCheckinCode принимает содержимое QR («SPORTSLOT:123456») или сам код.
func ParseCheckinCode(raw string) (string, bool) {
	m := codePattern.FindStringSubmatch(raw)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// CheckIn отмечает посещение и сразу спрашивает у пользователя, как прошло занятие.
func (s *PartnerService) CheckIn(ctx context.Context, raw string) (*model.BookingView, error) {
	code, ok := ParseCheckinCode(raw)
	if !ok {
		return nil, ErrBadCheckinCode
	}
	view, err := s.store.CheckIn(ctx, code, true)
	if err != nil {
		return view, err
	}
	if err := s.store.LogEvent(ctx, view.MaxUserID, "attended", map[string]interface{}{"booking_id": view.ID}); err != nil {
		log.Printf("event attended: %v", err)
	}
	if s.presenter != nil {
		go s.askFeedback(view.ID)
	}
	return view, nil
}

func (s *PartnerService) askFeedback(bookingID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	view, err := s.store.GetBookingView(ctx, bookingID)
	if err != nil {
		return
	}
	// Флаг ставится до отправки, чтобы фоновый цикл не задал тот же вопрос второй раз.
	if err := s.store.MarkFeedbackAsked(ctx, bookingID); err != nil {
		log.Printf("mark feedback asked %s: %v", bookingID, err)
		return
	}
	if err := s.presenter.AskFeedback(ctx, view); err != nil {
		log.Printf("feedback question %s: %v", bookingID, err)
	}
}

// SendDemoReminder отправляет напоминание прямо сейчас — чтобы на демо
// не ждать сутки до занятия. Работает только для демо-партнёров.
func (s *PartnerService) SendDemoReminder(ctx context.Context, bookingID string) error {
	view, err := s.store.GetBookingView(ctx, bookingID)
	if err != nil {
		return err
	}
	if !view.Venue.IsDemo() || view.Status != model.BookingStatusConfirmed {
		return ErrNotActive
	}
	if s.presenter == nil {
		return nil
	}
	return s.presenter.Reminder24h(ctx, view)
}
