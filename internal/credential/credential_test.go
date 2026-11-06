package credential

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseMode(t *testing.T) {
	cases := []struct {
		in   string
		want Mode
		err  bool
	}{
		{"", ModeAuto, false},
		{"auto", ModeAuto, false},
		{"FLAGS", ModeFlags, false},
		{" context ", ModeContext, false},
		{"interactive", ModeInteractive, false},
		{"bogus", "", true},
	}
	for _, c := range cases {
		got, err := ParseMode(c.in)
		if (err != nil) != c.err {
			t.Errorf("ParseMode(%q) err=%v, wantErr=%v", c.in, err, c.err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseMode(%q)=%q, want %q", c.in, got, c.want)
		}
	}
}

func baseInput() Input {
	return Input{
		FlagURL:      "",
		ContextURL:   "",
		ContextToken: "",
		TTY:          false,
	}
}

func TestResolveAutoFlags(t *testing.T) {
	in := baseInput()
	in.FlagURL = "http://nas:5244"
	in.FlagToken = "tok"
	in.ContextURL = "http://old:5244"
	in.ContextToken = "oldtok"

	c, err := Resolve(in)
	if err != nil {
		t.Fatal(err)
	}
	if c.Mode != ModeFlags || c.Source != SourceFlag {
		t.Fatalf("mode=%s source=%s, want flags/flag", c.Mode, c.Source)
	}
	if c.URL != "http://nas:5244" || c.Token != "tok" {
		t.Errorf("flag 应覆盖上下文：%+v", c)
	}
}

func TestResolveAutoContext(t *testing.T) {
	in := baseInput()
	in.ContextURL = "http://nas:5244"
	in.ContextToken = "tok"

	c, err := Resolve(in)
	if err != nil {
		t.Fatal(err)
	}
	if c.Mode != ModeContext || c.Source != SourceContext {
		t.Fatalf("mode=%s source=%s, want context/context", c.Mode, c.Source)
	}
	if c.Token != "tok" {
		t.Errorf("应读到已保存令牌：%+v", c)
	}
}

func TestResolveAutoInteractive(t *testing.T) {
	in := baseInput()
	in.TTY = true
	c, err := Resolve(in)
	if err != nil {
		t.Fatal(err)
	}
	if c.Mode != ModeInteractive || c.Source != SourcePrompt {
		t.Fatalf("mode=%s source=%s, want interactive/prompt", c.Mode, c.Source)
	}
	if c.URL != "" || c.Token != "" {
		t.Errorf("交互式应允许留空由询问补齐：%+v", c)
	}
	if c.Username != "admin" {
		t.Errorf("默认用户名应为 admin，实际 %q", c.Username)
	}
}

func TestResolveAutoNonTTYNoCredentials(t *testing.T) {
	_, err := Resolve(baseInput())
	if err == nil || !strings.Contains(err.Error(), "非交互环境缺少凭据") {
		t.Fatalf("非 tty 无凭据应直接报错，实际 %v", err)
	}
}

func TestResolveExplicitModes(t *testing.T) {
	t.Run("flags 缺字段报错", func(t *testing.T) {
		in := baseInput()
		in.Mode = ModeFlags
		in.FlagURL = "http://nas:5244"
		if _, err := Resolve(in); err == nil {
			t.Error("flags 模式缺 token 应报错，不应回退到上下文")
		}
	})

	t.Run("context 缺字段报错", func(t *testing.T) {
		in := baseInput()
		in.Mode = ModeContext
		if _, err := Resolve(in); err == nil {
			t.Error("context 模式无已保存凭据应报错")
		}
	})

	t.Run("interactive 非 tty 报错", func(t *testing.T) {
		in := baseInput()
		in.Mode = ModeInteractive
		if _, err := Resolve(in); err == nil {
			t.Error("非 tty 不应进入交互模式")
		}
	})

	t.Run("context 与 password 冲突", func(t *testing.T) {
		in := baseInput()
		in.Mode = ModeContext
		in.ContextURL = "http://nas:5244"
		in.ContextToken = "tok"
		in.UsePassword = true
		if _, err := Resolve(in); err == nil {
			t.Error("context 与 --password 同用应报错")
		}
	})
}

func TestResolvePasswordWithoutValueStaysInteractive(t *testing.T) {
	in := baseInput()
	in.TTY = true
	in.UsePassword = true
	in.FlagURL = "http://nas:5244"
	in.FlagPassword = ""

	c, err := Resolve(in)
	if err != nil {
		t.Fatal(err)
	}
	if c.Mode != ModeInteractive {
		t.Fatalf("--password 不带值应留给交互询问，实际 %s", c.Mode)
	}
}

func TestResolvePasswordWithValueIsFlags(t *testing.T) {
	in := baseInput()
	in.UsePassword = true
	in.FlagURL = "http://nas:5244"
	in.FlagPassword = "secret"
	c, err := Resolve(in)
	if err != nil {
		t.Fatal(err)
	}
	if c.Mode != ModeFlags || c.Password != "secret" {
		t.Fatalf("参数齐全的密码模式应为 flags：%+v", c)
	}
}

func TestResultJSON(t *testing.T) {
	r := Result{Mode: ModeFlags, Source: SourceFlag, URL: "http://nas:5244", Valid: true, Saved: true, User: "admin", Role: "admin"}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"mode", "source", "url", "valid", "saved", "user", "role"} {
		if _, ok := got[k]; !ok {
			t.Errorf("JSON 缺少字段 %s：%s", k, b)
		}
	}
	if _, ok := got["error"]; ok {
		t.Errorf("成功结果不应带 error：%s", b)
	}
}
