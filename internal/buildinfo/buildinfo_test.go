package buildinfo

import "testing"

func TestString(t *testing.T) {
	origVersion, origCommit, origDate := Version, Commit, Date
	t.Cleanup(func() { Version, Commit, Date = origVersion, origCommit, origDate })

	Version, Commit, Date = "0.2.0", "abc1234", "2026-11-01T10:00:00Z"
	if got, want := String(), "0.2.0 (abc1234, built 2026-11-01)"; got != want {
		t.Errorf("String() = %q，期望 %q", got, want)
	}

	Version, Commit, Date = "0.2.0", "", ""
	if got := String(); got != "0.2.0" {
		t.Errorf("只有版本号时应只输出版本：%q", got)
	}

	Version, Commit, Date = "0.2.0", "abc1234", ""
	if got, want := String(), "0.2.0 (abc1234)"; got != want {
		t.Errorf("String() = %q，期望 %q", got, want)
	}
}
