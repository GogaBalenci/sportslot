package model

import "time"

// VenueSlotResult - результат поиска: площадка вместе с одним подходящим слотом.
// Используется в ответах /api/v1/search.
type VenueSlotResult struct {
	VenueID   string   `json:"venue_id"`
	Name      string   `json:"name"`
	SportType string   `json:"sport_type"`
	Level     string   `json:"level"`
	Address   string   `json:"address"`
	Lat       float64  `json:"lat"`
	Lon       float64  `json:"lon"`
	SourceRef string   `json:"source_ref"`
	Slot      SlotInfo `json:"slot"`
}

type SlotInfo struct {
	SlotID         string    `json:"slot_id"`
	StartAt        time.Time `json:"start_at"`
	EndAt          time.Time `json:"end_at"`
	QuotaAvailable int       `json:"quota_available"`
}

// VenueDetails - детальная информация о площадке со всеми будущими слотами.
// Используется в ответе GET /api/v1/venues/{id}.
type VenueDetails struct {
	VenueID   string     `json:"venue_id"`
	Name      string     `json:"name"`
	SportType string     `json:"sport_type"`
	Level     string     `json:"level"`
	Address   string     `json:"address"`
	Lat       float64    `json:"lat"`
	Lon       float64    `json:"lon"`
	SourceRef string     `json:"source_ref"`
	AllSlots  []SlotInfo `json:"all_slots"`
}
