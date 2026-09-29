package model

import "time"

// Slot — конкретное занятие с квотой мест, которую студия выделила сервису.
type Slot struct {
	ID          string    `json:"id"`
	VenueID     string    `json:"venue_id"`
	Title       string    `json:"title"`
	SportType   string    `json:"sport_type"`
	Level       string    `json:"level"`
	StartAt     time.Time `json:"start_at"`
	EndAt       time.Time `json:"end_at"`
	QuotaTotal  int       `json:"quota_total"`
	QuotaBooked int       `json:"quota_booked"`
}

func (s *Slot) QuotaAvailable() int {
	if available := s.QuotaTotal - s.QuotaBooked; available > 0 {
		return available
	}
	return 0
}

// SlotInfo — слот в ответах API.
type SlotInfo struct {
	SlotID         string    `json:"slot_id"`
	Title          string    `json:"title"`
	SportType      string    `json:"sport_type"`
	StartAt        time.Time `json:"start_at"`
	EndAt          time.Time `json:"end_at"`
	QuotaTotal     int       `json:"quota_total"`
	QuotaAvailable int       `json:"quota_available"`
}

func (s *Slot) Info() SlotInfo {
	return SlotInfo{
		SlotID:         s.ID,
		Title:          s.Title,
		SportType:      s.SportType,
		StartAt:        s.StartAt,
		EndAt:          s.EndAt,
		QuotaTotal:     s.QuotaTotal,
		QuotaAvailable: s.QuotaAvailable(),
	}
}
