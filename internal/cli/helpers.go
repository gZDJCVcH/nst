package cli

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"nst/internal/client"
	"nst/internal/jsonx"
	"nst/internal/output"
	"nst/internal/pathx"
)

// asAPIError 是 errors.As 的薄封装，便于在命令里分支处理。
func asAPIError(err error, target **client.ApiError) bool {
	return errors.As(err, target)
}

func codeString(code *int) string {
	if code == nil {
		return "None"
	}
	return fmt.Sprintf("%d", *code)
}

// scalar 把任意 JSON 值渲染成便于阅读的单行文本。
func scalar(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		if t {
			return "true"
		}
		return "false"
	}
	if b, err := jsonx.Encode(v); err == nil {
		return string(b)
	}
	return fmt.Sprintf("%v", v)
}

// sha1OfFile 算文件 sha1（大写十六进制），用于和远端 hash_info 比对。
func sha1OfFile(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha1.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return strings.ToUpper(hex.EncodeToString(h.Sum(nil))), nil
}

// localFile 是本地文件枚举结果。
type localFile struct {
	rel string // 相对路径，posix 分隔
	abs string
}

// iterLocalFiles 枚举本地文件，跳过 macOS 的系统垃圾文件。
func iterLocalFiles(root string) ([]localFile, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return []localFile{{rel: "", abs: root}}, nil
	}

	var items []localFile
	err = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if name == ".DS_Store" || strings.HasPrefix(name, "._") {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		items = append(items, localFile{rel: filepath.ToSlash(rel), abs: p})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(items, func(i, j int) bool { return items[i].rel < items[j].rel })
	return items, nil
}

// iterUploadCandidates 按 -r 规则枚举待上传文件。
func iterUploadCandidates(localPath string, recursive bool) ([]localFile, error) {
	info, err := os.Stat(localPath)
	if err != nil {
		return nil, exitf(1, "本地路径不存在：%s", localPath)
	}
	if !info.IsDir() {
		return []localFile{{rel: "", abs: localPath}}, nil
	}
	if !recursive {
		return nil, exitf(1, "%s 是目录，上传目录需要 -r", localPath)
	}
	return iterLocalFiles(localPath)
}

// mkdirP 逐级创建远端目录。
func mkdirP(c *client.Client, ctx context.Context, p string) error {
	cur := ""
	for _, part := range strings.Split(pathx.NormalizeRemote(p), "/") {
		if part == "" {
			continue
		}
		cur += "/" + part
		if !c.Exists(ctx, cur) {
			if err := c.Mkdir(ctx, cur); err != nil {
				return err
			}
		}
	}
	return nil
}

// maskedAddition 对 storage 的 addition JSON 字符串逐键脱敏。解析失败返回 nil。
func maskedAddition(raw string, show bool) map[string]any {
	if strings.TrimSpace(raw) == "" {
		raw = "{}"
	}
	v, err := jsonx.DecodeString(raw)
	if err != nil {
		return nil
	}
	m := jsonx.Object(v)
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, val := range m {
		if output.IsSensitive(k) {
			out[k] = output.MaskSecret(val, show)
		} else {
			out[k] = val
		}
	}
	return out
}

// relJoin 在 posix 语义下拼接路径。
func relJoin(parts ...string) string { return path.Join(parts...) }
