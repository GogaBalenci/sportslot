package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"sportslot/internal/model"
	"sportslot/internal/repository"
)

type SlotRepo struct {
	pool *pgxpool.Pool
}

func NewSlotRepo(pool *pgxpool.Pool) *SlotRepo {
	return &SlotRepo{pool: pool}
}

var _ repository.SlotRepository = (*SlotRepo)(nil)

func (r *SlotRepo) Create(ctx context.Context, s *model.Slot) error {
	const q = `
		INSERT INTO slots (id, venue_id, start_at, end_at, quota_total, quota_booked)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (id) DO NOTHING`

	_, err := r.pool.Exec(ctx, q, s.ID, s.VenueID, s.StartAt, s.EndAt, s.QuotaTotal, s.QuotaBooked)
	if err != nil {
		return fmt.Errorf("create slot: %w", err)
	}
	return nil
}

func (r *SlotRepo) GetByID(ctx context.Context, id string) (*model.Slot, error) {
	const q = `
		SELECT id, venue_id, start_at, end_at, quota_total, quota_booked, created_at
		FROM slots WHERE id = $1`

	row := r.pool.QueryRow(ctx, q, id)
	var s model.Slot
	err := row.Scan(&s.ID, &s.VenueID, &s.StartAt, &s.EndAt, &s.QuotaTotal, &s.QuotaBooked, &s.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("get slot by id: %w", err)
	}
	return &s, nil
}

// Search ищет будущие слоты (start_at >= now()) со свободной квотой по
// критериям sport_type, level, диапазону дат, времени суток и радиусу
// от заданных координат (формула Haversine вычисляется прямо в SQL).
func (r *SlotRepo) Search(ctx context.Context, params repository.SlotSearchParams) ([]repository.SlotSearchRow, error) {
	var (
		conditions []string
		args       []interface{}
		argN       = 1
	)

	conditions = append(conditions, "s.start_at >= now()")
	conditions = append(conditions, "s.quota_booked < s.quota_total")

	if params.SportType != "" {
		conditions = append(conditions, fmt.Sprintf("v.sport_type = $%d", argN))
		args = append(args, params.SportType)
		argN++
	}
	if params.Level != "" {
		conditions = append(conditions, fmt.Sprintf("v.level = $%d", argN))
		args = append(args, params.Level)
		argN++
	}
	if !params.DateFrom.IsZero() {
		conditions = append(conditions, fmt.Sprintf("s.start_at >= $%d", argN))
		args = append(args, params.DateFrom)
		argN++
	}
	if !params.DateTo.IsZero() {
		conditions = append(conditions, fmt.Sprintf("s.start_at <= $%d", argN))
		args = append(args, params.DateTo)
		argN++
	}
	if params.TimeFrom != "" {
		conditions = append(conditions, fmt.Sprintf("s.start_at::time >= $%d::time", argN))
		args = append(args, params.TimeFrom)
		argN++
	}
	if params.TimeTo != "" {
		conditions = append(conditions, fmt.Sprintf("s.start_at::time <= $%d::time", argN))
		args = append(args, params.TimeTo)
		argN++
	}

	var radiusClause string
	if params.RadiusKM > 0 && (params.Lat != 0 || params.Lon != 0) {
		radiusClause = fmt.Sprintf(`
			AND (
				6371 * acos(
					LEAST(1.0, GREATEST(-1.0,
						cos(radians($%d)) * cos(radians(v.lat)) *
						cos(radians(v.lon) - radians($%d)) +
						sin(radians($%d)) * sin(radians(v.lat))
					))
				)
			) <= $%d`, argN, argN+1, argN, argN+2)
		args = append(args, params.Lat, params.Lon, params.RadiusKM)
		argN += 3
	}

	query := fmt.Sprintf(`
		SELECT
			v.id, v.name, v.sport_type, v.level, v.address, v.lat, v.lon, v.source_ref, v.created_at,
			s.id, s.venue_id, s.start_at, s.end_at, s.quota_total, s.quota_booked, s.created_at
		FROM slots s
		JOIN venues v ON v.id = s.venue_id
		WHERE %s
		%s
		ORDER BY s.start_at ASC
		LIMIT 50`, strings.Join(conditions, " AND "), radiusClause)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("search slots: %w", err)
	}
	defer rows.Close()

	var results []repository.SlotSearchRow
	for rows.Next() {
		var v model.Venue
		var s model.Slot
		if err := rows.Scan(
			&v.ID, &v.Name, &v.SportType, &v.Level, &v.Address, &v.Lat, &v.Lon, &v.SourceRef, &v.CreatedAt,
			&s.ID, &s.VenueID, &s.StartAt, &s.EndAt, &s.QuotaTotal, &s.QuotaBooked, &s.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan search row: %w", err)
		}
		results = append(results, repository.SlotSearchRow{Venue: v, Slot: s})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return results, nil
}

func (r *SlotRepo) ListFutureByVenueID(ctx context.Context, venueID string) ([]model.Slot, error) {
	const q = `
		SELECT id, venue_id, start_at, end_at, quota_total, quota_booked, created_at
		FROM slots
		WHERE venue_id = $1 AND start_at >= now()
		ORDER BY start_at ASC`

	rows, err := r.pool.Query(ctx, q, venueID)
	if err != nil {
		return nil, fmt.Errorf("list future slots: %w", err)
	}
	defer rows.Close()

	var slots []model.Slot
	for rows.Next() {
		var s model.Slot
		if err := rows.Scan(&s.ID, &s.VenueID, &s.StartAt, &s.EndAt, &s.QuotaTotal, &s.QuotaBooked, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan slot: %w", err)
		}
		slots = append(slots, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return slots, nil
}

// IncrementQuota атомарно списывает одно место из квоты слота.
// Возвращает ErrQuotaExceeded, если свободных мест уже нет.
func (r *SlotRepo) IncrementQuota(ctx context.Context, slotID string) error {
	const q = `
		UPDATE slots
		SET quota_booked = quota_booked + 1
		WHERE id = $1 AND quota_booked < quota_total`

	tag, err := r.pool.Exec(ctx, q, slotID)
	if err != nil {
		return fmt.Errorf("increment quota: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrQuotaExceeded
	}
	return nil
}

func (r *SlotRepo) DecrementQuota(ctx context.Context, slotID string) error {
	const q = `
		UPDATE slots
		SET quota_booked = GREATEST(quota_booked - 1, 0)
		WHERE id = $1`

	_, err := r.pool.Exec(ctx, q, slotID)
	if err != nil {
		return fmt.Errorf("decrement quota: %w", err)
	}
	return nil
}
