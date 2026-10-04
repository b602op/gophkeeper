package cli

import (
	"context"
	"errors"

	"github.com/spf13/cobra"

	"github.com/b602op/gophkeeper/internal/crypto"
	"github.com/b602op/gophkeeper/internal/domain"
)

// newGetCmd создаёт команду получения секрета.
func newGetCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Получить и расшифровать секрет",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runGet(cmd.Context(), args[0])
		},
	}
}

// runGet загружает секрет, при необходимости из сервера, и расшифровывает его.
func (a *app) runGet(ctx context.Context, id string) error {
	client, s, err := a.authenticatedClient()
	if err != nil {
		return err
	}
	store, err := a.storeForSession(s)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	secret, err := store.Get(id)
	if err != nil {
		if !errors.Is(err, domain.ErrSecretNotFound) {
			return err
		}
		secret, err = client.GetSecret(ctx, id)
		if err != nil {
			return err
		}
		if saveErr := store.Save(secret); saveErr != nil {
			return saveErr
		}
	}

	key, err := a.ensureMasterKey(ctx, s, store, client)
	if err != nil {
		return err
	}

	_, encoded, err := crypto.UnpackPayload(secret.Data)
	if err != nil {
		return err
	}
	plaintext, err := crypto.Decrypt(key, encoded)
	if err != nil {
		return err
	}

	a.print("%s\n", formatSecret(secret, plaintext))
	return nil
}
