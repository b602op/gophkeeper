package cli

import (
	"context"
	"errors"
	"time"

	"github.com/spf13/cobra"

	"github.com/b602op/gophkeeper/internal/api"
	"github.com/b602op/gophkeeper/internal/domain"
)

// newSyncCmd создаёт команду синхронизации.
func newSyncCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Синхронизировать данные с сервером",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.runSync(cmd.Context())
		},
	}
}

// runSync загружает серверные изменения и отправляет локальные.
//
// Серверные записи применяются к локальной базе; удалённые на сервере записи
// удаляются и локально. Локальные записи, изменённые после последней
// синхронизации, отправляются на сервер, а конфликты версий разрешаются
// интерактивно.
func (a *app) runSync(ctx context.Context) error {
	client, s, err := a.authenticatedClient()
	if err != nil {
		return err
	}
	store, err := a.storeForSession(s)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	lastSync, err := store.GetLastSync()
	if err != nil {
		return err
	}

	remote, err := client.Sync(ctx, lastSync)
	if err != nil {
		return err
	}

	remoteIDs := make(map[string]bool, len(remote))
	for _, secret := range remote {
		remoteIDs[secret.ID] = true

		if secret.DeletedAt != nil {
			if err := store.Delete(secret.ID); err != nil && !errors.Is(err, domain.ErrSecretNotFound) {
				return err
			}
			continue
		}
		if err := store.Save(secret); err != nil {
			return err
		}
	}

	if !lastSync.IsZero() {
		if err := a.pushLocalChanges(ctx, client, store, lastSync, remoteIDs); err != nil {
			return err
		}
	}

	if err := store.SetLastSync(a.now().UTC()); err != nil {
		return err
	}

	a.print("Синхронизация завершена: получено записей — %d.\n", len(remote))
	return nil
}

// pushLocalChanges отправляет на сервер записи, изменённые после lastSync.
func (a *app) pushLocalChanges(ctx context.Context, client apiClient, store secretStore, lastSync time.Time, remoteIDs map[string]bool) error {
	local, err := store.List()
	if err != nil {
		return err
	}

	for _, secret := range local {
		if remoteIDs[secret.ID] || !secret.UpdatedAt.After(lastSync) {
			continue
		}

		if _, err := client.UpdateSecret(ctx, secret); err != nil {
			if errors.Is(err, api.ErrConflict) {
				if conflictErr := a.resolveConflict(ctx, client, store, secret); conflictErr != nil {
					return conflictErr
				}
				continue
			}
			return err
		}
	}
	return nil
}
