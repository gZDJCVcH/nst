package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"openlist-cli/internal/client"
	"openlist-cli/internal/config"
	"openlist-cli/internal/jsonx"
)

const maxTokenTries = 3

func (a *App) authCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "登录/登出/身份",
	}
	cmd.AddCommand(a.authLoginCmd(), a.authLogoutCmd(), a.authTokenCmd(), a.authWhoamiCmd())
	return cmd
}

func (a *App) authLoginCmd() *cobra.Command {
	var username, password, otp string
	cmd := &cobra.Command{
		Use:   "login",
		Short: "登录并保存令牌（交互式）",
		Long: "默认交互式：问地址 → 探活 → 粘贴 token → 校验 → 通过才保存。\n" +
			"指定 --password 则改为账号密码换令牌。",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed("password") || otp != "" {
				return a.loginWithPassword(username, password, otp)
			}
			return a.loginWithToken()
		},
	}
	cmd.Flags().StringVarP(&username, "username", "u", "admin", "仅密码模式使用，默认 admin")
	cmd.Flags().StringVar(&password, "password", "", "给出则走账号密码模式；不带值则交互询问密码")
	cmd.Flags().Lookup("password").NoOptDefVal = ""
	cmd.Flags().StringVar(&otp, "otp", "", "两步验证码")
	return cmd
}

// loginWithToken 交互式：问地址 → 探活 → 要 token → 校验 → 保存。
func (a *App) loginWithToken() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	url := NormalizeURL(a.flagURL)
	if url == "" {
		if !a.isTTY() {
			return exitf(1, "非交互环境请同时指定 --url 和 --token")
		}
		def := config.Str(cfg, "url")
		if def == "" {
			def = "http://127.0.0.1:5244"
		}
		url, err = a.prompt("OpenList 地址（IP:端口 或域名，可省略 http://）", NormalizeURL(def))
		if err != nil {
			return err
		}
		url = NormalizeURL(url)
	}

	// 1) 探活：确认地址能连上、对方确实是 OpenList
	for {
		if url == "" {
			return exitf(1, "没有地址，已取消")
		}
		probe, err := client.New(url, "")
		if err != nil {
			return exitf(1, "%v", err)
		}
		probe.Timeout = 10 * time.Second
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		info, probeErr := probe.PublicSettings(ctx)
		cancel()

		if probeErr != nil {
			a.printf("✗ 连不上 %s：%v\n", url, probeErr)
			if !a.isTTY() {
				return exitf(2, "")
			}
			url, err = a.prompt("重新输入地址", url)
			if err != nil {
				return err
			}
			url = NormalizeURL(url)
			continue
		}

		rawVersion := jsonx.Str(info, "version")
		short := strings.TrimSpace(strings.SplitN(rawVersion, " (", 2)[0])
		if short == "" {
			short = "未知版本"
		}
		title := jsonx.Str(info, "site_title")
		if title == "" {
			title = "OpenList"
		}
		a.printf("✓ 已连接 %s   %s %s\n", url, title, short)
		break
	}

	// 2) token：校验通过才落盘，失败允许重输
	token := strings.TrimSpace(a.flagToken)
	tries := 0
	for {
		if token == "" {
			token, err = a.promptSecret("粘贴 token")
			if err != nil {
				return err
			}
		}
		if token == "" {
			return exitf(1, "没有输入 token，已取消")
		}

		c, err := client.New(url, token)
		if err != nil {
			return exitf(1, "%v", err)
		}
		if a.timeoutSec > 0 {
			c.Timeout = time.Duration(a.timeoutSec) * time.Second
		}
		ctx, cancel := context.WithTimeout(context.Background(), c.Timeout)
		me, meErr := c.Me(ctx)
		cancel()

		if meErr != nil {
			var apiErr *client.ApiError
			if asAPIError(meErr, &apiErr) {
				tries++
				a.printf("✗ 令牌校验失败：%s（code=%s）\n", apiErr.Message, codeString(apiErr.Code))
				if tries >= maxTokenTries {
					return exitf(1, "连续 %d 次校验失败，已退出。重新执行 auth login 再试", tries)
				}
				if !a.isTTY() {
					return exitf(3, "")
				}
				token = ""
				continue
			}
			return exitf(1, "✗ 校验时网络中断：%v", meErr)
		}

		cfg["url"] = url
		cfg["token"] = token
		cfg["username"] = jsonx.Str(me, "username")
		saved, err := config.Save(cfg)
		if err != nil {
			return err
		}
		a.printf("✓ 已登录：%s（%s）\n", jsonx.Str(me, "username"), roleOf(me))
		a.printf("  令牌已保存到 %s\n", saved)
		return nil
	}
}

// loginWithPassword 账号密码换令牌，适合脚本或忘记 token 时用。
func (a *App) loginWithPassword(username, password, otp string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	url := NormalizeURL(a.flagURL)
	if url == "" {
		url = NormalizeURL(config.Str(cfg, "url"))
	}
	if url == "" {
		return exitf(1, "请用 --url 指定地址，例如 http://127.0.0.1:5244")
	}
	if username == "" {
		username = "admin"
	}
	if password == "" {
		password, err = a.promptSecret(fmt.Sprintf("%s 的密码", username))
		if err != nil {
			return err
		}
	}

	c, err := client.New(url, "")
	if err != nil {
		return exitf(1, "%v", err)
	}
	if a.timeoutSec > 0 {
		c.Timeout = time.Duration(a.timeoutSec) * time.Second
	}
	ctx := context.Background()
	if _, err := c.Login(ctx, username, password, otp); err != nil {
		return err
	}
	me, err := c.Me(ctx)
	if err != nil {
		return err
	}

	cfg["url"] = url
	cfg["token"] = c.Token
	cfg["username"] = jsonx.Str(me, "username")
	saved, err := config.Save(cfg)
	if err != nil {
		return err
	}
	a.printf("✓ 已登录：%s（%s）\n", jsonx.Str(me, "username"), roleOf(me))
	a.printf("  令牌已保存到 %s\n", saved)
	return nil
}

func (a *App) authLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "清除本地令牌",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := config.ClearToken(); err != nil {
				return err
			}
			a.println("已清除本地令牌")
			return nil
		},
	}
}

func (a *App) authTokenCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "token",
		Short: "打印令牌",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			token := config.Str(cfg, "token")
			if token == "" {
				return exitf(1, "未登录")
			}
			a.println(token)
			return nil
		},
	}
}

func (a *App) authWhoamiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "显示身份",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := a.newClient(true)
			if err != nil {
				return err
			}
			me, err := c.Me(cmd.Context())
			if err != nil {
				return err
			}
			return a.emit(me, func() {
				a.printf("用户名：%s\n", jsonx.Str(me, "username"))
				a.printf("角色  ：%s\n", roleOf(me))
				a.printf("权限  ：%s\n", scalar(me["permission"]))
				if base := jsonx.Str(me, "base_path"); base != "" {
					a.printf("根路径：%s\n", base)
				}
			})
		},
	}
}

func roleOf(m map[string]any) string {
	if v := jsonx.Str(m, "role_name"); v != "" {
		return v
	}
	return jsonx.Str(m, "role")
}
