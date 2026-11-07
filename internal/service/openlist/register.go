package openlist

import (
	"time"

	"github.com/spf13/cobra"

	"nst/internal/app"
	"nst/internal/apperr"
	"nst/internal/config"
)

// store 是 OpenList 的配置读写器，沿用旧目录与环境变量名。
var store = config.OpenList()

// CLI 把公共运行环境与 OpenList 命令绑在一起。
type CLI struct{ *app.App }

// Register 在 root 下挂载 nst openlist，并返回该父命令供组合根补充全局子命令。
func Register(root *cobra.Command, a *app.App) *cobra.Command {
	c := &CLI{App: a}
	cmd := &cobra.Command{
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

	cmd.AddCommand(
		c.authCmd(),
		c.listCmd(),
		c.statCmd(),
		c.treeCmd(),
		c.searchCmd(),
		c.readCmd(),
		c.linkCmd(),
		c.uploadCmd(),
		c.downloadCmd(),
		c.removeCmd(),
		c.moveCmd(),
		c.copyCmd(),
		c.renameCmd(),
		c.mkdirCmd(),
		c.tasksCmd(),
		c.cancelTaskCmd(),
		c.driversCmd(),
		c.verifyCmd(),
		c.mountsCmd(),
		c.storageCmd(),
		c.driverCmd(),
		c.configCmd(),
	)
	root.AddCommand(cmd)
	return cmd
}

// newClient 依据全局参数 + 配置文件构造客户端。
func (a *CLI) newClient(needToken bool) (*Client, error) {
	cfg, err := store.Load()
	if err != nil {
		return nil, err
	}
	url := a.Options.URL
	if url == "" {
		url = config.Str(cfg, "url")
	}
	token := a.Options.Token
	if token == "" {
		token = config.Str(cfg, "token")
	}

	if url == "" {
		return nil, apperr.Exitf(1, "缺少服务地址。先登录一次：\n"+
			"  nst openlist auth login --url http://你的地址:5244 -u admin")
	}
	if needToken && token == "" {
		return nil, apperr.Exitf(1, "尚未登录。执行：nst openlist auth login --url <地址>")
	}

	c, err := New(normalizeURL(url), token)
	if err != nil {
		return nil, err
	}
	if a.Options.Timeout > 0 {
		c.Timeout = time.Duration(a.Options.Timeout) * time.Second
	}
	if a.Options.RateSet {
		c.CloudInterval = time.Duration(a.Options.RateMS) * time.Millisecond
	}
	if a.Options.LocalRateSet {
		c.LocalInterval = time.Duration(a.Options.LocalRate) * time.Millisecond
	}
	switch {
	case a.Options.NoCache:
		c.CacheTTL = 0
	case a.Options.CacheTTLSet:
		c.CacheTTL = time.Duration(a.Options.CacheTTL) * time.Second
	}
	return c, nil
}
