package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/b602op/gophkeeper/internal/crypto"
)

// newLoginCmd создаёт команду входа.
func newLoginCmd(a *app) *cobra.Command {
	var login string

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Войти и ввести мастер-пароль",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.runLogin(cmd.Context(), login)
		},
	}
	cmd.Flags().StringVar(&login, "login", "", "логин пользователя")
	return cmd
}

// runLogin аутентифицирует пользователя, сохраняет токен и кэширует мастер-ключ.
//
// Соль KDF восстанавливается из локальных или серверных записей, что позволяет
// войти с нового устройства и получить тот же ключ шифрования.
func (a *app) runLogin(ctx context.Context, login string) error {
	login = strings.TrimSpace(login)
	if login == "" {
		return errors.New("логин обязателен: укажите флаг --login")
	}

	password, err := a.readPassword("Пароль: ")
	if err != nil {
		return err
	}
	if password == "" {
		return errors.New("пароль не может быть пустым")
	}

	cfg, err := a.loadConfig()
	if err != nil {
		return err
	}
	client := a.newClient(cfg.ServerAddress)

	token, err := client.Login(ctx, login, password)
	if err != nil {
		return fmt.Errorf("вход: %w", err)
	}
	client.SetToken(token)

	s, err := a.newSession()
	if err != nil {
		return err
	}
	if setErr := s.SetToken(token); setErr != nil {
		return setErr
	}

	store, err := a.storeForSession(s)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	salt, err := a.resolveSalt(ctx, s, store, client)
	if err != nil {
		return err
	}

	master, err := a.readPassword("Мастер-пароль: ")
	if err != nil {
		return err
	}
	if master == "" {
		return errors.New("мастер-пароль не может быть пустым")
	}

	s.SetSalt(salt)
	s.SetMasterKey(crypto.DeriveKey(master, salt))

	a.print("Вход выполнен: %s\n", login)
	return nil
}
