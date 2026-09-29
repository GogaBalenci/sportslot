package postgres

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"time"

	"github.com/jackc/pgx/v5"

	"sportslot/internal/model"
	"sportslot/internal/repository"
)

const bookingColumns = `b.id, b.user_id, b.slot_id, b.status, b.source_channel, COALESCE(b.checkin_code, ''),
	b.reminder_24h_sent OR b.reminder_2h_sent, b.rating, b.created_at, b.cancelled_at, b.attended_at`

func scanBooking(row pgx.Row) (*model.Booking, error) {
	var b model.Booking
	var rating *int16
	if err := row.Scan(&b.ID, &b.UserID, &b.SlotID, &b.Status, &b.SourceChannel, &b.CheckinCode,
		&b.ReminderSent, &rating, &b.CreatedAt, &b.CancelledAt, &b.AttendedAt); err != nil {
		return nil, err
	}
	if rating != nil {
		r := int(*rating)
		b.Rating = &r
	}
	return &b, nil
}

const viewSelect = `SELECT ` + bookingColumns + `, u.max_user_id, COALESCE(u.name, ''), ` + slotColumns + `, ` + venueColumnsV + `
	FROM bookings b
	JOIN users u ON u.id = b.user_id
	JOIN slots s ON s.id = b.slot_id
	JOIN venues v ON v.id = s.venue_id`

const venueColumnsV = `v.id, COALESCE(v.external_id, ''), v.name, v.sport_type, v.sports, v.level, v.address, v.district,
	v.lat, v.lon, v.source, v.source_url, v.booking_mode, v.phone, v.website, v.opening_hours,
	v.description, v.what_to_bring, v.trial_price, v.verified_at, v.created_at`

func scanView(row pgx.Row) (*model.BookingView, error) {
	var bv model.BookingView
	var rating *int16
	v := &bv.Venue
	sl := &bv.Slot
	err := row.Scan(&bv.ID, &bv.UserID, &bv.SlotID, &bv.Status, &bv.SourceChannel, &bv.CheckinCode,
		&bv.ReminderSent, &rating, &bv.CreatedAt, &bv.CancelledAt, &bv.AttendedAt,
		&bv.MaxUserID, &bv.UserName,
		&sl.ID, &sl.VenueID, &sl.Title, &sl.SportType, &sl.Level, &sl.StartAt, &sl.EndAt, &sl.QuotaTotal, &sl.QuotaBooked,
		&v.ID, &v.ExternalID, &v.Name, &v.SportType, &v.Sports, &v.Level, &v.Address, &v.District,
		&v.Lat, &v.Lon, &v.Source, &v.SourceURL, &v.BookingMode, &v.Phone, &v.Website, &v.OpeningHours,
		&v.Description, &v.WhatToBring, &v.TrialPrice, &v.VerifiedAt, &v.CreatedAt)
	if err != nil {
		return nil, err
	}
	if rating != nil {
		r := int(*rating)
		bv.Rating = &r
	}
	return &bv, nil
}

func (s *Store) queryViews(ctx context.Context, where string, args ...interface{}) ([]model.BookingView, error) {
	rows, err := s.pool.Query(ctx, viewSelect+" "+where, args...)
	if err != nil {
		return nil, wrap("query bookings", err)
	}
	defer rows.Close()
	var views []model.BookingView
	for rows.Next() {
		bv, err := scanView(rows)
		if err != nil {
			return nil, wrap("scan booking", err)
		}
		views = append(views, *bv)
	}
	return views, wrap("query bookings", rows.Err())
}

func (s *Store) GetBooking(ctx context.Context, id string) (*model.Booking, error) {
	b, err := scanBooking(s.pool.QueryRow(ctx, `SELECT `+bookingColumns+` FROM bookings b WHERE b.id = $1`, id))
	return b, wrap("get booking", notFound(err))
}

func (s *Store) GetBookingView(ctx context.Context, id string) (*model.BookingView, error) {
	bv, err := scanView(s.pool.QueryRow(ctx, viewSelect+` WHERE b.id = $1`, id))
	return bv, wrap("get booking view", notFound(err))
}

func (s *Store) ListUserBookings(ctx context.Context, userID string) ([]model.Booking, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+bookingColumns+` FROM bookings b WHERE b.user_id = $1 ORDER BY b.created_at DESC`, userID)
	if err != nil {
		return nil, wrap("list bookings", err)
	}
	defer rows.Close()
	bookings := []model.Booking{}
	for rows.Next() {
		b, err := scanBooking(rows)
		if err != nil {
			return nil, wrap("scan booking", err)
		}
		bookings = append(bookings, *b)
	}
	return bookings, wrap("list bookings", rows.Err())
}

func (s *Store) ListUserBookingViews(ctx context.Context, userID string) ([]model.BookingView, error) {
	return s.queryViews(ctx, `WHERE b.user_id = $1 AND b.status <> 'cancelled' ORDER BY s.start_at`, userID)
}

// ListVenueBookings — записи в студиях за период, для администратора.
func (s *Store) ListVenueBookings(ctx context.Context, from, to time.Time, demoOnly bool) ([]model.BookingView, error) {
	return s.queryViews(ctx, `WHERE s.start_at >= $1 AND s.start_at < $2 AND b.status <> 'cancelled'
		AND (NOT $3 OR v.source = 'demo_partner') ORDER BY s.start_at, u.name`, from, to, demoOnly)
}

// CreateBooking записывает пользователя на занятие. Повторная запись на то же
// занятие возвращает существующую бронь (created=false).
func (s *Store) CreateBooking(ctx context.Context, userID, slotID string, channel model.SourceChannel) (*model.Booking, bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, false, wrap("begin", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	existing, err := scanBooking(tx.QueryRow(ctx, `SELECT `+bookingColumns+` FROM bookings b
		WHERE b.user_id = $1 AND b.slot_id = $2 AND b.status = 'confirmed'`, userID, slotID))
	if err == nil {
		return existing, false, nil
	}
	if err != pgx.ErrNoRows {
		return nil, false, wrap("find existing booking", err)
	}

	b, err := bookInTx(ctx, tx, userID, slotID, channel)
	if err != nil {
		return nil, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, false, wrap("commit", err)
	}
	return b, true, nil
}

func bookInTx(ctx context.Context, tx pgx.Tx, userID, slotID string, channel model.SourceChannel) (*model.Booking, error) {
	var startAt time.Time
	var total, booked int
	err := tx.QueryRow(ctx, `SELECT start_at, quota_total, quota_booked FROM slots WHERE id = $1 FOR UPDATE`, slotID).
		Scan(&startAt, &total, &booked)
	if err != nil {
		return nil, wrap("lock slot", notFound(err))
	}
	if !startAt.After(time.Now()) {
		return nil, repository.ErrSlotStarted
	}
	if booked >= total {
		return nil, repository.ErrQuotaExceeded
	}
	if _, err := tx.Exec(ctx, `UPDATE slots SET quota_booked = quota_booked + 1 WHERE id = $1`, slotID); err != nil {
		return nil, wrap("increment quota", err)
	}

	for attempt := 0; attempt < 5; attempt++ {
		code, err := checkinCode()
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `SAVEPOINT booking_insert`); err != nil {
			return nil, wrap("savepoint", err)
		}
		b, err := scanBooking(tx.QueryRow(ctx, `
			INSERT INTO bookings (user_id, slot_id, status, source_channel, checkin_code)
			VALUES ($1, $2, 'confirmed', $3, $4)
			RETURNING id, user_id, slot_id, status, source_channel, checkin_code, false, rating,
				created_at, cancelled_at, attended_at`, userID, slotID, string(channel), code))
		if err == nil {
			return b, nil
		}
		if _, rbErr := tx.Exec(ctx, `ROLLBACK TO SAVEPOINT booking_insert`); rbErr != nil {
			return nil, wrap("rollback savepoint", rbErr)
		}
		switch {
		case isUniqueViolation(err, "uq_bookings_active_checkin"):
			continue
		case isUniqueViolation(err, "uq_bookings_active_user_slot"):
			return nil, repository.ErrAlreadyBooked
		default:
			return nil, wrap("insert booking", err)
		}
	}
	return nil, fmt.Errorf("could not generate unique check-in code")
}

func checkinCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// Promotion — место, которое после отмены ушло первому из листа ожидания.
type Promotion struct {
	BookingID string
	MaxUserID string
}

// CancelBooking отменяет активную бронь и освобождает место. Если на занятие
// есть лист ожидания, место в той же транзакции отдаётся первому в очереди.
func (s *Store) CancelBooking(ctx context.Context, bookingID string) (*Promotion, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, wrap("begin", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var slotID string
	var status model.BookingStatus
	err = tx.QueryRow(ctx, `SELECT slot_id, status FROM bookings WHERE id = $1 FOR UPDATE`, bookingID).Scan(&slotID, &status)
	if err != nil {
		return nil, wrap("lock booking", notFound(err))
	}
	if status != model.BookingStatusConfirmed {
		return nil, repository.ErrNotActive
	}
	if _, err := tx.Exec(ctx, `UPDATE bookings SET status = 'cancelled', cancelled_at = now() WHERE id = $1`, bookingID); err != nil {
		return nil, wrap("cancel booking", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE slots SET quota_booked = GREATEST(quota_booked - 1, 0) WHERE id = $1`, slotID); err != nil {
		return nil, wrap("release quota", err)
	}

	promotion, err := promoteFromWaitlist(ctx, tx, slotID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, wrap("commit", err)
	}
	return promotion, nil
}

func promoteFromWaitlist(ctx context.Context, tx pgx.Tx, slotID string) (*Promotion, error) {
	var waitID, userID, maxUserID string
	err := tx.QueryRow(ctx, `SELECT w.id, w.user_id, u.max_user_id FROM waitlist w
		JOIN users u ON u.id = w.user_id
		JOIN slots s ON s.id = w.slot_id
		WHERE w.slot_id = $1 AND w.status = 'waiting' AND s.start_at > now()
		ORDER BY w.created_at LIMIT 1 FOR UPDATE OF w SKIP LOCKED`, slotID).Scan(&waitID, &userID, &maxUserID)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, wrap("pick waitlist", err)
	}
	b, err := bookInTx(ctx, tx, userID, slotID, model.SourceChannelBot)
	if err != nil {
		// Место не удалось отдать (например, у человека уже есть запись) — просто закрываем заявку.
		if _, updErr := tx.Exec(ctx, `UPDATE waitlist SET status = 'cancelled' WHERE id = $1`, waitID); updErr != nil {
			return nil, wrap("close waitlist", updErr)
		}
		return nil, nil
	}
	if _, err := tx.Exec(ctx, `UPDATE waitlist SET status = 'booked', booking_id = $2 WHERE id = $1`, waitID, b.ID); err != nil {
		return nil, wrap("update waitlist", err)
	}
	return &Promotion{BookingID: b.ID, MaxUserID: maxUserID}, nil
}

// RescheduleBooking переносит активную бронь на другое занятие с сохранением кода входа.
func (s *Store) RescheduleBooking(ctx context.Context, bookingID, newSlotID string) (*model.Booking, *Promotion, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, nil, wrap("begin", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var oldSlotID, userID string
	var status model.BookingStatus
	err = tx.QueryRow(ctx, `SELECT slot_id, user_id, status FROM bookings WHERE id = $1 FOR UPDATE`, bookingID).
		Scan(&oldSlotID, &userID, &status)
	if err != nil {
		return nil, nil, wrap("lock booking", notFound(err))
	}
	if status != model.BookingStatusConfirmed {
		return nil, nil, repository.ErrNotActive
	}
	if oldSlotID == newSlotID {
		b, err := scanBooking(tx.QueryRow(ctx, `SELECT `+bookingColumns+` FROM bookings b WHERE b.id = $1`, bookingID))
		return b, nil, wrap("get booking", err)
	}

	var startAt time.Time
	var total, booked int
	err = tx.QueryRow(ctx, `SELECT start_at, quota_total, quota_booked FROM slots WHERE id = $1 FOR UPDATE`, newSlotID).
		Scan(&startAt, &total, &booked)
	if err != nil {
		return nil, nil, wrap("lock new slot", notFound(err))
	}
	if !startAt.After(time.Now()) {
		return nil, nil, repository.ErrSlotStarted
	}
	if booked >= total {
		return nil, nil, repository.ErrQuotaExceeded
	}
	if _, err := tx.Exec(ctx, `UPDATE slots SET quota_booked = quota_booked + 1 WHERE id = $1`, newSlotID); err != nil {
		return nil, nil, wrap("increment quota", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE slots SET quota_booked = GREATEST(quota_booked - 1, 0) WHERE id = $1`, oldSlotID); err != nil {
		return nil, nil, wrap("release quota", err)
	}
	b, err := scanBooking(tx.QueryRow(ctx, `UPDATE bookings b SET slot_id = $2, reminder_24h_sent = false, reminder_2h_sent = false
		WHERE b.id = $1 RETURNING `+bookingColumns, bookingID, newSlotID))
	if err != nil {
		if isUniqueViolation(err, "uq_bookings_active_user_slot") {
			return nil, nil, repository.ErrAlreadyBooked
		}
		return nil, nil, wrap("move booking", err)
	}
	promotion, err := promoteFromWaitlist(ctx, tx, oldSlotID)
	if err != nil {
		return nil, nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, nil, wrap("commit", err)
	}
	return b, promotion, nil
}

// CheckIn отмечает посещение по коду входа. Возвращает ErrNotActive, если
// посещение по этому коду уже отмечено, и ErrNotFound, если кода нет.
func (s *Store) CheckIn(ctx context.Context, code string, demoOnly bool) (*model.BookingView, error) {
	views, err := s.queryViews(ctx, `WHERE b.checkin_code = $1 AND b.status IN ('confirmed', 'attended')
		AND (NOT $2 OR v.source = 'demo_partner')
		ORDER BY (b.status = 'confirmed') DESC, b.created_at DESC LIMIT 1`, code, demoOnly)
	if err != nil {
		return nil, err
	}
	if len(views) == 0 {
		return nil, repository.ErrNotFound
	}
	bv := views[0]
	if bv.Status != model.BookingStatusConfirmed {
		return &bv, repository.ErrNotActive
	}
	now := time.Now()
	if _, err := s.pool.Exec(ctx, `UPDATE bookings SET status = 'attended', attended_at = $2 WHERE id = $1 AND status = 'confirmed'`,
		bv.ID, now); err != nil {
		return nil, wrap("check in", err)
	}
	bv.Status = model.BookingStatusAttended
	bv.AttendedAt = &now
	return &bv, nil
}

func (s *Store) SetRating(ctx context.Context, bookingID string, rating int) error {
	tag, err := s.pool.Exec(ctx, `UPDATE bookings SET rating = $2 WHERE id = $1 AND status = 'attended'`, bookingID, rating)
	if err != nil {
		return wrap("set rating", err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotActive
	}
	return nil
}

// DueReminders24h — брони, которым пора прислать напоминание «за сутки».
// Если запись сделана позже, чем за сутки до начала, это напоминание не нужно.
func (s *Store) DueReminders24h(ctx context.Context, now time.Time) ([]model.BookingView, error) {
	return s.queryViews(ctx, `WHERE b.status = 'confirmed' AND NOT b.reminder_24h_sent
		AND s.start_at <= $1::timestamptz + interval '24 hours' AND s.start_at > $1::timestamptz + interval '3 hours'
		AND b.created_at < s.start_at - interval '24 hours'`, now)
}

func (s *Store) DueReminders2h(ctx context.Context, now time.Time) ([]model.BookingView, error) {
	return s.queryViews(ctx, `WHERE b.status = 'confirmed' AND NOT b.reminder_2h_sent
		AND s.start_at <= $1::timestamptz + interval '2 hours' AND s.start_at > $1::timestamptz
		AND b.created_at < s.start_at - interval '2 hours'`, now)
}

func (s *Store) MarkReminderSent(ctx context.Context, bookingID string, kind string) error {
	column := "reminder_2h_sent"
	if kind == "24h" {
		column = "reminder_24h_sent"
	}
	_, err := s.pool.Exec(ctx, `UPDATE bookings SET `+column+` = true WHERE id = $1`, bookingID)
	return wrap("mark reminder", err)
}

// PendingFeedback — отмеченные посещения, по которым ещё не спросили «Как прошло?».
func (s *Store) PendingFeedback(ctx context.Context) ([]model.BookingView, error) {
	return s.queryViews(ctx, `WHERE b.status = 'attended' AND NOT b.feedback_asked ORDER BY b.attended_at`)
}

func (s *Store) MarkFeedbackAsked(ctx context.Context, bookingID string) error {
	_, err := s.pool.Exec(ctx, `UPDATE bookings SET feedback_asked = true WHERE id = $1`, bookingID)
	return wrap("mark feedback", err)
}

// MarkNoShows закрывает брони занятий, которые закончились больше двух часов назад без отметки.
func (s *Store) MarkNoShows(ctx context.Context, now time.Time) (int64, error) {
	tag, err := s.pool.Exec(ctx, `UPDATE bookings b SET status = 'no_show' FROM slots s
		WHERE s.id = b.slot_id AND b.status = 'confirmed' AND s.end_at < $1::timestamptz - interval '2 hours'`, now)
	return tag.RowsAffected(), wrap("mark no-shows", err)
}

// JoinWaitlist ставит в очередь на занятие и возвращает позицию (с единицы).
func (s *Store) JoinWaitlist(ctx context.Context, userID, slotID string) (int, error) {
	if _, err := s.pool.Exec(ctx, `INSERT INTO waitlist (user_id, slot_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, userID, slotID); err != nil {
		return 0, wrap("join waitlist", err)
	}
	var position int
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM waitlist w
		WHERE w.slot_id = $1 AND w.status = 'waiting'
		  AND w.created_at <= (SELECT created_at FROM waitlist WHERE user_id = $2 AND slot_id = $1 AND status = 'waiting')`,
		slotID, userID).Scan(&position)
	return position, wrap("waitlist position", err)
}
