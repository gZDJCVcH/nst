package cli

import (
	"os"
	"os/exec"
	"runtime"
)

// tokenPageURL 返回 OpenList 生成访问令牌的页面地址。
//
// 令牌在后台「设置 → 其他」页面最下方生成，没有独立路由。
func tokenPageURL(base string) string {
	return NormalizeURL(base) + "/@manage/settings/other"
}

// canOpenBrowser 判断当前环境适合自动拉起浏览器。
//
// SSH 会话里 open/xdg-open 只会作用于服务端，没有意义，直接判否。
func canOpenBrowser() bool {
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

// openBrowser 用系统默认浏览器打开链接。
//
// 这里必须调外部命令：打开浏览器没有等价的 Go 标准库能力。
func openBrowser(url string) error {
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
