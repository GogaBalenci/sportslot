package model

import "time"

// Slot - конкретная тренировка на конкретную дату и время (start_at/end_at),
// с гарантированной квотой мест, выделенной залом-партнёром сервису
// (модель аллокации, см. продуктовые допущения MVP). Привязка к точному
// start_at/end_at (а не к шаблону дня недели) устраняет баг, при котором
// квота списывалась бы навсегда и не различала тренировки разных недель.
type Slot struct {
	ID          string    `json:"id" db:"id"`
	VenueID     string    `json:"venue_id" db:"venue_id"`
	StartAt     time.Time `json:"start_at" db:"start_at"`
	EndAt       time.Time `json:"end_at" db:"end_at"`
	QuotaTotal  int       `json:"quota_total" db:"quota_total"`
	QuotaBooked int       `json:"quota_booked" db:"quota_booked"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
}

// QuotaAvailable возвращает количество свободных мест в рамках выделенной квоты.
func (s *Slot) QuotaAvailable() int {
	available := s.QuotaTotal - s.QuotaBooked
	if available < 0 {
		return 0
	}
	return available
}
