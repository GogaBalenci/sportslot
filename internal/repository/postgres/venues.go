package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"sportslot/internal/model"
)

const venueColumns = `id, COALESCE(external_id, ''), name, sport_type, sports, level, address, district,
	lat, lon, source, source_url, booking_mode, phone, website, opening_hours,
	description, what_to_bring, trial_price, verified_at, created_at`

func scanVenue(row pgx.Row) (*model.Venue, error) {
	var v model.Venue
	err := row.Scan(&v.ID, &v.ExternalID, &v.Name, &v.SportType, &v.Sports, &v.Level, &v.Address, &v.District,
		&v.Lat, &v.Lon, &v.Source, &v.SourceURL, &v.BookingMode, &v.Phone, &v.Website, &v.OpeningHours,
		&v.Description, &v.WhatToBring, &v.TrialPrice, &v.VerifiedAt, &v.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// UpsertVenue создаёт или обновляет площадку по id.
func (s *Store) UpsertVenue(ctx context.Context, v *model.Venue) error {
	var externalID interface{}
	if v.ExternalID != "" {
		externalID = v.ExternalID
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO venues (id, external_id, name, sport_type, sports, level, address, district, lat, lon,
			source, source_ref, source_url, booking_mode, phone, website, opening_hours, description,
			what_to_bring, trial_price, verified_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)
		ON CONFLICT (id) DO UPDATE SET
			external_id = EXCLUDED.external_id, name = EXCLUDED.name, sport_type = EXCLUDED.sport_type,
			sports = EXCLUDED.sports, level = EXCLUDED.level, address = EXCLUDED.address,
			district = EXCLUDED.district, lat = EXCLUDED.lat, lon = EXCLUDED.lon, source = EXCLUDED.source,
			source_ref = EXCLUDED.source_ref, source_url = EXCLUDED.source_url,
			booking_mode = EXCLUDED.booking_mode, phone = EXCLUDED.phone, website = EXCLUDED.website,
			opening_hours = EXCLUDED.opening_hours, description = EXCLUDED.description,
			what_to_bring = EXCLUDED.what_to_bring, trial_price = EXCLUDED.trial_price,
			verified_at = EXCLUDED.verified_at`,
		v.ID, externalID, v.Name, v.SportType, v.Sports, v.Level, v.Address, v.District, v.Lat, v.Lon,
		string(v.Source), v.SourceURL, string(v.BookingMode), v.Phone, v.Website, v.OpeningHours,
		v.Description, v.WhatToBring, v.TrialPrice, v.VerifiedAt)
	return wrap("upsert venue", err)
}

type VenueQuery struct {
	Sport       string
	IncludeDemo bool
}

func (s *Store) ListVenues(ctx context.Context, q VenueQuery) ([]model.Venue, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+venueColumns+` FROM venues
		WHERE ($1 = '' OR $1 = ANY(sports))
		  AND ($2 OR source <> 'demo_partner')
		ORDER BY name`, q.Sport, q.IncludeDemo)
	if err != nil {
		return nil, wrap("list venues", err)
	}
	defer rows.Close()
	var venues []model.Venue
	for rows.Next() {
		v, err := scanVenue(rows)
		if err != nil {
			return nil, wrap("scan venue", err)
		}
		venues = append(venues, *v)
	}
	return venues, wrap("list venues", rows.Err())
}

func (s *Store) GetVenue(ctx context.Context, id string) (*model.Venue, error) {
	v, err := scanVenue(s.pool.QueryRow(ctx, `SELECT `+venueColumns+` FROM venues WHERE id = $1`, id))
	return v, wrap("get venue", notFound(err))
}

// DeleteCatalogVenuesExcept убирает площадки OSM, которых нет в новой выгрузке.
func (s *Store) DeleteCatalogVenuesExcept(ctx context.Context, keepIDs []string) (int64, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM venues v WHERE v.source = 'osm' AND NOT (v.id = ANY($1::uuid[]))
		AND NOT EXISTS (SELECT 1 FROM slots s WHERE s.venue_id = v.id)`, keepIDs)
	return tag.RowsAffected(), wrap("delete stale venues", err)
}
