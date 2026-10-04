package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/b602op/gophkeeper/internal/domain"
)

// Список колонок секрета в едином порядке для всех SELECT-запросов.
const secretColumns = `id, user_id, type, name, metadata, data, version, created_at, updated_at, deleted_at`

// SQL-запросы репозитория секретов вынесены в константы, чтобы тесты
// сравнивали их без риска рассинхронизации.
const (
	queryInsertSecret = `INSERT INTO secrets (id, user_id, type, name, metadata, data, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
	querySelectSecretByID = `SELECT ` + secretColumns + ` FROM secrets
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`
	querySelectSecretsByUser = `SELECT ` + secretColumns + ` FROM secrets
		WHERE user_id = $1 AND deleted_at IS NULL
		ORDER BY updated_at DESC`
	querySelectSecretsByUserSince = `SELECT ` + secretColumns + ` FROM secrets
		WHERE user_id = $1 AND updated_at > $2
		ORDER BY updated_at ASC`
	querySelectSecretVersion = `SELECT version FROM secrets
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`
	queryUpdateSecret = `UPDATE secrets
		SET type = $1, name = $2, metadata = $3, data = $4,
		    version = version + 1, updated_at = NOW()
		WHERE id = $5 AND user_id = $6 AND deleted_at IS NULL
		RETURNING id, user_id, type, name, metadata, data, version, created_at, updated_at`
	querySoftDeleteSecret = `UPDATE secrets
		SET deleted_at = NOW(), updated_at = NOW(), version = version + 1
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`
)

// SecretRepository предоставляет доступ к секретам в PostgreSQL.
type SecretRepository struct {
	db DBTXWithTx
}

// NewSecretRepository создаёт репозиторий секретов.
//
// Требуется DBTXWithTx, так как обновление секрета выполняется в транзакции.
func NewSecretRepository(db DBTXWithTx) *SecretRepository {
	return &SecretRepository{db: db}
}

// Create сохраняет новый секрет.
//
// При нарушении уникальности идентификатора возвращает
// domain.ErrSecretAlreadyExists.
func (r *SecretRepository) Create(ctx context.Context, secret *domain.Secret) error {
	_, err := r.db.ExecContext(ctx, queryInsertSecret,
		secret.ID, secret.UserID, secret.Type, secret.Name, secret.Metadata,
		secret.Data, secret.Version, secret.CreatedAt, secret.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("создание секрета %q: %w", secret.ID, domain.ErrSecretAlreadyExists)
		}
		return fmt.Errorf("создание секрета %q: %w", secret.ID, err)
	}
	return nil
}

// FindByID возвращает неудалённый секрет пользователя по идентификатору.
//
// Условие по user_id делает чужой секрет неотличимым от несуществующего: в обоих
// случаях возвращается domain.ErrSecretNotFound. Так сервер отвечает 404, а не
// 403, и не раскрывает факт существования чужой записи (защита от перебора).
func (r *SecretRepository) FindByID(ctx context.Context, userID, id string) (*domain.Secret, error) {
	return scanSecret(r.db.QueryRowContext(ctx, querySelectSecretByID, id, userID), id)
}

// FindByUser возвращает неудалённые секреты пользователя.
//
// Записи упорядочены по времени последнего изменения (сначала свежие).
func (r *SecretRepository) FindByUser(ctx context.Context, userID string) ([]*domain.Secret, error) {
	rows, err := r.db.QueryContext(ctx, querySelectSecretsByUser, userID)
	if err != nil {
		return nil, fmt.Errorf("выборка секретов пользователя %q: %w", userID, err)
	}
	defer func() { _ = rows.Close() }()

	return collectSecrets(rows, userID)
}

// FindByUserSince возвращает секреты, изменённые строго после since, включая
// удалённые: клиент должен узнать об удалении записей.
func (r *SecretRepository) FindByUserSince(ctx context.Context, userID string, since time.Time) ([]*domain.Secret, error) {
	rows, err := r.db.QueryContext(ctx, querySelectSecretsByUserSince, userID, since)
	if err != nil {
		return nil, fmt.Errorf("выборка изменённых секретов пользователя %q: %w", userID, err)
	}
	defer func() { _ = rows.Close() }()

	return collectSecrets(rows, userID)
}

// Update обновляет секрет с оптимистичной блокировкой по версии.
//
// В транзакции читается текущая версия; при несовпадении с secret.Version
// возвращается domain.ErrSecretVersionMismatch, при отсутствии записи —
// domain.ErrSecretNotFound. Условие по user_id защищает от изменения чужого
// секрета даже при ошибочной проверке на уровне сервиса.
//
// После успешного обновления в переданный secret записываются все поля из
// базы, включая created_at, version и updated_at: клиент должен получить
// актуальное состояние записи, иначе локальная копия разойдётся с сервером.
func (r *SecretRepository) Update(ctx context.Context, secret *domain.Secret) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("начало транзакции обновления секрета %q: %w", secret.ID, err)
	}
	// Откат после успешного Commit вернёт ошибку ErrTxDone, поэтому ошибку
	// отката намеренно игнорируем — она не несёт полезной информации.
	defer func() { _ = tx.Rollback() }()

	var currentVersion int64
	err = tx.QueryRowContext(ctx, querySelectSecretVersion, secret.ID, secret.UserID).Scan(&currentVersion)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("обновление секрета %q: %w", secret.ID, domain.ErrSecretNotFound)
		}
		return fmt.Errorf("чтение версии секрета %q: %w", secret.ID, err)
	}
	if currentVersion != secret.Version {
		return fmt.Errorf("обновление секрета %q: ожидалась версия %d, текущая %d: %w",
			secret.ID, secret.Version, currentVersion, domain.ErrSecretVersionMismatch)
	}

	row := tx.QueryRowContext(ctx, queryUpdateSecret,
		secret.Type, secret.Name, secret.Metadata, secret.Data, secret.ID, secret.UserID)
	if err := row.Scan(
		&secret.ID,
		&secret.UserID,
		&secret.Type,
		&secret.Name,
		&secret.Metadata,
		&secret.Data,
		&secret.Version,
		&secret.CreatedAt,
		&secret.UpdatedAt,
	); err != nil {
		return fmt.Errorf("обновление секрета %q: %w", secret.ID, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("фиксация обновления секрета %q: %w", secret.ID, err)
	}
	return nil
}

// SoftDelete помечает секрет удалённым, увеличивая версию и updated_at.
//
// Если запись не найдена, уже удалена или принадлежит другому пользователю,
// возвращает domain.ErrSecretNotFound.
func (r *SecretRepository) SoftDelete(ctx context.Context, id, userID string) error {
	result, err := r.db.ExecContext(ctx, querySoftDeleteSecret, id, userID)
	if err != nil {
		return fmt.Errorf("удаление секрета %q: %w", id, err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("проверка результата удаления секрета %q: %w", id, err)
	}
	if affected == 0 {
		return fmt.Errorf("удаление секрета %q: %w", id, domain.ErrSecretNotFound)
	}
	return nil
}

// scanSecret читает одну строку секрета и переводит sql.ErrNoRows в
// domain.ErrSecretNotFound.
func scanSecret(row rowScanner, id string) (*domain.Secret, error) {
	var (
		secret    domain.Secret
		deletedAt sql.NullTime
	)
	err := row.Scan(&secret.ID, &secret.UserID, &secret.Type, &secret.Name, &secret.Metadata,
		&secret.Data, &secret.Version, &secret.CreatedAt, &secret.UpdatedAt, &deletedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("секрет %q: %w", id, domain.ErrSecretNotFound)
		}
		return nil, fmt.Errorf("чтение секрета %q: %w", id, err)
	}
	if deletedAt.Valid {
		secret.DeletedAt = &deletedAt.Time
	}
	return &secret, nil
}

// collectSecrets вычитывает все строки результата и проверяет rows.Err.
func collectSecrets(rows *sql.Rows, userID string) ([]*domain.Secret, error) {
	secrets := make([]*domain.Secret, 0)
	for rows.Next() {
		secret, err := scanSecret(rows, userID)
		if err != nil {
			return nil, err
		}
		secrets = append(secrets, secret)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("чтение секретов пользователя %q: %w", userID, err)
	}
	return secrets, nil
}

// rowScanner абстрагирует *sql.Row и *sql.Rows, чтобы scanSecret был общим.
type rowScanner interface {
	Scan(dest ...any) error
}
