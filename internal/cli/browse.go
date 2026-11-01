package cli

import (
	"os"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"openlist-cli/internal/jsonx"
	"openlist-cli/internal/output"
	"openlist-cli/internal/pathx"
)

type listPayload struct {
	Path    string `json:"path"`
	Total   any    `json:"total"`
	Content any    `json:"content"`
}

func (a *App) listCmd() *cobra.Command {
	var (
		long     bool
		page     int
		perPage  int
		password string
		refresh  bool
	)
	cmd := &cobra.Command{
		Use:     "list <remote:路径>",
		Aliases: []string{"ls"},
		Short:   "列目录",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target, err := pathx.Remote(args[0])
			if err != nil {
				return err
			}
			c, err := a.newClient(true)
			if err != nil {
				return err
			}
			data, err := c.ListDir(cmd.Context(), target, password, page, perPage, refresh)
			if err != nil {
				return err
			}
			entries := jsonx.Objects(data["content"])
			payload := listPayload{Path: target, Total: data["total"], Content: data["content"]}
			return a.emit(payload, func() {
				if long {
					a.printf("目录：%s  （%d 项）\n", target, len(entries))
				}
				output.PrintEntries(a.Out, output.EntriesFrom(data["content"]), long)
			})
		},
	}
	cmd.Flags().BoolVarP(&long, "long", "l", false, "详细信息")
	cmd.Flags().IntVar(&page, "page", 1, "页码")
	cmd.Flags().IntVar(&perPage, "per-page", 0, "每页数量，0 表示不限")
	cmd.Flags().StringVar(&password, "password", "", "目录密码")
	cmd.Flags().BoolVar(&refresh, "refresh", false, "强刷缓存")
	return cmd
}

func (a *App) statCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stat <remote:路径>",
		Short: "文件/目录详情",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target, err := pathx.Remote(args[0])
			if err != nil {
				return err
			}
			c, err := a.newClient(true)
			if err != nil {
				return err
			}
			info, err := c.Stat(cmd.Context(), target, "")
			if err != nil {
				return err
			}
			return a.emit(info, func() {
				a.printf("名称  ：%s\n", jsonx.Str(info, "name"))
				kind := "文件"
				if jsonx.Bool(info, "is_dir") {
					kind = "目录"
				}
				a.printf("类型  ：%s\n", kind)
				if !jsonx.Bool(info, "is_dir") {
					a.printf("大小  ：%s\n", output.HumanSize(info["size"]))
				}
				a.printf("修改  ：%s\n", output.FmtTime(info["modified"]))
				if jsonx.Str(info, "sign") != "" || jsonx.Str(info, "raw_url") != "" {
					a.println("有直链：是")
				}
			})
		},
	}
}

func (a *App) treeCmd() *cobra.Command {
	var maxDepth int
	cmd := &cobra.Command{
		Use:   "tree <remote:路径>",
		Short: "树形展开",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := pathx.Remote(args[0])
			if err != nil {
				return err
			}
			c, err := a.newClient(true)
			if err != nil {
				return err
			}
			depthLimit := -1
			if cmd.Flags().Changed("depth") {
				depthLimit = maxDepth
			}

			var walk func(p, prefix string, depth int) error
			walk = func(p, prefix string, depth int) error {
				data, err := c.ListDir(cmd.Context(), p, "", 1, 0, false)
				if err != nil {
					return err
				}
				items := jsonx.Objects(data["content"])
				sort.SliceStable(items, func(i, j int) bool {
					di, dj := jsonx.Bool(items[i], "is_dir"), jsonx.Bool(items[j], "is_dir")
					if di != dj {
						return di
					}
					return jsonx.Str(items[i], "name") < jsonx.Str(items[j], "name")
				})
				for i, it := range items {
					last := i == len(items)-1
					branch := "├── "
					if last {
						branch = "└── "
					}
					name := jsonx.Str(it, "name")
					a.printf("%s%s%s\n", prefix, branch, name)
					if jsonx.Bool(it, "is_dir") && (depthLimit < 0 || depth+1 < depthLimit) {
						childPrefix := prefix + "│   "
						if last {
							childPrefix = prefix + "    "
						}
						if err := walk(pathx.JoinRemote(p, name), childPrefix, depth+1); err != nil {
							return err
						}
					}
				}
				return nil
			}

			a.println(root)
			return walk(root, "", 0)
		},
	}
	cmd.Flags().IntVarP(&maxDepth, "depth", "L", 0, "最大深度")
	return cmd
}

func (a *App) searchCmd() *cobra.Command {
	var (
		parent  string
		scope   int
		page    int
		perPage int
	)
	cmd := &cobra.Command{
		Use:   "search <关键词>",
		Short: "搜索",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := "/"
			if parent != "" {
				p, err := pathx.Remote(parent)
				if err != nil {
					return err
				}
				root = p
			}
			c, err := a.newClient(true)
			if err != nil {
				return err
			}
			data, err := c.Search(cmd.Context(), args[0], root, scope, page, perPage, "")
			if err != nil {
				return err
			}
			entries := jsonx.Objects(data["content"])
			return a.emit(data, func() {
				for _, it := range entries {
					mark := "  "
					if jsonx.Bool(it, "is_dir") {
						mark = "📁"
					}
					a.printf("%s %9s  %s\n", mark,
						output.HumanSize(it["size"]),
						relJoin(jsonx.Str(it, "parent"), jsonx.Str(it, "name")))
				}
				if len(entries) == 0 {
					a.println("（无匹配）")
				}
			})
		},
	}
	cmd.Flags().StringVar(&parent, "path", "", "限定目录，如 remote:/音乐")
	cmd.Flags().IntVar(&scope, "scope", 0, "0 全部 1 仅目录 2 仅文件")
	cmd.Flags().IntVar(&page, "page", 1, "页码")
	cmd.Flags().IntVar(&perPage, "per-page", 100, "每页数量")
	return cmd
}

func (a *App) readCmd() *cobra.Command {
	var (
		maxBytes int64
		encoding string
		force    bool
	)
	cmd := &cobra.Command{
		Use:   "read <remote:路径>",
		Short: "读取文本文件到 stdout",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target, err := pathx.Remote(args[0])
			if err != nil {
				return err
			}
			normalized := strings.ToLower(strings.ReplaceAll(encoding, "-", ""))
			if normalized != "utf8" {
				return exitf(1, "目前只支持 --encoding utf-8（收到 %q）；"+
					"需要其他编码请先用 iconv 转换文件", encoding)
			}

			c, err := a.newClient(true)
			if err != nil {
				return err
			}
			info, err := c.Stat(cmd.Context(), target, "")
			if err != nil {
				return err
			}
			if jsonx.Bool(info, "is_dir") {
				return exitf(1, "这是一个目录，不能 read")
			}
			size := jsonx.Int(info, "size")
			if size > maxBytes && !force {
				return exitf(1, "文件 %s 超过阈值 %s，加 --force 继续",
					output.HumanSize(size), output.HumanSize(maxBytes))
			}

			tmp, err := os.CreateTemp("", "openlist-cli-read-*")
			if err != nil {
				return err
			}
			tmpName := tmp.Name()
			tmp.Close()
			defer os.Remove(tmpName)

			if err := c.Fetch(cmd.Context(), target, tmpName, nil); err != nil {
				return err
			}
			body, err := os.ReadFile(tmpName)
			if err != nil {
				return err
			}
			body = toValidUTF8(body)
			_, err = a.Out.Write(body)
			return err
		},
	}
	cmd.Flags().Int64Var(&maxBytes, "max-bytes", 2<<20, "允许读取的最大字节数")
	cmd.Flags().StringVar(&encoding, "encoding", "utf-8", "文本编码（目前仅支持 utf-8）")
	cmd.Flags().BoolVar(&force, "force", false, "超过阈值也继续")
	return cmd
}

// toValidUTF8 把非法字节替换成 U+FFFD，而不是直接报错。
func toValidUTF8(b []byte) []byte {
	if utf8.Valid(b) {
		return b
	}
	out := make([]byte, 0, len(b)+8)
	for len(b) > 0 {
		r, size := utf8.DecodeRune(b)
		if r == utf8.RuneError && size == 1 {
			out = append(out, "\uFFFD"...)
			b = b[1:]
			continue
		}
		out = append(out, b[:size]...)
		b = b[size:]
	}
	return out
}

func (a *App) linkCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "link <remote:路径>",
		Short: "打印直链",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target, err := pathx.Remote(args[0])
			if err != nil {
				return err
			}
			c, err := a.newClient(true)
			if err != nil {
				return err
			}
			url, err := c.DownloadURL(cmd.Context(), target)
			if err != nil {
				return err
			}
			a.println(url)
			return nil
		},
	}
}
