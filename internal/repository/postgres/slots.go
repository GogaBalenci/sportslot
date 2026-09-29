package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"sportslot/internal/model"
)

const slotColumns = `s.id, s.venue_id, s.title, s.sport_type, s.level, s.start_at, s.end_at, s.quota_total, s.quota_booked`

func scanSlot(row pgx.Row) (*model.Slot, error) {
	var sl model.Slot
	if err := row.Scan(&sl.ID, &sl.VenueID, &sl.Title, &sl.SportType, &sl.Level, &sl.StartAt, &sl.EndAt,
		&sl.QuotaTotal, &sl.QuotaBooked); err != nil {
		return nil, err
	}
	return &sl, nil
}

// InsertSlotIfAbsent не трогает уже существующее занятие: у него могут быть записи.
func (s *Store) InsertSlotIfAbsent(ctx context.Context, sl *model.Slot) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO slots (id, venue_id, title, sport_type, level, start_at, end_at, quota_total, quota_booked)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (id) DO NOTHING`,
		sl.ID, sl.VenueID, sl.Title, sl.SportType, sl.Level, sl.StartAt, sl.EndAt, sl.QuotaTotal, sl.QuotaBooked)
	return wrap("insert slot", err)
}

// KeepSlotInFuture переносит служебное занятие с фиксированным id (оно нужно
// для воспроизводимых проверок API) на новое время, если старое уже близко.
func (s *Store) KeepSlotInFuture(ctx context.Context, sl *model.Slot) error {
	if err := s.InsertSlotIfAbsent(ctx, sl); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `UPDATE slots SET start_at = $2, end_at = $3
		WHERE id = $1 AND start_at < now() + interval '6 hours'`, sl.ID, sl.StartAt, sl.EndAt)
	return wrap("shift slot", err)
}

func (s *Store) GetSlot(ctx context.Context, id string) (*model.Slot, error) {
	sl, err := scanSlot(s.pool.QueryRow(ctx, `SELECT `+slotColumns+` FROM slots s WHERE s.id = $1`, id))
	return sl, wrap("get slot", notFound(err))
}

type SlotQuery struct {
	VenueIDs      []string
	Sport         string
	From, To      time.Time
	OnlyAvailable bool
}

func (s *Store) ListSlots(ctx context.Context, q SlotQuery) ([]model.Slot, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+slotColumns+` FROM slots s
		WHERE s.venue_id = ANY($1::uuid[])
		  AND ($2 = '' OR s.sport_type = $2)
		  AND s.start_at >= $3 AND s.start_at < $4
		  AND (NOT $5 OR s.quota_booked < s.quota_total)
		ORDER BY s.start_at, s.title`, q.VenueIDs, q.Sport, q.From, q.To, q.OnlyAvailable)
	if err != nil {
		return nil, wrap("list slots", err)
	}
	defer rows.Close()
	var slots []model.Slot
	for rows.Next() {
		sl, err := scanSlot(rows)
		if err != nil {
			return nil, wrap("scan slot", err)
		}
		slots = append(slots, *sl)
	}
	return slots, wrap("list slots", rows.Err())
}

// FindSlotNextWeek ищет то же занятие через неделю — для кнопки «Записаться снова».
func (s *Store) FindSlotNextWeek(ctx context.Context, sl *model.Slot) (*model.Slot, error) {
	next, err := scanSlot(s.pool.QueryRow(ctx, `SELECT `+slotColumns+` FROM slots s
		WHERE s.venue_id = $1 AND s.title = $2
		  AND s.start_at BETWEEN $3 AND $4 AND s.quota_booked < s.quota_total
		ORDER BY abs(extract(epoch FROM s.start_at - $5)) LIMIT 1`,
		sl.VenueID, sl.Title, sl.StartAt.Add(6*24*time.Hour), sl.StartAt.Add(8*24*time.Hour),
		sl.StartAt.Add(7*24*time.Hour)))
	return next, wrap("find next week slot", notFound(err))
}
