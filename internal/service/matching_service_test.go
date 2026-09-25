package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"sportslot/internal/model"
	"sportslot/internal/repository"
	"sportslot/internal/service"
)

type mockVenueRepo struct {
	getByIDFunc func(ctx context.Context, id string) (*model.Venue, error)
}

func (m *mockVenueRepo) GetByID(ctx context.Context, id string) (*model.Venue, error) {
	if m.getByIDFunc != nil {
		return m.getByIDFunc(ctx, id)
	}
	return nil, repository.ErrNotFound
}
func (m *mockVenueRepo) Create(ctx context.Context, v *model.Venue) error { return nil }
func (m *mockVenueRepo) List(ctx context.Context) ([]model.Venue, error)   { return nil, nil }

func TestFindSlots_Validation(t *testing.T) {
	svc := service.NewMatchingService(&mockVenueRepo{}, &mockSlotRepo{})

	_, err := svc.FindSlots(context.Background(), service.SearchCriteria{
		SportType: "",
	})
	if err == nil || err.Error() != "sport_type is required" {
		t.Fatalf("expected 'sport_type is required', got: %v", err)
	}
}

func TestFindSlots_Success(t *testing.T) {
	now := time.Now()
	// override Search
	mockRows := []repository.SlotSearchRow{
		{
			Venue: model.Venue{
				ID:        "v-1",
				Name:      "Атлант",
				SportType: "boxing",
				Level:     "beginner",
				Address:   "ул. Ленина 10",
				Lat:       55.75,
				Lon:       37.61,
			},
			Slot: model.Slot{
				ID:          "s-1",
				StartAt:     now.Add(24 * time.Hour),
				EndAt:       now.Add(25 * time.Hour),
				QuotaTotal:  3,
				QuotaBooked: 1,
			},
		},
	}

	searchSlotRepo := &searchSlotRepoMock{rows: mockRows}
	svc := service.NewMatchingService(&mockVenueRepo{}, searchSlotRepo)

	results, err := svc.FindSlots(context.Background(), service.SearchCriteria{
		SportType: "boxing",
		Lat:       55.75,
		Lon:       37.61,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Name != "Атлант" {
		t.Errorf("expected Атлант, got %s", results[0].Name)
	}
	if results[0].Slot.QuotaAvailable != 2 {
		t.Errorf("expected 2 available slots, got %d", results[0].Slot.QuotaAvailable)
	}
}

type searchSlotRepoMock struct {
	mockSlotRepo
	rows []repository.SlotSearchRow
}

func (s *searchSlotRepoMock) Search(ctx context.Context, params repository.SlotSearchParams) ([]repository.SlotSearchRow, error) {
	return s.rows, nil
}

func (s *searchSlotRepoMock) ListFutureByVenueID(ctx context.Context, venueID string) ([]model.Slot, error) {
	return []model.Slot{
		{
			ID:          "s-future",
			StartAt:     time.Now().Add(10 * time.Hour),
			EndAt:       time.Now().Add(11 * time.Hour),
			QuotaTotal:  5,
			QuotaBooked: 2,
		},
	}, nil
}

func TestGetVenueDetails(t *testing.T) {
	ctx := context.Background()

	venueRepo := &mockVenueRepo{
		getByIDFunc: func(ctx context.Context, id string) (*model.Venue, error) {
			if id == "v-1" {
				return &model.Venue{ID: "v-1", Name: "Атлант", SportType: "boxing"}, nil
			}
			return nil, repository.ErrNotFound
		},
	}
	slotRepo := &searchSlotRepoMock{}
	svc := service.NewMatchingService(venueRepo, slotRepo)

	// 1. Success
	details, err := svc.GetVenueDetails(ctx, "v-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if details.Name != "Атлант" {
		t.Errorf("expected Атлант, got %s", details.Name)
	}
	if len(details.AllSlots) != 1 {
		t.Errorf("expected 1 slot, got %d", len(details.AllSlots))
	}
	if details.AllSlots[0].QuotaAvailable != 3 {
		t.Errorf("expected 3 quota available, got %d", details.AllSlots[0].QuotaAvailable)
	}

	// 2. Not found
	_, err = svc.GetVenueDetails(ctx, "v-missing")
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}
