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

// newSecretRepo создаёт репозиторий секретов поверх sqlmock.
func newSecretRepo(t *testing.T) (*SecretRepository, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return NewSecretRepository(db), mock
}

// secretColumnsList возвращает имена колонок для строк sqlmock.
func secretColumnsList() []string {
	return []string{"id", "user_id", "type", "name", "metadata", "data", "version", "created_at", "updated_at", "deleted_at"}
}

// testSecret возвращает секрет с фиксированными полями.
func testSecret() *domain.Secret {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	return &domain.Secret{
		ID:        "secret-1",
		UserID:    "user-1",
		Type:      domain.SecretTypeCredentials,
		Name:      "Почта",
		Metadata:  "личное",
		Data:      []byte("ciphertext"),
		Version:   1,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// secretRow возвращает строку результата по секрету.
func secretRow(s *domain.Secret) *sqlmock.Rows {
	var deletedAt any
	if s.DeletedAt != nil {
		deletedAt = *s.DeletedAt
	}
	return sqlmock.NewRows(secretColumnsList()).AddRow(
		s.ID, s.UserID, s.Type, s.Name, s.Metadata, s.Data,
		s.Version, s.CreatedAt, s.UpdatedAt, deletedAt)
}

// updatedSecretRow возвращает строку результата UPDATE ... RETURNING
// (без deleted_at — он в RETURNING не возвращается).
func updatedSecretRow(s *domain.Secret, version int64, updatedAt time.Time) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "user_id", "type", "name", "metadata", "data", "version", "created_at", "updated_at",
	}).AddRow(s.ID, s.UserID, s.Type, s.Name, s.Metadata, s.Data, version, s.CreatedAt, updatedAt)
}

func TestSecretRepositoryCreate(t *testing.T) {
	t.Run("успех", func(t *testing.T) {
		repo, mock := newSecretRepo(t)
		s := testSecret()
		mock.ExpectExec(regexp.QuoteMeta(queryInsertSecret)).
			WithArgs(s.ID, s.UserID, s.Type, s.Name, s.Metadata, s.Data, s.Version, s.CreatedAt, s.UpdatedAt).
			WillReturnResult(sqlmock.NewResult(0, 1))

		require.NoError(t, repo.Create(context.Background(), s))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("дубликат", func(t *testing.T) {
		repo, mock := newSecretRepo(t)
		s := testSecret()
		mock.ExpectExec(regexp.QuoteMeta(queryInsertSecret)).
			WithArgs(s.ID, s.UserID, s.Type, s.Name, s.Metadata, s.Data, s.Version, s.CreatedAt, s.UpdatedAt).
			WillReturnError(&pgconn.PgError{Code: pgUniqueViolation})

		err := repo.Create(context.Background(), s)
		require.ErrorIs(t, err, domain.ErrSecretAlreadyExists)
	})

	t.Run("ошибка БД", func(t *testing.T) {
		repo, mock := newSecretRepo(t)
		s := testSecret()
		dbErr := errors.New("сбой")
		mock.ExpectExec(regexp.QuoteMeta(queryInsertSecret)).
			WithArgs(s.ID, s.UserID, s.Type, s.Name, s.Metadata, s.Data, s.Version, s.CreatedAt, s.UpdatedAt).
			WillReturnError(dbErr)

		err := repo.Create(context.Background(), s)
		require.ErrorIs(t, err, dbErr)
		require.NotErrorIs(t, err, domain.ErrSecretAlreadyExists)
	})
}

func TestSecretRepositoryFindByID(t *testing.T) {
	t.Run("успех", func(t *testing.T) {
		repo, mock := newSecretRepo(t)
		s := testSecret()
		mock.ExpectQuery(regexp.QuoteMeta(querySelectSecretByID)).
			WithArgs(s.ID, s.UserID).
			WillReturnRows(secretRow(s))

		got, err := repo.FindByID(context.Background(), s.UserID, s.ID)
		require.NoError(t, err)
		require.Equal(t, s, got)
		require.Nil(t, got.DeletedAt)
	})

	t.Run("удалённый секрет не возвращается", func(t *testing.T) {
		repo, mock := newSecretRepo(t)
		mock.ExpectQuery(regexp.QuoteMeta(querySelectSecretByID)).
			WithArgs("deleted", "user-1").
			WillReturnError(sql.ErrNoRows)

		got, err := repo.FindByID(context.Background(), "user-1", "deleted")
		require.ErrorIs(t, err, domain.ErrSecretNotFound)
		require.Nil(t, got)
	})

	t.Run("чужой секрет неотличим от несуществующего", func(t *testing.T) {
		// Запрос фильтрует по user_id, поэтому для чужого пользователя
		// sql.ErrNoRows маппится в domain.ErrSecretNotFound (HTTP 404).
		repo, mock := newSecretRepo(t)
		mock.ExpectQuery(regexp.QuoteMeta(querySelectSecretByID)).
			WithArgs("secret-1", "intruder").
			WillReturnError(sql.ErrNoRows)

		got, err := repo.FindByID(context.Background(), "intruder", "secret-1")
		require.ErrorIs(t, err, domain.ErrSecretNotFound)
		require.Nil(t, got)
	})

	t.Run("ошибка БД", func(t *testing.T) {
		repo, mock := newSecretRepo(t)
		dbErr := errors.New("таймаут")
		mock.ExpectQuery(regexp.QuoteMeta(querySelectSecretByID)).
			WithArgs("x", "user-1").
			WillReturnError(dbErr)

		_, err := repo.FindByID(context.Background(), "user-1", "x")
		require.ErrorIs(t, err, dbErr)
	})
}

func TestSecretRepositoryFindByUser(t *testing.T) {
	t.Run("пустой список", func(t *testing.T) {
		repo, mock := newSecretRepo(t)
		mock.ExpectQuery(regexp.QuoteMeta(querySelectSecretsByUser)).
			WithArgs("user-1").
			WillReturnRows(sqlmock.NewRows(secretColumnsList()))

		got, err := repo.FindByUser(context.Background(), "user-1")
		require.NoError(t, err)
		require.Empty(t, got)
		require.NotNil(t, got)
	})

	t.Run("несколько записей", func(t *testing.T) {
		repo, mock := newSecretRepo(t)
		first := testSecret()
		second := testSecret()
		second.ID = "secret-2"
		rows := sqlmock.NewRows(secretColumnsList())
		for _, s := range []*domain.Secret{first, second} {
			var deletedAt any
			if s.DeletedAt != nil {
				deletedAt = *s.DeletedAt
			}
			rows.AddRow(s.ID, s.UserID, s.Type, s.Name, s.Metadata, s.Data, s.Version, s.CreatedAt, s.UpdatedAt, deletedAt)
		}
		mock.ExpectQuery(regexp.QuoteMeta(querySelectSecretsByUser)).
			WithArgs("user-1").
			WillReturnRows(rows)

		got, err := repo.FindByUser(context.Background(), "user-1")
		require.NoError(t, err)
		require.Len(t, got, 2)
	})

	t.Run("ошибка БД", func(t *testing.T) {
		repo, mock := newSecretRepo(t)
		dbErr := errors.New("сбой")
		mock.ExpectQuery(regexp.QuoteMeta(querySelectSecretsByUser)).
			WithArgs("user-1").
			WillReturnError(dbErr)

		_, err := repo.FindByUser(context.Background(), "user-1")
		require.ErrorIs(t, err, dbErr)
	})

	t.Run("ошибка в строке результата", func(t *testing.T) {
		repo, mock := newSecretRepo(t)
		rows := sqlmock.NewRows(secretColumnsList()).
			AddRow("id", "user-1", "credentials", "name", "", []byte("x"), 1, time.Now(), time.Now(), "не-время")
		mock.ExpectQuery(regexp.QuoteMeta(querySelectSecretsByUser)).
			WithArgs("user-1").
			WillReturnRows(rows)

		_, err := repo.FindByUser(context.Background(), "user-1")
		require.Error(t, err)
	})
}

func TestSecretRepositoryFindByUserSince(t *testing.T) {
	t.Run("включая удалённые", func(t *testing.T) {
		repo, mock := newSecretRepo(t)
		since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		deleted := testSecret()
		deleted.ID = "secret-deleted"
		deletedAt := since.Add(time.Hour)
		deleted.DeletedAt = &deletedAt

		mock.ExpectQuery(regexp.QuoteMeta(querySelectSecretsByUserSince)).
			WithArgs("user-1", since).
			WillReturnRows(secretRow(deleted))

		got, err := repo.FindByUserSince(context.Background(), "user-1", since)
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.NotNil(t, got[0].DeletedAt)
	})

	t.Run("ошибка БД", func(t *testing.T) {
		repo, mock := newSecretRepo(t)
		since := time.Now()
		dbErr := errors.New("сбой")
		mock.ExpectQuery(regexp.QuoteMeta(querySelectSecretsByUserSince)).
			WithArgs("user-1", since).
			WillReturnError(dbErr)

		_, err := repo.FindByUserSince(context.Background(), "user-1", since)
		require.ErrorIs(t, err, dbErr)
	})
}

func TestSecretRepositoryUpdate(t *testing.T) {
	t.Run("успех", func(t *testing.T) {
		repo, mock := newSecretRepo(t)
		s := testSecret()
		updatedAt := time.Now().UTC()
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(querySelectSecretVersion)).
			WithArgs(s.ID, s.UserID).
			WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(int64(1)))
		mock.ExpectQuery(regexp.QuoteMeta(queryUpdateSecret)).
			WithArgs(s.Type, s.Name, s.Metadata, s.Data, s.ID, s.UserID, s.Version).
			WillReturnRows(updatedSecretRow(s, 2, updatedAt))
		mock.ExpectCommit()

		require.NoError(t, repo.Update(context.Background(), s))
		require.Equal(t, int64(2), s.Version)
		require.WithinDuration(t, updatedAt, s.UpdatedAt, time.Second)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("конфликт версий", func(t *testing.T) {
		repo, mock := newSecretRepo(t)
		s := testSecret()
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(querySelectSecretVersion)).
			WithArgs(s.ID, s.UserID).
			WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(int64(5)))
		mock.ExpectRollback()

		err := repo.Update(context.Background(), s)
		require.ErrorIs(t, err, domain.ErrSecretVersionMismatch)
	})

	t.Run("изменено параллельно", func(t *testing.T) {
		repo, mock := newSecretRepo(t)
		s := testSecret()
		mock.ExpectBegin()
		// Fast-path проходит: прочитанная версия совпадает с версией секрета.
		mock.ExpectQuery(regexp.QuoteMeta(querySelectSecretVersion)).
			WithArgs(s.ID, s.UserID).
			WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(s.Version))
		// Но UPDATE не затрагивает строк: версия изменилась между SELECT и UPDATE.
		mock.ExpectQuery(regexp.QuoteMeta(queryUpdateSecret)).
			WithArgs(s.Type, s.Name, s.Metadata, s.Data, s.ID, s.UserID, s.Version).
			WillReturnRows(sqlmock.NewRows([]string{
				"id", "user_id", "type", "name", "metadata", "data", "version", "created_at", "updated_at",
			}))
		mock.ExpectRollback()

		err := repo.Update(context.Background(), s)
		require.ErrorIs(t, err, domain.ErrSecretVersionMismatch)
		// Версия не должна «уехать» вперёд при неуспешном обновлении.
		require.Equal(t, int64(1), s.Version)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("секрет не найден", func(t *testing.T) {
		repo, mock := newSecretRepo(t)
		s := testSecret()
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(querySelectSecretVersion)).
			WithArgs(s.ID, s.UserID).
			WillReturnError(sql.ErrNoRows)
		mock.ExpectRollback()

		err := repo.Update(context.Background(), s)
		require.ErrorIs(t, err, domain.ErrSecretNotFound)
	})

	t.Run("ошибка начала транзакции", func(t *testing.T) {
		repo, mock := newSecretRepo(t)
		s := testSecret()
		dbErr := errors.New("нет соединения")
		mock.ExpectBegin().WillReturnError(dbErr)

		err := repo.Update(context.Background(), s)
		require.ErrorIs(t, err, dbErr)
	})

	t.Run("ошибка обновления", func(t *testing.T) {
		repo, mock := newSecretRepo(t)
		s := testSecret()
		dbErr := errors.New("сбой записи")
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(querySelectSecretVersion)).
			WithArgs(s.ID, s.UserID).
			WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(int64(1)))
		mock.ExpectQuery(regexp.QuoteMeta(queryUpdateSecret)).
			WithArgs(s.Type, s.Name, s.Metadata, s.Data, s.ID, s.UserID, s.Version).
			WillReturnError(dbErr)
		mock.ExpectRollback()

		err := repo.Update(context.Background(), s)
		require.ErrorIs(t, err, dbErr)
	})

	t.Run("ошибка фиксации", func(t *testing.T) {
		repo, mock := newSecretRepo(t)
		s := testSecret()
		dbErr := errors.New("сбой коммита")
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(querySelectSecretVersion)).
			WithArgs(s.ID, s.UserID).
			WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(int64(1)))
		mock.ExpectQuery(regexp.QuoteMeta(queryUpdateSecret)).
			WithArgs(s.Type, s.Name, s.Metadata, s.Data, s.ID, s.UserID, s.Version).
			WillReturnRows(updatedSecretRow(s, 2, time.Now()))
		mock.ExpectCommit().WillReturnError(dbErr)

		err := repo.Update(context.Background(), s)
		require.ErrorIs(t, err, dbErr)
	})
}

func TestSecretRepositorySoftDelete(t *testing.T) {
	t.Run("успех", func(t *testing.T) {
		repo, mock := newSecretRepo(t)
		mock.ExpectExec(regexp.QuoteMeta(querySoftDeleteSecret)).
			WithArgs("secret-1", "user-1").
			WillReturnResult(sqlmock.NewResult(0, 1))

		require.NoError(t, repo.SoftDelete(context.Background(), "secret-1", "user-1"))
	})

	t.Run("не найдено", func(t *testing.T) {
		repo, mock := newSecretRepo(t)
		mock.ExpectExec(regexp.QuoteMeta(querySoftDeleteSecret)).
			WithArgs("absent", "user-1").
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := repo.SoftDelete(context.Background(), "absent", "user-1")
		require.ErrorIs(t, err, domain.ErrSecretNotFound)
	})

	t.Run("ошибка БД", func(t *testing.T) {
		repo, mock := newSecretRepo(t)
		dbErr := errors.New("сбой")
		mock.ExpectExec(regexp.QuoteMeta(querySoftDeleteSecret)).
			WithArgs("secret-1", "user-1").
			WillReturnError(dbErr)

		err := repo.SoftDelete(context.Background(), "secret-1", "user-1")
		require.ErrorIs(t, err, dbErr)
	})

	t.Run("ошибка получения RowsAffected", func(t *testing.T) {
		repo, mock := newSecretRepo(t)
		mock.ExpectExec(regexp.QuoteMeta(querySoftDeleteSecret)).
			WithArgs("secret-1", "user-1").
			WillReturnResult(sqlmock.NewErrorResult(errors.New("сбой результата")))

		err := repo.SoftDelete(context.Background(), "secret-1", "user-1")
		require.Error(t, err)
	})
}
