package service

import (
	"context"
	"fmt"
	"time"

	"sportslot/internal/model"
	"sportslot/internal/repository"
)

const defaultSearchWindowDays = 14
const defaultRadiusKM = 5

type MatchingService struct {
	venueRepo repository.VenueRepository
	slotRepo  repository.SlotRepository
}

func NewMatchingService(venueRepo repository.VenueRepository, slotRepo repository.SlotRepository) *MatchingService {
	return &MatchingService{venueRepo: venueRepo, slotRepo: slotRepo}
}

type SearchCriteria struct {
	SportType string
	Level     string
	Lat       float64
	Lon       float64
	RadiusKM  float64
	DateFrom  time.Time
	DateTo    time.Time
	TimeFrom  string
	TimeTo    string
}

// FindSlots возвращает все подходящие варианты по критериям поиска,
// отсортированные по времени начала тренировки (ближайшие первыми).
func (m *MatchingService) FindSlots(ctx context.Context, c SearchCriteria) ([]model.VenueSlotResult, error) {
	if c.SportType == "" {
		return nil, fmt.Errorf("sport_type is required")
	}

	dateFrom := c.DateFrom
	if dateFrom.IsZero() {
		dateFrom = time.Now()
	}

	dateTo := c.DateTo
	if dateTo.IsZero() {
		dateTo = dateFrom.Add(defaultSearchWindowDays * 24 * time.Hour)
	}

	radius := c.RadiusKM
	if radius <= 0 {
		radius = defaultRadiusKM
	}

	rows, err := m.slotRepo.Search(ctx, repository.SlotSearchParams{
		SportType: c.SportType,
		Level:     c.Level,
		DateFrom:  dateFrom,
		DateTo:    dateTo,
		TimeFrom:  c.TimeFrom,
		TimeTo:    c.TimeTo,
		Lat:       c.Lat,
		Lon:       c.Lon,
		RadiusKM:  radius,
	})
	if err != nil {
		return nil, fmt.Errorf("search slots: %w", err)
	}

	results := make([]model.VenueSlotResult, 0, len(rows))
	for _, row := range rows {
		results = append(results, model.VenueSlotResult{
			VenueID:   row.Venue.ID,
			Name:      row.Venue.Name,
			SportType: row.Venue.SportType,
			Level:     row.Venue.Level,
			Address:   row.Venue.Address,
			Lat:       row.Venue.Lat,
			Lon:       row.Venue.Lon,
			SourceRef: row.Venue.SourceRef,
			Slot: model.SlotInfo{
				SlotID:         row.Slot.ID,
				StartAt:        row.Slot.StartAt,
				EndAt:          row.Slot.EndAt,
				QuotaAvailable: row.Slot.QuotaAvailable(),
			},
		})
	}

	return results, nil
}

// GetVenueDetails возвращает детальную информацию о площадке со всеми
// будущими слотами - используется в GET /api/v1/venues/{id}.
func (m *MatchingService) GetVenueDetails(ctx context.Context, venueID string) (*model.VenueDetails, error) {
	venue, err := m.venueRepo.GetByID(ctx, venueID)
	if err != nil {
		return nil, err
	}

	slots, err := m.slotRepo.ListFutureByVenueID(ctx, venueID)
	if err != nil {
		return nil, fmt.Errorf("list venue slots: %w", err)
	}

	slotInfos := make([]model.SlotInfo, 0, len(slots))
	for _, s := range slots {
		slotInfos = append(slotInfos, model.SlotInfo{
			SlotID:         s.ID,
			StartAt:        s.StartAt,
			EndAt:          s.EndAt,
			QuotaAvailable: s.QuotaAvailable(),
		})
	}

	return &model.VenueDetails{
		VenueID:   venue.ID,
		Name:      venue.Name,
		SportType: venue.SportType,
		Level:     venue.Level,
		Address:   venue.Address,
		Lat:       venue.Lat,
		Lon:       venue.Lon,
		SourceRef: venue.SourceRef,
		AllSlots:  slotInfos,
	}, nil
}
