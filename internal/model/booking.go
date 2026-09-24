package model

import "time"

type BookingStatus string

const (
	BookingStatusConfirmed BookingStatus = "confirmed"
	BookingStatusCancelled BookingStatus = "cancelled"
)

type SourceChannel string

const (
	SourceChannelBot     SourceChannel = "bot"
	SourceChannelMiniApp SourceChannel = "miniapp"
)

// Booking - бронь пользователя на конкретный слот (тренировку).
type Booking struct {
	ID            string        `json:"id" db:"id"`
	UserID        string        `json:"user_id" db:"user_id"`
	SlotID        string        `json:"slot_id" db:"slot_id"`
	Status        BookingStatus `json:"status" db:"status"`
	SourceChannel SourceChannel `json:"source_channel" db:"source_channel"`
	ReminderSent  bool          `json:"reminder_sent" db:"reminder_sent"`
	CreatedAt     time.Time     `json:"created_at" db:"created_at"`
	CancelledAt   *time.Time    `json:"cancelled_at,omitempty" db:"cancelled_at"`
}

// UpcomingReminder - агрегированная строка для NotifierService: бронь,
// у которой скоро начинается тренировка и ещё не отправлено напоминание.
type UpcomingReminder struct {
	BookingID   string
	MaxUserID   string
	VenueName   string
	SlotStartAt time.Time
}
