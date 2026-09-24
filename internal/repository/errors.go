package repository

import "errors"

var (
	ErrNotFound      = errors.New("resource not found")
	ErrQuotaExceeded = errors.New("quota exceeded")
)
