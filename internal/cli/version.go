package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"openlist-cli/internal/buildinfo"
)

func (a *App) versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "显示版本",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			a.println(fmt.Sprintf("openlist-cli %s", buildinfo.String()))
			return nil
		},
	}
}
