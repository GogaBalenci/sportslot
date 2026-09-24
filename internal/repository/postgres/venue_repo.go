package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"sportslot/internal/model"
	"sportslot/internal/repository"
)

type VenueRepo struct {
	pool *pgxpool.Pool
}

func NewVenueRepo(pool *pgxpool.Pool) *VenueRepo {
	return &VenueRepo{pool: pool}
}

var _ repository.VenueRepository = (*VenueRepo)(nil)

func (r *VenueRepo) GetByID(ctx context.Context, id string) (*model.Venue, error) {
	const q = `
		SELECT id, name, sport_type, level, address, lat, lon, source_ref, created_at
		FROM venues WHERE id = $1`

	row := r.pool.QueryRow(ctx, q, id)
	var v model.Venue
	err := row.Scan(&v.ID, &v.Name, &v.SportType, &v.Level, &v.Address, &v.Lat, &v.Lon, &v.SourceRef, &v.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("get venue by id: %w", err)
	}
	return &v, nil
}

func (r *VenueRepo) Create(ctx context.Context, v *model.Venue) error {
	const q = `
		INSERT INTO venues (id, name, sport_type, level, address, lat, lon, source_ref)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (id) DO NOTHING`

	_, err := r.pool.Exec(ctx, q, v.ID, v.Name, v.SportType, v.Level, v.Address, v.Lat, v.Lon, v.SourceRef)
	if err != nil {
		return fmt.Errorf("create venue: %w", err)
	}
	return nil
}

func (r *VenueRepo) List(ctx context.Context) ([]model.Venue, error) {
	const q = `
		SELECT id, name, sport_type, level, address, lat, lon, source_ref, created_at
		FROM venues ORDER BY created_at`

	rows, err := r.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list venues: %w", err)
	}
	defer rows.Close()

	var venues []model.Venue
	for rows.Next() {
		var v model.Venue
		if err := rows.Scan(&v.ID, &v.Name, &v.SportType, &v.Level, &v.Address, &v.Lat, &v.Lon, &v.SourceRef, &v.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan venue: %w", err)
		}
		venues = append(venues, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return venues, nil
}
