package output

import (
	"bytes"
	"strings"
	"testing"
)

func TestHumanSize(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{0, "0B"},
		{1023, "1023B"},
		{1024, "1.0KB"},
		{1536, "1.5KB"},
		{1024 * 1024 * 1024, "1.0GB"},
		{2.5 * 1024 * 1024 * 1024 * 1024, "2.5TB"},
		{nil, "-"},
		{"abc", "-"},
	}
	for _, c := range cases {
		if got := HumanSize(c.in); got != c.want {
			t.Errorf("HumanSize(%v) = %q，期望 %q", c.in, got, c.want)
		}
	}
}

func TestFmtTime(t *testing.T) {
	if got := FmtTime("2026-11-01T19:30:24+08:00"); got != "2026-11-01 19:30:24" {
		t.Errorf("FmtTime = %q", got)
	}
	if got := FmtTime(nil); got != "-" {
		t.Errorf("FmtTime(nil) = %q", got)
	}
	if got := FmtTime(""); got != "-" {
		t.Errorf("FmtTime(\"\") = %q", got)
	}
}

func TestSecrets(t *testing.T) {
	for _, key := range []string{"access_token", "refresh_token", "client_secret", "Cookie", "password", "api_key"} {
		if !IsSensitive(key) {
			t.Errorf("%q 应判定为敏感", key)
		}
	}
	for _, key := range []string{"mount_path", "page_size", "root_folder_id", "order"} {
		if IsSensitive(key) {
			t.Errorf("%q 不应判定为敏感", key)
		}
	}

	masked := MaskSecret("tr14b.abcdefghijklmnopqrstuvwxyz89ef", false)
	if !strings.HasPrefix(masked, "tr14b.") || !strings.HasSuffix(masked, "89ef") || !strings.Contains(masked, "…") {
		t.Errorf("打码结果不符合预期：%q", masked)
	}
	if MaskSecret("short", false) != "***" {
		t.Error("短值应整体打码")
	}
	if MaskSecret("secret-token-value", true) != "secret-token-value" {
		t.Error("show=true 应返回原值")
	}
	if MaskSecret(nil, false) != "" || MaskSecret("", false) != "" {
		t.Error("空值应返回空串")
	}
}

func TestPrintEntries(t *testing.T) {
	entries := []Entry{
		{Name: "b.txt", IsDir: false, Size: 1},
		{Name: "a", IsDir: true},
	}
	var buf bytes.Buffer
	PrintEntries(&buf, entries, false)
	text := buf.String()
	if strings.Index(text, "a") > strings.Index(text, "b.txt") {
		t.Errorf("目录应排在文件前面：\n%s", text)
	}

	buf.Reset()
	PrintEntries(&buf, []Entry{{
		Name: "x.cue", Size: 774, Modified: "2026-11-01T19:30:24+08:00",
	}}, true)
	text = buf.String()
	if !strings.Contains(text, "774B") || !strings.Contains(text, "2026-11-01 19:30:24") {
		t.Errorf("长格式缺少字段：%q", text)
	}

	buf.Reset()
	PrintEntries(&buf, nil, false)
	if !strings.Contains(buf.String(), "空") {
		t.Error("空列表应有提示")
	}
}

func TestConfirmNonInteractive(t *testing.T) {
	var errBuf bytes.Buffer
	if Confirm(strings.NewReader("y\n"), &errBuf, "确认？", false, false) {
		t.Error("非交互环境应默认取消")
	}
	if !strings.Contains(errBuf.String(), "默认取消") {
		t.Errorf("缺少非交互提示：%q", errBuf.String())
	}
	if !Confirm(strings.NewReader(""), &errBuf, "确认？", true, false) {
		t.Error("assumeYes 应直接通过")
	}
	var out bytes.Buffer
	if !Confirm(strings.NewReader("y\n"), &out, "确认？", false, true) {
		t.Error("交互输入 y 应通过")
	}
}
