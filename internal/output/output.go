// Package output 负责终端渲染：人类可读格式、JSON、进度条、脱敏、确认。
package output

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"nst/internal/jsonx"
)

// HumanSize 把字节数格式化成 1.5KB 这种形式。
func HumanSize(v any) string {
	var n float64
	switch t := v.(type) {
	case nil:
		return "-"
	case string:
		if t == "" {
			return "-"
		}
		f, err := strconv.ParseFloat(t, 64)
		if err != nil {
			return "-"
		}
		n = f
	default:
		f, ok := jsonx.FloatOf(v)
		if !ok {
			return "-"
		}
		n = f
	}

	units := []string{"B", "KB", "MB", "GB", "TB"}
	for _, unit := range units {
		if n < 1024 || unit == "TB" {
			if unit == "B" {
				return fmt.Sprintf("%.0f%s", n, unit)
			}
			return fmt.Sprintf("%.1f%s", n, unit)
		}
		n /= 1024
	}
	return "-"
}

// FmtTime 把 ISO 时间压成 "YYYY-MM-DD HH:MM:SS"，不做时区换算（避免偏移）。
func FmtTime(v any) string {
	s := jsonx.StrOf(v)
	if s == "" {
		return "-"
	}
	s = strings.ReplaceAll(s, "T", " ")
	if i := strings.Index(s, "+"); i >= 0 {
		s = s[:i]
	}
	if len(s) > 19 {
		s = s[:19]
	}
	return s
}

// DumpJSON 输出缩进 JSON（不转义 HTML 字符，数字保持原样）。
func DumpJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// Entry 是目录列表里的一项。
type Entry struct {
	Name     string
	IsDir    bool
	Size     any
	Modified any
}

// EntriesFrom 把接口返回的 content 数组转成 Entry 列表。
func EntriesFrom(v any) []Entry {
	items := jsonx.Objects(v)
	out := make([]Entry, 0, len(items))
	for _, it := range items {
		out = append(out, Entry{
			Name:     jsonx.Str(it, "name"),
			IsDir:    jsonx.Bool(it, "is_dir"),
			Size:     it["size"],
			Modified: it["modified"],
		})
	}
	return out
}

// PrintEntries 渲染目录列表：目录在前，按名字排序。
func PrintEntries(w io.Writer, entries []Entry, long bool) {
	if len(entries) == 0 {
		fmt.Fprintln(w, "（空）")
		return
	}
	ordered := make([]Entry, len(entries))
	copy(ordered, entries)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].IsDir != ordered[j].IsDir {
			return ordered[i].IsDir
		}
		return ordered[i].Name < ordered[j].Name
	})

	if long {
		for _, it := range ordered {
			kind := "-"
			if it.IsDir {
				kind = "d"
			}
			fmt.Fprintf(w, "%s  %10s  %s  %s\n",
				kind, HumanSize(it.Size), FmtTime(it.Modified), it.Name)
		}
		return
	}
	for _, it := range ordered {
		prefix := "   "
		if it.IsDir {
			prefix = "📁 "
		}
		fmt.Fprintln(w, prefix+it.Name)
	}
}

// Progress 是单行进度条，只在总量已知时显示百分比。
type Progress struct {
	label       string
	w           io.Writer
	minInterval time.Duration
	last        time.Time
	started     bool
	finished    bool
}

// NewProgress 创建进度条；w 通常是 stderr。
func NewProgress(label string, w io.Writer) *Progress {
	return &Progress{label: label, w: w, minInterval: 100 * time.Millisecond}
}

// Update 汇报进度。完成后忽略后续调用，避免重复打印同一行。
func (p *Progress) Update(done, total int64) {
	if p.finished {
		return
	}
	now := time.Now()
	if total > 0 && now.Sub(p.last) < p.minInterval && done < total {
		return
	}
	p.last = now
	p.started = true

	var text string
	if total > 0 {
		pct := done * 100 / total
		text = fmt.Sprintf("%s  %3d%%  %s/%s", p.label, pct, HumanSize(done), HumanSize(total))
	} else {
		text = fmt.Sprintf("%s  %s", p.label, HumanSize(done))
	}
	fmt.Fprintf(p.w, "\r%s\033[K", text)
	if total > 0 && done >= total {
		fmt.Fprintln(p.w)
		p.finished = true
		p.started = false
	}
}

// Done 收尾，避免后续输出被进度条覆盖。
func (p *Progress) Done() {
	if p.started {
		p.started = false
		fmt.Fprintln(p.w)
	}
}

var sensitiveHints = []string{"token", "secret", "cookie", "password", "passwd", "apikey", "api_key"}

// IsSensitive 判断字段名是否属于凭据。
func IsSensitive(key string) bool {
	lowered := strings.ToLower(key)
	for _, hint := range sensitiveHints {
		if strings.Contains(lowered, hint) {
			return true
		}
	}
	return false
}

// MaskSecret 把凭据打码：保留头 6 尾 4。
func MaskSecret(v any, show bool) string {
	text := jsonx.StrOf(v)
	if show || text == "" {
		return text
	}
	if len([]rune(text)) <= 12 {
		return "***"
	}
	r := []rune(text)
	return string(r[:6]) + "…" + string(r[len(r)-4:])
}

// Confirm 询问是否继续。assumeYes 直接通过；非交互环境默认取消。
func Confirm(in io.Reader, errW io.Writer, prompt string, assumeYes, isTTY bool) bool {
	if assumeYes {
		return true
	}
	if !isTTY {
		fmt.Fprintf(errW, "%s [y/N] 非交互环境，默认取消\n", prompt)
		return false
	}
	fmt.Fprintf(errW, "%s [y/N] ", prompt)
	sc := bufio.NewScanner(in)
	if !sc.Scan() {
		fmt.Fprintln(errW)
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(sc.Text()))
	return answer == "y" || answer == "yes"
}
