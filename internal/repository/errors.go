package repository

import "errors"

var (
	ErrNotFound      = errors.New("resource not found")
	ErrQuotaExceeded = errors.New("quota exceeded")
	ErrSlotStarted   = errors.New("slot already started")
	ErrNotActive     = errors.New("booking is not active")
)

var ErrAlreadyBooked = errors.New("user already booked this slot")
