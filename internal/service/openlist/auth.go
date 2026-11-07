package openlist

import (
	"context"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"nst/internal/apperr"
	"nst/internal/auth"
	"nst/internal/config"
	"nst/internal/jsonx"
)

const maxTokenTries = 3

func (a *CLI) authCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "登录/登出/身份",
	}
	cmd.AddCommand(a.authLoginCmd(), a.authLogoutCmd(), a.authTokenCmd(), a.authWhoamiCmd())
	return cmd
}

func (a *CLI) authLoginCmd() *cobra.Command {
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
			flow := a.loginFlow()
			mode, err := auth.ParseMode(modeRaw)
			if err != nil {
				return flow.Fail(apperr.Exitf(2, "%v", err))
			}
			if err := a.loadLoginContext(flow); err != nil {
				return flow.Fail(err)
			}
			return flow.Run(cmd.Context(), auth.Input{
				Mode:         mode,
				FlagURL:      a.Options.URL,
				FlagToken:    a.Options.Token,
				FlagUsername: username,
				FlagPassword: password,
				FlagOTP:      otp,
				UsePassword:  cmd.Flags().Changed("password") || otp != "",
				TTY:          a.IsTTY(),
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

// loginFlow 组装登录流程：终端能力来自 CLI，服务细节来自 openListAuth。
func (a *CLI) loginFlow() *auth.Flow {
	return &auth.Flow{
		Host:          a,
		Service:       openListAuth{a: a},
		FlagToken:     a.Options.Token,
		MaxTokenTries: maxTokenTries,
	}
}

func (a *CLI) loadLoginContext(flow *auth.Flow) error {
	cfg, err := store.Load()
	if err != nil {
		return err
	}
	flow.ContextURL = config.Str(cfg, "url")
	flow.ContextToken = config.Str(cfg, "token")
	return nil
}

// openListAuth 实现 auth.Service 的 OpenList 部分。
type openListAuth struct{ a *CLI }

func (s openListAuth) AddressPrompt() (string, string) {
	return "OpenList 地址（IP:端口 或域名，可省略 http://）", "http://127.0.0.1:5244"
}

func (s openListAuth) NormalizeURL(raw string) string { return normalizeURL(raw) }

func (s openListAuth) TokenPage(base string) (link, hint string, ok bool) {
	return tokenPageURL(base), "登录后台 → 设置 → 其他 → 页面最下方「令牌」", true
}

func (s openListAuth) Probe(ctx context.Context, url string) (string, string, error) {
	c, err := authClient(url, "")
	if err != nil {
		return "", "", err
	}
	c.Timeout = 10 * time.Second
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
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

func (s openListAuth) Exchange(ctx context.Context, url, username, password, otp string) (string, error) {
	c, err := authClient(url, "")
	if err != nil {
		return "", err
	}
	if s.a.Options.Timeout > 0 {
		c.Timeout = time.Duration(s.a.Options.Timeout) * time.Second
	}
	if _, err := c.Login(ctx, username, password, otp); err != nil {
		return "", err
	}
	return c.Token, nil
}

func (s openListAuth) Validate(ctx context.Context, url, token string) (string, string, error) {
	c, err := authClient(url, token)
	if err != nil {
		return "", "", err
	}
	if s.a.Options.Timeout > 0 {
		c.Timeout = time.Duration(s.a.Options.Timeout) * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	me, err := c.Me(ctx)
	if err != nil {
		return "", "", err
	}
	return jsonx.Str(me, "username"), roleOf(me), nil
}

func (s openListAuth) Save(url, token, user string) (string, error) {
	cfg, err := store.Load()
	if err != nil {
		return "", err
	}
	cfg["url"] = url
	cfg["token"] = token
	cfg["username"] = user
	return store.Save(cfg)
}

// authClient 构造登录链路用的客户端。认证接口不涉及存储驱动，不套网盘限流。
func authClient(url, token string) (*Client, error) {
	c, err := New(url, token)
	if err != nil {
		return nil, err
	}
	c.CloudInterval = 0
	c.LocalInterval = 0
	return c, nil
}

func (a *CLI) authLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "清除本地令牌",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := store.ClearToken(); err != nil {
				return err
			}
			a.Println("已清除本地令牌")
			return nil
		},
	}
}

func (a *CLI) authTokenCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "token",
		Short: "打印令牌",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := store.Load()
			if err != nil {
				return err
			}
			token := config.Str(cfg, "token")
			if token == "" {
				return apperr.Exitf(1, "未登录")
			}
			a.Println(token)
			return nil
		},
	}
}

func (a *CLI) authWhoamiCmd() *cobra.Command {
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
			return a.Emit(me, func() {
				a.Printf("用户名：%s\n", jsonx.Str(me, "username"))
				a.Printf("角色  ：%s\n", roleOf(me))
				a.Printf("权限  ：%s\n", scalar(me["permission"]))
				if base := jsonx.Str(me, "base_path"); base != "" {
					a.Printf("根路径：%s\n", base)
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
