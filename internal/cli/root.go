package cli

import (
	"github.com/spf13/cobra"

	"nst/internal/app"
	"nst/internal/service/openlist"
)

func buildRoot(a *app.App) *cobra.Command {
	root := &cobra.Command{
		Use:   "nst",
		Short: "NAS 工具箱",
		Long: "NAS 工具箱（单二进制，无运行时依赖）\n\n" +
			"当前子命令：\n" +
			"  openlist   OpenList 文件与任务管理\n" +
			"  version    显示版本\n" +
			"  upgrade    检查并升级到最新版本",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	a.BindGlobalFlags(root)

	openlistCmd := openlist.Register(root, a)
	openlistCmd.AddCommand(versionCmd(a), upgradeCmd(a))

	root.AddCommand(
		versionCmd(a),
		upgradeCmd(a),
	)
	return root
}
