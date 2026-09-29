package model

import "time"

type BookingStatus string

const (
	BookingStatusConfirmed BookingStatus = "confirmed"
	BookingStatusCancelled BookingStatus = "cancelled"
	BookingStatusAttended  BookingStatus = "attended"
	BookingStatusNoShow    BookingStatus = "no_show"
)

type SourceChannel string

const (
	SourceChannelBot     SourceChannel = "bot"
	SourceChannelMiniApp SourceChannel = "miniapp"
)

type Booking struct {
	ID            string        `json:"id"`
	UserID        string        `json:"user_id"`
	SlotID        string        `json:"slot_id"`
	Status        BookingStatus `json:"status"`
	SourceChannel SourceChannel `json:"source_channel"`
	CheckinCode   string        `json:"checkin_code"`
	ReminderSent  bool          `json:"reminder_sent"`
	Rating        *int          `json:"rating,omitempty"`
	CreatedAt     time.Time     `json:"created_at"`
	CancelledAt   *time.Time    `json:"cancelled_at,omitempty"`
	AttendedAt    *time.Time    `json:"attended_at,omitempty"`
}

// BookingView — бронь вместе с занятием и площадкой: то, что видят
// пользователь в «Моих занятиях» и администратор студии.
type BookingView struct {
	Booking
	MaxUserID string `json:"-"`
	UserName  string `json:"user_name,omitempty"`
	Slot      Slot   `json:"slot"`
	Venue     Venue  `json:"venue"`
}

type User struct {
	ID        string    `json:"id"`
	MaxUserID string    `json:"max_user_id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}
