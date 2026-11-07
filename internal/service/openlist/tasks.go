package openlist

import (
	"fmt"

	"github.com/spf13/cobra"

	"nst/internal/apierr"
	"nst/internal/apperr"
	"nst/internal/jsonx"
)

// taskStates 是服务端任务状态码。
var taskStates = map[int64]string{
	0: "等待", 1: "进行中", 2: "成功", 3: "失败", 4: "取消中",
	5: "已取消", 6: "出错", 7: "失败中", 8: "等待重试", 9: "重试前",
}

type taskRow struct {
	Type string
	Item map[string]any
}

func (a *CLI) tasksCmd() *cobra.Command {
	var (
		types     []string
		done      bool
		clear     bool
		succeeded bool
	)
	cmd := &cobra.Command{
		Use:   "tasks",
		Short: "异步任务列表",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := a.newClient(true)
			if err != nil {
				return err
			}
			ctx := cmd.Context()

			selected := types
			if len(selected) == 0 {
				selected = TaskTypes
			}

			if clear {
				cleared := 0
				for _, one := range selected {
					if err := c.TaskClear(ctx, one, succeeded); err != nil {
						var apiErr *apierr.ApiError
						if asAPIError(err, &apiErr) {
							a.Errorf("· %s: %s\n", one, apiErr.Message)
							continue
						}
						return err
					}
					a.Printf("✓ 已清理 %s 的已完成任务\n", one)
					cleared++
				}
				if cleared == 0 {
					return apperr.Exitf(1, "")
				}
				return nil
			}

			var rows []taskRow
			for _, one := range selected {
				items, err := c.TaskList(ctx, one, done)
				if err != nil {
					var apiErr *apierr.ApiError
					if asAPIError(err, &apiErr) {
						a.Errorf("· %s: %s\n", one, apiErr.Message)
						continue
					}
					return err
				}
				for _, it := range items {
					if m := jsonx.Object(it); m != nil {
						rows = append(rows, taskRow{Type: one, Item: m})
					}
				}
			}

			payload := make([]map[string]any, 0, len(rows))
			for _, row := range rows {
				merged := map[string]any{"type": row.Type}
				for k, v := range row.Item {
					merged[k] = v
				}
				payload = append(payload, merged)
			}

			return a.Emit(payload, func() {
				if len(rows) == 0 {
					a.Println("（没有任务）")
					return
				}
				nameWidth := 0
				for _, row := range rows {
					if n := len([]rune(jsonx.Str(row.Item, "name"))); n > nameWidth {
						nameWidth = n
					}
				}
				for _, row := range rows {
					progressText := "—"
					if f, ok := jsonx.Float(row.Item, "progress"); ok {
						progressText = fmt.Sprintf("%.0f%%", f)
					}
					errText := ""
					if e := jsonx.Str(row.Item, "error"); e != "" {
						errText = "  " + e
					}
					state := taskStates[jsonx.Int(row.Item, "state")]
					if state == "" {
						state = fmt.Sprintf("state=%d", jsonx.Int(row.Item, "state"))
					}
					id := jsonx.Str(row.Item, "id")
					if len(id) > 8 {
						id = id[:8]
					}
					name := jsonx.Str(row.Item, "name")
					a.Printf("%-26s %-8s %-*s %5s  %s%s\n",
						row.Type, id, nameWidth, name, progressText, state, errText)
				}
			})
		},
	}
	cmd.Flags().StringArrayVar(&types, "type", nil, "限定任务类型，可重复；默认全部 7 类")
	cmd.Flags().BoolVar(&done, "done", false, "连同已完成的一起列出")
	cmd.Flags().BoolVar(&clear, "clear", false, "清理已完成任务")
	cmd.Flags().BoolVar(&succeeded, "succeeded", false, "配合 --clear，只清理成功的")
	return cmd
}

func (a *CLI) cancelTaskCmd() *cobra.Command {
	var (
		taskType string
		action   string
	)
	cmd := &cobra.Command{
		Use:   "cancel-task <任务ID>",
		Short: "取消/删除/重试任务",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := a.newClient(true)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			id := args[0]

			candidates := TaskTypes
			if taskType != "" {
				candidates = []string{taskType}
			}

			target := ""
			for _, one := range candidates {
				if _, err := c.TaskInfo(ctx, one, id); err == nil {
					target = one
					break
				}
			}
			if target == "" {
				return apperr.Exitf(1, "没找到任务 %s（已尝试 %d 种类型）", id, len(candidates))
			}

			verb := "取消"
			var runErr error
			switch action {
			case "delete":
				verb = "删除"
				runErr = c.TaskDelete(ctx, target, id)
			case "retry":
				verb = "重试"
				runErr = c.TaskRetry(ctx, target, id)
			default:
				runErr = c.TaskCancel(ctx, target, id)
			}
			if runErr != nil {
				return runErr
			}
			a.Printf("✓ 已%s任务 %s（类型 %s）\n", verb, id, target)
			return nil
		},
	}
	cmd.Flags().StringVar(&taskType, "type", "", "任务类型；不给则逐个类型探测（每类一次请求）")
	cmd.Flags().StringVar(&action, "action", "cancel", "cancel|delete|retry")
	return cmd
}
