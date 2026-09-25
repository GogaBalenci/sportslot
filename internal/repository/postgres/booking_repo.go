package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"sportslot/internal/model"
	"sportslot/internal/repository"
)

type BookingRepo struct {
	pool *pgxpool.Pool
}

func NewBookingRepo(pool *pgxpool.Pool) *BookingRepo {
	return &BookingRepo{pool: pool}
}

var _ repository.BookingRepository = (*BookingRepo)(nil)

// CreateConfirmed атомарно списывает квоту слота и создаёт подтверждённую
// бронь в рамках одной транзакции. Если квота уже исчерпана - возвращает
// ErrQuotaExceeded и не создаёт запись брони.
func (r *BookingRepo) CreateConfirmed(ctx context.Context, b *model.Booking) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	const updateQ = `
		UPDATE slots
		SET quota_booked = quota_booked + 1
		WHERE id = $1 AND quota_booked < quota_total`

	tag, err := tx.Exec(ctx, updateQ, b.SlotID)
	if err != nil {
		return fmt.Errorf("increment quota: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrQuotaExceeded
	}

	const insertQ = `
		INSERT INTO bookings (id, user_id, slot_id, status, source_channel, reminder_sent, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`

	_, err = tx.Exec(ctx, insertQ, b.ID, b.UserID, b.SlotID, b.Status, b.SourceChannel, b.ReminderSent, b.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert booking: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

func (r *BookingRepo) GetByID(ctx context.Context, id string) (*model.Booking, error) {
	const q = `
		SELECT id, user_id, slot_id, status, source_channel, reminder_sent, created_at, cancelled_at
		FROM bookings WHERE id = $1`

	row := r.pool.QueryRow(ctx, q, id)
	var b model.Booking
	err := row.Scan(&b.ID, &b.UserID, &b.SlotID, &b.Status, &b.SourceChannel, &b.ReminderSent, &b.CreatedAt, &b.CancelledAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("get booking by id: %w", err)
	}
	return &b, nil
}

func (r *BookingRepo) ListByUserID(ctx context.Context, userID string) ([]model.Booking, error) {
	const q = `
		SELECT id, user_id, slot_id, status, source_channel, reminder_sent, created_at, cancelled_at
		FROM bookings WHERE user_id = $1 ORDER BY created_at DESC`

	rows, err := r.pool.Query(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("list bookings: %w", err)
	}
	defer rows.Close()

	var bookings []model.Booking
	for rows.Next() {
		var b model.Booking
		if err := rows.Scan(&b.ID, &b.UserID, &b.SlotID, &b.Status, &b.SourceChannel, &b.ReminderSent, &b.CreatedAt, &b.CancelledAt); err != nil {
			return nil, fmt.Errorf("scan booking: %w", err)
		}
		bookings = append(bookings, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return bookings, nil
}

// CancelAndReleaseQuota атомарно помечает бронь отменённой и возвращает
// место в квоту слота. Идемпотентна: повторный вызов на уже отменённой
// брони не возвращает ошибку.
func (r *BookingRepo) CancelAndReleaseQuota(ctx context.Context, bookingID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var slotID string
	var status model.BookingStatus

	const selectQ = `SELECT slot_id, status FROM bookings WHERE id = $1 FOR UPDATE`
	err = tx.QueryRow(ctx, selectQ, bookingID).Scan(&slotID, &status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return repository.ErrNotFound
		}
		return fmt.Errorf("select booking for cancel: %w", err)
	}

	if status == model.BookingStatusCancelled {
		return nil
	}

	const updateBookingQ = `
		UPDATE bookings SET status = 'cancelled', cancelled_at = now() WHERE id = $1`
	if _, err := tx.Exec(ctx, updateBookingQ, bookingID); err != nil {
		return fmt.Errorf("cancel booking: %w", err)
	}

	const releaseQuotaQ = `
		UPDATE slots SET quota_booked = GREATEST(quota_booked - 1, 0) WHERE id = $1`
	if _, err := tx.Exec(ctx, releaseQuotaQ, slotID); err != nil {
		return fmt.Errorf("release quota: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// RescheduleWithQuota атомарно переносит бронь на другой слот: возвращает
// квоту старого слота и списывает квоту нового. При исчерпании квоты
// нового слота откатывает всю операцию и возвращает ErrQuotaExceeded.
func (r *BookingRepo) RescheduleWithQuota(ctx context.Context, bookingID, newSlotID string) (*model.Booking, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var b model.Booking
	const selectQ = `
		SELECT id, user_id, slot_id, status, source_channel, reminder_sent, created_at, cancelled_at
		FROM bookings WHERE id = $1 FOR UPDATE`
	err = tx.QueryRow(ctx, selectQ, bookingID).Scan(
		&b.ID, &b.UserID, &b.SlotID, &b.Status, &b.SourceChannel, &b.ReminderSent, &b.CreatedAt, &b.CancelledAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("select booking for reschedule: %w", err)
	}

	if b.Status == model.BookingStatusCancelled {
		return nil, fmt.Errorf("cannot reschedule cancelled booking")
	}

	oldSlotID := b.SlotID
	if oldSlotID == newSlotID {
		return &b, nil
	}

	// Блокируем строки обоих слотов в детерминированном порядке (ORDER BY id),
	// чтобы исключить взаимные блокировки (deadlock) при параллельных перекрестных переносах.
	firstID, secondID := oldSlotID, newSlotID
	if firstID > secondID {
		firstID, secondID = secondID, firstID
	}
	lockRows, err := tx.Query(ctx, `SELECT id FROM slots WHERE id IN ($1, $2) ORDER BY id FOR UPDATE`, firstID, secondID)
	if err != nil {
		return nil, fmt.Errorf("lock slots for reschedule: %w", err)
	}
	lockRows.Close()

	const decQ = `
		UPDATE slots SET quota_booked = GREATEST(quota_booked - 1, 0) WHERE id = $1`
	if _, err := tx.Exec(ctx, decQ, oldSlotID); err != nil {
		return nil, fmt.Errorf("release old slot quota: %w", err)
	}

	const incQ = `
		UPDATE slots SET quota_booked = quota_booked + 1
		WHERE id = $1 AND quota_booked < quota_total`
	tag, err := tx.Exec(ctx, incQ, newSlotID)
	if err != nil {
		return nil, fmt.Errorf("increment new slot quota: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, repository.ErrQuotaExceeded
	}

	const updateBookingQ = `
		UPDATE bookings SET slot_id = $1, reminder_sent = false WHERE id = $2`
	if _, err := tx.Exec(ctx, updateBookingQ, newSlotID, bookingID); err != nil {
		return nil, fmt.Errorf("update booking slot: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}

	b.SlotID = newSlotID
	b.ReminderSent = false
	return &b, nil
}

func (r *BookingRepo) FindUpcomingForReminder(ctx context.Context, windowStart, windowEnd time.Time) ([]model.UpcomingReminder, error) {
	const q = `
		SELECT b.id, u.max_user_id, v.name, s.start_at
		FROM bookings b
		JOIN slots s ON s.id = b.slot_id
		JOIN venues v ON v.id = s.venue_id
		JOIN users u ON u.id = b.user_id
		WHERE b.status = 'confirmed'
		  AND b.reminder_sent = false
		  AND s.start_at BETWEEN $1 AND $2`

	rows, err := r.pool.Query(ctx, q, windowStart, windowEnd)
	if err != nil {
		return nil, fmt.Errorf("find upcoming reminders: %w", err)
	}
	defer rows.Close()

	var reminders []model.UpcomingReminder
	for rows.Next() {
		var rem model.UpcomingReminder
		if err := rows.Scan(&rem.BookingID, &rem.MaxUserID, &rem.VenueName, &rem.SlotStartAt); err != nil {
			return nil, fmt.Errorf("scan reminder: %w", err)
		}
		reminders = append(reminders, rem)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return reminders, nil
}

func (r *BookingRepo) MarkReminderSent(ctx context.Context, id string) error {
	const q = `UPDATE bookings SET reminder_sent = true WHERE id = $1`
	_, err := r.pool.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("mark reminder sent: %w", err)
	}
	return nil
}
