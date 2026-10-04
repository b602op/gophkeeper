package cli

import (
	"context"
	"errors"
	"strings"

	"github.com/spf13/cobra"

	"github.com/b602op/gophkeeper/internal/crypto"
	"github.com/b602op/gophkeeper/internal/domain"
)

// newAddCmd создаёт команду добавления секрета.
func newAddCmd(a *app) *cobra.Command {
	data := &secretData{}

	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Добавить секрет",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runAdd(cmd.Context(), args[0], data)
		},
	}
	data.register(cmd, string(domain.SecretTypeCredentials))
	return cmd
}

// runAdd шифрует данные мастер-ключом и создаёт запись на сервере.
//
// Сначала запись создаётся на сервере (он присваивает идентификатор и версию),
// затем сохраняется в локальную базу.
func (a *app) runAdd(ctx context.Context, name string, data *secretData) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("название записи не может быть пустым")
	}

	client, s, err := a.authenticatedClient()
	if err != nil {
		return err
	}
	store, err := a.storeForSession(s)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	key, err := a.ensureMasterKey(ctx, s, store, client)
	if err != nil {
		return err
	}
	salt, err := s.GetSalt()
	if err != nil {
		return err
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

	created, err := client.CreateSecret(ctx, &domain.Secret{
		Type:     secretType,
		Name:     name,
		Metadata: data.metadata,
		Data:     stored,
		Version:  1,
	})
	if err != nil {
		return err
	}

	if err := store.Save(created); err != nil {
		return err
	}

	a.print("Секрет добавлен: %s\n", created.ID)
	return nil
}
