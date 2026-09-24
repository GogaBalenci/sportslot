package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"sportslot/internal/model"
	"sportslot/internal/repository"
)

type UserRepo struct {
	pool *pgxpool.Pool
}

func NewUserRepo(pool *pgxpool.Pool) *UserRepo {
	return &UserRepo{pool: pool}
}

var _ repository.UserRepository = (*UserRepo)(nil)

// GetOrCreateByMaxUserID резолвит пользователя MAX в локальную сущность User.
// Используется middleware'ом авторизации по X-MAX-User-ID: при первом
// обращении пользователь создаётся автоматически (upsert по max_user_id).
func (r *UserRepo) GetOrCreateByMaxUserID(ctx context.Context, maxUserID string, name string) (*model.User, error) {
	const selectQ = `
		SELECT id, max_user_id, name, created_at FROM users WHERE max_user_id = $1`

	row := r.pool.QueryRow(ctx, selectQ, maxUserID)
	var u model.User
	err := row.Scan(&u.ID, &u.MaxUserID, &u.Name, &u.CreatedAt)
	if err == nil {
		return &u, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("get user by max_user_id: %w", err)
	}

	newID := uuid.NewString()
	const insertQ = `
		INSERT INTO users (id, max_user_id, name)
		VALUES ($1, $2, $3)
		ON CONFLICT (max_user_id) DO UPDATE SET max_user_id = EXCLUDED.max_user_id
		RETURNING id, max_user_id, name, created_at`

	row = r.pool.QueryRow(ctx, insertQ, newID, maxUserID, name)
	if err := row.Scan(&u.ID, &u.MaxUserID, &u.Name, &u.CreatedAt); err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	return &u, nil
}

func (r *UserRepo) GetByID(ctx context.Context, id string) (*model.User, error) {
	const q = `SELECT id, max_user_id, name, created_at FROM users WHERE id = $1`
	row := r.pool.QueryRow(ctx, q, id)
	var u model.User
	err := row.Scan(&u.ID, &u.MaxUserID, &u.Name, &u.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return &u, nil
}
