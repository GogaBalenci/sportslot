package model

import "time"

// Venue - спортивная площадка (зал, секция, студия).
// Данные MVP тестовые: SourceRef по умолчанию "test-data".
type Venue struct {
	ID        string    `json:"id" db:"id"`
	Name      string    `json:"name" db:"name"`
	SportType string    `json:"sport_type" db:"sport_type"`
	Level     string    `json:"level" db:"level"`
	Address   string    `json:"address" db:"address"`
	Lat       float64   `json:"lat" db:"lat"`
	Lon       float64   `json:"lon" db:"lon"`
	SourceRef string    `json:"source_ref" db:"source_ref"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}
