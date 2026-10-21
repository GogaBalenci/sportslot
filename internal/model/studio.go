package model

import "time"

// Template — повторяющееся занятие в недельном расписании студии.
type Template struct {
	ID          string    `json:"id"`
	VenueID     string    `json:"venue_id"`
	Weekday     int       `json:"weekday"` // 0 — воскресенье, как time.Weekday
	StartMinute int       `json:"start_minute"`
	DurationMin int       `json:"duration_min"`
	Title       string    `json:"title"`
	SportType   string    `json:"sport_type"`
	Level       string    `json:"level"`
	Quota       int       `json:"quota"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type StudioAdmin struct {
	VenueID   string    `json:"venue_id"`
	MaxUserID string    `json:"max_user_id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type Invite struct {
	Token     string     `json:"token"`
	VenueID   string     `json:"venue_id"`
	ExpiresAt time.Time  `json:"expires_at"`
	UsedBy    string     `json:"used_by,omitempty"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
}

// StudioSlot — занятие в кабинете студии вместе с числом записей.
type StudioSlot struct {
	Slot
	Booked int `json:"booked"` // реальные записи через СпортСлот
}

type Message struct {
	ID        int64     `json:"id"`
	VenueID   string    `json:"venue_id"`
	UserID    string    `json:"user_id"`
	Author    string    `json:"author"` // client | studio
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
}

// Thread — переписка студии с одним клиентом.
type Thread struct {
	UserID    string    `json:"user_id"`
	UserName  string    `json:"user_name"`
	MaxUserID string    `json:"-"`
	LastText  string    `json:"last_text"`
	LastAt    time.Time `json:"last_at"`
	LastBy    string    `json:"last_by"`
	Unread    int       `json:"unread"`
}

type StudioStats struct {
	Since          time.Time `json:"since"`
	Bookings       int       `json:"bookings"`
	Attended       int       `json:"attended"`
	NoShow         int       `json:"no_show"`
	CancelledUser  int       `json:"cancelled_by_client"`
	Upcoming       int       `json:"upcoming"`
	Clients        int       `json:"clients"`
	ReturningUsers int       `json:"returning_clients"`
	RatingAvg      float64   `json:"rating_avg"`
	Ratings        int       `json:"ratings"`
}
