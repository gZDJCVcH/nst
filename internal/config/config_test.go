package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// 配置目录固定为 ~/.config/openlist-cli，不能用 os.UserConfigDir()（macOS 指向 ~/Library/Application Support）。
func TestDirKeepsDotConfigLayout(t *testing.T) {
	home := t.TempDir()
	// Windows 上 os.UserHomeDir 读 USERPROFILE，两个都设才跨平台
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", "")

	want := filepath.Join(home, ".config", "openlist-cli")
	if got := Dir(); got != want {
		t.Errorf("Dir() = %q，期望 %q", got, want)
	}
	if got, wantFile := File(), filepath.Join(want, "config.json"); got != wantFile {
		t.Errorf("File() = %q，期望 %q", got, wantFile)
	}
}

func TestDirHonorsXDG(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	if got, want := Dir(), filepath.Join(xdg, "openlist-cli"); got != want {
		t.Errorf("Dir() = %q，期望 %q", got, want)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv(EnvURL, "")
	t.Setenv(EnvToken, "")

	cfg := map[string]any{"url": "http://127.0.0.1:5244", "token": "abc", "username": "admin"}
	saved, err := Save(cfg)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(saved)
	if err != nil {
		t.Fatal(err)
	}
	// Windows 没有 Unix 权限位，OpenFile 的 0600 只影响只读属性，跳过断言
	if runtime.GOOS != "windows" {
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("配置文件权限应为 0600，实际 %o", perm)
		}
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if Str(loaded, "url") != "http://127.0.0.1:5244" || Str(loaded, "token") != "abc" {
		t.Errorf("读回的内容不对：%v", loaded)
	}
}

func TestEnvOverridesFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if _, err := Save(map[string]any{"url": "http://file", "token": "file-token"}); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvURL, "http://env")
	t.Setenv(EnvToken, "env-token")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if Str(cfg, "url") != "http://env" || Str(cfg, "token") != "env-token" {
		t.Errorf("环境变量应覆盖文件：%v", cfg)
	}
}

func TestClearToken(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv(EnvToken, "")
	if _, err := Save(map[string]any{"url": "http://x", "token": "t"}); err != nil {
		t.Fatal(err)
	}
	if err := ClearToken(); err != nil {
		t.Fatalf("ClearToken: %v", err)
	}
	cfg, _ := Load()
	if Str(cfg, "token") != "" {
		t.Error("令牌应被清除")
	}
	if Str(cfg, "url") != "http://x" {
		t.Error("其他字段应保留")
	}
}

func TestBrokenConfigIsReported(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "openlist-cli")
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "config.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(); err == nil {
		t.Fatal("损坏的配置应报错")
	}
}
