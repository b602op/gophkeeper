package cli

import (
	"time"

	"github.com/spf13/cobra"
)

// newSearchCmd создаёт команду поиска секретов.
func newSearchCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "search <query>",
		Short: "Найти секреты по названию и метаинформации",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return a.runSearch(args[0])
		},
	}
}

// runSearch ищет секреты в локальной базе.
func (a *app) runSearch(query string) error {
	_, s, err := a.authenticatedClient()
	if err != nil {
		return err
	}
	store, err := a.storeForSession(s)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	secrets, err := store.Search(query)
	if err != nil {
		return err
	}
	if len(secrets) == 0 {
		a.print("Ничего не найдено.\n")
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
