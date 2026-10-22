// Package service — бизнес-логика СпортСлота поверх хранилища.
package service

import (
	"errors"

	"sportslot/internal/repository"
)

var (
	ErrNotFound       = repository.ErrNotFound
	ErrQuotaExceeded  = repository.ErrQuotaExceeded
	ErrSlotStarted    = repository.ErrSlotStarted
	ErrNotActive      = repository.ErrNotActive
	ErrAlreadyBooked  = repository.ErrAlreadyBooked
	ErrBookingClosed  = repository.ErrBookingClosed
	ErrForbidden      = errors.New("booking belongs to another user")
	ErrExternalVenue  = errors.New("venue has no online booking")
	ErrInvalidRequest = errors.New("invalid request")
	ErrOwnInvite      = repository.ErrOwnInvite

	ErrPaymentsOff   = errors.New("online payments are not enabled for the studio")
	ErrNothingToPay  = errors.New("nothing to pay")
	ErrPaymentFailed = errors.New("payment provider rejected the request")
	ErrNoSecretKey   = errors.New("payment secret encryption is not configured")
	ErrNoFriendPlace = errors.New("no second place for a friend")
)
