package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// newRegisterCmd создаёт команду регистрации.
func newRegisterCmd(a *app) *cobra.Command {
	var login string

	cmd := &cobra.Command{
		Use:   "register",
		Short: "Зарегистрировать нового пользователя",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.runRegister(cmd.Context(), login)
		},
	}
	cmd.Flags().StringVar(&login, "login", "", "логин пользователя")
	return cmd
}

// runRegister регистрирует пользователя и сразу выполняет вход.
//
// Сервер не выдаёт токен при регистрации, поэтому после создания учётной записи
// клиент входит тем же паролем и сохраняет полученный JWT.
func (a *app) runRegister(ctx context.Context, login string) error {
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

	if _, regErr := client.Register(ctx, login, password); regErr != nil {
		return fmt.Errorf("регистрация: %w", regErr)
	}

	token, err := client.Login(ctx, login, password)
	if err != nil {
		return fmt.Errorf("вход после регистрации: %w", err)
	}

	s, err := a.newSession()
	if err != nil {
		return err
	}
	if err := s.SetToken(token); err != nil {
		return err
	}

	a.print("Пользователь %q зарегистрирован.\n", login)
	return nil
}
