package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"nst/internal/app"
	"nst/internal/buildinfo"
)

func versionCmd(a *app.App) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "显示版本",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			a.Println(fmt.Sprintf("nst %s", buildinfo.String()))
			return nil
		},
	}
}
