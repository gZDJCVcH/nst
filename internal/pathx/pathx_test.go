package pathx

import (
	"errors"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	cases := []struct {
		raw  string
		kind string
		path string
	}{
		{"local:/tmp/a.cue", "local", "/tmp/a.cue"},
		{"local:./a.cue", "local", "./a.cue"},
		{"remote:/音乐/周杰伦", "remote", "/音乐/周杰伦"},
		{"remote:", "remote", "/"},
		{"LOCAL:/tmp", "local", "/tmp"},
		{"Remote:/a", "remote", "/a"},
	}
	for _, c := range cases {
		kind, p, err := Parse(c.raw)
		if err != nil {
			t.Fatalf("Parse(%q) 报错：%v", c.raw, err)
		}
		if kind != c.kind || p != c.path {
			t.Errorf("Parse(%q) = (%q,%q)，期望 (%q,%q)", c.raw, kind, p, c.kind, c.path)
		}
	}
}

func TestParseRejects(t *testing.T) {
	if _, _, err := Parse("remote:音乐"); err == nil {
		t.Error("remote: 相对路径应报错")
	} else if !strings.Contains(err.Error(), "必须是绝对路径") {
		t.Errorf("错误文案不对：%v", err)
	}

	for _, raw := range []string{"/音乐", "a.cue", "./a.cue", "音乐/周杰伦"} {
		_, _, err := Parse(raw)
		if err == nil {
			t.Errorf("Parse(%q) 应该报错", raw)
			continue
		}
		if !strings.Contains(err.Error(), "必须带前缀") {
			t.Errorf("Parse(%q) 错误文案不对：%v", raw, err)
		}
		var pe *PathError
		if !errors.As(err, &pe) {
			t.Errorf("Parse(%q) 应返回 *PathError", raw)
		}
	}

	if _, _, err := Parse(""); err == nil {
		t.Error("空字符串应报错")
	}
}

func TestTypedAccessors(t *testing.T) {
	if p, err := Remote("remote:/a"); err != nil || p != "/a" {
		t.Errorf("Remote 解析错误：%q %v", p, err)
	}
	if _, err := Remote("local:/a"); err == nil {
		t.Error("Remote 遇到 local: 应报错")
	}
	if p, err := Local("local:/a"); err != nil || p != "/a" {
		t.Errorf("Local 解析错误：%q %v", p, err)
	}
	if _, err := Local("remote:/a"); err == nil {
		t.Error("Local 遇到 remote: 应报错")
	}
}

func TestNormalizeRemote(t *testing.T) {
	cases := map[string]string{
		"//a///b/": "/a/b",
		"/":        "/",
		"a/b":      "/a/b",
		".":        "/",
		"/a/./b":   "/a/b",
	}
	for in, want := range cases {
		if got := NormalizeRemote(in); got != want {
			t.Errorf("NormalizeRemote(%q) = %q，期望 %q", in, got, want)
		}
	}
}

func TestJoinSplit(t *testing.T) {
	if got := JoinRemote("/", "x.cue"); got != "/x.cue" {
		t.Errorf("JoinRemote 根目录拼接错误：%q", got)
	}
	if got := JoinRemote("/a", "b.cue"); got != "/a/b.cue" {
		t.Errorf("JoinRemote 嵌套拼接错误：%q", got)
	}
	if dir, name := SplitRemote("/a/b.cue"); dir != "/a" || name != "b.cue" {
		t.Errorf("SplitRemote = (%q,%q)", dir, name)
	}
	if dir, name := SplitRemote("/"); dir != "/" || name != "" {
		t.Errorf("SplitRemote 根目录 = (%q,%q)", dir, name)
	}
}

func TestQuote(t *testing.T) {
	got := QuoteForHeader("/a b/周杰伦.cue")
	want := "/a%20b/%E5%91%A8%E6%9D%B0%E4%BC%A6.cue"
	if got != want {
		t.Errorf("QuoteForHeader = %q，期望 %q", got, want)
	}
	if !strings.HasPrefix(QuoteForHeader("/a/b"), "/a/") {
		t.Error("QuoteForHeader 应保留斜杠")
	}
	// sign 里的 + = : 必须编码，否则服务端验签失败
	if got := QuoteComponent("a+b=c:0"); got != "a%2Bb%3Dc%3A0" {
		t.Errorf("QuoteComponent = %q", got)
	}
}
