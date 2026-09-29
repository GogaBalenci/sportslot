// Package seed загружает данные при старте API: каталог площадок Ростова
// из OpenStreetMap и модельных демо-партнёров с расписанием.
package seed

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"

	"sportslot/internal/model"
	"sportslot/internal/repository/postgres"
)

// CatalogFile — формат seed-data/rostov_catalog.json (его пишет cmd/import-osm).
type CatalogFile struct {
	Source     string         `json:"source"`
	License    string         `json:"license"`
	ExportedAt time.Time      `json:"exported_at"`
	Venues     []CatalogVenue `json:"venues"`
}

type CatalogVenue struct {
	ExternalID   string   `json:"external_id"`
	Name         string   `json:"name"`
	Sports       []string `json:"sports"`
	Lat          float64  `json:"lat"`
	Lon          float64  `json:"lon"`
	Address      string   `json:"address"`
	District     string   `json:"district"`
	Phone        string   `json:"phone,omitempty"`
	Website      string   `json:"website,omitempty"`
	OpeningHours string   `json:"opening_hours,omitempty"`
	SourceURL    string   `json:"source_url"`
}

var idNamespace = uuid.MustParse("6f1c2a7e-3b4d-4c55-9e0a-5a1f0b2c3d4e")

// stableID даёт один и тот же uuid для одного и того же ключа между перезапусками.
func stableID(key string) string {
	return uuid.NewSHA1(idNamespace, []byte(key)).String()
}

// LoadCatalog загружает каталог в базу и возвращает дату выгрузки.
func LoadCatalog(ctx context.Context, store *postgres.Store, path string) (*CatalogFile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read catalog: %w", err)
	}
	var file CatalogFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("parse catalog: %w", err)
	}

	ids := make([]string, 0, len(file.Venues))
	for _, cv := range file.Venues {
		if len(cv.Sports) == 0 {
			continue
		}
		exported := file.ExportedAt
		v := &model.Venue{
			ID:           stableID(cv.ExternalID),
			ExternalID:   cv.ExternalID,
			Name:         cv.Name,
			SportType:    cv.Sports[0],
			Sports:       cv.Sports,
			Level:        "any",
			Address:      cv.Address,
			District:     cv.District,
			Lat:          cv.Lat,
			Lon:          cv.Lon,
			Source:       model.VenueSourceOSM,
			SourceURL:    cv.SourceURL,
			BookingMode:  model.BookingModeExternal,
			Phone:        cv.Phone,
			Website:      cv.Website,
			OpeningHours: cv.OpeningHours,
			VerifiedAt:   &exported,
		}
		if err := store.UpsertVenue(ctx, v); err != nil {
			return nil, fmt.Errorf("venue %s: %w", cv.ExternalID, err)
		}
		ids = append(ids, v.ID)
	}
	if _, err := store.DeleteCatalogVenuesExcept(ctx, ids); err != nil {
		return nil, err
	}
	return &file, nil
}
