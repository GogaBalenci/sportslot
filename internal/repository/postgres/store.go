package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"sportslot/internal/repository"
)

// Store — доступ к данным СпортСлота. Все операции, меняющие квоту мест,
// выполняются в транзакциях с блокировкой строки занятия.
type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return repository.ErrNotFound
	}
	return err
}

func isUniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && (constraint == "" || pgErr.ConstraintName == constraint)
}

func wrap(op string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, repository.ErrNotFound) || errors.Is(err, repository.ErrQuotaExceeded) ||
		errors.Is(err, repository.ErrSlotStarted) || errors.Is(err, repository.ErrNotActive) ||
		errors.Is(err, repository.ErrAlreadyBooked) {
		return err
	}
	return fmt.Errorf("%s: %w", op, err)
}
