package cli

import (
	"time"

	"github.com/spf13/cobra"
)

// newListCmd создаёт команду вывода списка секретов.
func newListCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Показать список секретов",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return a.runList()
		},
	}
}

// runList печатает локальные секреты пользователя.
func (a *app) runList() error {
	_, s, err := a.authenticatedClient()
	if err != nil {
		return err
	}
	store, err := a.storeForSession(s)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	secrets, err := store.List()
	if err != nil {
		return err
	}
	if len(secrets) == 0 {
		a.print("Секретов нет. Выполните sync для загрузки с сервера.\n")
		return nil
	}

	a.print("%-36s  %-12s  %-20s  %s\n", "ID", "ТИП", "ОБНОВЛЁН", "НАЗВАНИЕ")
	for _, secret := range secrets {
		a.print("%-36s  %-12s  %-20s  %s\n",
			secret.ID,
			secret.Type,
			secret.UpdatedAt.Format(time.RFC3339),
			secret.Name,
		)
	}
	return nil
}
