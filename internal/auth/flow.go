package auth

import (
	"context"
	"errors"
	"fmt"

	"nst/internal/apperr"
)

// Host 是登录流程需要的终端能力，由命令层实现。
type Host interface {
	Printf(format string, args ...any)
	Prompt(text, def string) (string, error)
	PromptSecret(text string) (string, error)
	JSON() bool
	Emit(obj any, render func()) error
	CanOpenBrowser() bool
	OpenBrowser(url string) error
}

// Service 是各服务需要实现的登录细节。
type Service interface {
	// AddressPrompt 返回询问地址时的提示文本与默认值。
	AddressPrompt() (text, def string)
	// NormalizeURL 补协议、去尾部斜杠。
	NormalizeURL(raw string) string
	// TokenPage 返回令牌页面地址与页内位置提示；ok=false 表示该服务不走令牌页面。
	TokenPage(base string) (link, hint string, ok bool)
	Probe(ctx context.Context, url string) (title, version string, err error)
	Exchange(ctx context.Context, url, username, password, otp string) (token string, err error)
	Validate(ctx context.Context, url, token string) (user, role string, err error)
	Save(url, token, user string) (path string, err error)
}

// Flow 是三种登录模式共用的流程：探活 → 取凭据 → 校验 → 按需保存 → 输出。
type Flow struct {
	Host    Host
	Service Service

	ContextURL   string
	ContextToken string
	FlagToken    string

	MaxTokenTries int
}

// Run 执行一次登录。in 由命令层填充，Context 字段会以 Flow 上的值为准。
func (f *Flow) Run(ctx context.Context, in Input, noOpen bool) error {
	in.ContextURL, in.ContextToken = f.ContextURL, f.ContextToken
	c, err := Resolve(in)
	if err != nil {
		return f.Fail(err)
	}

	maxTries := f.MaxTokenTries
	if maxTries <= 0 {
		maxTries = 3
	}

	// 1) 地址：交互模式允许询问与重输，非交互模式只探活一次，失败立即返回。
	url := f.Service.NormalizeURL(c.URL)
	if url == "" && c.Mode == ModeInteractive {
		text, def := f.Service.AddressPrompt()
		v, promptErr := f.Host.Prompt(text, def)
		if promptErr != nil {
			return f.Fail(promptErr)
		}
		url = f.Service.NormalizeURL(v)
	}
	if url == "" {
		return f.Fail(errors.New("没有地址，已取消"))
	}
	for {
		title, ver, probeErr := f.Service.Probe(ctx, url)
		if probeErr == nil {
			if !f.Host.JSON() {
				f.Host.Printf("✓ 已连接 %s   %s %s\n", url, title, ver)
			}
			break
		}
		if c.Mode != ModeInteractive {
			return f.Fail(probeErr)
		}
		if !f.Host.JSON() {
			f.Host.Printf("✗ 连不上 %s：%v\n", url, probeErr)
		}
		v, promptErr := f.Host.Prompt("重新输入地址", url)
		if promptErr != nil {
			return f.Fail(promptErr)
		}
		url = f.Service.NormalizeURL(v)
		if url == "" {
			return f.Fail(errors.New("没有地址，已取消"))
		}
	}

	// 2) 凭据：只有交互模式才询问密码、引导令牌页面。
	token := c.Token
	password := c.Password
	if c.Mode == ModeInteractive {
		if c.UsePassword && password == "" {
			password, err = f.Host.PromptSecret(fmt.Sprintf("%s 的密码", c.Username))
			if err != nil {
				return f.Fail(err)
			}
		}
		if !c.UsePassword && token == "" {
			f.GuideTokenPage(url, noOpen)
		}
	}

	// 3) 账号密码换令牌。
	if c.UsePassword {
		token, err = f.Service.Exchange(ctx, url, c.Username, password, c.OTP)
		if err != nil {
			return f.Fail(err)
		}
	}

	// 4) 校验；交互模式允许重输 token，其余模式一次失败即返回。
	tries := 0
	for {
		if token == "" {
			if c.Mode != ModeInteractive {
				return f.Fail(errors.New("缺少令牌"))
			}
			tk, promptErr := f.Host.PromptSecret("粘贴 token")
			if promptErr != nil {
				return f.Fail(promptErr)
			}
			if tk == "" {
				return f.Fail(errors.New("没有输入 token，已取消"))
			}
			token = tk
		}

		user, role, validateErr := f.Service.Validate(ctx, url, token)
		if validateErr == nil {
			saved := false
			savedPath := ""
			if c.Mode != ModeContext {
				savedPath, err = f.Service.Save(url, token, user)
				if err != nil {
					return f.Fail(err)
				}
				saved = true
			}
			res := Result{
				Mode: c.Mode, Source: c.Source, URL: url,
				Valid: true, Saved: saved, User: user, Role: role,
			}
			return f.Host.Emit(res, func() {
				f.Host.Printf("✓ 已登录：%s（%s）  模式 %s / 来源 %s\n",
					res.User, res.Role, res.Mode, res.Source)
				if res.Saved {
					f.Host.Printf("  令牌已保存到 %s\n", savedPath)
				} else {
					f.Host.Printf("  凭据来自配置或环境变量，无需保存\n")
				}
			})
		}

		tries++
		if c.Mode != ModeInteractive || tries >= maxTries {
			return f.Fail(validateErr)
		}
		if !f.Host.JSON() {
			f.Host.Printf("✗ 令牌校验失败：%v\n", validateErr)
		}
		token = ""
	}
}

// Fail 按统一格式输出登录失败结果。
func (f *Flow) Fail(err error) error {
	code := apperr.CodeOf(err)
	if f.Host.JSON() {
		_ = f.Host.Emit(Result{Valid: false, Error: err.Error()}, func() {})
		return apperr.Exitf(code, "")
	}
	return apperr.Exitf(code, "✗ 登录失败：%v", err)
}

// GuideTokenPage 打印令牌页面地址；本地图形会话下顺手打开浏览器。
func (f *Flow) GuideTokenPage(base string, noOpen bool) {
	// 已经用 --token 传入时不必引导
	if f.FlagToken != "" {
		return
	}

	link, hint, ok := f.Service.TokenPage(base)
	if !ok || link == "" {
		return
	}
	f.Host.Printf("令牌页面：%s\n", link)
	if hint != "" {
		f.Host.Printf("          （%s）\n", hint)
	}

	if noOpen {
		return
	}
	if !f.Host.CanOpenBrowser() {
		f.Host.Printf("          复制上面的地址到浏览器打开\n")
		return
	}
	if err := f.Host.OpenBrowser(link); err != nil {
		f.Host.Printf("          自动打开失败（%v），请手动打开上面的地址\n", err)
		return
	}
	f.Host.Printf("          已在浏览器中打开\n")
}
