// Package service содержит бизнес-логику GophKeeper.
//
// Сервисы образуют фасад, который используют все транспорты (HTTP, gRPC, CLI),
// поэтому логика не дублируется между ними. Сервисы зависят только от
// интерфейсов репозиториев и доменных моделей.
package service

import (
	"context"

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
