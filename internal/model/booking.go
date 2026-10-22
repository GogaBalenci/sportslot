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
	// CancelReason — кто отменил: user или studio.
	CancelReason string     `json:"cancel_reason,omitempty"`
	Review       string     `json:"review,omitempty"`
	ReviewedAt   *time.Time `json:"reviewed_at,omitempty"`
	// GuestToken — место для друга, которое он ещё не принял по ссылке.
	// Наружу не отдаётся: ссылку получает только пригласивший.
	GuestToken    string     `json:"-"`
	IsGuest       bool       `json:"is_guest,omitempty"`
	HostBookingID string     `json:"host_booking_id,omitempty"`
	UserPassID    string     `json:"user_pass_id,omitempty"`
	PaidAt        *time.Time `json:"paid_at,omitempty"`
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
