package main

import (
	"os"
	"path/filepath"
	"strings"

	"nst/internal/cli"
)

func main() {
	os.Exit(cli.New().Execute(argv()))
}

// argv 兼容以 openlist-cli 名字调用的场景，自动补上 openlist 子命令。
func argv() []string {
	args := os.Args[1:]
	base := strings.TrimSuffix(strings.ToLower(filepath.Base(os.Args[0])), ".exe")
	if base == "openlist-cli" {
		return append([]string{"openlist"}, args...)
	}
	return args
}
