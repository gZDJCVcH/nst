// Package cli 是命令树的组合根：只做装配，不承载具体服务逻辑。
package cli

import "nst/internal/app"

// CLI 持有公共运行环境，并负责构建与执行命令树。
type CLI struct {
	app *app.App
}

// New 构造命令行入口。
func New() *CLI { return &CLI{app: app.New()} }

// App 返回公共运行环境，供测试替换 IO。
func (c *CLI) App() *app.App { return c.app }

// Execute 解析并执行命令，返回进程退出码。
func (c *CLI) Execute(argv []string) int {
	return c.app.Execute(argv, buildRoot)
}
