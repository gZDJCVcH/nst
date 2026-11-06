package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"nst/internal/client"
	"nst/internal/config"
	"nst/internal/credential"
	"nst/internal/jsonx"
	"nst/internal/output"
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
	var noOpen bool
	var modeRaw string
	cmd := &cobra.Command{
		Use:   "login",
		Short: "登录并保存令牌",
		Long: "三种登录模式：\n" +
			"  context      读取已保存配置与环境变量，不交互\n" +
			"  flags        用 --url 与 --token / --password 一次传入，不交互\n" +
			"  interactive  逐步询问，可打开令牌页面\n\n" +
			"默认 auto：参数齐全走 flags，已有完整配置/环境变量走 context，" +
			"否则 tty 下走 interactive；非 tty 无凭据直接报错。",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			mode, err := credential.ParseMode(modeRaw)
			if err != nil {
				return a.loginFailure(exitf(2, "%v", err))
			}
			return a.runLogin(credential.Input{
				Mode:         mode,
				FlagURL:      a.flagURL,
				FlagToken:    a.flagToken,
				FlagUsername: username,
				FlagPassword: password,
				FlagOTP:      otp,
				UsePassword:  cmd.Flags().Changed("password") || otp != "",
				TTY:          a.isTTY(),
			}, noOpen)
		},
	}
	cmd.Flags().StringVarP(&username, "username", "u", "admin", "仅密码模式使用，默认 admin")
	cmd.Flags().StringVar(&password, "password", "", "给出则走账号密码模式；不带值则交互询问密码")
	cmd.Flags().Lookup("password").NoOptDefVal = ""
	cmd.Flags().StringVar(&otp, "otp", "", "两步验证码")
	cmd.Flags().BoolVar(&noOpen, "no-open", false, "不自动打开浏览器（仅打印令牌页面地址）")
	cmd.Flags().StringVar(&modeRaw, "mode", "auto", "登录模式：auto / context / flags / interactive")
	return cmd
}

// runLogin 统一执行三种模式：解析凭据 → 探活 → 校验 → 按需保存 → 输出结果。
func (a *App) runLogin(in credential.Input, noOpen bool) error {
	cfg, err := config.Load()
	if err != nil {
		return a.loginFailure(err)
	}
	in.ContextURL = config.Str(cfg, "url")
	in.ContextToken = config.Str(cfg, "token")

	c, err := credential.Resolve(in)
	if err != nil {
		return a.loginFailure(err)
	}

	// 1) 地址：交互模式允许询问与重输，非交互模式只探活一次，失败立即返回。
	url := NormalizeURL(c.URL)
	if url == "" && c.Mode == credential.ModeInteractive {
		v, promptErr := a.prompt("OpenList 地址（IP:端口 或域名，可省略 http://）", "http://127.0.0.1:5244")
		if promptErr != nil {
			return a.loginFailure(promptErr)
		}
		url = NormalizeURL(v)
	}
	if url == "" {
		return a.loginFailure(errors.New("没有地址，已取消"))
	}
	for {
		title, ver, probeErr := a.probeOpenList(url)
		if probeErr == nil {
			if !a.jsonOut {
				a.printf("✓ 已连接 %s   %s %s\n", url, title, ver)
			}
			break
		}
		if c.Mode != credential.ModeInteractive {
			return a.loginFailure(probeErr)
		}
		if !a.jsonOut {
			a.printf("✗ 连不上 %s：%v\n", url, probeErr)
		}
		v, promptErr := a.prompt("重新输入地址", url)
		if promptErr != nil {
			return a.loginFailure(promptErr)
		}
		url = NormalizeURL(v)
		if url == "" {
			return a.loginFailure(errors.New("没有地址，已取消"))
		}
	}

	// 2) 凭据：只有交互模式才询问密码、引导令牌页面。
	token := c.Token
	password := c.Password
	if c.Mode == credential.ModeInteractive {
		if c.UsePassword && password == "" {
			password, err = a.promptSecret(fmt.Sprintf("%s 的密码", c.Username))
			if err != nil {
				return a.loginFailure(err)
			}
		}
		if !c.UsePassword && token == "" {
			a.guideTokenPage(url, noOpen)
		}
	}

	// 3) 账号密码换令牌。
	if c.UsePassword {
		token, err = a.exchangePassword(url, c.Username, password, c.OTP)
		if err != nil {
			return a.loginFailure(err)
		}
	}

	// 4) 校验；交互模式允许重输 token，其余模式一次失败即返回。
	tries := 0
	for {
		if token == "" {
			if c.Mode != credential.ModeInteractive {
				return a.loginFailure(errors.New("缺少令牌"))
			}
			tk, promptErr := a.promptSecret("粘贴 token")
			if promptErr != nil {
				return a.loginFailure(promptErr)
			}
			if tk == "" {
				return a.loginFailure(errors.New("没有输入 token，已取消"))
			}
			token = tk
		}

		me, validateErr := a.validateToken(url, token)
		if validateErr == nil {
			saved := false
			if c.Mode != credential.ModeContext {
				cfg["url"] = url
				cfg["token"] = token
				cfg["username"] = jsonx.Str(me, "username")
				if _, saveErr := config.Save(cfg); saveErr != nil {
					return a.loginFailure(saveErr)
				}
				saved = true
			}
			return a.loginResult(credential.Result{
				Mode:   c.Mode,
				Source: c.Source,
				URL:    url,
				Valid:  true,
				Saved:  saved,
				User:   jsonx.Str(me, "username"),
				Role:   roleOf(me),
			})
		}

		tries++
		if c.Mode != credential.ModeInteractive || tries >= maxTokenTries {
			return a.loginFailure(validateErr)
		}
		if !a.jsonOut {
			a.printf("✗ 令牌校验失败：%v\n", validateErr)
		}
		token = ""
	}
}

// authClient 构造登录链路用的客户端。认证接口不涉及存储驱动，不套网盘限流。
func authClient(url, token string) (*client.Client, error) {
	c, err := client.New(url, token)
	if err != nil {
		return nil, err
	}
	c.CloudInterval = 0
	c.LocalInterval = 0
	return c, nil
}

func (a *App) probeOpenList(url string) (string, string, error) {
	c, err := authClient(url, "")
	if err != nil {
		return "", "", err
	}
	c.Timeout = 10 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	info, err := c.PublicSettings(ctx)
	if err != nil {
		return "", "", err
	}
	ver := strings.TrimSpace(strings.SplitN(jsonx.Str(info, "version"), " (", 2)[0])
	if ver == "" {
		ver = "未知版本"
	}
	title := jsonx.Str(info, "site_title")
	if title == "" {
		title = "OpenList"
	}
	return title, ver, nil
}

func (a *App) validateToken(url, token string) (map[string]any, error) {
	c, err := authClient(url, token)
	if err != nil {
		return nil, err
	}
	if a.timeoutSec > 0 {
		c.Timeout = time.Duration(a.timeoutSec) * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout)
	defer cancel()
	return c.Me(ctx)
}

func (a *App) exchangePassword(url, username, password, otp string) (string, error) {
	c, err := authClient(url, "")
	if err != nil {
		return "", err
	}
	if a.timeoutSec > 0 {
		c.Timeout = time.Duration(a.timeoutSec) * time.Second
	}
	if _, err := c.Login(context.Background(), username, password, otp); err != nil {
		return "", err
	}
	return c.Token, nil
}

func (a *App) loginResult(r credential.Result) error {
	return a.emit(r, func() {
		a.printf("✓ 已登录：%s（%s）  模式 %s / 来源 %s\n", r.User, r.Role, r.Mode, r.Source)
		if r.Saved {
			a.printf("  令牌已保存到 %s\n", config.File())
		} else {
			a.printf("  凭据来自配置或环境变量，无需保存\n")
		}
	})
}

func (a *App) loginFailure(err error) error {
	code := loginExitCode(err)
	if a.jsonOut {
		_ = output.DumpJSON(a.Out, credential.Result{Valid: false, Error: err.Error()})
		return exitf(code, "")
	}
	return exitf(code, "✗ 登录失败：%v", err)
}

func loginExitCode(err error) int {
	var ee *ExitError
	if errors.As(err, &ee) {
		return ee.Code
	}
	var ne *client.NetworkError
	if errors.As(err, &ne) {
		return 4
	}
	var ae *client.ApiError
	if errors.As(err, &ae) {
		return 3
	}
	var ce *config.ConfigError
	if errors.As(err, &ce) {
		return 2
	}
	return 1
}

// guideTokenPage 打印令牌页面地址；本地图形会话下顺手打开浏览器。
func (a *App) guideTokenPage(base string, noOpen bool) {
	// 已经用 --token 传入时不必引导
	if a.flagToken != "" {
		return
	}

	link := tokenPageURL(base)
	a.printf("令牌页面：%s\n", link)
	a.printf("          （登录后台 → 设置 → 其他 → 页面最下方「令牌」）\n")

	if noOpen {
		return
	}
	if !canOpenBrowser() {
		a.printf("          复制上面的地址到浏览器打开\n")
		return
	}
	if err := openBrowser(link); err != nil {
		a.printf("          自动打开失败（%v），请手动打开上面的地址\n", err)
		return
	}
	a.printf("          已在浏览器中打开\n")
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
