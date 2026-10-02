package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/b602op/gophkeeper/internal/domain"
)

// SQL-запросы репозитория пользователей вынесены в константы, чтобы тесты
// сравнивали их без риска рассинхронизации.
const (
	queryInsertUser = `INSERT INTO users (id, login, password_hash, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)`
	querySelectUserByLogin = `SELECT id, login, password_hash, created_at, updated_at
		FROM users WHERE login = $1`
	querySelectUserByID = `SELECT id, login, password_hash, created_at, updated_at
		FROM users WHERE id = $1`
)

// UserRepository предоставляет доступ к пользователям в PostgreSQL.
type UserRepository struct {
	db DBTX
}

// NewUserRepository создаёт репозиторий пользователей поверх переданного
// соединения или транзакции.
func NewUserRepository(db DBTX) *UserRepository {
	return &UserRepository{db: db}
}

// Create сохраняет нового пользователя.
//
// При нарушении уникальности логина возвращает domain.ErrUserAlreadyExists,
// чтобы вызывающий слой мог отдать клиенту понятный ответ.
func (r *UserRepository) Create(ctx context.Context, user *domain.User) error {
	_, err := r.db.ExecContext(ctx, queryInsertUser,
		user.ID, user.Login, user.PasswordHash, user.CreatedAt, user.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("создание пользователя %q: %w", user.Login, domain.ErrUserAlreadyExists)
		}
		return fmt.Errorf("создание пользователя %q: %w", user.Login, err)
	}
	return nil
}

// GetByLogin возвращает пользователя по логину.
//
// Если пользователь не найден, возвращает domain.ErrUserNotFound.
func (r *UserRepository) GetByLogin(ctx context.Context, login string) (*domain.User, error) {
	return scanUser(r.db.QueryRowContext(ctx, querySelectUserByLogin, login), login)
}

// GetByID возвращает пользователя по идентификатору.
//
// Если пользователь не найден, возвращает domain.ErrUserNotFound.
func (r *UserRepository) GetByID(ctx context.Context, id string) (*domain.User, error) {
	return scanUser(r.db.QueryRowContext(ctx, querySelectUserByID, id), id)
}

// scanUser читает одну строку пользователя и переводит sql.ErrNoRows в
// доменную ошибку domain.ErrUserNotFound.
func scanUser(row *sql.Row, key string) (*domain.User, error) {
	var user domain.User
	if err := row.Scan(&user.ID, &user.Login, &user.PasswordHash, &user.CreatedAt, &user.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("пользователь %q: %w", key, domain.ErrUserNotFound)
		}
		return nil, fmt.Errorf("чтение пользователя %q: %w", key, err)
	}
	return &user, nil
}

// isUniqueViolation проверяет, что ошибка — нарушение уникальности PostgreSQL.
//
// Используется errors.As, чтобы корректно работать с обёрнутыми ошибками и не
// зависеть от текста сообщения.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation
}
