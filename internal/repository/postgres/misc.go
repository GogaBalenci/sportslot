package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"sportslot/internal/model"
)

// EnsureUser создаёт пользователя MAX при первом обращении и обновляет имя.
func (s *Store) EnsureUser(ctx context.Context, maxUserID, name string) (*model.User, error) {
	var u model.User
	err := s.pool.QueryRow(ctx, `
		INSERT INTO users (max_user_id, name) VALUES ($1, $2)
		ON CONFLICT (max_user_id) DO UPDATE SET name = COALESCE(NULLIF(EXCLUDED.name, ''), users.name)
		RETURNING id, max_user_id, COALESCE(name, ''), created_at`, maxUserID, name).
		Scan(&u.ID, &u.MaxUserID, &u.Name, &u.CreatedAt)
	return &u, wrap("ensure user", err)
}

// EnsureUserWithID нужен для тестовых учётных записей с фиксированным id.
func (s *Store) EnsureUserWithID(ctx context.Context, id, maxUserID, name string) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO users (id, max_user_id, name) VALUES ($1, $2, $3)
		ON CONFLICT DO NOTHING`, id, maxUserID, name)
	return wrap("ensure test user", err)
}

func (s *Store) GetUser(ctx context.Context, id string) (*model.User, error) {
	var u model.User
	err := s.pool.QueryRow(ctx, `SELECT id, max_user_id, COALESCE(name, ''), created_at FROM users WHERE id = $1`, id).
		Scan(&u.ID, &u.MaxUserID, &u.Name, &u.CreatedAt)
	return &u, wrap("get user", notFound(err))
}

func (s *Store) LoadDialog(ctx context.Context, maxUserID string, dst interface{}) (bool, error) {
	var raw []byte
	err := s.pool.QueryRow(ctx, `SELECT state FROM dialog_states WHERE max_user_id = $1`, maxUserID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, wrap("load dialog", err)
	}
	return true, json.Unmarshal(raw, dst)
}

func (s *Store) SaveDialog(ctx context.Context, maxUserID string, state interface{}) error {
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO dialog_states (max_user_id, state, updated_at) VALUES ($1, $2, now())
		ON CONFLICT (max_user_id) DO UPDATE SET state = EXCLUDED.state, updated_at = now()`, maxUserID, raw)
	return wrap("save dialog", err)
}

func (s *Store) CreatePartnerSession(ctx context.Context, token string, ttl time.Duration) (time.Time, error) {
	expires := time.Now().Add(ttl)
	_, err := s.pool.Exec(ctx, `INSERT INTO partner_sessions (token, expires_at) VALUES ($1, $2)`, token, expires)
	return expires, wrap("create partner session", err)
}

func (s *Store) PartnerSessionValid(ctx context.Context, token string) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM partner_sessions WHERE token = $1 AND expires_at > now())`, token).Scan(&ok)
	return ok, wrap("check partner session", err)
}

func (s *Store) LogEvent(ctx context.Context, maxUserID, kind string, payload map[string]interface{}) error {
	if payload == nil {
		payload = map[string]interface{}{}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO events (max_user_id, kind, payload) VALUES ($1, $2, $3)`, maxUserID, kind, raw)
	return wrap("log event", err)
}

// FunnelStats — число уникальных пользователей по шагам воронки с момента since.
func (s *Store) FunnelStats(ctx context.Context, since time.Time) (map[string]int, error) {
	rows, err := s.pool.Query(ctx, `SELECT kind, count(DISTINCT max_user_id) FROM events
		WHERE created_at >= $1 GROUP BY kind`, since)
	if err != nil {
		return nil, wrap("funnel stats", err)
	}
	defer rows.Close()
	stats := map[string]int{}
	for rows.Next() {
		var kind string
		var n int
		if err := rows.Scan(&kind, &n); err != nil {
			return nil, err
		}
		stats[kind] = n
	}
	return stats, rows.Err()
}
