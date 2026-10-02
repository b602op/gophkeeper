// Package repository реализует доступ к PostgreSQL для GophKeeper.
//
// Репозитории работают через интерфейс DBTX, который реализуют *sql.DB и
// *sql.Tx. Это позволяет переиспользовать одни и те же запросы внутри
// транзакций и подменять хранилище в юнит-тестах через sqlmock, не поднимая
// реальную базу данных.
package repository

import (
	"context"
	"database/sql"
)

// DBTX описывает подмножество методов database/sql, необходимое репозиториям.
type DBTX interface {
	// ExecContext выполняет запрос без возврата строк.
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	// QueryContext выполняет запрос, возвращающий строки.
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	// QueryRowContext выполняет запрос, возвращающий одну строку.
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// pgUniqueViolation — код ошибки PostgreSQL при нарушении уникальности.
const pgUniqueViolation = "23505"
