package cli

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"nst/internal/buildinfo"
)

// defaultRepo 是发布仓库（与 install.sh、.goreleaser.yaml 保持一致）。
// 可以用 NST_REPO=owner/repo 覆盖；OPENLIST_CLI_REPO 作为旧名继续兼容。
const defaultRepo = "gZDJCVcH/nst"

func repoSlug() string {
	if v := os.Getenv("NST_REPO"); v != "" {
		return v
	}
	if v := os.Getenv("OPENLIST_CLI_REPO"); v != "" {
		return v
	}
	return defaultRepo
}

func downloadBase() string {
	for _, key := range []string{"NST_DOWNLOAD_BASE", "OPENLIST_CLI_DOWNLOAD_BASE"} {
		if v := os.Getenv(key); v != "" {
			return strings.TrimRight(v, "/")
		}
	}
	return "https://github.com"
}

func (a *App) upgradeCmd() *cobra.Command {
	var (
		check       bool
		pinned      string
		force       bool
		showVersion bool
	)
	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "检查并升级到最新版本",
		Long: "默认检查最新版本并原地替换当前可执行文件。\n" +
			"只想看看有没有新版本：nst upgrade --check\n" +
			"国内网络可先设置 NST_DOWNLOAD_BASE 指向镜像。",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if showVersion {
				a.println(buildinfo.String())
				return nil
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()

			tag, err := resolveLatestTag(ctx, pinned)
			if err != nil {
				return err
			}
			current := buildinfo.Version

			if !force && !isNewer(tag, current) {
				if check {
					a.printf("已是最新版本 %s（远端 %s）\n", current, tag)
				} else {
					a.printf("已是最新版本 %s\n", current)
				}
				return nil
			}
			if check {
				a.printf("有新版本 %s（当前 %s）\n升级：nst upgrade\n", tag, current)
				return nil
			}

			a.printf("正在升级 %s → %s ...\n", current, tag)
			if err := applyUpgrade(ctx, tag, a.Err); err != nil {
				return err
			}
			a.printf("✓ 已升级到 %s\n", tag)
			return nil
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "只检查，不安装")
	cmd.Flags().StringVar(&pinned, "version", "", "升级到指定版本，如 v0.3.0")
	cmd.Flags().BoolVar(&force, "force", false, "与当前版本相同也重新安装")
	cmd.Flags().BoolVar(&showVersion, "current", false, "只打印当前版本")
	return cmd
}

// resolveLatestTag 解析最新 release 的 tag。
// 用跟随 302 的方式，不消耗 GitHub API 配额（匿名只有 60 次/小时）。
func resolveLatestTag(ctx context.Context, pinned string) (string, error) {
	if pinned != "" {
		if !strings.HasPrefix(pinned, "v") {
			pinned = "v" + pinned
		}
		return pinned, nil
	}
	url := fmt.Sprintf("%s/%s/releases/latest", downloadBase(), repoSlug())
	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", &netHint{Msg: fmt.Sprintf("无法查询最新版本：%v", err)}
	}
	defer resp.Body.Close()

	loc := resp.Header.Get("Location")
	if loc == "" {
		return "", &netHint{Msg: fmt.Sprintf(
			"无法解析最新版本（HTTP %d）。可能是仓库还没发布过 release，"+
				"或网络被拦截；可用 --version 手动指定", resp.StatusCode)}
	}
	tag := loc[strings.LastIndex(loc, "/")+1:]
	if tag == "" || tag == "latest" {
		return "", &netHint{Msg: "无法从重定向地址解析版本号：" + loc}
	}
	return tag, nil
}

// netHint 用于升级流程里的可读错误。
type netHint struct{ Msg string }

func (e *netHint) Error() string { return e.Msg }

func assetName(tag string) (string, error) {
	arch := runtime.GOARCH
	if arch == "arm" {
		return "", errors.New("暂不支持自动升级 32 位 ARM，请改用 install.sh 重新安装")
	}
	ext := "tar.gz"
	if runtime.GOOS == "windows" {
		ext = "zip"
	}
	return fmt.Sprintf("nst_%s_%s_%s.%s",
		strings.TrimPrefix(tag, "v"), runtime.GOOS, arch, ext), nil
}

func applyUpgrade(ctx context.Context, tag string, progressOut io.Writer) error {
	asset, err := assetName(tag)
	if err != nil {
		return err
	}
	base := fmt.Sprintf("%s/%s/releases/download/%s", downloadBase(), repoSlug(), tag)

	tmpDir, err := os.MkdirTemp("", "nst-upgrade-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	archivePath := filepath.Join(tmpDir, asset)
	if err := fetchFile(ctx, base+"/"+asset, archivePath); err != nil {
		return fmt.Errorf("下载失败：%w", err)
	}

	sumsPath := filepath.Join(tmpDir, "checksums.txt")
	if err := fetchFile(ctx, base+"/checksums.txt", sumsPath); err != nil {
		return fmt.Errorf("下载校验文件失败：%w", err)
	}
	if err := verifyChecksum(archivePath, sumsPath); err != nil {
		return err
	}

	binPath := filepath.Join(tmpDir, "nst")
	if runtime.GOOS == "windows" {
		binPath += ".exe"
	}
	if err := extractBinary(archivePath, binPath); err != nil {
		return fmt.Errorf("解包失败：%w", err)
	}
	if err := os.Chmod(binPath, 0o755); err != nil {
		return err
	}
	return replaceExecutable(binPath)
}

func fetchFile(ctx context.Context, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := (&http.Client{Timeout: 5 * time.Minute}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

func verifyChecksum(filePath, sumsPath string) error {
	sums, err := os.ReadFile(sumsPath)
	if err != nil {
		return err
	}
	name := filepath.Base(filePath)
	expected := ""
	for _, line := range strings.Split(string(sums), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == name {
			expected = strings.ToLower(fields[0])
			break
		}
	}
	if expected == "" {
		return fmt.Errorf("校验文件里没有 %s", name)
	}

	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	actual := hex.EncodeToString(h.Sum(nil))
	if actual != expected {
		return fmt.Errorf("sha256 校验失败（期望 %s，实际 %s）", expected, actual)
	}
	return nil
}

// extractBinary 从 tar.gz / zip 里取出可执行文件。
func extractBinary(archivePath, dest string) error {
	if strings.HasSuffix(archivePath, ".zip") {
		return extractFromZip(archivePath, dest)
	}
	return extractFromTarGz(archivePath, dest)
}

func extractFromTarGz(archivePath, dest string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if !isTargetBinary(hdr.Name) {
			continue
		}
		return writeExecutable(dest, tr)
	}
	return errors.New("压缩包里没找到 nst 可执行文件")
}

func extractFromZip(archivePath, dest string) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.FileInfo().IsDir() || !isTargetBinary(f.Name) {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()
		return writeExecutable(dest, rc)
	}
	return errors.New("压缩包里没找到 nst 可执行文件")
}

func isTargetBinary(name string) bool {
	base := filepath.Base(name)
	return base == "nst" || base == "nst.exe"
}

func writeExecutable(dest string, r io.Reader) error {
	f, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// replaceExecutable 原子替换当前可执行文件。
// Windows 上运行中的 exe 不能直接覆盖，先改名再写入。
func replaceExecutable(newPath string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}

	old := exe + ".old"
	os.Remove(old)
	if err := os.Rename(exe, old); err != nil {
		return fmt.Errorf("无法替换 %s（权限不足？试试 sudo）：%w", exe, err)
	}
	if err := os.Rename(newPath, exe); err != nil {
		_ = os.Rename(old, exe)
		return fmt.Errorf("替换失败，已回滚：%w", err)
	}
	os.Remove(old)
	return nil
}

// isNewer 比较 tag 是否比 current 新。解析失败时返回 false（保守不升级）。
func isNewer(tag, current string) bool {
	tn, ok1 := parseVersion(tag)
	cn, ok2 := parseVersion(current)
	if !ok1 || !ok2 {
		return false
	}
	for i := 0; i < 3; i++ {
		if tn[i] != cn[i] {
			return tn[i] > cn[i]
		}
	}
	return false
}

func parseVersion(s string) ([3]int, bool) {
	var out [3]int
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "v")
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) < 2 {
		return out, false
	}
	for i := 0; i < 3 && i < len(parts); i++ {
		n := 0
		if parts[i] == "" {
			return out, false
		}
		for _, c := range parts[i] {
			if c < '0' || c > '9' {
				return out, false
			}
			n = n*10 + int(c-'0')
		}
		out[i] = n
	}
	return out, true
}
