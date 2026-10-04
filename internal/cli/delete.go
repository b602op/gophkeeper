package cli

import (
	"context"
	"errors"

	"github.com/spf13/cobra"

	"github.com/b602op/gophkeeper/internal/api"
	"github.com/b602op/gophkeeper/internal/domain"
)

// newDeleteCmd создаёт команду удаления секрета.
func newDeleteCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Удалить секрет",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runDelete(cmd.Context(), args[0])
		},
	}
}

// runDelete помечает секрет удалённым на сервере и удаляет локальную копию.
func (a *app) runDelete(ctx context.Context, id string) error {
	client, s, err := a.authenticatedClient()
	if err != nil {
		return err
	}
	store, err := a.storeForSession(s)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	if err := client.DeleteSecret(ctx, id); err != nil && !errors.Is(err, api.ErrNotFound) {
		return err
	}

	if err := store.Delete(id); err != nil && !errors.Is(err, domain.ErrSecretNotFound) {
		return err
	}

	a.print("Секрет удалён: %s\n", id)
	return nil
}
