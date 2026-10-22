package service

import (
	"context"
	"math"
	"sort"
	"time"

	"sportslot/internal/catalog"
	"sportslot/internal/model"
	"sportslot/internal/repository/postgres"
	"sportslot/internal/routing"
)

type SearchService struct {
	store       *postgres.Store
	includeDemo bool
	catalogDate time.Time
	router      routing.Router
}

func NewSearchService(store *postgres.Store, includeDemo bool, catalogDate time.Time) *SearchService {
	return &SearchService{store: store, includeDemo: includeDemo, catalogDate: catalogDate, router: routing.Estimator{}}
}

// SetRouter подключает расчёт времени в пути (по умолчанию — оценка по расстоянию).
func (s *SearchService) SetRouter(r routing.Router) { s.router = r }

// routedLimit — сколько ближайших площадок получают точное время в пути за один
// поиск (две матрицы 2ГИС по 24 точки). Остальным достаётся оценка.
const routedLimit = 48

// minWithinLimit — если в лимит попало меньше залов, ниже показываются
// ближайшие «чуть дальше», чтобы выдача не была пустой.
const minWithinLimit = 4

func (s *SearchService) DemoEnabled() bool      { return s.includeDemo }
func (s *SearchService) CatalogDate() time.Time { return s.catalogDate }

type VenueSearch struct {
	Sport    string
	When     string
	District string
	// Lat/Lon — точка пользователя; если не заданы, берётся центр района или города.
	Lat, Lon    float64
	HasPoint    bool
	InstantOnly bool
	// Mode и MaxMinutes — способ передвижения и лимит времени в пути.
	// Учитываются только вместе с точкой пользователя.
	Mode       routing.Mode
	MaxMinutes int
}

// Travel — время в пути от пользователя; Estimated — оценка без маршрута.
type Travel struct {
	Mode      routing.Mode `json:"mode"`
	Minutes   int          `json:"minutes"`
	Estimated bool         `json:"estimated"`
}

type VenueCard struct {
	Venue      model.Venue `json:"venue"`
	DistanceKM float64     `json:"distance_km"`
	Travel     *Travel     `json:"travel,omitempty"`
	// BeyondLimit — зал дальше выбранного лимита времени, показан как запасной.
	BeyondLimit    bool             `json:"beyond_limit,omitempty"`
	Slots          []model.SlotInfo `json:"slots"`
	AvailableSlots int              `json:"available_slots"`
	// Rating — средняя оценка, если студия включила показ отзывов.
	Rating *model.ReviewSummary `json:"rating,omitempty"`
}

func (q VenueSearch) origin() (float64, float64) {
	if q.HasPoint {
		return q.Lat, q.Lon
	}
	if d, ok := catalog.DistrictByID(q.District); ok {
		return d.Lat, d.Lon
	}
	return catalog.CityCenter.Lat, catalog.CityCenter.Lon
}

// FindVenues возвращает площадки по фильтрам. Сначала — студии с онлайн-записью
// и подходящими занятиями, затем реальные площадки из каталога; внутри групп —
// по расстоянию. Для города масштаба Ростова (около сотни площадок) фильтрация
// в памяти проще и быстрее геоиндекса.
func (s *SearchService) FindVenues(ctx context.Context, q VenueSearch, now time.Time) ([]VenueCard, error) {
	venues, err := s.store.ListVenues(ctx, postgres.VenueQuery{Sport: q.Sport, IncludeDemo: s.includeDemo})
	if err != nil {
		return nil, err
	}
	lat, lon := q.origin()

	var instantIDs []string
	for _, v := range venues {
		if v.BookingMode == model.BookingModeInstant {
			instantIDs = append(instantIDs, v.ID)
		}
	}
	slotsByVenue := map[string][]model.Slot{}
	if len(instantIDs) > 0 {
		from, to := catalog.WhenRange(q.When, now)
		slots, err := s.store.ListSlots(ctx, postgres.SlotQuery{VenueIDs: instantIDs, Sport: q.Sport, From: from, To: to})
		if err != nil {
			return nil, err
		}
		for _, sl := range slots {
			if catalog.WhenMatches(q.When, sl.StartAt) {
				slotsByVenue[sl.VenueID] = append(slotsByVenue[sl.VenueID], sl)
			}
		}
	}

	cards := make([]VenueCard, 0, len(venues))
	for _, v := range venues {
		card := VenueCard{Venue: v, DistanceKM: round1(catalog.DistanceKM(lat, lon, v.Lat, v.Lon)), Slots: []model.SlotInfo{}}
		if v.BookingMode == model.BookingModeInstant {
			for _, sl := range slotsByVenue[v.ID] {
				card.Slots = append(card.Slots, sl.Info())
				if sl.QuotaAvailable() > 0 {
					card.AvailableSlots++
				}
			}
			if len(card.Slots) == 0 {
				continue
			}
		} else if q.InstantOnly {
			continue
		}
		cards = append(cards, card)
	}

	if q.HasPoint && q.Mode != "" {
		cards = s.withTravel(ctx, cards, q)
	}
	if len(instantIDs) > 0 {
		if ratings, err := s.store.PublicRatings(ctx, instantIDs); err == nil {
			for i := range cards {
				if r, ok := ratings[cards[i].Venue.ID]; ok {
					r := r
					cards[i].Rating = &r
				}
			}
		}
	}

	sort.SliceStable(cards, func(i, j int) bool {
		gi, gj := group(cards[i]), group(cards[j])
		if gi != gj {
			return gi < gj
		}
		if cards[i].Travel != nil && cards[j].Travel != nil && cards[i].Travel.Minutes != cards[j].Travel.Minutes {
			return cards[i].Travel.Minutes < cards[j].Travel.Minutes
		}
		return cards[i].DistanceKM < cards[j].DistanceKM
	})
	return cards, nil
}

// withTravel добавляет время в пути и отбрасывает площадки за пределами лимита.
// Заведомо далёкие отсекаются по прямому расстоянию ещё до запроса маршрутов.
func (s *SearchService) withTravel(ctx context.Context, cards []VenueCard, q VenueSearch) []VenueCard {
	if q.MaxMinutes > 0 {
		limit := routing.MaxStraightKM(q.Mode, extendedLimit(q.MaxMinutes))
		kept := cards[:0]
		for _, c := range cards {
			if c.DistanceKM <= limit {
				kept = append(kept, c)
			}
		}
		cards = kept
	}
	sort.SliceStable(cards, func(i, j int) bool { return cards[i].DistanceKM < cards[j].DistanceKM })

	from := routing.Point{Lat: q.Lat, Lon: q.Lon}
	n := len(cards)
	if n > routedLimit {
		n = routedLimit
	}
	targets := make([]routing.Point, n)
	for i := 0; i < n; i++ {
		targets[i] = routing.Point{Lat: cards[i].Venue.Lat, Lon: cards[i].Venue.Lon}
	}
	trips := s.router.Trips(ctx, from, targets, q.Mode)
	for i := range cards {
		trip := routing.Estimate(from, routing.Point{Lat: cards[i].Venue.Lat, Lon: cards[i].Venue.Lon}, q.Mode)
		if i < len(trips) {
			trip = trips[i]
		}
		cards[i].Travel = &Travel{Mode: q.Mode, Minutes: trip.Minutes(), Estimated: trip.Estimated}
	}

	if q.MaxMinutes <= 0 {
		return cards
	}
	var within, beyond []VenueCard
	for _, c := range cards {
		switch {
		case c.Travel.Minutes <= q.MaxMinutes:
			within = append(within, c)
		case c.Travel.Minutes <= extendedLimit(q.MaxMinutes):
			c.BeyondLimit = true
			beyond = append(beyond, c)
		}
	}
	if len(within) >= minWithinLimit {
		return within
	}
	sort.SliceStable(beyond, func(i, j int) bool { return beyond[i].Travel.Minutes < beyond[j].Travel.Minutes })
	if extra := minWithinLimit - len(within); len(beyond) > extra {
		beyond = beyond[:extra]
	}
	return append(within, beyond...)
}

// extendedLimit — до скольки минут ищем запасные залы «чуть дальше».
func extendedLimit(minutes int) int {
	if minutes*2 > minutes+20 {
		return minutes + 20
	}
	return minutes * 2
}

func group(c VenueCard) int {
	switch {
	case c.BeyondLimit:
		return 3
	case c.Venue.BookingMode == model.BookingModeInstant && c.AvailableSlots > 0:
		return 0
	case c.Venue.BookingMode == model.BookingModeInstant:
		return 2
	}
	return 1
}

func round1(x float64) float64 { return math.Round(x*10) / 10 }

type VenueDetails struct {
	model.Venue
	VenueID  string           `json:"venue_id"`
	AllSlots []model.SlotInfo `json:"all_slots"`
	// Reviews — только если студия включила показ отзывов.
	Reviews *model.ReviewSummary `json:"reviews,omitempty"`
	Passes  []model.Pass         `json:"passes,omitempty"`
	Online  *OnlinePayment       `json:"online_payment,omitempty"`
}

// OnlinePayment — что у студии можно оплатить онлайн.
type OnlinePayment struct {
	Trial  bool `json:"trial"`
	Passes bool `json:"passes"`
}

// VenueDetails — площадка и её расписание на две недели.
func (s *SearchService) VenueDetails(ctx context.Context, id string, now time.Time) (*VenueDetails, error) {
	v, err := s.store.GetVenue(ctx, id)
	if err != nil {
		return nil, err
	}
	if v.IsDemo() && !s.includeDemo {
		return nil, ErrNotFound
	}
	d := &VenueDetails{Venue: *v, VenueID: v.ID, AllSlots: []model.SlotInfo{}}
	if v.BookingMode == model.BookingModeInstant {
		slots, err := s.store.ListSlots(ctx, postgres.SlotQuery{VenueIDs: []string{v.ID}, From: now, To: now.Add(catalog.SearchHorizon)})
		if err != nil {
			return nil, err
		}
		for _, sl := range slots {
			d.AllSlots = append(d.AllSlots, sl.Info())
		}
		if err := s.studioExtras(ctx, d); err != nil {
			return nil, err
		}
	}
	return d, nil
}

// studioExtras — отзывы, абонементы и онлайн-оплата студии с онлайн-записью.
func (s *SearchService) studioExtras(ctx context.Context, d *VenueDetails) error {
	settings, err := s.store.GetSettings(ctx, d.ID)
	if err != nil {
		return err
	}
	if settings.ShowReviews {
		if d.Reviews, err = s.store.VenueReviews(ctx, d.ID, 5, false); err != nil {
			return err
		}
	}
	if d.Passes, err = s.store.ListPasses(ctx, d.ID, true); err != nil {
		return err
	}
	if settings.PaymentsReady() {
		price := 0
		if d.TrialPrice != nil {
			price = *d.TrialPrice
		}
		d.Online = &OnlinePayment{Trial: settings.TrialOnline && price > 0, Passes: len(d.Passes) > 0}
	}
	return nil
}

// SlotSearch — параметры исторического POST /api/v1/search (контракт DATA-API).
type SlotSearch struct {
	Sport            string
	Lat, Lon         float64
	RadiusKM         float64
	DateFrom, DateTo time.Time
	TimeFrom, TimeTo string
}

type SlotResult struct {
	VenueID   string         `json:"venue_id"`
	Name      string         `json:"name"`
	SportType string         `json:"sport_type"`
	Level     string         `json:"level"`
	Address   string         `json:"address"`
	District  string         `json:"district"`
	Lat       float64        `json:"lat"`
	Lon       float64        `json:"lon"`
	Source    string         `json:"source"`
	SourceRef string         `json:"source_ref"`
	Slot      model.SlotInfo `json:"slot"`
}

// FindSlots — поиск свободных занятий с онлайн-записью в радиусе от точки.
func (s *SearchService) FindSlots(ctx context.Context, q SlotSearch, now time.Time) ([]SlotResult, error) {
	venues, err := s.store.ListVenues(ctx, postgres.VenueQuery{Sport: q.Sport, IncludeDemo: s.includeDemo})
	if err != nil {
		return nil, err
	}
	byID := map[string]model.Venue{}
	var ids []string
	for _, v := range venues {
		if v.BookingMode != model.BookingModeInstant {
			continue
		}
		if q.RadiusKM > 0 && (q.Lat != 0 || q.Lon != 0) && catalog.DistanceKM(q.Lat, q.Lon, v.Lat, v.Lon) > q.RadiusKM {
			continue
		}
		byID[v.ID] = v
		ids = append(ids, v.ID)
	}
	results := []SlotResult{}
	if len(ids) == 0 {
		return results, nil
	}
	from, to := q.DateFrom, q.DateTo
	if from.Before(now) {
		from = now
	}
	if to.IsZero() {
		to = from.Add(catalog.SearchHorizon)
	}
	slots, err := s.store.ListSlots(ctx, postgres.SlotQuery{VenueIDs: ids, Sport: q.Sport, From: from, To: to, OnlyAvailable: true})
	if err != nil {
		return nil, err
	}
	for _, sl := range slots {
		hm := sl.StartAt.In(catalog.Moscow).Format("15:04")
		if (q.TimeFrom != "" && hm < q.TimeFrom) || (q.TimeTo != "" && hm > q.TimeTo) {
			continue
		}
		v := byID[sl.VenueID]
		results = append(results, SlotResult{
			VenueID: v.ID, Name: v.Name, SportType: sl.SportType, Level: sl.Level, Address: v.Address,
			District: v.District, Lat: v.Lat, Lon: v.Lon, Source: string(v.Source), SourceRef: string(v.Source),
			Slot: sl.Info(),
		})
	}
	return results, nil
}

// VenueSlots — ближайшие занятия площадки по виду спорта (для «Другое время» в боте).
func (s *SearchService) VenueSlots(ctx context.Context, venueID, sport string, now time.Time) (*model.Venue, []model.Slot, error) {
	v, err := s.store.GetVenue(ctx, venueID)
	if err != nil {
		return nil, nil, err
	}
	slots, err := s.store.ListSlots(ctx, postgres.SlotQuery{VenueIDs: []string{venueID}, Sport: sport, From: now, To: now.Add(catalog.SearchHorizon)})
	return v, slots, err
}
