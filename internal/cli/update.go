package cli

import (
	"context"
	"errors"
	"strings"

	"github.com/spf13/cobra"

	"github.com/b602op/gophkeeper/internal/api"
	"github.com/b602op/gophkeeper/internal/crypto"
	"github.com/b602op/gophkeeper/internal/domain"
)

// newUpdateCmd создаёт команду обновления секрета.
func newUpdateCmd(a *app) *cobra.Command {
	data := &secretData{}

	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Обновить секрет",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runUpdate(cmd.Context(), args[0], data)
		},
	}
	// Пустой тип по умолчанию означает «сохранить тип существующей записи».
	data.register(cmd, "")
	return cmd
}

// runUpdate перешифровывает данные и обновляет запись на сервере.
//
// При конфликте версий (409) пользователю предлагается выбрать сторону.
func (a *app) runUpdate(ctx context.Context, id string, data *secretData) error {
	client, s, err := a.authenticatedClient()
	if err != nil {
		return err
	}
	store, err := a.storeForSession(s)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	existing, err := store.Get(id)
	if err != nil {
		if !errors.Is(err, domain.ErrSecretNotFound) {
			return err
		}
		existing, err = client.GetSecret(ctx, id)
		if err != nil {
			return err
		}
	}

	key, err := a.ensureMasterKey(ctx, s, store, client)
	if err != nil {
		return err
	}
	salt, err := s.GetSalt()
	if err != nil {
		return err
	}

	if strings.TrimSpace(data.secretType) == "" {
		data.secretType = string(existing.Type)
	}
	secretType, payload, err := a.buildPayload(data)
	if err != nil {
		return err
	}

	encrypted, err := crypto.Encrypt(key, payload)
	if err != nil {
		return err
	}
	stored, err := crypto.PackPayload(salt, encrypted)
	if err != nil {
		return err
	}

	updated := *existing
	updated.Type = secretType
	updated.Data = stored
	if data.metadata != "" {
		updated.Metadata = data.metadata
	}

	result, err := client.UpdateSecret(ctx, &updated)
	if err != nil {
		if errors.Is(err, api.ErrConflict) {
			return a.resolveConflict(ctx, client, store, &updated)
		}
		return err
	}

	if err := store.Save(result); err != nil {
		return err
	}
	a.print("Секрет обновлён: %s\n", result.ID)
	return nil
}

// resolveConflict предлагает пользователю выбрать локальную или серверную
// версию записи и применяет выбор.
func (a *app) resolveConflict(ctx context.Context, client apiClient, store secretStore, local *domain.Secret) error {
	a.print("Конфликт: %s\n", local.Name)
	a.print("  Локальная версия: %d (%s)\n", local.Version, local.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"))

	remote, err := client.GetSecret(ctx, local.ID)
	if err != nil {
		return err
	}
	a.print("  Серверная версия: %d (%s)\n", remote.Version, remote.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"))

	choice, err := a.readLine("Выберите: [l]ocal / [r]emote / [s]kip: ")
	if err != nil {
		return err
	}

	switch strings.ToLower(strings.TrimSpace(choice)) {
	case "l", "local":
		forced := *local
		forced.Version = remote.Version
		result, err := client.UpdateSecret(ctx, &forced)
		if err != nil {
			return err
		}
		return store.Save(result)
	case "r", "remote":
		return store.Save(remote)
	case "s", "skip", "":
		a.print("Конфликт пропущен.\n")
		return nil
	default:
		return errors.New("неизвестный вариант разрешения конфликта")
	}
}
