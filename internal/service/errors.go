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
	ErrForbidden      = errors.New("booking belongs to another user")
	ErrExternalVenue  = errors.New("venue has no online booking")
	ErrInvalidRequest = errors.New("invalid request")
)
