package model

import "time"

// StudioSettings — то, что студия включает в кабинете: отзывы, сводка, оплата.
type StudioSettings struct {
	VenueID         string `json:"venue_id"`
	ShowReviews     bool   `json:"show_reviews"`
	DailyDigest     bool   `json:"daily_digest"`
	PaymentProvider string `json:"payment_provider"` // "", yookassa, tbank
	PaymentAccount  string `json:"payment_account"`  // shopId или TerminalKey
	// PaymentSecret — зашифрованный ключ; наружу не отдаётся.
	PaymentSecret     []byte    `json:"-"`
	PaymentSecretHint string    `json:"payment_secret_hint"`
	PaymentsEnabled   bool      `json:"payments_enabled"`
	TrialOnline       bool      `json:"trial_online"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// PaymentsReady — оплата включена и данные магазина заполнены.
func (s *StudioSettings) PaymentsReady() bool {
	return s != nil && s.PaymentsEnabled && s.PaymentProvider != "" && s.PaymentAccount != "" && len(s.PaymentSecret) > 0
}

// Pass — абонемент, который продаёт студия.
type Pass struct {
	ID        string    `json:"id"`
	VenueID   string    `json:"venue_id"`
	Title     string    `json:"title"`
	Visits    int       `json:"visits"`
	ValidDays int       `json:"valid_days"`
	Price     int       `json:"price"` // рубли
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
}

const (
	UserPassRequested = "requested"
	UserPassActive    = "active"
	UserPassUsed      = "used"
	UserPassExpired   = "expired"
	UserPassCancelled = "cancelled"
)

// UserPass — абонемент клиента.
type UserPass struct {
	ID          string     `json:"id"`
	PassID      string     `json:"pass_id,omitempty"`
	VenueID     string     `json:"venue_id"`
	UserID      string     `json:"user_id"`
	Title       string     `json:"title"`
	VisitsTotal int        `json:"visits_total"`
	VisitsLeft  int        `json:"visits_left"`
	ValidDays   int        `json:"valid_days"`
	Price       int        `json:"price"`
	Status      string     `json:"status"`
	PaidBy      string     `json:"paid_by,omitempty"`
	ValidUntil  *time.Time `json:"valid_until,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	ActivatedAt *time.Time `json:"activated_at,omitempty"`

	// Для кабинета и уведомлений.
	VenueName string `json:"venue_name,omitempty"`
	UserName  string `json:"user_name,omitempty"`
	MaxUserID string `json:"-"`
}

const (
	PaymentPending   = "pending"
	PaymentSucceeded = "succeeded"
	PaymentCanceled  = "canceled"

	PaymentKindTrial = "trial"
	PaymentKindPass  = "pass"
)

// Payment — онлайн-оплата пробного занятия или абонемента. Сумма в копейках.
type Payment struct {
	ID                string     `json:"id"`
	VenueID           string     `json:"venue_id"`
	UserID            string     `json:"user_id"`
	Provider          string     `json:"provider"`
	Kind              string     `json:"kind"`
	BookingID         string     `json:"booking_id,omitempty"`
	UserPassID        string     `json:"user_pass_id,omitempty"`
	Amount            int        `json:"amount"`
	Description       string     `json:"description"`
	Status            string     `json:"status"`
	ProviderPaymentID string     `json:"-"`
	ConfirmationURL   string     `json:"confirmation_url,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	PaidAt            *time.Time `json:"paid_at,omitempty"`

	UserName  string `json:"user_name,omitempty"`
	MaxUserID string `json:"-"`
}

// Review — оценка и отзыв клиента после посещения.
type Review struct {
	Rating int       `json:"rating"`
	Text   string    `json:"text,omitempty"`
	Author string    `json:"author"` // только имя, без фамилии
	Date   time.Time `json:"date"`
}

// ReviewSummary — средняя оценка и последние отзывы для карточки студии.
type ReviewSummary struct {
	Average float64  `json:"average"`
	Count   int      `json:"count"`
	Items   []Review `json:"items"`
}

// ResponseStats — как быстро студия отвечает на сообщения клиентов.
type ResponseStats struct {
	Answered      int     `json:"answered"`
	MedianMinutes float64 `json:"median_minutes"`
	WithinHour    int     `json:"within_hour"`
	Waiting       int     `json:"waiting"`
}

// PaymentTotals — сумма оплат за период.
type PaymentTotals struct {
	Count  int `json:"count"`
	Amount int `json:"amount"` // рубли
}
