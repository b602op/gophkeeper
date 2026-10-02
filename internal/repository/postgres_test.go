package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestOpenWith(t *testing.T) {
	t.Run("успешное открытие", func(t *testing.T) {
		db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		mock.ExpectPing()
		mock.ExpectClose()

		opener := func(_, _ string) (*sql.DB, error) { return db, nil }
		pool, err := openWith(context.Background(), opener, "pgx", "dsn")
		require.NoError(t, err)
		require.NotNil(t, pool)
		require.NotNil(t, pool.DB())
		require.NoError(t, pool.Close())
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("ошибка открытия", func(t *testing.T) {
		wantErr := errors.New("драйвер недоступен")
		opener := func(_, _ string) (*sql.DB, error) { return nil, wantErr }

		pool, err := openWith(context.Background(), opener, "pgx", "dsn")
		require.ErrorIs(t, err, wantErr)
		require.Nil(t, pool)
	})

	t.Run("ошибка проверки соединения", func(t *testing.T) {
		db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		wantErr := errors.New("нет связи с БД")
		mock.ExpectPing().WillReturnError(wantErr)

		opener := func(_, _ string) (*sql.DB, error) { return db, nil }
		pool, err := openWith(context.Background(), opener, "pgx", "dsn")
		require.ErrorIs(t, err, wantErr)
		require.Nil(t, pool)
	})
}

func TestPostgresCloseIdempotent(t *testing.T) {
	var nilPool *Postgres
	require.NoError(t, nilPool.Close())
	require.NoError(t, (&Postgres{}).Close())
}
