// Package app 提供命令行的公共运行环境：IO、全局参数、退出码与交互提示。
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"nst/internal/apierr"
	"nst/internal/apperr"
	"nst/internal/config"
	"nst/internal/output"
	"nst/internal/pathx"
)

// Options 是全局参数。各服务按需读取，不要依赖自己的 flag 名。
type Options struct {
	URL       string
	Token     string
	JSON      bool
	Timeout   int
	RateMS    int
	LocalRate int
	CacheTTL  int
	NoCache   bool

	RateSet      bool
	LocalRateSet bool
	CacheTTLSet  bool
}

// App 持有 IO 与全局参数，便于测试时替换。
type App struct {
	Out io.Writer
	Err io.Writer
	In  io.Reader

	Options Options

	stdinTTY    bool
	ttyOverride *bool
}

// New 构造带默认 IO 的 App。
func New() *App {
	return &App{
		Out:      os.Stdout,
		Err:      os.Stderr,
		In:       os.Stdin,
		stdinTTY: term.IsTerminal(int(os.Stdin.Fd())),
	}
}

// IsTTY 报告 stdin 是否为终端。
func (a *App) IsTTY() bool {
	if a.ttyOverride != nil {
		return *a.ttyOverride
	}
	return a.stdinTTY
}

// SetTTY 覆盖终端判断，供测试使用。
func (a *App) SetTTY(v bool) { a.ttyOverride = &v }

// JSON 报告是否要求结构化输出。
func (a *App) JSON() bool { return a.Options.JSON }

// BindGlobalFlags 注册全局参数，并记录用户显式指定了哪些零值有意义的参数。
func (a *App) BindGlobalFlags(root *cobra.Command) {
	pf := root.PersistentFlags()
	pf.StringVar(&a.Options.URL, "url", "", "服务地址，覆盖配置")
	pf.StringVar(&a.Options.Token, "token", "", "令牌，覆盖配置")
	pf.BoolVar(&a.Options.JSON, "json", false, "输出原始 JSON")
	pf.IntVar(&a.Options.Timeout, "timeout", 30, "请求超时秒数")
	pf.IntVar(&a.Options.RateMS, "rate", 0, "网盘请求最小间隔（毫秒，默认 1000）")
	pf.IntVar(&a.Options.LocalRate, "local-rate", 0, "本地盘请求最小间隔（毫秒，默认 50）")
	pf.IntVar(&a.Options.CacheTTL, "cache-ttl", 0, "目录列表缓存秒数（默认 60，0 表示不缓存）")
	pf.BoolVar(&a.Options.NoCache, "no-cache", false, "禁用目录缓存")

	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		a.Options.RateSet = cmd.Flags().Changed("rate")
		a.Options.LocalRateSet = cmd.Flags().Changed("local-rate")
		a.Options.CacheTTLSet = cmd.Flags().Changed("cache-ttl")
		return nil
	}
}

// Execute 解析并执行命令，返回进程退出码。
// buildRoot 由组合根提供，避免 app 反向依赖具体服务。
func (a *App) Execute(argv []string, buildRoot func(*App) *cobra.Command) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	root := buildRoot(a)
	root.SetArgs(normalizeOptionalValueFlags(argv))
	root.SetOut(a.Out)
	root.SetErr(a.Err)
	root.SetIn(a.In)
	root.SetContext(ctx)

	err := root.Execute()
	if err == nil {
		return 0
	}
	if ctx.Err() != nil && errors.Is(err, context.Canceled) {
		fmt.Fprintln(a.Err, "\n已中断")
		return 130
	}

	var ee *apperr.ExitError
	if errors.As(err, &ee) {
		if ee.Msg != "" {
			fmt.Fprintln(a.Err, ee.Msg)
		}
		return ee.Code
	}

	var pe *pathx.PathError
	if errors.As(err, &pe) {
		fmt.Fprintf(a.Err, "路径错误：%v\n", err)
		return 2
	}

	var ce *config.ConfigError
	if errors.As(err, &ce) {
		fmt.Fprintf(a.Err, "配置错误：%v\n", err)
		return 2
	}

	var apiErr *apierr.ApiError
	if errors.As(err, &apiErr) {
		fmt.Fprintf(a.Err, "接口错误：%v\n", err)
		return 3
	}

	var netErr *apierr.NetworkError
	if errors.As(err, &netErr) {
		fmt.Fprintf(a.Err, "网络错误：%v\n", err)
		return 4
	}

	if errors.Is(err, context.Canceled) {
		fmt.Fprintln(a.Err, "\n已中断")
		return 130
	}

	fmt.Fprintf(a.Err, "%v\n", err)
	return 1
}

// Emit 有 --json 就输出 JSON，否则走人类可读渲染。
func (a *App) Emit(obj any, render func()) error {
	if a.Options.JSON {
		return output.DumpJSON(a.Out, obj)
	}
	render()
	return nil
}

// Printf 写 stdout。
func (a *App) Printf(format string, args ...any) {
	fmt.Fprintf(a.Out, format, args...)
}

// Println 写一行 stdout。
func (a *App) Println(args ...any) {
	fmt.Fprintln(a.Out, args...)
}

// Errorf 写 stderr。
func (a *App) Errorf(format string, args ...any) {
	fmt.Fprintf(a.Err, format, args...)
}

// Prompt 读一行输入，支持默认值。
func (a *App) Prompt(text, def string) (string, error) {
	suffix := ""
	if def != "" {
		suffix = " [" + def + "]"
	}
	fmt.Fprintf(a.Out, "%s%s: ", text, suffix)
	line, err := readLine(a.In)
	if err != nil {
		return "", apperr.Exitf(1, "已取消")
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return def, nil
	}
	return line, nil
}

// PromptSecret 读密码：tty 下不回显，非 tty 直接报错（提示改用参数）。
func (a *App) PromptSecret(text string) (string, error) {
	if !a.IsTTY() {
		return "", apperr.Exitf(1, "非交互环境无法询问「%s」，请改用参数传入", text)
	}
	fmt.Fprintf(a.Out, "%s: ", text)
	raw, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(a.Out)
	if err != nil {
		return "", apperr.Exitf(1, "已取消")
	}
	return strings.TrimSpace(string(raw)), nil
}

func readLine(r io.Reader) (string, error) {
	var sb strings.Builder
	buf := make([]byte, 1)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			if buf[0] == '\n' {
				return sb.String(), nil
			}
			if buf[0] != '\r' {
				sb.WriteByte(buf[0])
			}
		}
		if err != nil {
			if sb.Len() > 0 {
				return sb.String(), nil
			}
			return "", err
		}
	}
}

// normalizeOptionalValueFlags 让 `--password secret` 与 `--password=secret` 等价。
//
// pflag 的 NoOptDefVal 只认 `--password=值` 形式，这里做一次预处理，
// 让空格分隔的写法也生效。`--password` 后面紧跟另一个 flag 时保持为空值。
func normalizeOptionalValueFlags(argv []string) []string {
	out := make([]string, 0, len(argv))
	for i := 0; i < len(argv); i++ {
		if argv[i] == "--password" && i+1 < len(argv) && !strings.HasPrefix(argv[i+1], "-") {
			out = append(out, "--password="+argv[i+1])
			i++
			continue
		}
		out = append(out, argv[i])
	}
	return out
}
