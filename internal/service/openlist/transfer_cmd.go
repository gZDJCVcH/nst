package openlist

import (
	"context"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"nst/internal/apperr"
	"nst/internal/jsonx"
	"nst/internal/output"
	"nst/internal/pathx"
)

func (a *CLI) uploadCmd() *cobra.Command {
	var (
		recursive   bool
		skipSame    bool
		noOverwrite bool
		asTask      bool
		stopOnError bool
		quiet       bool
	)
	cmd := &cobra.Command{
		Use:     "upload <local:路径>... <remote:目录>",
		Aliases: []string{"put"},
		Short:   "上传/覆盖",
		Args:    cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			remoteArg := args[len(args)-1]
			localArgs := args[:len(args)-1]

			remoteDir, err := pathx.Remote(remoteArg)
			if err != nil {
				return err
			}

			var files []localFile
			for _, raw := range localArgs {
				p, err := pathx.Local(raw)
				if err != nil {
					return err
				}
				got, err := iterUploadCandidates(p, recursive)
				if err != nil {
					return err
				}
				files = append(files, got...)
			}
			if len(files) == 0 {
				return apperr.Exitf(1, "没有可上传的文件")
			}

			c, err := a.newClient(true)
			if err != nil {
				return err
			}
			ctx := cmd.Context()

			var uploaded, skipped, failed int
			for _, f := range files {
				targetDir := remoteDir
				if f.rel != "" {
					targetDir = pathx.JoinRemote(remoteDir, path.Dir(f.rel))
					if err := mkdirP(c, ctx, targetDir); err != nil {
						a.Errorf("✗ 建目录失败 %s：%v\n", targetDir, err)
						failed++
						if stopOnError {
							break
						}
						continue
					}
				}

				name := path.Base(f.abs)
				target := pathx.JoinRemote(targetDir, name)

				if skipSame && c.Exists(ctx, target) {
					if info, statErr := c.Stat(ctx, target, ""); statErr == nil {
						if st, err := os.Stat(f.abs); err == nil && jsonx.Int(info, "size") == st.Size() {
							a.Printf("· 跳过（大小相同）：%s\n", target)
							skipped++
							continue
						}
					}
				} else if noOverwrite && c.Exists(ctx, target) {
					a.Printf("· 跳过（已存在）：%s\n", target)
					skipped++
					continue
				}

				var progress *output.Progress
				var cb func(int64, int64)
				if !quiet {
					progress = output.NewProgress("↑ "+name, a.Err)
					cb = progress.Update
				}

				uploadErr := c.Upload(ctx, f.abs, targetDir, name, asTask, cb)
				if progress != nil {
					progress.Done()
				}
				if uploadErr != nil {
					a.Errorf("✗ %s：%v\n", target, uploadErr)
					failed++
					if stopOnError {
						break
					}
					continue
				}

				c.Invalidate(targetDir)
				if st, err := os.Stat(f.abs); err == nil {
					a.Printf("✓ %s  (%s)\n", target, output.HumanSize(st.Size()))
				} else {
					a.Printf("✓ %s\n", target)
				}
				uploaded++
			}

			a.Printf("\n完成：成功 %d，跳过 %d，失败 %d\n", uploaded, skipped, failed)
			if failed > 0 {
				return apperr.Exitf(1, "")
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&recursive, "recursive", "r", false, "递归上传目录")
	cmd.Flags().BoolVar(&skipSame, "skip-same", false, "目标同名且大小相同则跳过")
	cmd.Flags().BoolVar(&noOverwrite, "no-overwrite", false, "目标存在则跳过")
	cmd.Flags().BoolVar(&asTask, "as-task", false, "转异步任务")
	cmd.Flags().BoolVar(&stopOnError, "stop-on-error", false, "遇错停止")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "不显示进度")
	return cmd
}

func (a *CLI) downloadCmd() *cobra.Command {
	var (
		recursive bool
		quiet     bool
	)
	cmd := &cobra.Command{
		Use:     "download <remote:路径> [local:路径]",
		Aliases: []string{"get"},
		Short:   "下载",
		Args:    cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			remotePath, err := pathx.Remote(args[0])
			if err != nil {
				return err
			}

			localPath := path.Base(remotePath)
			if len(args) == 2 {
				localPath, err = pathx.Local(args[1])
				if err != nil {
					return err
				}
			}

			c, err := a.newClient(true)
			if err != nil {
				return err
			}
			ctx := cmd.Context()

			info, err := c.Stat(ctx, remotePath, "")
			if err != nil {
				return err
			}

			if jsonx.Bool(info, "is_dir") {
				if !recursive {
					return apperr.Exitf(1, "这是目录，下载目录需要 -r")
				}
				if err := os.MkdirAll(localPath, 0o755); err != nil {
					return err
				}
				count, err := a.downloadTree(ctx, c, remotePath, localPath, quiet)
				if err != nil {
					return err
				}
				a.Printf("完成：%d 个文件\n", count)
				return nil
			}

			if fi, err := os.Stat(localPath); err == nil && fi.IsDir() {
				localPath = path.Join(localPath, path.Base(remotePath))
			}
			if dir := path.Dir(localPath); dir != "" && dir != "." {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					return err
				}
			}

			var cb func(int64, int64)
			var progress *output.Progress
			if !quiet {
				progress = output.NewProgress("↓ "+path.Base(remotePath), a.Err)
				cb = progress.Update
			}
			err = c.Fetch(ctx, remotePath, localPath, cb)
			if progress != nil {
				progress.Done()
			}
			if err != nil {
				return err
			}
			a.Printf("✓ %s\n", localPath)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&recursive, "recursive", "r", false, "递归下载目录")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "不显示进度")
	return cmd
}

func (a *CLI) downloadTree(ctx context.Context, c *Client, remoteDir, localDir string, quiet bool) (int, error) {
	data, err := c.ListDir(ctx, remoteDir, "", 1, 0, false)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, it := range jsonx.Objects(data["content"]) {
		name := jsonx.Str(it, "name")
		rpath := pathx.JoinRemote(remoteDir, name)
		lpath := path.Join(localDir, name)
		if jsonx.Bool(it, "is_dir") {
			if err := os.MkdirAll(lpath, 0o755); err != nil {
				return count, err
			}
			n, err := a.downloadTree(ctx, c, rpath, lpath, quiet)
			if err != nil {
				return count, err
			}
			count += n
			continue
		}

		var cb func(int64, int64)
		var progress *output.Progress
		if !quiet {
			progress = output.NewProgress("↓ "+name, a.Err)
			cb = progress.Update
		}
		err := c.Fetch(ctx, rpath, lpath, cb)
		if progress != nil {
			progress.Done()
		}
		if err != nil {
			return count, err
		}
		a.Printf("✓ %s\n", lpath)
		count++
	}
	return count, nil
}

func (a *CLI) removeCmd() *cobra.Command {
	var (
		recursive bool
		yes       bool
	)
	cmd := &cobra.Command{
		Use:     "remove <remote:路径>...",
		Aliases: []string{"rm"},
		Short:   "删除",
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := a.newClient(true)
			if err != nil {
				return err
			}
			ctx := cmd.Context()

			type planItem struct {
				kind     string
				target   string
				children []map[string]any
			}
			var plan []planItem

			for _, raw := range args {
				target, err := pathx.Remote(raw)
				if err != nil {
					return err
				}
				if target == "/" {
					return apperr.Exitf(1, "拒绝删除根目录")
				}
				info, err := c.Stat(ctx, target, "")
				if err != nil {
					return err
				}
				if jsonx.Bool(info, "is_dir") {
					data, err := c.ListDir(ctx, target, "", 1, 0, false)
					if err != nil {
						return err
					}
					children := jsonx.Objects(data["content"])
					if len(children) > 0 && !recursive {
						return apperr.Exitf(1, "目录非空：%s（%d 项）\n  递归删除请加 -r", target, len(children))
					}
					plan = append(plan, planItem{kind: "dir", target: target, children: children})
				} else {
					plan = append(plan, planItem{kind: "file", target: target})
				}
			}

			if !yes {
				var sb strings.Builder
				for _, item := range plan {
					kind := "文件"
					if item.kind == "dir" {
						kind = "目录"
					}
					sb.WriteString("  - " + item.target + "（" + kind + "）\n")
				}
				prompt := "确认删除以下 " + strconv.Itoa(len(plan)) + " 项？\n" + strings.TrimRight(sb.String(), "\n")
				if !output.Confirm(a.In, a.Err, prompt, false, a.IsTTY()) {
					a.Println("已取消")
					return apperr.Exitf(1, "")
				}
			}

			removed := 0
			for _, item := range plan {
				if item.kind == "dir" {
					n, err := a.removeTree(ctx, c, item.target)
					if err != nil {
						return err
					}
					removed += n
					continue
				}
				if err := c.Remove(ctx, item.target); err != nil {
					return err
				}
				c.Invalidate(item.target)
				removed++
				a.Printf("✓ 已删除 %s\n", item.target)
			}
			a.Printf("完成：共删除 %d 项\n", removed)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&recursive, "recursive", "r", false, "递归删除非空目录")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "跳过确认")
	return cmd
}

func (a *CLI) removeTree(ctx context.Context, c *Client, p string) (int, error) {
	data, err := c.ListDir(ctx, p, "", 1, 0, false)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, it := range jsonx.Objects(data["content"]) {
		child := pathx.JoinRemote(p, jsonx.Str(it, "name"))
		if jsonx.Bool(it, "is_dir") {
			n, err := a.removeTree(ctx, c, child)
			if err != nil {
				return count, err
			}
			count += n
			continue
		}
		if err := c.Remove(ctx, child); err != nil {
			return count, err
		}
		count++
	}
	if err := c.Remove(ctx, p); err != nil {
		return count, err
	}
	a.Printf("✓ 已删除 %s\n", p)
	return count + 1, nil
}
