package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"sportslot/internal/repository"
	"sportslot/internal/service"
)

type SearchHandler struct {
	matching *service.MatchingService
}

func NewSearchHandler(matching *service.MatchingService) *SearchHandler {
	return &SearchHandler{matching: matching}
}

type searchRequest struct {
	SportType string  `json:"sport_type"`
	Lat       float64 `json:"lat"`
	Lon       float64 `json:"lon"`
	RadiusKM  float64 `json:"radius_km"`
	DateFrom  string  `json:"date_from"`
	DateTo    string  `json:"date_to"`
	TimeFrom  string  `json:"time_from"`
	TimeTo    string  `json:"time_to"`
	Level     string  `json:"level"`
}

type searchMeta struct {
	DataSource  string    `json:"data_source"`
	GeneratedAt time.Time `json:"generated_at"`
}

func (h *SearchHandler) Search(w http.ResponseWriter, r *http.Request) {
	var req searchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Некорректное тело запроса")
		return
	}

	if req.SportType == "" {
		writeError(w, http.StatusBadRequest, "missing_sport_type", "Поле sport_type обязательно")
		return
	}

	var dateFrom, dateTo time.Time
	var err error

	if req.DateFrom != "" {
		dateFrom, err = time.Parse("2006-01-02", req.DateFrom)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_date_from", "Некорректный формат date_from, ожидается YYYY-MM-DD")
			return
		}
	}

	if req.DateTo != "" {
		dateTo, err = time.Parse("2006-01-02", req.DateTo)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_date_to", "Некорректный формат date_to, ожидается YYYY-MM-DD")
			return
		}
		dateTo = dateTo.Add(24*time.Hour - time.Second)
	}

	results, err := h.matching.FindSlots(r.Context(), service.SearchCriteria{
		SportType: req.SportType,
		Level:     req.Level,
		Lat:       req.Lat,
		Lon:       req.Lon,
		RadiusKM:  req.RadiusKM,
		DateFrom:  dateFrom,
		DateTo:    dateTo,
		TimeFrom:  req.TimeFrom,
		TimeTo:    req.TimeTo,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, "search_failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"results": results,
		"meta": searchMeta{
			DataSource:  "test-data",
			GeneratedAt: time.Now(),
		},
	})
}

func (h *SearchHandler) GetVenue(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing_id", "Не указан id площадки")
		return
	}

	details, err := h.matching.GetVenueDetails(r.Context(), id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			writeError(w, http.StatusNotFound, "venue_not_found", "Площадка не найдена")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "Внутренняя ошибка")
		return
	}

	writeJSON(w, http.StatusOK, details)
}
