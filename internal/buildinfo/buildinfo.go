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
		parts = append(parts, "built "+Date)
	}
	if len(parts) == 0 {
		return Version
	}
	return Version + " (" + strings.Join(parts, ", ") + ")"
}
