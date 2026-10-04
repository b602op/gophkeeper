package cli

import (
	"github.com/spf13/cobra"

	"github.com/b602op/gophkeeper/internal/buildinfo"
)

// newVersionCmd создаёт команду вывода версии.
func newVersionCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Показать версию и дату сборки",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			a.print("%s\n", buildinfo.String())
			return nil
		},
	}
}
