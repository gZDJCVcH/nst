package openlist

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"nst/internal/app"
)

func loginServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		write := func(v any) {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": v})
		}
		switch r.URL.Path {
		case "/api/public/settings":
			write(map[string]any{"version": "4.2.9", "site_title": "OpenList"})
		case "/api/me":
			write(map[string]any{"username": "admin", "role_name": "管理员", "permission": 1})
		case "/api/auth/login":
			write(map[string]any{"token": "exchanged-token"})
		default:
			_, _ = w.Write([]byte(`{"code":404,"message":"not found"}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func cleanLoginEnv(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv(store.EnvURL, "")
	t.Setenv(store.EnvToken, "")
}

func decodeResult(t *testing.T, out string) map[string]any {
	t.Helper()
	if !json.Valid([]byte(strings.TrimSpace(out))) {
		t.Fatalf("stdout 不是合法 JSON：%q", out)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("解析结果失败：%v", err)
	}
	return m
}

func TestLoginFlagsModeJSON(t *testing.T) {
	cleanLoginEnv(t)
	srv := loginServer(t)

	out, _, code := runCLI(t, "--json", "auth", "login", "--mode=flags", "--url", srv.URL, "--token", "tok")
	if code != 0 {
		t.Fatalf("退出码 %d，stdout=%q", code, out)
	}
	m := decodeResult(t, out)
	if m["mode"] != "flags" || m["source"] != "flag" {
		t.Errorf("应为 flags/flag：%v", m)
	}
	if m["valid"] != true || m["saved"] != true {
		t.Errorf("应校验通过并保存：%v", m)
	}
	if _, ok := m["error"]; ok {
		t.Errorf("成功结果不应带 error：%v", m)
	}
}

func TestLoginAutoInfersFlags(t *testing.T) {
	cleanLoginEnv(t)
	srv := loginServer(t)

	out, _, code := runCLI(t, "--json", "auth", "login", "--url", srv.URL, "--token", "tok")
	if code != 0 {
		t.Fatalf("退出码 %d，stdout=%q", code, out)
	}
	if m := decodeResult(t, out); m["mode"] != "flags" {
		t.Errorf("参数齐全应自动判为 flags：%v", m)
	}
}

func TestLoginContextFromConfig(t *testing.T) {
	cleanLoginEnv(t)
	srv := loginServer(t)
	if _, err := store.Save(map[string]any{"url": srv.URL, "token": "saved-tok"}); err != nil {
		t.Fatal(err)
	}

	out, _, code := runCLI(t, "--json", "auth", "login", "--mode=context")
	if code != 0 {
		t.Fatalf("退出码 %d，stdout=%q", code, out)
	}
	m := decodeResult(t, out)
	if m["mode"] != "context" || m["source"] != "context" {
		t.Errorf("应为 context/context：%v", m)
	}
	if m["saved"] != false {
		t.Errorf("上下文模式不应重复落盘：%v", m)
	}
}

func TestLoginContextFromEnv(t *testing.T) {
	cleanLoginEnv(t)
	srv := loginServer(t)
	t.Setenv(store.EnvURL, srv.URL)
	t.Setenv(store.EnvToken, "env-tok")

	out, _, code := runCLI(t, "--json", "auth", "login")
	if code != 0 {
		t.Fatalf("退出码 %d，stdout=%q", code, out)
	}
	m := decodeResult(t, out)
	if m["mode"] != "context" || m["source"] != "context" {
		t.Errorf("环境变量应自动走 context：%v", m)
	}
	if m["saved"] != false {
		t.Errorf("环境变量凭据不应落盘：%v", m)
	}
}

func TestLoginNonTTYNoCredentialsReturnsStructuredError(t *testing.T) {
	cleanLoginEnv(t)

	out, _, code := runCLI(t, "--json", "auth", "login")
	if code == 0 {
		t.Fatalf("缺凭据应失败，stdout=%q", out)
	}
	m := decodeResult(t, out)
	if m["valid"] != false || m["error"] == nil || m["error"] == "" {
		t.Errorf("应返回结构化失败：%v", m)
	}
}

func TestLoginInteractiveRequiresTTY(t *testing.T) {
	cleanLoginEnv(t)

	out, _, code := runCLI(t, "--json", "auth", "login", "--mode=interactive")
	if code == 0 {
		t.Fatalf("非 tty 不应进入交互模式，stdout=%q", out)
	}
	m := decodeResult(t, out)
	if !strings.Contains(m["error"].(string), "终端") {
		t.Errorf("应提示需要终端：%v", m)
	}
}

func TestLoginFlagsModeRejectsMissingToken(t *testing.T) {
	cleanLoginEnv(t)
	srv := loginServer(t)

	out, _, code := runCLI(t, "--json", "auth", "login", "--mode=flags", "--url", srv.URL)
	if code == 0 {
		t.Fatalf("flags 模式缺 token 应失败，stdout=%q", out)
	}
	m := decodeResult(t, out)
	if !strings.Contains(m["error"].(string), "token") {
		t.Errorf("应提示缺少 token：%v", m)
	}
}

func TestLoginUnknownMode(t *testing.T) {
	cleanLoginEnv(t)

	out, _, code := runCLI(t, "--json", "auth", "login", "--mode=bogus")
	if code == 0 {
		t.Fatalf("未知模式应失败，stdout=%q", out)
	}
	if m := decodeResult(t, out); !strings.Contains(m["error"].(string), "未知登录模式") {
		t.Errorf("应提示未知模式：%v", m)
	}
}

func TestLoginPasswordFlagsMode(t *testing.T) {
	cleanLoginEnv(t)
	srv := loginServer(t)

	out, _, code := runCLI(t, "--json", "auth", "login", "--mode=flags", "--url", srv.URL, "--password=secret")
	if code != 0 {
		t.Fatalf("退出码 %d，stdout=%q", code, out)
	}
	m := decodeResult(t, out)
	if m["mode"] != "flags" || m["valid"] != true || m["saved"] != true {
		t.Errorf("密码换令牌应走 flags 并保存：%v", m)
	}
}

func TestLoginInteractivePromptsForMissingURL(t *testing.T) {
	cleanLoginEnv(t)
	srv := loginServer(t)

	var out, errBuf bytes.Buffer
	a := app.New()
	a.Out = &out
	a.Err = &errBuf
	a.In = strings.NewReader(srv.URL + "\n")
	a.SetTTY(true)

	// 用 --token 给齐凭据，避免走到需要真实终端的密码输入框。
	root := testRoot(a)
	code := a.Execute([]string{"openlist", "auth", "login", "--mode=interactive", "--token", "tok"}, func(*app.App) *cobra.Command { return root })
	if code != 0 {
		t.Fatalf("退出码 %d，stdout=%q stderr=%q", code, out.String(), errBuf.String())
	}
	if !strings.Contains(out.String(), "OpenList 地址") {
		t.Errorf("地址为空时应先询问地址：%q", out.String())
	}
}

func TestLoginJSONFlagAtEnd(t *testing.T) {
	cleanLoginEnv(t)
	srv := loginServer(t)

	out, _, code := runCLI(t, "auth", "login", "--mode=flags", "--url", srv.URL, "--token", "tok", "--json")
	if code != 0 {
		t.Fatalf("退出码 %d，stdout=%q", code, out)
	}
	decodeResult(t, out)
}

func TestLoginJSONOutputHasNoHumanNoise(t *testing.T) {
	cleanLoginEnv(t)
	srv := loginServer(t)

	out, _, _ := runCLI(t, "--json", "auth", "login", "--url", srv.URL, "--token", "tok")
	for _, noise := range []string{"已连接", "令牌页面", "已登录", "浏览器"} {
		if strings.Contains(out, noise) {
			t.Errorf("--json 输出不应含人读文案 %q：%s", noise, out)
		}
	}
}
