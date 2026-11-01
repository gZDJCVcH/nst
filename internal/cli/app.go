// Package cli 实现命令行树与各子命令。
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"openlist-cli/internal/client"
	"openlist-cli/internal/config"
	"openlist-cli/internal/output"
	"openlist-cli/internal/pathx"
)

// ExitError 携带退出码与要打印的消息，由 Execute 统一处理。
type ExitError struct {
	Code int
	Msg  string
}

func (e *ExitError) Error() string { return e.Msg }

func exitf(code int, format string, args ...any) error {
	return &ExitError{Code: code, Msg: fmt.Sprintf(format, args...)}
}

// App 持有 IO 与全局参数，便于测试时替换。
type App struct {
	Out io.Writer
	Err io.Writer
	In  io.Reader

	// 全局参数
	flagURL    string
	flagToken  string
	jsonOut    bool
	timeoutSec int
	rateMS     int
	localRate  int
	cacheTTL   int
	noCache    bool

	rateSet      bool
	localRateSet bool
	cacheTTLSet  bool
	activeCmd    *cobra.Command

	stdinTTY      bool
	isTTYOverride *bool

	root *cobra.Command
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

func (a *App) isTTY() bool {
	if a.isTTYOverride != nil {
		return *a.isTTYOverride
	}
	return a.stdinTTY
}

// Execute 解析并执行命令，返回进程退出码。
func (a *App) Execute(argv []string) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	a.root = a.buildRoot()
	a.root.SetArgs(normalizeOptionalValueFlags(argv))
	a.root.SetOut(a.Out)
	a.root.SetErr(a.Err)
	a.root.SetIn(a.In)
	a.root.SetContext(ctx)

	err := a.root.Execute()
	if err == nil {
		return 0
	}
	if ctx.Err() != nil && errors.Is(err, context.Canceled) {
		fmt.Fprintln(a.Err, "\n已中断")
		return 130
	}

	var ee *ExitError
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

	var apiErr *client.ApiError
	if errors.As(err, &apiErr) {
		fmt.Fprintf(a.Err, "接口错误：%v\n", err)
		return 3
	}

	var netErr *client.NetworkError
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

// newClient 依据全局参数 + 配置文件构造客户端。
func (a *App) newClient(needToken bool) (*client.Client, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	url := a.flagURL
	if url == "" {
		url = config.Str(cfg, "url")
	}
	token := a.flagToken
	if token == "" {
		token = config.Str(cfg, "token")
	}

	if url == "" {
		return nil, exitf(1, "缺少服务地址。先登录一次：\n"+
			"  openlist-cli auth login --url http://你的地址:5244 -u admin")
	}
	if needToken && token == "" {
		return nil, exitf(1, "尚未登录。执行：openlist-cli auth login --url <地址>")
	}

	c, err := client.New(NormalizeURL(url), token)
	if err != nil {
		return nil, err
	}
	if a.timeoutSec > 0 {
		c.Timeout = time.Duration(a.timeoutSec) * time.Second
	}
	if a.rateSet {
		c.CloudInterval = time.Duration(a.rateMS) * time.Millisecond
	}
	if a.localRateSet {
		c.LocalInterval = time.Duration(a.localRate) * time.Millisecond
	}
	switch {
	case a.noCache:
		c.CacheTTL = 0
	case a.cacheTTLSet:
		c.CacheTTL = time.Duration(a.cacheTTL) * time.Second
	}
	return c, nil
}

// NormalizeURL 补协议、去尾部斜杠。空串原样返回。
func NormalizeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimRight(raw, "/")
	if raw == "" {
		return ""
	}
	lowered := strings.ToLower(raw)
	if !strings.HasPrefix(lowered, "http://") && !strings.HasPrefix(lowered, "https://") {
		raw = "http://" + raw
	}
	return raw
}

// emit 有 --json 就输出 JSON，否则走人类可读渲染。
func (a *App) emit(obj any, render func()) error {
	if a.jsonOut {
		return output.DumpJSON(a.Out, obj)
	}
	render()
	return nil
}

func (a *App) printf(format string, args ...any) {
	fmt.Fprintf(a.Out, format, args...)
}

func (a *App) println(args ...any) {
	fmt.Fprintln(a.Out, args...)
}

func (a *App) errorf(format string, args ...any) {
	fmt.Fprintf(a.Err, format, args...)
}

// prompt 读一行输入，支持默认值。
func (a *App) prompt(text, def string) (string, error) {
	suffix := ""
	if def != "" {
		suffix = " [" + def + "]"
	}
	fmt.Fprintf(a.Out, "%s%s: ", text, suffix)
	line, err := readLine(a.In)
	if err != nil {
		return "", exitf(1, "已取消")
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return def, nil
	}
	return line, nil
}

// promptSecret 读密码：tty 下不回显，非 tty 直接报错（提示改用参数）。
func (a *App) promptSecret(text string) (string, error) {
	if !a.isTTY() {
		return "", exitf(1, "非交互环境无法询问「%s」，请改用参数传入", text)
	}
	fmt.Fprintf(a.Out, "%s: ", text)
	var raw []byte
	raw, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(a.Out)
	if err != nil {
		return "", exitf(1, "已取消")
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
// pflag 的 NoOptDefVal 只认后者；Python 版 argparse 的 nargs="?" 两种都认，
// 这里做一次预处理保持行为一致。`--password` 后面紧跟另一个 flag 时保持为空值。
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
