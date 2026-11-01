package cli

import (
	"os"
	"path"

	"github.com/spf13/cobra"

	"openlist-cli/internal/jsonx"
	"openlist-cli/internal/output"
	"openlist-cli/internal/pathx"
)

type verifyRow struct {
	Status     string
	File       string
	LocalSize  int64
	RemoteSize int64
	HasRemote  bool
	Detail     string
}

type verifyItem struct {
	Status     string `json:"status"`
	File       string `json:"file"`
	LocalSize  int64  `json:"local_size"`
	RemoteSize *int64 `json:"remote_size"`
	Detail     string `json:"detail"`
}

type verifyPayload struct {
	Local      string       `json:"local"`
	Remote     string       `json:"remote"`
	Matched    int          `json:"matched"`
	Mismatched int          `json:"mismatched"`
	Items      []verifyItem `json:"items"`
}

func (a *App) verifyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "verify <local:路径> <remote:路径>",
		Short: "按大小+sha1 比对本地与远端，不下载文件",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			localRoot, err := pathx.Local(args[0])
			if err != nil {
				return err
			}
			remoteRoot, err := pathx.Remote(args[1])
			if err != nil {
				return err
			}
			c, err := a.newClient(true)
			if err != nil {
				return err
			}
			ctx := cmd.Context()

			if _, err := os.Stat(localRoot); err != nil {
				return exitf(1, "本地路径不存在：%s", localRoot)
			}
			remoteInfo, err := c.Stat(ctx, remoteRoot, "")
			if err != nil {
				return exitf(1, "远端路径不存在：%s", remoteRoot)
			}
			remoteIsDir := jsonx.Bool(remoteInfo, "is_dir")

			items, err := iterLocalFiles(localRoot)
			if err != nil {
				return err
			}
			if len(items) == 0 {
				return exitf(1, "本地没有可比对的文件：%s", localRoot)
			}

			listing := map[string]map[string]map[string]any{}
			listingOf := func(dir string) (map[string]map[string]any, error) {
				if m, ok := listing[dir]; ok {
					return m, nil
				}
				data, err := c.ListDir(ctx, dir, "", 1, 0, false)
				if err != nil {
					return nil, err
				}
				m := map[string]map[string]any{}
				for _, it := range jsonx.Objects(data["content"]) {
					m[jsonx.Str(it, "name")] = it
				}
				listing[dir] = m
				return m, nil
			}

			rows := make([]verifyRow, 0, len(items))
			for _, f := range items {
				label := f.rel
				if label == "" {
					label = path.Base(f.abs)
				}

				var (
					remotePath string
					entry      map[string]any
				)
				if remoteIsDir {
					if f.rel == "" {
						remotePath = pathx.JoinRemote(remoteRoot, path.Base(f.abs))
					} else {
						remotePath = pathx.JoinRemote(remoteRoot, f.rel)
					}
					dir, name := pathx.SplitRemote(remotePath)
					m, err := listingOf(dir)
					if err != nil {
						return err
					}
					entry = m[name]
				} else {
					remotePath = remoteRoot
					entry = remoteInfo
				}

				st, err := os.Stat(f.abs)
				if err != nil {
					return err
				}
				localSize := st.Size()

				if entry == nil {
					rows = append(rows, verifyRow{Status: "仅本地", File: label, LocalSize: localSize})
					continue
				}

				remoteSize := jsonx.Int(entry, "size")
				if remoteSize != localSize {
					rows = append(rows, verifyRow{
						Status: "大小不同", File: label,
						LocalSize: localSize, RemoteSize: remoteSize, HasRemote: true,
					})
					continue
				}

				info, err := c.Stat(ctx, remotePath, "")
				if err != nil {
					return err
				}
				remoteSHA := upper(jsonx.Str(jsonx.Object(info["hash_info"]), "sha1"))
				if remoteSHA == "" {
					rows = append(rows, verifyRow{
						Status: "仅大小一致", File: label,
						LocalSize: localSize, RemoteSize: remoteSize, HasRemote: true,
					})
					continue
				}
				localSHA, err := sha1OfFile(f.abs)
				if err != nil {
					return err
				}
				row := verifyRow{
					Status: "内容不同", File: label,
					LocalSize: localSize, RemoteSize: remoteSize, HasRemote: true,
					Detail: "本地 " + short(localSHA) + " 远端 " + short(remoteSHA),
				}
				if localSHA == remoteSHA {
					row.Status = "一致"
					row.Detail = "sha1 ✓"
				}
				rows = append(rows, row)
			}

			bad := 0
			for _, r := range rows {
				if r.Status != "一致" && r.Status != "仅大小一致" {
					bad++
				}
			}

			if a.jsonOut {
				payload := verifyPayload{
					Local:      localRoot,
					Remote:     remoteRoot,
					Matched:    len(rows) - bad,
					Mismatched: bad,
					Items:      make([]verifyItem, 0, len(rows)),
				}
				for _, r := range rows {
					item := verifyItem{
						Status: r.Status, File: r.File,
						LocalSize: r.LocalSize, Detail: r.Detail,
					}
					if r.HasRemote {
						size := r.RemoteSize
						item.RemoteSize = &size
					}
					payload.Items = append(payload.Items, item)
				}
				if err := output.DumpJSON(a.Out, payload); err != nil {
					return err
				}
			} else {
				width := 0
				for _, r := range rows {
					if n := dispWidth(r.File); n > width {
						width = n
					}
				}
				// 状态列按显示宽度对齐（中文占两列）
				for _, r := range rows {
					mark := "✗"
					if r.Status == "一致" || r.Status == "仅大小一致" {
						mark = "✓"
					}
					remoteSize := "—"
					if r.HasRemote {
						remoteSize = output.HumanSize(r.RemoteSize)
					}
					pad := width - dispWidth(r.File)
					line := mark + " " + padRight(r.Status, 8) + " " + r.File +
						repeat(" ", pad) + "  " +
						padLeft(output.HumanSize(r.LocalSize), 8) + " / " +
						padLeft(remoteSize, 8) + "  " + r.Detail
					a.println(trimRightSpaces(line))
				}
				a.println()
				a.printf("共 %d 项：一致 %d，异常 %d\n", len(rows), len(rows)-bad, bad)
			}

			if bad > 0 {
				return exitf(1, "")
			}
			return nil
		},
	}
}

func upper(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'a' && b[i] <= 'z' {
			b[i] -= 32
		}
	}
	return string(b)
}

func short(sha string) string {
	if len(sha) > 10 {
		return sha[:10]
	}
	return sha
}

func repeat(s string, n int) string {
	if n <= 0 {
		return ""
	}
	out := make([]byte, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, s...)
	}
	return string(out)
}

func padRight(s string, width int) string {
	n := width - dispWidth(s)
	if n <= 0 {
		return s
	}
	return s + repeat(" ", n)
}

func padLeft(s string, width int) string {
	n := width - dispWidth(s)
	if n <= 0 {
		return s
	}
	return repeat(" ", n) + s
}

func trimRightSpaces(s string) string {
	i := len(s)
	for i > 0 && s[i-1] == ' ' {
		i--
	}
	return s[:i]
}

// dispWidth 计算终端显示宽度：中日韩字符占两列。
func dispWidth(s string) int {
	w := 0
	for _, r := range s {
		if isWideRune(r) {
			w += 2
		} else {
			w++
		}
	}
	return w
}

func isWideRune(r rune) bool {
	switch {
	case r >= 0x1100 && r <= 0x115F,
		r >= 0x2E80 && r <= 0xA4CF,
		r >= 0xAC00 && r <= 0xD7A3,
		r >= 0xF900 && r <= 0xFAFF,
		r >= 0xFE30 && r <= 0xFE6F,
		r >= 0xFF00 && r <= 0xFF60,
		r >= 0xFFE0 && r <= 0xFFE6:
		return true
	}
	return false
}
