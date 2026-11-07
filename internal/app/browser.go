package app

import (
	"os"
	"os/exec"
	"runtime"
)

// CanOpenBrowser 判断当前环境适合自动拉起浏览器。
//
// SSH 会话里 open/xdg-open 只会作用于服务端，没有意义，直接判否。
func CanOpenBrowser() bool {
	if os.Getenv("SSH_CONNECTION") != "" || os.Getenv("SSH_TTY") != "" || os.Getenv("SSH_CLIENT") != "" {
		return false
	}
	switch runtime.GOOS {
	case "darwin", "windows":
		return true
	case "linux":
		return os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != ""
	}
	return false
}

// OpenBrowser 用系统默认浏览器打开链接。
//
// 这里必须调外部命令：打开浏览器没有等价的 Go 标准库能力。
func OpenBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

// CanOpenBrowser 报告当前环境是否适合自动拉起浏览器。
func (a *App) CanOpenBrowser() bool { return CanOpenBrowser() }

// OpenBrowser 用系统默认浏览器打开链接。
func (a *App) OpenBrowser(url string) error { return OpenBrowser(url) }
