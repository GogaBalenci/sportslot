package model

import "time"

type VenueSource string

const (
	// VenueSourceOSM — реальный объект из выгрузки OpenStreetMap.
	VenueSourceOSM VenueSource = "osm"
	// VenueSourceDemo — вымышленная студия-партнёр с модельным расписанием.
	VenueSourceDemo VenueSource = "demo_partner"
)

type BookingMode string

const (
	// BookingModeInstant — запись в сервисе с квотой мест.
	BookingModeInstant BookingMode = "instant"
	// BookingModeExternal — сервис показывает контакты, запись у самой площадки.
	BookingModeExternal BookingMode = "external"
)

type Venue struct {
	ID           string      `json:"id"`
	ExternalID   string      `json:"external_id,omitempty"`
	Name         string      `json:"name"`
	SportType    string      `json:"sport_type"`
	Sports       []string    `json:"sports"`
	Level        string      `json:"level"`
	Address      string      `json:"address"`
	District     string      `json:"district"`
	Lat          float64     `json:"lat"`
	Lon          float64     `json:"lon"`
	Source       VenueSource `json:"source"`
	SourceURL    string      `json:"source_url,omitempty"`
	BookingMode  BookingMode `json:"booking_mode"`
	Phone        string      `json:"phone,omitempty"`
	Website      string      `json:"website,omitempty"`
	OpeningHours string      `json:"opening_hours,omitempty"`
	Description  string      `json:"description,omitempty"`
	WhatToBring  string      `json:"what_to_bring,omitempty"`
	// TrialPrice — цена пробного занятия в рублях; nil — неизвестна.
	TrialPrice *int       `json:"trial_price,omitempty"`
	VerifiedAt *time.Time `json:"verified_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

func (v *Venue) IsDemo() bool { return v.Source == VenueSourceDemo }

func (v *Venue) HasSport(sport string) bool {
	for _, s := range v.Sports {
		if s == sport {
			return true
		}
	}
	return false
}
