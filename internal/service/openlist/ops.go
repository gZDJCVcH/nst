package openlist

import (
	"context"
	"path"
	"strings"

	"github.com/spf13/cobra"

	"nst/internal/jsonx"
	"nst/internal/pathx"
)

// resolveDst 判断 move/copy 的目标是目录还是完整路径。
func (a *CLI) resolveDst(ctx context.Context, c *Client, src, dstRaw string) (string, error) {
	dstPath, err := pathx.Remote(dstRaw)
	if err != nil {
		return "", err
	}
	name := path.Base(pathx.NormalizeRemote(src))

	if strings.HasSuffix(dstRaw, "/") || dstPath == "/" {
		return pathx.JoinRemote(dstPath, name), nil
	}
	if c.Exists(ctx, dstPath) {
		if info, err := c.Stat(ctx, dstPath, ""); err == nil && jsonx.Bool(info, "is_dir") {
			return pathx.JoinRemote(dstPath, name), nil
		}
	}
	return dstPath, nil
}

func (a *CLI) mkdirCmd() *cobra.Command {
	var parents bool
	cmd := &cobra.Command{
		Use:   "mkdir <remote:路径>",
		Short: "建目录",
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
			ctx := cmd.Context()
			if parents {
				if err := mkdirP(c, ctx, target); err != nil {
					return err
				}
			} else {
				if err := c.Mkdir(ctx, target); err != nil {
					return err
				}
				c.Invalidate(target)
			}
			a.Printf("✓ %s\n", target)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&parents, "parents", "p", false, "自动创建父目录")
	return cmd
}

func (a *CLI) moveCmd() *cobra.Command {
	var noOverwrite bool
	cmd := &cobra.Command{
		Use:     "move <remote:源> <remote:目标>",
		Aliases: []string{"mv"},
		Short:   "移动/改名",
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			src, err := pathx.Remote(args[0])
			if err != nil {
				return err
			}
			c, err := a.newClient(true)
			if err != nil {
				return err
			}
			ctx := cmd.Context()

			dst, err := a.resolveDst(ctx, c, src, args[1])
			if err != nil {
				return err
			}
			srcParent, srcName := pathx.SplitRemote(src)
			dstParent, dstName := pathx.SplitRemote(dst)

			if srcParent == dstParent && srcName == dstName {
				a.Println("源与目标相同，什么都没做")
				return nil
			}

			if err := c.Move(ctx, src, dstParent, !noOverwrite); err != nil {
				return err
			}
			if dstName != srcName {
				if err := c.Rename(ctx, pathx.JoinRemote(dstParent, srcName), dstName); err != nil {
					return err
				}
			}
			a.Printf("✓ %s → %s\n", src, dst)
			return nil
		},
	}
	cmd.Flags().BoolVar(&noOverwrite, "no-overwrite", false, "目标存在时不覆盖")
	return cmd
}

func (a *CLI) copyCmd() *cobra.Command {
	var noOverwrite bool
	cmd := &cobra.Command{
		Use:     "copy <remote:源> <remote:目标>",
		Aliases: []string{"cp"},
		Short:   "复制",
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			src, err := pathx.Remote(args[0])
			if err != nil {
				return err
			}
			c, err := a.newClient(true)
			if err != nil {
				return err
			}
			ctx := cmd.Context()

			dst, err := a.resolveDst(ctx, c, src, args[1])
			if err != nil {
				return err
			}
			dstParent, _ := pathx.SplitRemote(dst)
			if err := c.Copy(ctx, src, dstParent, !noOverwrite); err != nil {
				return err
			}
			a.Printf("✓ %s → %s\n", src, dst)
			return nil
		},
	}
	cmd.Flags().BoolVar(&noOverwrite, "no-overwrite", false, "目标存在时不覆盖")
	return cmd
}

func (a *CLI) renameCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rename <remote:路径> <新名字>",
		Short: "重命名",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			src, err := pathx.Remote(args[0])
			if err != nil {
				return err
			}
			c, err := a.newClient(true)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			if err := c.Rename(ctx, src, args[1]); err != nil {
				return err
			}
			parent, _ := pathx.SplitRemote(src)
			a.Printf("✓ %s → %s\n", src, pathx.JoinRemote(parent, args[1]))
			return nil
		},
	}
}
