package app

import (
	"strings"
	"testing"
)

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

func TestCanOpenBrowserFalseInSSH(t *testing.T) {
	t.Setenv("SSH_CONNECTION", "10.0.0.1 1 10.0.0.2 22")
	if CanOpenBrowser() {
		t.Error("SSH 会话不应自动拉起浏览器")
	}
	t.Setenv("SSH_CONNECTION", "")
	t.Setenv("SSH_TTY", "/dev/pts/0")
	if CanOpenBrowser() {
		t.Error("存在 SSH_TTY 时也不应自动拉起浏览器")
	}
}
