package repository

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	"github.com/b602op/gophkeeper/internal/domain"
)

// newUserRepo создаёт репозиторий поверх sqlmock и регистрирует очистку.
func newUserRepo(t *testing.T) (*UserRepository, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return NewUserRepository(db), mock
}

// testUser возвращает пользователя с фиксированными полями.
func testUser() *domain.User {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	return &domain.User{
		ID:           "11111111-1111-1111-1111-111111111111",
		Login:        "alice",
		PasswordHash: "$2a$12$hash",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

func TestUserRepositoryCreate(t *testing.T) {
	t.Run("успех", func(t *testing.T) {
		repo, mock := newUserRepo(t)
		user := testUser()
		mock.ExpectExec(regexp.QuoteMeta(queryInsertUser)).
			WithArgs(user.ID, user.Login, user.PasswordHash, user.CreatedAt, user.UpdatedAt).
			WillReturnResult(sqlmock.NewResult(0, 1))

		require.NoError(t, repo.Create(context.Background(), user))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("дубликат логина", func(t *testing.T) {
		repo, mock := newUserRepo(t)
		user := testUser()
		mock.ExpectExec(regexp.QuoteMeta(queryInsertUser)).
			WithArgs(user.ID, user.Login, user.PasswordHash, user.CreatedAt, user.UpdatedAt).
			WillReturnError(&pgconn.PgError{Code: pgUniqueViolation})

		err := repo.Create(context.Background(), user)
		require.ErrorIs(t, err, domain.ErrUserAlreadyExists)
	})

	t.Run("прочая ошибка БД", func(t *testing.T) {
		repo, mock := newUserRepo(t)
		user := testUser()
		dbErr := errors.New("сбой соединения")
		mock.ExpectExec(regexp.QuoteMeta(queryInsertUser)).
			WithArgs(user.ID, user.Login, user.PasswordHash, user.CreatedAt, user.UpdatedAt).
			WillReturnError(dbErr)

		err := repo.Create(context.Background(), user)
		require.ErrorIs(t, err, dbErr)
		require.NotErrorIs(t, err, domain.ErrUserAlreadyExists)
	})
}

func TestUserRepositoryGetByLogin(t *testing.T) {
	t.Run("успех", func(t *testing.T) {
		repo, mock := newUserRepo(t)
		user := testUser()
		rows := sqlmock.NewRows([]string{"id", "login", "password_hash", "created_at", "updated_at"}).
			AddRow(user.ID, user.Login, user.PasswordHash, user.CreatedAt, user.UpdatedAt)
		mock.ExpectQuery(regexp.QuoteMeta(querySelectUserByLogin)).
			WithArgs(user.Login).
			WillReturnRows(rows)

		got, err := repo.GetByLogin(context.Background(), user.Login)
		require.NoError(t, err)
		require.Equal(t, user, got)
	})

	t.Run("не найдено", func(t *testing.T) {
		repo, mock := newUserRepo(t)
		mock.ExpectQuery(regexp.QuoteMeta(querySelectUserByLogin)).
			WithArgs("bob").
			WillReturnError(sql.ErrNoRows)

		got, err := repo.GetByLogin(context.Background(), "bob")
		require.ErrorIs(t, err, domain.ErrUserNotFound)
		require.Nil(t, got)
	})

	t.Run("ошибка БД", func(t *testing.T) {
		repo, mock := newUserRepo(t)
		dbErr := errors.New("таймаут")
		mock.ExpectQuery(regexp.QuoteMeta(querySelectUserByLogin)).
			WithArgs("bob").
			WillReturnError(dbErr)

		_, err := repo.GetByLogin(context.Background(), "bob")
		require.ErrorIs(t, err, dbErr)
	})
}

func TestUserRepositoryGetByID(t *testing.T) {
	t.Run("успех", func(t *testing.T) {
		repo, mock := newUserRepo(t)
		user := testUser()
		rows := sqlmock.NewRows([]string{"id", "login", "password_hash", "created_at", "updated_at"}).
			AddRow(user.ID, user.Login, user.PasswordHash, user.CreatedAt, user.UpdatedAt)
		mock.ExpectQuery(regexp.QuoteMeta(querySelectUserByID)).
			WithArgs(user.ID).
			WillReturnRows(rows)

		got, err := repo.GetByID(context.Background(), user.ID)
		require.NoError(t, err)
		require.Equal(t, user, got)
	})

	t.Run("не найдено", func(t *testing.T) {
		repo, mock := newUserRepo(t)
		mock.ExpectQuery(regexp.QuoteMeta(querySelectUserByID)).
			WithArgs("absent").
			WillReturnError(sql.ErrNoRows)

		got, err := repo.GetByID(context.Background(), "absent")
		require.ErrorIs(t, err, domain.ErrUserNotFound)
		require.Nil(t, got)
	})
}
