package cli

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nst/internal/client"
	"nst/internal/jsonx"
)

func newClientForTest(base string) (*client.Client, error) {
	c, err := client.New(base, "token")
	if err != nil {
		return nil, err
	}
	c.CloudInterval = 0
	c.LocalInterval = 0
	return c, nil
}

// runCLI 用假 IO 跑一条命令，返回 stdout / stderr / 退出码。
func runCLI(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	// 业务命令都在 nst openlist 下；--help 用来检查顶层帮助，保持原样。
	if len(args) > 0 && args[0] != "--help" && args[0] != "-h" {
		args = append([]string{"openlist"}, args...)
	}
	var out, errBuf bytes.Buffer
	app := New()
	app.Out = &out
	app.Err = &errBuf
	app.In = strings.NewReader("")
	tty := false
	app.isTTYOverride = &tty
	code := app.Execute(args)
	return out.String(), errBuf.String(), code
}

// fakeOpenList 模拟 OpenList 的若干只读接口。
type fakeOpenList struct {
	storageByID map[string]any
	driverInfo  any
	listing     map[string][]map[string]any
	stat        map[string]map[string]any
	lastStorage string
}

func (f *fakeOpenList) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		write := func(v any) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": v})
		}
		switch r.URL.Path {
		case "/api/admin/storage/list":
			// 权限不足：限流逻辑会回落到网盘间隔
			_, _ = w.Write([]byte(`{"code":403,"message":"permission denied"}`))
		case "/api/admin/storage/get":
			f.lastStorage = r.URL.RequestURI()
			write(f.storageByID[r.URL.Query().Get("id")])
		case "/api/admin/driver/info":
			write(f.driverInfo)
		case "/api/fs/list":
			var payload struct {
				Path string `json:"path"`
			}
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &payload)
			items := f.listing[payload.Path]
			content := make([]any, 0, len(items))
			for _, it := range items {
				content = append(content, it)
			}
			write(map[string]any{"content": content, "total": len(content)})
		case "/api/fs/get":
			var payload struct {
				Path string `json:"path"`
			}
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &payload)
			if info, ok := f.stat[payload.Path]; ok {
				write(info)
				return
			}
			_, _ = w.Write([]byte(`{"code":404,"message":"object not found"}`))
		default:
			_, _ = w.Write([]byte(`{"code":404,"message":"not found"}`))
		}
	}
}

func TestResolveDst(t *testing.T) {
	fake := &fakeOpenList{
		listing: map[string][]map[string]any{
			"/":         {{"name": "115", "is_dir": true}},
			"/115":      {{"name": "deep", "is_dir": true}, {"name": "b.cue"}},
			"/115/deep": {},
		},
		stat: map[string]map[string]any{
			"/115/deep":  {"name": "deep", "is_dir": true},
			"/115/b.cue": {"name": "b.cue", "is_dir": false},
		},
	}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()

	c, err := newClientForTest(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	app := New()

	cases := []struct {
		name string
		src  string
		dst  string
		want string
	}{
		{"尾部斜杠视为目录", "/115/a.cue", "remote:/115/deep/", "/115/deep/a.cue"},
		{"根目录视为目录", "/115/a.cue", "remote:/", "/a.cue"},
		{"已存在目录则拼接文件名", "/115/a.cue", "remote:/115/deep", "/115/deep/a.cue"},
		{"目标不存在视为完整路径", "/115/a.cue", "remote:/115/b.cue", "/115/b.cue"},
		{"已存在文件视为完整路径", "/115/a.cue", "remote:/115/b.cue", "/115/b.cue"},
	}
	for _, c2 := range cases {
		t.Run(c2.name, func(t *testing.T) {
			got, err := app.resolveDst(t.Context(), c, c2.src, c2.dst)
			if err != nil {
				t.Fatalf("resolveDst: %v", err)
			}
			if got != c2.want {
				t.Errorf("resolveDst = %q，期望 %q", got, c2.want)
			}
		})
	}

	got, err := app.resolveDst(t.Context(), c, "/115/a.cue", "remote:/115/x/")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "/115/x/") || strings.Contains(got, "remote:") {
		t.Errorf("remote: 前缀不该进路径：%q", got)
	}

	if _, err := app.resolveDst(t.Context(), c, "/115/a.cue", "local:/tmp/a.cue"); err == nil {
		t.Error("传 local: 应报错")
	}
}

func TestIterLocalFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.cue", ".DS_Store", "._a.cue"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "b.cue"), []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}

	items, err := iterLocalFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	var rels []string
	for _, it := range items {
		rels = append(rels, it.rel)
	}
	joined := strings.Join(rels, ",")
	for _, want := range []string{"a.cue", "sub/b.cue"} {
		if !strings.Contains(joined, want) {
			t.Errorf("缺少 %q：%s", want, joined)
		}
	}
	for _, bad := range []string{".DS_Store", "._a.cue"} {
		if strings.Contains(joined, bad) {
			t.Errorf("不该包含 %q：%s", bad, joined)
		}
	}

	single := filepath.Join(dir, "a.cue")
	got, err := iterLocalFiles(single)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].rel != "" || got[0].abs != single {
		t.Errorf("单文件枚举不对：%+v", got)
	}
}

func TestSha1OfFile(t *testing.T) {
	content := []byte("REM GENRE Pop\r\n")
	path := filepath.Join(t.TempDir(), "a.cue")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	h := sha1.Sum(content)
	want := strings.ToUpper(hex.EncodeToString(h[:]))
	got, err := sha1OfFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("sha1 = %s，期望 %s", got, want)
	}
}

func TestMaskedAddition(t *testing.T) {
	raw := `{"access_token":"tr14b.abcdefghijklmnop89ef","page_size":200}`
	out := maskedAddition(raw, false)
	if out == nil {
		t.Fatal("应能解析")
	}
	token, _ := out["access_token"].(string)
	if !strings.Contains(token, "…") || !strings.HasSuffix(token, "89ef") {
		t.Errorf("access_token 未正确打码：%q", token)
	}
	if got := jsonx.StrOf(out["page_size"]); got != "200" {
		t.Errorf("非敏感字段应原样保留：%v", out["page_size"])
	}

	if got := maskedAddition(`{"refresh_token":"abcdefghijklmnop"}`, true)["refresh_token"]; got != "abcdefghijklmnop" {
		t.Errorf("show=true 应返回原值：%v", got)
	}
	if maskedAddition("{not json", false) != nil {
		t.Error("非法 JSON 应返回 nil")
	}
	if maskedAddition("[1, 2]", false) != nil {
		t.Error("数组应返回 nil")
	}
	if got := maskedAddition("", false); got == nil || len(got) != 0 {
		t.Errorf("空串应返回空 map：%v", got)
	}
}

const storageInfoJSON = `{
  "id": 3, "mount_path": "/115", "driver": "115 Open", "status": "work",
  "disabled": false, "web_proxy": false, "cache_expiration": 30,
  "addition": "{\"page_size\":200,\"access_token\":\"tr14b.SECRETVALUE12345689ef\"}"
}`

func TestStorageGetCommand(t *testing.T) {
	var info map[string]any
	if err := json.Unmarshal([]byte(storageInfoJSON), &info); err != nil {
		t.Fatal(err)
	}
	fake := &fakeOpenList{storageByID: map[string]any{"3": info}}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()

	base := []string{"--url", srv.URL, "--token", "t", "--rate", "0", "--local-rate", "0"}

	out, _, code := runCLI(t, append([]string{"storage", "get", "3"}, base...)...)
	if code != 0 {
		t.Fatalf("退出码 %d", code)
	}
	if strings.Contains(out, "SECRETVALUE123456") {
		t.Error("令牌泄漏到输出里")
	}
	for _, want := range []string{"…", "/115", "敏感字段已打码"} {
		if !strings.Contains(out, want) {
			t.Errorf("输出缺少 %q：\n%s", want, out)
		}
	}

	out, _, _ = runCLI(t, append([]string{"storage", "get", "3", "--show-secrets"}, base...)...)
	if !strings.Contains(out, "SECRETVALUE123456") {
		t.Error("--show-secrets 应输出原值")
	}

	out, _, _ = runCLI(t, append([]string{"storage", "get", "3", "--json"}, base...)...)
	if strings.Contains(out, "SECRETVALUE123456") {
		t.Error("--json 默认也应打码")
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("JSON 输出无法解析：%v\n%s", err, out)
	}
	addition, _ := payload["addition"].(map[string]any)
	if addition["page_size"] == nil {
		t.Error("addition.page_size 应保留")
	}

	if _, _, code := runCLI(t, append([]string{"storage", "get", "9"}, base...)...); code == 0 {
		t.Error("不存在的存储应返回非零退出码")
	}
	if !strings.Contains(fake.lastStorage, "/api/admin/storage/get?id=") {
		t.Errorf("存储详情请求路径不对：%s", fake.lastStorage)
	}
}

func TestDriverInfoCommand(t *testing.T) {
	data := map[string]any{
		"common": []any{map[string]any{
			"name": "mount_path", "type": "string", "required": true, "default": "", "options": "",
		}},
		"additional": []any{map[string]any{
			"name": "webdav_policy", "type": "select", "required": true,
			"default": "302_redirect", "options": "302_redirect,use_proxy_url,native_proxy",
		}},
	}
	fake := &fakeOpenList{driverInfo: data}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()

	base := []string{"--url", srv.URL, "--token", "t", "--rate", "0"}
	out, _, code := runCLI(t, append([]string{"driver", "info", "115 Open"}, base...)...)
	if code != 0 {
		t.Fatalf("退出码 %d", code)
	}
	for _, want := range []string{"必填", "302_redirect,use_proxy_url,native_proxy", "通用配置"} {
		if !strings.Contains(out, want) {
			t.Errorf("输出缺少 %q：\n%s", want, out)
		}
	}

	out, _, _ = runCLI(t, append([]string{"driver", "info", "115 Open", "--json"}, base...)...)
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("JSON 输出无法解析：%v", err)
	}
	additional, _ := payload["additional"].([]any)
	first, _ := additional[0].(map[string]any)
	if first["name"] != "webdav_policy" {
		t.Errorf("JSON 结构不对：%v", payload)
	}
}

func TestListCommandAndAlias(t *testing.T) {
	fake := &fakeOpenList{listing: map[string][]map[string]any{
		"/115": {
			{"name": "b.mkv", "is_dir": false, "size": 1024},
			{"name": "a", "is_dir": true, "size": 0},
		},
	}}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()
	base := []string{"--url", srv.URL, "--token", "t", "--rate", "0"}

	out, _, code := runCLI(t, append([]string{"list", "remote:/115"}, base...)...)
	if code != 0 {
		t.Fatalf("退出码 %d", code)
	}
	if !strings.Contains(out, "a") || !strings.Contains(out, "b.mkv") {
		t.Errorf("列表内容缺失：\n%s", out)
	}

	aliasOut, _, _ := runCLI(t, append([]string{"ls", "remote:/115"}, base...)...)
	if aliasOut != out {
		t.Errorf("ls 别名输出应与 list 一致：\n%s\n---\n%s", aliasOut, out)
	}

	jsonOut, _, _ := runCLI(t, append([]string{"list", "remote:/115", "--json"}, base...)...)
	var payload map[string]any
	if err := json.Unmarshal([]byte(jsonOut), &payload); err != nil {
		t.Fatalf("JSON 输出无法解析：%v", err)
	}
	if payload["path"] != "/115" {
		t.Errorf("path 字段不对：%v", payload["path"])
	}
	if _, ok := payload["content"]; !ok {
		t.Error("缺少 content 字段")
	}
}

func TestCommandTree(t *testing.T) {
	app := New()
	root := app.buildRoot()

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
	out, _, _ := runCLI(t, "--help")
	for _, bad := range []string{"(default -1)", "default -1"} {
		if strings.Contains(out, bad) {
			t.Errorf("帮助里不应出现 %q", bad)
		}
	}
	if !strings.Contains(out, "--no-cache") {
		t.Error("帮助里应列出 --no-cache")
	}
}

func TestPathErrorExitCode(t *testing.T) {
	_, errOut, code := runCLI(t, "list", "/音乐", "--url", "http://127.0.0.1:1", "--token", "t")
	if code != 2 {
		t.Errorf("路径错误退出码应为 2，实际 %d", code)
	}
	if !strings.Contains(errOut, "路径错误") {
		t.Errorf("应提示路径错误：%s", errOut)
	}
}

func TestNormalizeOptionalValueFlags(t *testing.T) {
	cases := []struct {
		in   []string
		want []string
	}{
		{[]string{"auth", "login", "--password", "secret"}, []string{"auth", "login", "--password=secret"}},
		{[]string{"list", "--password", "p", "remote:/x"}, []string{"list", "--password=p", "remote:/x"}},
		{[]string{"auth", "login", "--password"}, []string{"auth", "login", "--password"}},
		{[]string{"auth", "login", "--password", "--otp", "123"}, []string{"auth", "login", "--password", "--otp", "123"}},
		{[]string{"auth", "login", "--password="}, []string{"auth", "login", "--password="}},
	}
	for _, c := range cases {
		got := normalizeOptionalValueFlags(c.in)
		if strings.Join(got, "|") != strings.Join(c.want, "|") {
			t.Errorf("normalize(%v) = %v，期望 %v", c.in, got, c.want)
		}
	}
}

func TestTokenPageURL(t *testing.T) {
	cases := []string{"http://nas.com:5244", "nas.com:5244/", "https://nas.com/", "https://nas.com:5244"}
	want := "/@manage/settings/other"
	for _, in := range cases {
		got := tokenPageURL(in)
		if !strings.HasSuffix(got, want) {
			t.Errorf("tokenPageURL(%q) = %q，应以 %q 结尾", in, got, want)
		}
		if !strings.HasPrefix(got, "http://") && !strings.HasPrefix(got, "https://") {
			t.Errorf("tokenPageURL(%q) = %q，缺少协议前缀", in, got)
		}
	}
	if got, want2 := tokenPageURL("nas.com:5244"), "http://nas.com:5244/@manage/settings/other"; got != want2 {
		t.Errorf("tokenPageURL = %q，期望 %q", got, want2)
	}
}

func TestGuideTokenPagePrintsLink(t *testing.T) {
	// SSH 会话下只打印，不真的拉浏览器
	t.Setenv("SSH_CONNECTION", "10.0.0.1 1 10.0.0.2 22")

	var out bytes.Buffer
	app := New()
	app.Out = &out
	app.Err = &out
	app.guideTokenPage("nas.com:5244", false)

	text := out.String()
	for _, want := range []string{
		"http://nas.com:5244/@manage/settings/other",
		"设置 → 其他",
		"复制上面的地址",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("输出缺少 %q：\n%s", want, text)
		}
	}
}

func TestGuideTokenPageSkipsOpenWhenNoOpen(t *testing.T) {
	t.Setenv("SSH_CONNECTION", "")

	var out bytes.Buffer
	app := New()
	app.Out = &out
	app.Err = &out
	app.guideTokenPage("http://nas.com:5244", true)

	text := out.String()
	if !strings.Contains(text, "/@manage/settings/other") {
		t.Error("即使不打开浏览器也应打印地址")
	}
	if strings.Contains(text, "复制上面的地址") || strings.Contains(text, "已在浏览器中打开") {
		t.Errorf("--no-open 时不应有打开相关的提示：\n%s", text)
	}
}

func TestGuideTokenPageSkipsWhenTokenGiven(t *testing.T) {
	t.Setenv("SSH_CONNECTION", "")

	var out bytes.Buffer
	app := New()
	app.Out = &out
	app.Err = &out
	app.flagToken = "already-have-token"
	app.guideTokenPage("http://nas.com:5244", false)

	if out.Len() != 0 {
		t.Errorf("已通过 --token 传入时不该有任何引导输出：%q", out.String())
	}
}

func TestCanOpenBrowserFalseInSSH(t *testing.T) {
	t.Setenv("SSH_CONNECTION", "10.0.0.1 1 10.0.0.2 22")
	if canOpenBrowser() {
		t.Error("SSH 会话不应自动拉起浏览器")
	}
	t.Setenv("SSH_CONNECTION", "")
	t.Setenv("SSH_TTY", "/dev/pts/0")
	if canOpenBrowser() {
		t.Error("存在 SSH_TTY 时也不应自动拉起浏览器")
	}
}
