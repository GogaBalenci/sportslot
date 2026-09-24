package seed

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"sportslot/internal/model"
	"sportslot/internal/repository"
)

type seedFile struct {
	Meta struct {
		Source      string `json:"source"`
		BasedOn     string `json:"based_on"`
		Note        string `json:"note"`
		GeneratedAt string `json:"generated_at"`
	} `json:"meta"`
	Venues []struct {
		ID        string  `json:"id"`
		Name      string  `json:"name"`
		SportType string  `json:"sport_type"`
		Level     string  `json:"level"`
		Address   string  `json:"address"`
		Lat       float64 `json:"lat"`
		Lon       float64 `json:"lon"`
		SourceRef string  `json:"source_ref"`
		Slots     []struct {
			ID          string `json:"id"`
			StartAt     string `json:"start_at"`
			EndAt       string `json:"end_at"`
			QuotaTotal  int    `json:"quota_total"`
			QuotaBooked int    `json:"quota_booked"`
		} `json:"slots"`
	} `json:"venues"`
	TestUsers []struct {
		ID        string `json:"id"`
		MaxUserID string `json:"max_user_id"`
		Name      string `json:"name"`
	} `json:"test_users"`
}

type Loader struct {
	venueRepo repository.VenueRepository
	slotRepo  repository.SlotRepository
	userRepo  repository.UserRepository
}

func NewLoader(venueRepo repository.VenueRepository, slotRepo repository.SlotRepository, userRepo repository.UserRepository) *Loader {
	return &Loader{venueRepo: venueRepo, slotRepo: slotRepo, userRepo: userRepo}
}

// LoadIfEmpty загружает тестовые данные из JSON-файла, если в БД ещё нет
// ни одной площадки. Идемпотентно: повторный запуск при уже заполненной
// БД ничего не делает.
func (l *Loader) LoadIfEmpty(ctx context.Context, path string) error {
	existing, err := l.venueRepo.List(ctx)
	if err != nil {
		return fmt.Errorf("check existing venues: %w", err)
	}
	if len(existing) > 0 {
		return nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read seed file %s: %w", path, err)
	}

	var sf seedFile
	if err := json.Unmarshal(data, &sf); err != nil {
		return fmt.Errorf("unmarshal seed file: %w", err)
	}

	for _, v := range sf.Venues {
		venue := &model.Venue{
			ID:        v.ID,
			Name:      v.Name,
			SportType: v.SportType,
			Level:     v.Level,
			Address:   v.Address,
			Lat:       v.Lat,
			Lon:       v.Lon,
			SourceRef: v.SourceRef,
		}
		if err := l.venueRepo.Create(ctx, venue); err != nil {
			return fmt.Errorf("create seed venue %s: %w", v.ID, err)
		}

		for _, s := range v.Slots {
			startAt, err := time.Parse(time.RFC3339, s.StartAt)
			if err != nil {
				return fmt.Errorf("parse start_at for slot %s: %w", s.ID, err)
			}
			endAt, err := time.Parse(time.RFC3339, s.EndAt)
			if err != nil {
				return fmt.Errorf("parse end_at for slot %s: %w", s.ID, err)
			}

			slot := &model.Slot{
				ID:          s.ID,
				VenueID:     v.ID,
				StartAt:     startAt,
				EndAt:       endAt,
				QuotaTotal:  s.QuotaTotal,
				QuotaBooked: s.QuotaBooked,
			}
			if err := l.slotRepo.Create(ctx, slot); err != nil {
				return fmt.Errorf("create seed slot %s: %w", s.ID, err)
			}
		}
	}

	for _, u := range sf.TestUsers {
		if _, err := l.userRepo.GetOrCreateByMaxUserID(ctx, u.MaxUserID, u.Name); err != nil {
			return fmt.Errorf("create seed user %s: %w", u.MaxUserID, err)
		}
	}

	return nil
}
