package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	// Драйвер pgx регистрируется в database/sql под именем "pgx".
	_ "github.com/jackc/pgx/v5/stdlib"
)

// Параметры пула соединений.
const (
	maxOpenConns    = 25
	maxIdleConns    = 5
	connMaxLifetime = 30 * time.Minute
)

// Postgres — обёртка над пулом соединений database/sql с драйвером pgx.
type Postgres struct {
	db *sql.DB
}

// Open создаёт пул соединений с PostgreSQL и проверяет его доступность.
//
// Проверка соединения сразу при старте реализует принцип fail early: сервис не
// должен запускаться с недоступной базой.
func Open(ctx context.Context, dsn string) (*Postgres, error) {
	return openWith(ctx, sql.Open, "pgx", dsn)
}

// openWith — тестируемая реализация Open с внедряемой функцией открытия
// соединения. Отдельная функция позволяет проверить обработку ошибок без
// реальной базы данных.
func openWith(
	ctx context.Context,
	opener func(driverName, dsn string) (*sql.DB, error),
	driverName, dsn string,
) (*Postgres, error) {
	db, err := opener(driverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("открытие соединения с PostgreSQL: %w", err)
	}
	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxIdleConns)
	db.SetConnMaxLifetime(connMaxLifetime)

	if err := db.PingContext(ctx); err != nil {
		if closeErr := db.Close(); closeErr != nil {
			return nil, fmt.Errorf("проверка соединения с PostgreSQL: %w (закрытие пула: %v)", err, closeErr)
		}
		return nil, fmt.Errorf("проверка соединения с PostgreSQL: %w", err)
	}
	return &Postgres{db: db}, nil
}

// DB возвращает нижележащий пул для передачи в репозитории.
func (p *Postgres) DB() *sql.DB {
	return p.db
}

// Close закрывает пул соединений. Метод идемпотентен: повторный вызов или
// вызов на нулевом значении не приводит к ошибке.
func (p *Postgres) Close() error {
	if p == nil || p.db == nil {
		return nil
	}
	if err := p.db.Close(); err != nil {
		return fmt.Errorf("закрытие пула соединений: %w", err)
	}
	return nil
}
