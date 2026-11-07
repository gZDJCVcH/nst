package openlist

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"nst/internal/apierr"
)

// recorder 记录收到的请求，并可返回固定响应。
type recorder struct {
	mu       sync.Mutex
	requests []string
	body     any
}

func (r *recorder) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		r.mu.Lock()
		r.requests = append(r.requests, req.Method+" "+req.URL.RequestURI())
		body := r.body
		r.mu.Unlock()

		if body == nil {
			body = map[string]any{"code": 200, "data": nil}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}
}

func (r *recorder) last(t *testing.T) string {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.requests) == 0 {
		t.Fatal("没有收到任何请求")
	}
	return r.requests[len(r.requests)-1]
}

func newTestClient(t *testing.T, base string) *Client {
	t.Helper()
	c, err := New(base, "token")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	c.CloudInterval = 0
	c.LocalInterval = 0
	return c
}

func TestApiErrorHints(t *testing.T) {
	cases := []struct {
		name   string
		err    *apierr.ApiError
		has    []string
		hasNot []string
	}{
		{
			name: "401",
			err:  NewApiError(401, "token is invalidated", "/api/me"),
			has:  []string{"令牌无效或已过期", "auth login"},
		},
		{name: "403", err: NewApiError(403, "permission denied", ""), has: []string{"权限不足"}},
		{name: "404 路径", err: NewApiError(404, "object not found", ""), has: []string{"路径不存在"}},
		{
			name: "404 功能不可用",
			err:  NewApiError(404, "search not available", ""),
			has:  []string{"不可用"},
			hasNot: []string{
				"路径不存在",
			},
		},
		{
			name:   "404 资源不存在",
			err:    NewApiError(404, "driver [NotExist] not found", ""),
			has:    []string{"driver"},
			hasNot: []string{"路径不存在"},
		},
		{name: "405", err: NewApiError(405, "method not allowed", ""), has: []string{"115"}},
		{
			name: "保留原始信息",
			err:  NewApiError(500, "boom", "/api/fs/list"),
			has:  []string{"boom", "/api/fs/list"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			msg := c.err.Error()
			for _, want := range c.has {
				if !strings.Contains(msg, want) {
					t.Errorf("缺少 %q：%s", want, msg)
				}
			}
			for _, bad := range c.hasNot {
				if strings.Contains(msg, bad) {
					t.Errorf("不该出现 %q：%s", bad, msg)
				}
			}
		})
	}
}

func TestDownloadURLs(t *testing.T) {
	c := newTestClient(t, "http://example.test:5244")
	ctx := context.Background()

	urls, err := c.DownloadURLs(ctx, "/a/b.cue", map[string]any{
		"sign": "abc:0", "raw_url": "https://cdn.example/x?k=1",
	})
	if err != nil {
		t.Fatalf("DownloadURLs: %v", err)
	}
	if len(urls) != 2 {
		t.Fatalf("应有两个候选地址，实际 %d", len(urls))
	}
	if !strings.HasPrefix(urls[0], "http://example.test:5244/d/a/b.cue?sign=") {
		t.Errorf("首选应是 /d 代理：%s", urls[0])
	}
	if urls[1] != "https://cdn.example/x?k=1" {
		t.Errorf("回落地址不对：%s", urls[1])
	}

	urls, _ = c.DownloadURLs(ctx, "/a", map[string]any{"sign": "a+b=c:0", "raw_url": "https://cdn/x"})
	if !strings.Contains(urls[0], "sign=a%2Bb%3Dc%3A0") {
		t.Errorf("sign 必须百分号编码：%s", urls[0])
	}

	urls, _ = c.DownloadURLs(ctx, "/音乐/周杰伦.cue", map[string]any{"sign": "s", "raw_url": "https://cdn/x"})
	if !strings.Contains(urls[0], "%E5%91%A8%E6%9D%B0%E4%BC%A6") {
		t.Errorf("中文路径必须编码：%s", urls[0])
	}

	urls, _ = c.DownloadURLs(ctx, "/a", map[string]any{"raw_url": "https://cdn/x"})
	if len(urls) != 1 || urls[0] != "https://cdn/x" {
		t.Errorf("没有 sign 时只能直连：%v", urls)
	}

	if _, err := c.DownloadURLs(ctx, "/a", map[string]any{}); err == nil {
		t.Error("没有任何地址时应报错")
	}
}

func TestPathHint(t *testing.T) {
	if got := PathHint(map[string]any{"path": "/a"}, nil); got != "/a" {
		t.Errorf("PathHint = %q", got)
	}
	if got := PathHint(map[string]any{"path": "/a", "dir": "/b"}, nil); got != "/a" {
		t.Errorf("path 应优先于 dir：%q", got)
	}
	if got := PathHint(nil, map[string]string{"File-Path": "/%E9%9F%B3%E4%B9%90/x.cue"}); got != "/音乐/x.cue" {
		t.Errorf("File-Path 应解码：%q", got)
	}
	if got := PathHint(nil, nil); got != "" {
		t.Errorf("空输入应为空：%q", got)
	}
	if got := PathHint(map[string]any{}, map[string]string{}); got != "" {
		t.Errorf("空 map 应为空：%q", got)
	}
}

func TestThrottleByDriver(t *testing.T) {
	rec := &recorder{body: map[string]any{
		"code": 200,
		"data": map[string]any{"content": []any{
			map[string]any{"mount_path": "/cloud/夸克", "driver": "Quark"},
			map[string]any{"mount_path": "/local", "driver": "Local"},
			map[string]any{"mount_path": "/115/", "driver": "115 Open"},
		}},
	}}
	srv := httptest.NewServer(rec.handler())
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	c.CloudInterval = time.Second
	c.LocalInterval = 50 * time.Millisecond

	mount, driver, interval := c.DescribeTarget("/115/音乐/x.cue")
	if mount != "/115" || driver != "115 Open" || interval != time.Second {
		t.Errorf("网盘挂载点判断错误：%q %q %v", mount, driver, interval)
	}

	_, driver, interval = c.DescribeTarget("/local/pt/a")
	if driver != "Local" || interval != 50*time.Millisecond {
		t.Errorf("本地盘应使用短间隔：%q %v", driver, interval)
	}

	mount, driver, _ = c.DescribeTarget("/cloud/夸克/电影/a.mkv")
	if mount != "/cloud/夸克" || driver != "Quark" {
		t.Errorf("最长前缀应优先：%q %q", mount, driver)
	}

	_, driver, interval = c.DescribeTarget("/not-mounted/a")
	if driver != "(未知)" || interval != time.Second {
		t.Errorf("未知路径应回落到网盘间隔：%q %v", driver, interval)
	}
}

func TestMountsFailureFallsBack(t *testing.T) {
	rec := &recorder{body: map[string]any{"code": 403, "message": "no permission"}}
	srv := httptest.NewServer(rec.handler())
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	c.CloudInterval = time.Second

	if got := c.Mounts(context.Background(), true); len(got) != 0 {
		t.Errorf("没有权限时应返回空：%v", got)
	}
	_, _, interval := c.DescribeTarget("/local/x")
	if interval != c.CloudInterval {
		t.Errorf("拿不到存储列表时应按网盘间隔：%v", interval)
	}
}

func TestStorageEndpoints(t *testing.T) {
	ctx := context.Background()

	rec := &recorder{body: map[string]any{
		"code": 200,
		"data": map[string]any{"content": []any{
			map[string]any{"id": 1}, map[string]any{"id": 2},
		}},
	}}
	srv := httptest.NewServer(rec.handler())
	defer srv.Close()
	c := newTestClient(t, srv.URL)

	items, err := c.StorageList(ctx)
	if err != nil {
		t.Fatalf("StorageList: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("应解包出 2 项，实际 %d", len(items))
	}

	rec.body = map[string]any{"code": 200, "data": map[string]any{"id": 3}}
	if _, err := c.StorageGet(ctx, 3); err != nil {
		t.Fatalf("StorageGet: %v", err)
	}
	if got := rec.last(t); got != "GET /api/admin/storage/get?id=3" {
		t.Errorf("storage get 请求不对：%s", got)
	}

	rec.body = map[string]any{"code": 200, "data": map[string]any{}}
	if _, err := c.DriverInfo(ctx, "115 Open"); err != nil {
		t.Fatalf("DriverInfo: %v", err)
	}
	if got := rec.last(t); got != "GET /api/admin/driver/info?driver=115+Open" {
		t.Errorf("driver info 编码不对（空格应为 +）：%s", got)
	}

	if _, err := c.DriverInfo(ctx, "阿里云盘"); err != nil {
		t.Fatalf("DriverInfo: %v", err)
	}
	if got := rec.last(t); !strings.Contains(got, "driver=%E9%98%BF") {
		t.Errorf("中文驱动名应编码：%s", got)
	}
}

func TestTaskRoutes(t *testing.T) {
	ctx := context.Background()
	rec := &recorder{body: map[string]any{"code": 200, "data": []any{}}}
	srv := httptest.NewServer(rec.handler())
	defer srv.Close()
	c := newTestClient(t, srv.URL)

	if _, err := c.TaskList(ctx, "copy", false); err != nil {
		t.Fatalf("TaskList: %v", err)
	}
	if got := rec.last(t); got != "GET /api/task/copy/undone" {
		t.Errorf("未完成任务路由不对：%s", got)
	}

	_, _ = c.TaskList(ctx, "upload", true)
	if got := rec.last(t); got != "GET /api/task/upload/done" {
		t.Errorf("已完成任务路由不对：%s", got)
	}

	rec.body = map[string]any{"code": 200, "data": map[string]any{}}
	for _, action := range []string{"cancel", "delete", "retry", "info"} {
		rec.mu.Lock()
		rec.requests = nil
		rec.mu.Unlock()

		var err error
		switch action {
		case "cancel":
			err = c.TaskCancel(ctx, "upload", "TID123")
		case "delete":
			err = c.TaskDelete(ctx, "upload", "TID123")
		case "retry":
			err = c.TaskRetry(ctx, "upload", "TID123")
		case "info":
			_, err = c.TaskInfo(ctx, "upload", "TID123")
		}
		if err != nil {
			t.Fatalf("%s: %v", action, err)
		}
		got := rec.last(t)
		if !strings.HasPrefix(got, "POST ") || !strings.HasSuffix(got, "?tid=TID123") {
			t.Errorf("%s 的 tid 必须走查询参数：%s", action, got)
		}
	}

	rec.mu.Lock()
	rec.requests = nil
	rec.mu.Unlock()
	_ = c.TaskClear(ctx, "move", false)
	_ = c.TaskClear(ctx, "move", true)
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if !strings.HasSuffix(rec.requests[0], "/clear_done") {
		t.Errorf("clear_done 路由不对：%s", rec.requests[0])
	}
	if !strings.HasSuffix(rec.requests[1], "/clear_succeeded") {
		t.Errorf("clear_succeeded 路由不对：%s", rec.requests[1])
	}
}

func TestUploadStreamsWithProgress(t *testing.T) {
	var (
		gotPath     string
		gotLength   int64
		gotBodySize int
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.Header.Get("File-Path")
		gotLength = r.ContentLength
		body, _ := io.ReadAll(r.Body)
		gotBodySize = len(body)
		_, _ = w.Write([]byte(`{"code":200,"data":null}`))
	}))
	defer srv.Close()

	f, err := os.CreateTemp(t.TempDir(), "up-*")
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("0123456789")
	if _, err := f.Write(payload); err != nil {
		t.Fatal(err)
	}
	f.Close()

	var lastDone, lastTotal int64
	c := newTestClient(t, srv.URL)
	err = c.Upload(context.Background(), f.Name(), "/音乐", "周杰伦.cue", false,
		func(done, total int64) { lastDone, lastTotal = done, total })
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if gotBodySize != len(payload) {
		t.Errorf("上传字节数不对：%d", gotBodySize)
	}
	if gotLength != int64(len(payload)) {
		t.Errorf("Content-Length 不对：%d", gotLength)
	}
	if !strings.Contains(gotPath, "%E5%91%A8%E6%9D%B0%E4%BC%A6") {
		t.Errorf("File-Path 应百分号编码：%s", gotPath)
	}
	if lastDone != int64(len(payload)) || lastTotal != int64(len(payload)) {
		t.Errorf("进度回调不对：done=%d total=%d", lastDone, lastTotal)
	}
}

func TestListDirCacheAndInvalidate(t *testing.T) {
	ctx := context.Background()
	rec := &recorder{body: map[string]any{
		"code": 200,
		"data": map[string]any{"content": []any{map[string]any{"name": "a.cue"}}},
	}}
	srv := httptest.NewServer(rec.handler())
	defer srv.Close()
	c := newTestClient(t, srv.URL)

	if _, err := c.ListDir(ctx, "/115", "", 1, 0, false); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ListDir(ctx, "/115", "", 1, 0, false); err != nil {
		t.Fatal(err)
	}
	if n := countPath(rec, "/api/fs/list"); n != 1 {
		t.Errorf("第二次应命中缓存，实际请求 %d 次", n)
	}

	if !c.Exists(ctx, "/115/a.cue") {
		t.Error("Exists 应命中缓存并返回 true")
	}
	if c.Exists(ctx, "/115/b.cue") {
		t.Error("不存在的名字应返回 false")
	}

	c.Invalidate("/115/a.cue")
	before := countPath(rec, "/api/fs/list")
	if _, err := c.ListDir(ctx, "/115", "", 1, 0, false); err != nil {
		t.Fatal(err)
	}
	if countPath(rec, "/api/fs/list") == before {
		t.Error("Invalidate 后应重新请求")
	}
}

func TestNonJSONResponseIsFriendly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<!DOCTYPE html><html><body>SPA</body></html>"))
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	_, err := c.StorageList(context.Background())
	if err == nil {
		t.Fatal("应报错")
	}
	if !strings.Contains(err.Error(), "服务端返回了网页") {
		t.Errorf("应提示接口不存在：%v", err)
	}
}

// countPath 统计某个路径被请求了多少次。
func countPath(r *recorder, needle string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, req := range r.requests {
		if strings.Contains(req, needle) {
			n++
		}
	}
	return n
}
