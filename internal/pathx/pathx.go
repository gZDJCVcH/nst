// Package pathx 处理 local: / remote: 前缀路径。
//
// 设计约束：路径必须显式带前缀，绝不智能推断。
// 远端路径一律用 path 包处理，不能用 filepath —— 后者在 Windows 上会把 / 变成 \。
package pathx

import (
	"fmt"
	"path"
	"strings"
)

const (
	LocalPrefix  = "local:"
	RemotePrefix = "remote:"
)

// PathError 表示路径书写不合规范。
type PathError struct {
	Msg string
}

func (e *PathError) Error() string { return e.Msg }

func pathErrorf(format string, args ...any) *PathError {
	return &PathError{Msg: fmt.Sprintf(format, args...)}
}

// Parse 把带前缀的路径拆成 (kind, 路径)。kind 取 "local" 或 "remote"。
func Parse(raw string) (kind, p string, err error) {
	if raw == "" {
		return "", "", pathErrorf("路径不能为空，需要 local:<路径> 或 remote:/<路径>")
	}

	lowered := strings.ToLower(raw)
	if strings.HasPrefix(lowered, LocalPrefix) {
		rest := raw[len(LocalPrefix):]
		if rest == "" {
			return "", "", pathErrorf("local: 后面缺少路径")
		}
		return "local", rest, nil
	}

	if strings.HasPrefix(lowered, RemotePrefix) {
		rest := raw[len(RemotePrefix):]
		if rest == "" {
			return "remote", "/", nil
		}
		if !strings.HasPrefix(rest, "/") {
			return "", "", pathErrorf(
				"remote: 后面必须是绝对路径（以 / 开头），收到 %q\n  正确写法：remote:/音乐/周杰伦", rest)
		}
		return "remote", NormalizeRemote(rest), nil
	}

	hint := suggest(raw)
	if hint != "" {
		hint = "\n  " + hint
	}
	return "", "", pathErrorf(
		"路径必须带前缀，收到 %q\n  本地文件：%s%s\n  远程路径：%s/%s%s",
		raw, LocalPrefix, raw, RemotePrefix, strings.TrimLeft(raw, "/"), hint)
}

func suggest(raw string) string {
	if strings.HasPrefix(raw, "/") && !strings.HasPrefix(raw, "//") {
		return "（以 / 开头并不代表远程，请显式写 remote: 或 local:）"
	}
	return ""
}

// Remote 解析并要求是远端路径。
func Remote(raw string) (string, error) {
	kind, p, err := Parse(raw)
	if err != nil {
		return "", err
	}
	if kind != "remote" {
		return "", pathErrorf("这里需要远程路径，收到 %q", raw)
	}
	return p, nil
}

// Local 解析并要求是本地路径。
func Local(raw string) (string, error) {
	kind, p, err := Parse(raw)
	if err != nil {
		return "", err
	}
	if kind != "local" {
		return "", pathErrorf("这里需要本地路径，收到 %q", raw)
	}
	return p, nil
}

// NormalizeRemote 折叠重复斜杠、去掉尾部斜杠（根目录除外）。
func NormalizeRemote(p string) string {
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	for strings.Contains(p, "//") {
		p = strings.ReplaceAll(p, "//", "/")
	}
	p = path.Clean(p)
	if p == "." {
		return "/"
	}
	return p
}

// JoinRemote 在远程目录下拼一个文件名。
func JoinRemote(dir, name string) string {
	dir = NormalizeRemote(dir)
	if dir == "/" {
		return "/" + name
	}
	return dir + "/" + name
}

// SplitRemote 拆成 (父目录, 名字)。根目录返回 ("/", "")。
func SplitRemote(p string) (dir, name string) {
	p = NormalizeRemote(p)
	if p == "/" {
		return "/", ""
	}
	return path.Dir(p), path.Base(p)
}

// escape 按 RFC 3986 unreserved 规则做百分号编码：只有 A-Z a-z 0-9 - . _ ~
// 保留，其余（含 + = : 空格 中文）全部编码。
func escape(s string, keepSlash bool) string {
	const upperhex = "0123456789ABCDEF"
	var b strings.Builder
	b.Grow(len(s) * 3)
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case 'A' <= c && c <= 'Z', 'a' <= c && c <= 'z', '0' <= c && c <= '9',
			c == '-', c == '.', c == '_', c == '~':
			b.WriteByte(c)
		case c == '/' && keepSlash:
			b.WriteByte(c)
		default:
			b.WriteByte('%')
			b.WriteByte(upperhex[c>>4])
			b.WriteByte(upperhex[c&0x0f])
		}
	}
	return b.String()
}

// QuoteForHeader 把远程路径转成可放进 HTTP 头的百分号编码（保留 /）。
func QuoteForHeader(p string) string {
	return escape(NormalizeRemote(p), true)
}

// QuoteComponent 编码单个片段（如 sign），不允许任何保留字符。
func QuoteComponent(s string) string {
	return escape(s, false)
}
