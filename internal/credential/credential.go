// Package credential 定义登录模式与凭据解析约定，与具体服务无关。
package credential

import (
	"fmt"
	"strings"
)

// Mode 是凭据的提供方式。
type Mode string

const (
	ModeAuto        Mode = "auto"
	ModeContext     Mode = "context"
	ModeFlags       Mode = "flags"
	ModeInteractive Mode = "interactive"
)

// ParseMode 解析 --mode 的值，空串视为 auto。
func ParseMode(s string) (Mode, error) {
	m := Mode(strings.ToLower(strings.TrimSpace(s)))
	switch m {
	case "", ModeAuto:
		return ModeAuto, nil
	case ModeContext, ModeFlags, ModeInteractive:
		return m, nil
	default:
		return "", fmt.Errorf("未知登录模式 %q，可用：auto / context / flags / interactive", s)
	}
}

// Source 标识凭据实际来自哪一层。
type Source string

const (
	SourceFlag    Source = "flag"
	SourceContext Source = "context"
	SourcePrompt  Source = "prompt"
)

// Input 汇集各层凭据，不含任何服务相关逻辑。
// ContextURL / ContextToken 来自 config.Load()：配置文件叠加环境变量覆盖。
type Input struct {
	Mode Mode

	FlagURL      string
	FlagToken    string
	FlagUsername string
	FlagPassword string
	FlagOTP      string

	ContextURL   string
	ContextToken string

	UsePassword bool
	TTY         bool
}

// Credentials 是解析后的凭据与所用模式。
type Credentials struct {
	URL         string
	Token       string
	Username    string
	Password    string
	OTP         string
	UsePassword bool
	Mode        Mode
	Source      Source
}

// Resolve 按统一优先级确定模式与凭据，不执行任何 IO。
//
// 字段优先级：flag > 上下文（配置 + 环境变量）> 交互询问。
// auto 判定顺序：参数齐全走 flags，已有完整上下文走 context，否则 tty 下走
// interactive；非 tty 且无凭据时直接报错，绝不进入阻塞式询问。
func Resolve(in Input) (Credentials, error) {
	mode := in.Mode
	if mode == "" {
		mode = ModeAuto
	}

	if mode == ModeAuto {
		switch {
		case in.FlagURL != "" && (in.FlagToken != "" || (in.UsePassword && in.FlagPassword != "")):
			mode = ModeFlags
		case in.ContextURL != "" && in.ContextToken != "" && !in.UsePassword:
			mode = ModeContext
		case in.TTY:
			mode = ModeInteractive
		default:
			return Credentials{}, fmt.Errorf("非交互环境缺少凭据：请用 --url 与 --token（或 --password），或设置 OPENLIST_URL / OPENLIST_TOKEN")
		}
	}

	if mode == ModeInteractive && !in.TTY {
		return Credentials{}, fmt.Errorf("交互模式需要终端（tty）；非交互环境请用 flags 或 context")
	}
	if mode == ModeContext && in.UsePassword {
		return Credentials{}, fmt.Errorf("上下文模式只使用已保存的令牌，不能与 --password 同用")
	}

	c := Credentials{
		Mode:        mode,
		UsePassword: in.UsePassword,
		Username:    in.FlagUsername,
		OTP:         in.FlagOTP,
	}
	if c.Username == "" {
		c.Username = "admin"
	}

	switch mode {
	case ModeFlags:
		c.URL, c.Token, c.Password = in.FlagURL, in.FlagToken, in.FlagPassword
		c.Source = SourceFlag
	case ModeContext:
		c.URL, c.Token = in.ContextURL, in.ContextToken
		c.Source = SourceContext
	case ModeInteractive:
		c.URL, c.Token, c.Password = in.FlagURL, in.FlagToken, in.FlagPassword
		if c.URL == "" {
			c.URL = in.ContextURL
		}
		if c.Token == "" {
			c.Token = in.ContextToken
		}
		c.Source = SourcePrompt
		if in.FlagURL != "" || in.FlagToken != "" || in.FlagPassword != "" {
			c.Source = SourceFlag
		} else if in.ContextURL != "" || in.ContextToken != "" {
			c.Source = SourceContext
		}
	default:
		return Credentials{}, fmt.Errorf("未处理的登录模式 %q", mode)
	}

	if mode != ModeInteractive {
		var missing []string
		if c.URL == "" {
			missing = append(missing, "url")
		}
		if c.UsePassword {
			if c.Password == "" {
				missing = append(missing, "password")
			}
		} else if c.Token == "" {
			missing = append(missing, "token")
		}
		if len(missing) > 0 {
			return Credentials{}, fmt.Errorf("模式 %s 下缺少 %s", mode, strings.Join(missing, "、"))
		}
	}

	return c, nil
}

// Result 是登录结果，供 --json 与 human 输出共用。
type Result struct {
	Mode   Mode   `json:"mode,omitempty"`
	Source Source `json:"source,omitempty"`
	URL    string `json:"url,omitempty"`
	Valid  bool   `json:"valid"`
	Saved  bool   `json:"saved"`
	User   string `json:"user,omitempty"`
	Role   string `json:"role,omitempty"`
	Error  string `json:"error,omitempty"`
}
