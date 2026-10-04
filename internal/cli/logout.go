package cli

import (
	"github.com/spf13/cobra"
)

// newLogoutCmd создаёт команду выхода.
func newLogoutCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Выйти и очистить токен и мастер-ключ",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			s, err := a.newSession()
			if err != nil {
				return err
			}
			if err := s.Clear(); err != nil {
				return err
			}
			a.print("Выход выполнен.\n")
			return nil
		},
	}
}
