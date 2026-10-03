package service

import (
	"context"
	"fmt"
	"time"

	"github.com/b602op/gophkeeper/internal/domain"
	"github.com/b602op/gophkeeper/internal/idgen"
)

// Ограничения на размеры полей секрета.
const (
	// maxSecretSize — максимальный размер зашифрованных данных секрета (1 МиБ).
	maxSecretSize = 1 << 20
	// maxMetadataSize — максимальный размер метаинформации (64 КиБ).
	maxMetadataSize = 64 << 10
	// maxSecretNameLen — максимальная длина названия записи.
	maxSecretNameLen = 255
)

// SecretService реализует бизнес-логику работы с секретами.
type SecretService struct {
	repo  SecretRepository
	idgen idgen.Generator

	// now внедряется для детерминированных тестов.
	now func() time.Time
}

// NewSecretService создаёт сервис секретов.
func NewSecretService(repo SecretRepository, generator idgen.Generator) *SecretService {
	return &SecretService{
		repo:  repo,
		idgen: generator,
		now:   time.Now,
	}
}

// Create валидирует и создаёт секрет.
//
// Идентификатор и версия проставляются сервисом; переданные клиентом значения
// этих полей игнорируются. При невалидных данных возвращает
// domain.ErrInvalidSecretType или domain.ErrInvalidSecretData.
func (s *SecretService) Create(ctx context.Context, userID string, secret *domain.Secret) error {
	if err := validateSecret(secret); err != nil {
		return err
	}

	now := s.now()
	secret.ID = s.idgen.Generate()
	secret.UserID = userID
	secret.Version = 1
	secret.CreatedAt = now
	secret.UpdatedAt = now
	secret.DeletedAt = nil

	if err := s.repo.Create(ctx, secret); err != nil {
		return fmt.Errorf("создание секрета пользователя %q: %w", userID, err)
	}
	return nil
}

// Get возвращает секрет, если он принадлежит пользователю.
//
// Отсутствующий секрет и секрет чужого пользователя различимы: первый даёт
// domain.ErrSecretNotFound, второй — domain.ErrForbidden.
func (s *SecretService) Get(ctx context.Context, userID, id string) (*domain.Secret, error) {
	secret, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("получение секрета %q: %w", id, err)
	}
	if secret.UserID != userID {
		return nil, fmt.Errorf("получение секрета %q: %w", id, domain.ErrForbidden)
	}
	return secret, nil
}

// List возвращает все неудалённые секреты пользователя.
func (s *SecretService) List(ctx context.Context, userID string) ([]*domain.Secret, error) {
	secrets, err := s.repo.FindByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("список секретов пользователя %q: %w", userID, err)
	}
	return secrets, nil
}

// Update обновляет секрет с проверкой владельца и версии.
//
// Версия берётся из переданного секрета: несовпадение с хранилищем приводит к
// domain.ErrSecretVersionMismatch, что защищает от потери параллельных правок.
func (s *SecretService) Update(ctx context.Context, userID string, secret *domain.Secret) error {
	if err := validateSecret(secret); err != nil {
		return err
	}

	existing, err := s.repo.FindByID(ctx, secret.ID)
	if err != nil {
		return fmt.Errorf("обновление секрета %q: %w", secret.ID, err)
	}
	if existing.UserID != userID {
		return fmt.Errorf("обновление секрета %q: %w", secret.ID, domain.ErrForbidden)
	}

	secret.UserID = userID
	secret.UpdatedAt = s.now()
	if err := s.repo.Update(ctx, secret); err != nil {
		return fmt.Errorf("обновление секрета %q: %w", secret.ID, err)
	}
	return nil
}

// Delete помечает секрет удалённым, предварительно проверив владельца.
func (s *SecretService) Delete(ctx context.Context, userID, id string) error {
	existing, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return fmt.Errorf("удаление секрета %q: %w", id, err)
	}
	if existing.UserID != userID {
		return fmt.Errorf("удаление секрета %q: %w", id, domain.ErrForbidden)
	}

	if err := s.repo.SoftDelete(ctx, id, userID); err != nil {
		return fmt.Errorf("удаление секрета %q: %w", id, err)
	}
	return nil
}

// Sync возвращает секреты, изменённые строго после since, включая удалённые.
func (s *SecretService) Sync(ctx context.Context, userID string, since time.Time) ([]*domain.Secret, error) {
	secrets, err := s.repo.FindByUserSince(ctx, userID, since)
	if err != nil {
		return nil, fmt.Errorf("синхронизация секретов пользователя %q: %w", userID, err)
	}
	return secrets, nil
}

// validateSecret проверяет тип и размеры полей секрета.
func validateSecret(secret *domain.Secret) error {
	if secret == nil {
		return fmt.Errorf("%w: секрет не задан", domain.ErrInvalidSecretData)
	}
	if !secret.Type.IsValid() {
		return fmt.Errorf("%w: тип %q", domain.ErrInvalidSecretType, secret.Type)
	}
	if secret.Name == "" {
		return fmt.Errorf("%w: название записи не может быть пустым", domain.ErrInvalidSecretData)
	}
	if len(secret.Name) > maxSecretNameLen {
		return fmt.Errorf("%w: название записи длиннее %d символов", domain.ErrInvalidSecretData, maxSecretNameLen)
	}
	if len(secret.Data) == 0 {
		return fmt.Errorf("%w: данные секрета не могут быть пустыми", domain.ErrInvalidSecretData)
	}
	if len(secret.Data) > maxSecretSize {
		return fmt.Errorf("%w: данные секрета больше %d байт", domain.ErrInvalidSecretData, maxSecretSize)
	}
	if len(secret.Metadata) > maxMetadataSize {
		return fmt.Errorf("%w: метаинформация больше %d байт", domain.ErrInvalidSecretData, maxMetadataSize)
	}
	return nil
}
