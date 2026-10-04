// Package service содержит бизнес-логику GophKeeper.
//
// Сервисы образуют фасад, который используют все транспорты (HTTP, gRPC, CLI),
// поэтому логика не дублируется между ними. Сервисы зависят только от
// интерфейсов репозиториев и доменных моделей.
package service

import (
	"context"
	"time"

	"github.com/b602op/gophkeeper/internal/domain"
)

// UserRepository описывает доступ к пользователям, необходимый сервисам.
//
// Интерфейс объявлен на стороне потребителя, чтобы сервис не зависел от
// конкретной реализации хранилища и легко подменялся в тестах.
type UserRepository interface {
	// Create сохраняет нового пользователя.
	Create(ctx context.Context, user *domain.User) error
	// GetByLogin возвращает пользователя по логину.
	GetByLogin(ctx context.Context, login string) (*domain.User, error)
	// GetByID возвращает пользователя по идентификатору.
	GetByID(ctx context.Context, id string) (*domain.User, error)
}

// SecretRepository описывает доступ к секретам, необходимый сервисам.
type SecretRepository interface {
	// Create сохраняет новый секрет.
	Create(ctx context.Context, secret *domain.Secret) error
	// FindByID возвращает неудалённый секрет пользователя по идентификатору.
	//
	// Чужой секрет неотличим от несуществующего: возвращается
	// domain.ErrSecretNotFound.
	FindByID(ctx context.Context, userID, id string) (*domain.Secret, error)
	// FindByUser возвращает неудалённые секреты пользователя.
	FindByUser(ctx context.Context, userID string) ([]*domain.Secret, error)
	// FindByUserSince возвращает секреты, изменённые после since, включая удалённые.
	FindByUserSince(ctx context.Context, userID string, since time.Time) ([]*domain.Secret, error)
	// Update обновляет секрет с проверкой версии.
	Update(ctx context.Context, secret *domain.Secret) error
	// SoftDelete помечает секрет удалённым.
	SoftDelete(ctx context.Context, id, userID string) error
}
