package model

import "time"

// User - пользователь платформы MAX, идентифицируемый по max_user_id.
type User struct {
	ID        string    `json:"id" db:"id"`
	MaxUserID string    `json:"max_user_id" db:"max_user_id"`
	Name      string    `json:"name" db:"name"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}
