// Package buildinfo 保存构建期注入的版本信息。
package buildinfo

import (
	"runtime/debug"
	"strings"
)

// 由 -ldflags -X 注入；本地 go build 时保留默认值。
var (
	Version = "dev"
	Commit  = ""
	Date    = ""
)

func init() {
	// 构建系统可能注入 "none" / 零值时间，统一当成"未提供"
	if Commit == "none" {
		Commit = ""
	}
	if Date == "" || strings.HasPrefix(Date, "0001-01-01") {
		Date = ""
	}

	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	if Commit == "" {
		for _, s := range bi.Settings {
			if s.Key == "vcs.revision" && s.Value != "" {
				Commit = s.Value
				if len(Commit) > 12 {
					Commit = Commit[:12]
				}
				break
			}
		}
	}
	// go install / go run 场景没有 ldflags，退回模块版本
	if Version == "dev" && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		Version = bi.Main.Version
	}
}

// String 返回单行版本串，例如 "0.2.0 (a1b2c3d, built 2026-11-01)"。
func String() string {
	parts := make([]string, 0, 2)
	if Commit != "" {
		parts = append(parts, Commit)
	}
	if Date != "" {
		date := Date
		// 只保留日期部分，避免 "built 2026-11-01T14:23:29Z" 这种噪音
		if len(date) > 10 && date[10] == 'T' {
			date = date[:10]
		}
		parts = append(parts, "built "+date)
	}
	if len(parts) == 0 {
		return Version
	}
	return Version + " (" + strings.Join(parts, ", ") + ")"
}
