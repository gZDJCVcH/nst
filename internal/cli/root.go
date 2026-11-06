package cli

import (
	"github.com/spf13/cobra"
)

func (a *App) buildRoot() *cobra.Command {
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
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			a.activeCmd = cmd
			a.rateSet = cmd.Flags().Changed("rate")
			a.localRateSet = cmd.Flags().Changed("local-rate")
			a.cacheTTLSet = cmd.Flags().Changed("cache-ttl")
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	pf := root.PersistentFlags()
	pf.StringVar(&a.flagURL, "url", "", "OpenList 地址，覆盖配置")
	pf.StringVar(&a.flagToken, "token", "", "令牌，覆盖配置")
	pf.BoolVar(&a.jsonOut, "json", false, "输出原始 JSON")
	pf.IntVar(&a.timeoutSec, "timeout", 30, "请求超时秒数")
	pf.IntVar(&a.rateMS, "rate", 0, "网盘请求最小间隔（毫秒，默认 1000）")
	pf.IntVar(&a.localRate, "local-rate", 0, "本地盘请求最小间隔（毫秒，默认 50）")
	pf.IntVar(&a.cacheTTL, "cache-ttl", 0, "目录列表缓存秒数（默认 60，0 表示不缓存）")
	pf.BoolVar(&a.noCache, "no-cache", false, "禁用目录缓存")

	root.AddCommand(
		a.versionCmd(),
		a.upgradeCmd(),
		a.openlistCmd(),
	)
	return root
}

// openlistCmd 把 OpenList 的全部能力收纳到 nst openlist 下。
func (a *App) openlistCmd() *cobra.Command {
	openlist := &cobra.Command{
		Use:   "openlist",
		Short: "OpenList 文件与任务管理",
		Long: "OpenList 命令行客户端\n\n" +
			"路径必须带前缀，不做任何智能推断：\n" +
			"  local:<本地路径>    例如 local:/tmp/a.mkv\n" +
			"  remote:/<远端路径>  例如 remote:/115/音乐",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	openlist.AddCommand(
		a.authCmd(),
		a.listCmd(),
		a.statCmd(),
		a.treeCmd(),
		a.searchCmd(),
		a.readCmd(),
		a.linkCmd(),
		a.uploadCmd(),
		a.downloadCmd(),
		a.removeCmd(),
		a.moveCmd(),
		a.copyCmd(),
		a.renameCmd(),
		a.mkdirCmd(),
		a.tasksCmd(),
		a.cancelTaskCmd(),
		a.driversCmd(),
		a.verifyCmd(),
		a.mountsCmd(),
		a.storageCmd(),
		a.driverCmd(),
		a.configCmd(),
		a.versionCmd(),
		a.upgradeCmd(),
	)
	return openlist
}
