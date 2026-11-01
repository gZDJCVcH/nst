package cli

import (
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"openlist-cli/internal/jsonx"
	"openlist-cli/internal/output"
)

var storageFieldOrder = []string{
	"id", "mount_path", "driver", "status", "disabled", "order", "remark",
	"web_proxy", "webdav_policy", "proxy_range", "down_proxy_url",
	"enable_sign", "disable_proxy_sign", "cache_expiration",
	"custom_cache_policies", "disable_index", "extract_folder",
	"seed_policy", "order_by", "order_direction", "modified",
}

func (a *App) driversCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "drivers",
		Short: "列出支持的存储驱动",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := a.newClient(true)
			if err != nil {
				return err
			}
			items, err := c.Drivers(cmd.Context())
			if err != nil {
				return err
			}
			names := make([]string, 0, len(items))
			for name := range items {
				names = append(names, name)
			}
			sort.Strings(names)

			return a.emit(items, func() {
				for _, name := range names {
					spec := jsonx.Object(items[name])
					extra := len(jsonx.List(spec["additional"]))
					a.printf("%-28s 专属配置项 %d 个\n", name, extra)
				}
			})
		},
	}
}

func (a *App) mountsCmd() *cobra.Command {
	var refresh bool
	cmd := &cobra.Command{
		Use:   "mounts",
		Short: "列出挂载点、驱动与限流间隔",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := a.newClient(true)
			if err != nil {
				return err
			}
			items := c.Mounts(cmd.Context(), refresh)

			payload := make([]map[string]any, 0, len(items))
			for _, m := range items {
				payload = append(payload, map[string]any{"mount_path": m.Path, "driver": m.Driver})
			}

			return a.emit(payload, func() {
				if len(items) == 0 {
					a.println("拿不到存储列表（需要管理员权限）；所有请求将按网盘间隔限流")
					return
				}
				a.printf("%-16s%-16s%s\n", "挂载点", "驱动", "限流间隔")
				for _, m := range items {
					_, _, interval := c.DescribeTarget(m.Path)
					flag := "不限"
					if interval >= 200*time.Millisecond {
						flag = fmt.Sprintf("%.2fs", interval.Seconds())
					}
					a.printf("%-16s%-16s%s\n", m.Path, m.Driver, flag)
				}
			})
		},
	}
	cmd.Flags().BoolVar(&refresh, "refresh", false, "忽略本地缓存重新拉取")
	return cmd
}

func (a *App) storageCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "storage",
		Short: "存储只读查询",
	}
	cmd.AddCommand(a.storageGetCmd())
	return cmd
}

func (a *App) storageGetCmd() *cobra.Command {
	var showSecrets bool
	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "查看单个存储详情",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.Atoi(args[0])
			if err != nil {
				return exitf(1, "存储 id 必须是整数：%q", args[0])
			}
			c, err := a.newClient(true)
			if err != nil {
				return err
			}
			info, err := c.StorageGet(cmd.Context(), id)
			if err != nil {
				return err
			}
			if len(info) == 0 {
				return exitf(1, "没有 id=%d 的存储", id)
			}

			addition := maskedAddition(jsonx.Str(info, "addition"), showSecrets)

			if a.jsonOut {
				payload := make(map[string]any, len(info))
				for k, v := range info {
					payload[k] = v
				}
				if addition != nil {
					payload["addition"] = addition
				}
				return output.DumpJSON(a.Out, payload)
			}

			seen := map[string]bool{}
			for _, key := range storageFieldOrder {
				value, ok := info[key]
				if !ok {
					continue
				}
				seen[key] = true
				a.printf("%-22s %s\n", key, renderField(value))
			}
			rest := make([]string, 0, len(info))
			for k := range info {
				if seen[k] || k == "addition" {
					continue
				}
				rest = append(rest, k)
			}
			sort.Strings(rest)
			for _, key := range rest {
				if v := info[key]; v == nil || jsonx.StrOf(v) == "" {
					continue
				}
				a.printf("%-22s %s\n", key, renderField(info[key]))
			}

			switch {
			case addition != nil:
				a.println("\naddition:")
				keys := make([]string, 0, len(addition))
				for k := range addition {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					a.printf("  %-22s %s\n", k, renderField(addition[k]))
				}
			case jsonx.Str(info, "addition") != "":
				a.printf("\naddition（无法解析为 JSON）:\n  %s\n", jsonx.Str(info, "addition"))
			default:
				a.println("\naddition:（空）")
			}

			if !showSecrets {
				a.println("\n敏感字段已打码，如需原值加 --show-secrets")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&showSecrets, "show-secrets", false, "不脱敏，显示 token 等原值（谨慎）")
	return cmd
}

// renderField 把字段值转成适合终端的一行文本。
func renderField(v any) string {
	if b, ok := v.(bool); ok {
		if b {
			return "是"
		}
		return "否"
	}
	return scalar(v)
}

func (a *App) driverCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "driver",
		Short: "驱动信息",
	}
	cmd.AddCommand(a.driverInfoCmd())
	return cmd
}

func (a *App) driverInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info <驱动名>",
		Short: "某个驱动的配置项定义",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := a.newClient(true)
			if err != nil {
				return err
			}
			data, err := c.DriverInfo(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if len(data) == 0 {
				return exitf(1, "没有名为 %s 的驱动（先用 drivers 看列表）", args[0])
			}

			if a.jsonOut {
				return output.DumpJSON(a.Out, data)
			}

			for _, group := range []struct{ key, label string }{
				{"common", "通用配置"},
				{"additional", "驱动专属配置"},
			} {
				items := jsonx.Objects(data[group.key])
				a.printf("\n【%s】%d 项\n", group.label, len(items))
				if len(items) == 0 {
					continue
				}
				width := 0
				for _, it := range items {
					if n := len([]rune(jsonx.Str(it, "name"))); n > width {
						width = n
					}
				}
				for _, it := range items {
					name := jsonx.Str(it, "name")
					kind := jsonx.Str(it, "type")
					need := "可选"
					if jsonx.Bool(it, "required") {
						need = "必填"
					}
					tail := ""
					if def := it["default"]; def != nil && jsonx.StrOf(def) != "" {
						tail = "  默认=" + jsonx.StrOf(def)
					}
					if opts := it["options"]; opts != nil && jsonx.StrOf(opts) != "" {
						tail += "  可选: " + jsonx.StrOf(opts)
					}
					a.printf("  %-*s  %-8s %s%s\n", width, name, kind, need, tail)
				}
			}

			if data["config"] != nil {
				a.println("\n（config 结构较大，用 --json 查看）")
			}
			return nil
		},
	}
}

func (a *App) configCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "本地配置",
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "show",
			Short: "查看配置（令牌已打码）",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				cfg, err := loadConfigMasked()
				if err != nil {
					return err
				}
				return output.DumpJSON(a.Out, cfg)
			},
		},
		&cobra.Command{
			Use:   "path",
			Short: "配置文件路径",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				a.println(configFile())
				return nil
			},
		},
		&cobra.Command{
			Use:   "set <键> <值>",
			Short: "写入一个键",
			Args:  cobra.ExactArgs(2),
			RunE: func(cmd *cobra.Command, args []string) error {
				cfg, err := loadConfig()
				if err != nil {
					return err
				}
				cfg[args[0]] = args[1]
				saved, err := saveConfig(cfg)
				if err != nil {
					return err
				}
				a.printf("已写入 %s\n", saved)
				return nil
			},
		},
	)
	return cmd
}
