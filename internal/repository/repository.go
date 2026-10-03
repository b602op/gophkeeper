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

// DBTXWithTx расширяет DBTX возможностью начинать транзакции.
//
// Интерфейс реализуют *sql.DB и *sql.Tx, что позволяет репозиториям выполнять
// атомарные операции с оптимистичной блокировкой.
type DBTXWithTx interface {
	DBTX
	// BeginTx начинает транзакцию.
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}

// pgUniqueViolation — код ошибки PostgreSQL при нарушении уникальности.
const pgUniqueViolation = "23505"
