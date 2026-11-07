package cli

import (
	"bytes"
	"strings"
	"testing"

	"nst/internal/app"
)

func runRoot(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	var out, errBuf bytes.Buffer
	a := app.New()
	a.Out = &out
	a.Err = &errBuf
	a.In = strings.NewReader("")
	a.SetTTY(false)
	code := a.Execute(args, buildRoot)
	return out.String(), errBuf.String(), code
}

func TestCommandTree(t *testing.T) {
	a := app.New()
	root := buildRoot(a)

	for _, name := range []string{"openlist", "version", "upgrade"} {
		cmd, _, err := root.Find([]string{name})
		if err != nil || cmd.Name() != name {
			t.Errorf("顶层缺少命令 %q：%v / %v", name, cmd.Name(), err)
		}
	}

	openlist, _, err := root.Find([]string{"openlist"})
	if err != nil {
		t.Fatal(err)
	}

	want := []string{
		"auth", "list", "stat", "tree", "search", "read", "link",
		"upload", "download", "remove", "move", "copy", "rename", "mkdir",
		"tasks", "cancel-task", "drivers", "verify", "mounts", "storage",
		"driver", "config", "version", "upgrade",
	}
	found := map[string]bool{}
	for _, c := range openlist.Commands() {
		found[c.Name()] = true
	}
	for _, name := range want {
		if !found[name] {
			t.Errorf("openlist 缺少命令 %q", name)
		}
	}

	aliases := map[string]string{
		"ls": "list", "put": "upload", "get": "download",
		"rm": "remove", "mv": "move", "cp": "copy",
	}
	for alias, target := range aliases {
		cmd, _, err := openlist.Find([]string{alias})
		if err != nil || cmd.Name() != target {
			t.Errorf("别名 %q 应指向 %q，实际 %v / %v", alias, target, cmd.Name(), err)
		}
	}
}

func TestFlagDefaults(t *testing.T) {
	out, _, _ := runRoot(t, "--help")
	for _, bad := range []string{"(default -1)", "default -1"} {
		if strings.Contains(out, bad) {
			t.Errorf("帮助里不应出现 %q", bad)
		}
	}
	if !strings.Contains(out, "--no-cache") {
		t.Error("帮助里应列出 --no-cache")
	}
}
