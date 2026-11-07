package openlist

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"time"

	"nst/internal/jsonx"
	"nst/internal/pathx"
)

// ProgressFunc 汇报传输进度；total 未知时为 0。
type ProgressFunc func(done, total int64)

type progressReader struct {
	r     io.Reader
	total int64
	done  int64
	cb    ProgressFunc
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	if n > 0 {
		p.done += int64(n)
		if p.cb != nil {
			p.cb(p.done, p.total)
		}
	}
	return n, err
}

// Upload 流式上传，不把文件读进内存。
func (c *Client) Upload(ctx context.Context, localPath, remoteDir, remoteName string,
	asTask bool, progress ProgressFunc) error {

	f, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("打开本地文件失败：%w", err)
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return fmt.Errorf("读取本地文件信息失败：%w", err)
	}
	size := st.Size()

	name := remoteName
	if name == "" {
		name = path.Base(localPath)
	}
	target := pathx.JoinRemote(remoteDir, name)

	headers := map[string]string{
		"File-Path":      pathx.QuoteForHeader(target),
		"Content-Length": fmt.Sprintf("%d", size),
	}
	if asTask {
		headers["As-Task"] = "true"
	}

	var body io.Reader = f
	if progress != nil {
		body = &progressReader{r: f, total: size, cb: progress}
	}

	// Timeout < 0：大文件不设总超时，靠 context 取消
	_, err = c.do(ctx, http.MethodPut, "/api/fs/put", reqOptions{
		Body:    body,
		BodyLen: size,
		Headers: headers,
		Timeout: -1,
	})
	return err
}

// proxyURL 拼 OpenList 自己的 /d 通道，由服务端代理取文件。
func (c *Client) proxyURL(p string, info map[string]any) string {
	sign := jsonx.Str(info, "sign")
	if sign == "" {
		return ""
	}
	return c.Base + "/d" + pathx.QuoteForHeader(p) + "?sign=" + pathx.QuoteComponent(sign)
}

// DownloadURLs 按优先级返回候选下载地址。
//
// 115 这类网盘的 raw_url 带时效签名，客户端直连常被拒（invalid signature），
// 因此优先走服务端 /d 代理，再回落到 raw_url。
func (c *Client) DownloadURLs(ctx context.Context, p string, info map[string]any) ([]string, error) {
	if info == nil {
		var err error
		info, err = c.Stat(ctx, p, "")
		if err != nil {
			return nil, err
		}
	}
	candidates := make([]string, 0, 2)
	if proxy := c.proxyURL(p, info); proxy != "" {
		candidates = append(candidates, proxy)
	}
	if direct := jsonx.Str(info, "raw_url"); direct != "" {
		candidates = append(candidates, direct)
	} else if direct := jsonx.Str(info, "url"); direct != "" {
		candidates = append(candidates, direct)
	}
	if len(candidates) == 0 {
		return nil, NewApiErrorNoCode("该文件没有可用的下载地址", "/api/fs/get")
	}
	return candidates, nil
}

// DownloadURL 返回首选下载地址。
func (c *Client) DownloadURL(ctx context.Context, p string) (string, error) {
	urls, err := c.DownloadURLs(ctx, p, nil)
	if err != nil {
		return "", err
	}
	return urls[0], nil
}

// Fetch 下载：逐个候选地址尝试，成功即写盘。
func (c *Client) Fetch(ctx context.Context, remotePath, localPath string, progress ProgressFunc) error {
	info, err := c.Stat(ctx, remotePath, "")
	if err != nil {
		return err
	}
	total := jsonx.Int(info, "size")
	urls, err := c.DownloadURLs(ctx, remotePath, info)
	if err != nil {
		return err
	}

	headers := map[string]string{}
	if h := jsonx.Object(info["header"]); h != nil {
		for k, v := range h {
			headers[k] = jsonx.StrOf(v)
		}
	}

	var lastErr error
	for _, u := range urls {
		if err := c.fetchOne(ctx, u, headers, localPath, total, progress); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	return NewApiErrorNoCode(fmt.Sprintf("所有下载地址都失败：%v", lastErr), "/api/fs/get")
}

func (c *Client) fetchOne(ctx context.Context, url string, headers map[string]string,
	localPath string, total int64, progress ProgressFunc) error {

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	out, err := os.Create(localPath)
	if err != nil {
		return err
	}
	defer out.Close()

	if total == 0 {
		total = resp.ContentLength
	}
	buf := make([]byte, 256*1024)
	var done int64
	lastTick := time.Time{}
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := out.Write(buf[:n]); err != nil {
				return err
			}
			done += int64(n)
			// 限流回调频率，避免每 256KB 都刷一次终端
			if progress != nil && time.Since(lastTick) > 100*time.Millisecond {
				lastTick = time.Now()
				progress(done, total)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	if progress != nil {
		progress(done, total)
	}
	return nil
}
