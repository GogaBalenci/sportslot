package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"sportslot/internal/model"
)

const venueColumns = `id, COALESCE(external_id, ''), name, sport_type, sports, level, address, district,
	lat, lon, source, source_url, booking_mode, phone, website, opening_hours,
	description, what_to_bring, trial_price, verified_at, created_at, logo_version`

func scanVenue(row pgx.Row) (*model.Venue, error) {
	var v model.Venue
	var logo string
	err := row.Scan(&v.ID, &v.ExternalID, &v.Name, &v.SportType, &v.Sports, &v.Level, &v.Address, &v.District,
		&v.Lat, &v.Lon, &v.Source, &v.SourceURL, &v.BookingMode, &v.Phone, &v.Website, &v.OpeningHours,
		&v.Description, &v.WhatToBring, &v.TrialPrice, &v.VerifiedAt, &v.CreatedAt, &logo)
	if err != nil {
		return nil, err
	}
	v.SetLogo(logo)
	return &v, nil
}

// UpsertVenue создаёт или обновляет площадку по id.
func (s *Store) UpsertVenue(ctx context.Context, v *model.Venue) error {
	return s.upsertVenue(ctx, v, "")
}

// UpsertCatalogVenue обновляет площадку из выгрузки OpenStreetMap, но не трогает
// залы, которые уже подключены к онлайн-записи как студии-партнёры.
func (s *Store) UpsertCatalogVenue(ctx context.Context, v *model.Venue) error {
	return s.upsertVenue(ctx, v, " WHERE venues.source = 'osm'")
}

func (s *Store) upsertVenue(ctx context.Context, v *model.Venue, where string) error {
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
			booking_mode = EXCLUDED.booking_mode, opening_hours = EXCLUDED.opening_hours,
			verified_at = EXCLUDED.verified_at,
			-- Профиль, изменённый в кабинете, сохраняется.
			phone = CASE WHEN venues.profile_edited THEN venues.phone ELSE EXCLUDED.phone END,
			website = CASE WHEN venues.profile_edited THEN venues.website ELSE EXCLUDED.website END,
			description = CASE WHEN venues.profile_edited THEN venues.description ELSE EXCLUDED.description END,
			what_to_bring = CASE WHEN venues.profile_edited THEN venues.what_to_bring ELSE EXCLUDED.what_to_bring END,
			trial_price = CASE WHEN venues.profile_edited THEN venues.trial_price ELSE EXCLUDED.trial_price END`+where,
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

// --- Логотипы ---

func (s *Store) SaveLogo(ctx context.Context, venueID, contentType, version string, data []byte) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return wrap("begin", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, `INSERT INTO venue_logos (venue_id, content_type, data) VALUES ($1, $2, $3)
		ON CONFLICT (venue_id) DO UPDATE SET content_type = EXCLUDED.content_type, data = EXCLUDED.data, updated_at = now()`,
		venueID, contentType, data); err != nil {
		return wrap("save logo", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE venues SET logo_version = $2 WHERE id = $1`, venueID, version); err != nil {
		return wrap("logo version", err)
	}
	return wrap("commit", tx.Commit(ctx))
}

func (s *Store) DeleteLogo(ctx context.Context, venueID string) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM venue_logos WHERE venue_id = $1`, venueID); err != nil {
		return wrap("delete logo", err)
	}
	_, err := s.pool.Exec(ctx, `UPDATE venues SET logo_version = '' WHERE id = $1`, venueID)
	return wrap("logo version", err)
}

func (s *Store) Logo(ctx context.Context, venueID string) (string, []byte, error) {
	var ct string
	var data []byte
	err := s.pool.QueryRow(ctx, `SELECT content_type, data FROM venue_logos WHERE venue_id = $1`, venueID).Scan(&ct, &data)
	return ct, data, wrap("get logo", notFound(err))
}
