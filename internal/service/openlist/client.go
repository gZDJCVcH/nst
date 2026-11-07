// Package openlist 实现 OpenList 命令行客户端：API、限流、缓存、传输与命令。
package openlist

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"nst/internal/apierr"
	"nst/internal/jsonx"
	"nst/internal/pathx"
)

// 默认参数。
const (
	DefaultTimeout  = 30 * time.Second
	CloudInterval   = 1 * time.Second
	LocalInterval   = 50 * time.Millisecond
	CacheTTL        = 60 * time.Second
	MountsTTL       = 300 * time.Second
	chunkSize       = 1 << 20
	taskQueryPrefix = "tid="
)

// TaskTypes 是服务端支持的全部任务类型（没有全局任务列表接口）。
var TaskTypes = []string{
	"upload", "copy", "move", "offline_download",
	"offline_download_transfer", "decompress", "decompress_upload",
}

// localDrivers 是不受网盘限流约束的驱动。
var localDrivers = map[string]bool{
	"local": true, "s3": true, "ftp": true, "sftp": true, "smb": true, "webdav": true,
}

// openListHints 是 OpenList 专属的错误提示。
var openListHints = apierr.Hints{
	Unauthorized: "\n  令牌无效或已过期，重新登录：nst openlist auth login --url <地址>",
	Forbidden:    "\n  权限不足（该操作需要管理员，或存储为只读）",
	Unsupported:  "\n  该存储不支持此操作（115 令牌过期时写操作常报此错）",
}

// NewApiError 构造带状态码的错误。
func NewApiError(code int, message, endpoint string) *apierr.ApiError {
	return &apierr.ApiError{Code: &code, Message: message, Endpoint: endpoint, Hints: openListHints}
}

// NewApiErrorNoCode 构造没有状态码的错误（响应不是 JSON 时用）。
func NewApiErrorNoCode(message, endpoint string) *apierr.ApiError {
	return &apierr.ApiError{Message: message, Endpoint: endpoint, Hints: openListHints}
}

// Mount 是一个挂载点及其驱动。
type Mount struct {
	Path   string
	Driver string
}

type dirCacheEntry struct {
	at   time.Time
	data map[string]any
}

// Client 是 OpenList 客户端。零值不可用，请用 New 构造。
type Client struct {
	Base  string
	Token string

	CloudInterval time.Duration
	LocalInterval time.Duration
	CacheTTL      time.Duration
	Timeout       time.Duration
	Hints         apierr.Hints

	http *http.Client

	mu           sync.Mutex
	lastByKey    map[string]time.Time
	dirCache     map[string]dirCacheEntry
	mountsCache  []Mount
	mountsAt     time.Time
	loadingMount bool
}

// New 创建客户端。url 为空会返回错误。
func New(base, token string) (*Client, error) {
	if base == "" {
		return nil, errors.New("缺少服务地址")
	}
	return &Client{
		Base:          strings.TrimRight(base, "/"),
		Token:         token,
		CloudInterval: CloudInterval,
		LocalInterval: LocalInterval,
		CacheTTL:      CacheTTL,
		Timeout:       DefaultTimeout,
		Hints:         openListHints,
		http: &http.Client{
			Transport: &http.Transport{
				Proxy:                 http.ProxyFromEnvironment,
				MaxIdleConnsPerHost:   4,
				IdleConnTimeout:       90 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: 60 * time.Second,
			},
		},
		lastByKey: map[string]time.Time{},
		dirCache:  map[string]dirCacheEntry{},
	}, nil
}

// ------------------------------------------------------------------ 底层

type reqOptions struct {
	Payload any
	Body    io.Reader
	BodyLen int64
	Headers map[string]string
	// Timeout < 0 表示不设总超时（传大文件用）。
	Timeout time.Duration
}

func (c *Client) headers(extra map[string]string) map[string]string {
	h := map[string]string{
		"User-Agent": "nst/0.1",
		"Accept":     "application/json",
	}
	if c.Token != "" {
		h["Authorization"] = c.Token
	}
	for k, v := range extra {
		h[k] = v
	}
	return h
}

func (c *Client) do(ctx context.Context, method, endpoint string, opt reqOptions) (any, error) {
	c.Throttle(PathHint(opt.Payload, opt.Headers))

	url := c.Base + endpoint
	var body io.Reader
	if opt.Payload != nil {
		buf, err := jsonx.Encode(opt.Payload)
		if err != nil {
			return nil, err
		}
		body = strings.NewReader(string(buf))
		opt.BodyLen = int64(len(buf))
		if opt.Headers == nil {
			opt.Headers = map[string]string{}
		}
		if _, ok := opt.Headers["Content-Type"]; !ok {
			opt.Headers["Content-Type"] = "application/json"
		}
	} else if opt.Body != nil {
		body = opt.Body
		if opt.Headers == nil {
			opt.Headers = map[string]string{}
		}
		if _, ok := opt.Headers["Content-Type"]; !ok {
			opt.Headers["Content-Type"] = "application/octet-stream"
		}
	}

	timeout := c.Timeout
	if opt.Timeout != 0 {
		timeout = opt.Timeout
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, &apierr.NetworkError{Msg: fmt.Sprintf("请求构造失败：%v", err)}
	}
	if opt.BodyLen > 0 {
		req.ContentLength = opt.BodyLen
	}
	for k, v := range c.headers(opt.Headers) {
		req.Header.Set(k, v)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, networkError(url, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, networkError(url, err)
	}

	value, err := jsonx.Decode(raw)
	if err != nil {
		head := strings.ToLower(string(raw))
		if len(head) > 120 {
			head = head[:120]
		}
		trimmed := strings.TrimSpace(head)
		if strings.HasPrefix(trimmed, "<!doctype") || strings.HasPrefix(trimmed, "<html") {
			return nil, NewApiErrorNoCode(
				"该接口在当前 OpenList 版本中不存在（服务端返回了网页而不是 JSON）", endpoint)
		}
		return nil, NewApiErrorNoCode(fmt.Sprintf("返回的不是 JSON：%q", head), endpoint)
	}

	obj := jsonx.Object(value)
	if obj == nil {
		return nil, NewApiErrorNoCode(fmt.Sprintf("返回的不是预期结构：%d 字节", len(raw)), endpoint)
	}
	if code := jsonx.Int(obj, "code"); code != 200 {
		status := int(code)
		return nil, &apierr.ApiError{Code: &status, Message: jsonx.Str(obj, "message"), Endpoint: endpoint, Hints: c.Hints}
	}
	return obj["data"], nil
}

func networkError(url string, err error) *apierr.NetworkError {
	var nerr net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &nerr) && nerr.Timeout()) {
		return &apierr.NetworkError{Msg: fmt.Sprintf("请求超时：%s", url)}
	}
	reason := err.Error()
	var uerr interface{ Unwrap() error }
	if errors.As(err, &uerr) && uerr.Unwrap() != nil {
		reason = uerr.Unwrap().Error()
	}
	return &apierr.NetworkError{Msg: fmt.Sprintf("无法连接 %s：%s", url, reason)}
}

func (c *Client) post(ctx context.Context, endpoint string, payload map[string]any) (any, error) {
	return c.do(ctx, http.MethodPost, endpoint, reqOptions{Payload: payload})
}

func (c *Client) get(ctx context.Context, endpoint string) (any, error) {
	return c.do(ctx, http.MethodGet, endpoint, reqOptions{})
}

// PathHint 从请求体或头部推断路径，用于按驱动选择限流间隔。
func PathHint(payload any, headers map[string]string) string {
	if m := jsonx.Object(payload); m != nil {
		for _, key := range []string{"path", "dir", "src_dir", "dst_dir"} {
			if v := jsonx.Str(m, key); v != "" {
				return v
			}
		}
	}
	if headers != nil {
		if v, ok := headers["File-Path"]; ok && v != "" {
			if decoded, err := urlUnquote(v); err == nil {
				return decoded
			}
		}
	}
	return ""
}

// ------------------------------------------------------------------ 限流与挂载点

// Mounts 返回挂载点列表，按路径长度降序，便于最长前缀匹配。
// 需要管理员权限；拿不到就返回空，此时全部请求按网盘间隔限流。
func (c *Client) Mounts(ctx context.Context, refresh bool) []Mount {
	c.mu.Lock()
	if !refresh && c.mountsCache != nil && time.Since(c.mountsAt) < MountsTTL {
		cached := c.mountsCache
		c.mu.Unlock()
		return cached
	}
	if c.loadingMount {
		cached := c.mountsCache
		c.mu.Unlock()
		return cached
	}
	c.loadingMount = true
	c.mu.Unlock()

	result := []Mount{}
	items, err := c.StorageList(ctx)
	if err == nil {
		for _, item := range items {
			mp := strings.TrimRight(jsonx.Str(item, "mount_path"), "/")
			if mp == "" {
				mp = "/"
			}
			result = append(result, Mount{Path: mp, Driver: jsonx.Str(item, "driver")})
		}
	}
	sortMounts(result)

	c.mu.Lock()
	c.mountsCache = result
	c.mountsAt = time.Now()
	c.loadingMount = false
	c.mu.Unlock()
	return result
}

// DescribeTarget 返回 (挂载点, 驱动, 限流间隔)。
func (c *Client) DescribeTarget(path string) (mount, driver string, interval time.Duration) {
	if path == "" {
		path = "/"
	} else {
		path = pathx.NormalizeRemote(path)
	}
	for _, m := range c.Mounts(context.Background(), false) {
		if path == m.Path || strings.HasPrefix(path, m.Path+"/") {
			interval := c.CloudInterval
			if localDrivers[strings.ToLower(m.Driver)] {
				interval = c.LocalInterval
			}
			return m.Path, m.Driver, interval
		}
	}
	return "/", "(未知)", c.CloudInterval
}

// Throttle 按目标驱动等待足够间隔。
func (c *Client) Throttle(path string) {
	mount, _, interval := c.DescribeTarget(path)
	key := mount
	if path == "" {
		key = "*"
	}

	c.mu.Lock()
	last := c.lastByKey[key]
	c.mu.Unlock()

	if interval > 0 {
		if wait := interval - time.Since(last); wait > 0 {
			time.Sleep(wait)
		}
	}

	c.mu.Lock()
	c.lastByKey[key] = time.Now()
	c.mu.Unlock()
}

// ------------------------------------------------------------------ 认证

// Login 用账号密码换令牌。
func (c *Client) Login(ctx context.Context, username, password, otp string) (string, error) {
	data, err := c.post(ctx, "/api/auth/login", map[string]any{
		"username": username, "password": password, "otp_code": otp,
	})
	if err != nil {
		return "", err
	}
	token := ""
	if s, ok := data.(string); ok {
		token = s
	} else if m := jsonx.Object(data); m != nil {
		token = jsonx.Str(m, "token")
	}
	if token == "" {
		return "", NewApiErrorNoCode("登录成功但没有拿到令牌", "/api/auth/login")
	}
	c.Token = token
	return token, nil
}

// Me 返回当前用户信息。
func (c *Client) Me(ctx context.Context) (map[string]any, error) {
	data, err := c.get(ctx, "/api/me")
	if err != nil {
		return nil, err
	}
	if m := jsonx.Object(data); m != nil {
		return m, nil
	}
	return map[string]any{}, nil
}

// PublicSettings 不需要令牌，用来探活并确认对方是 OpenList。
func (c *Client) PublicSettings(ctx context.Context) (map[string]any, error) {
	data, err := c.get(ctx, "/api/public/settings")
	if err != nil {
		return nil, err
	}
	if m := jsonx.Object(data); m != nil {
		return m, nil
	}
	return map[string]any{}, nil
}

// ------------------------------------------------------------------ 浏览

// ListDir 列目录。默认走 TTL 缓存，避免对网盘造成无谓请求。
func (c *Client) ListDir(ctx context.Context, path, password string, page, perPage int, refresh bool) (map[string]any, error) {
	key := pathx.NormalizeRemote(path)
	useCache := !refresh && c.CacheTTL > 0 && page == 1 && perPage == 0

	if useCache {
		c.mu.Lock()
		entry, ok := c.dirCache[key]
		c.mu.Unlock()
		if ok && time.Since(entry.at) < c.CacheTTL {
			return entry.data, nil
		}
	}

	data, err := c.post(ctx, "/api/fs/list", map[string]any{
		"path": key, "password": password, "page": page,
		"per_page": perPage, "refresh": refresh,
	})
	if err != nil {
		return nil, err
	}
	obj := jsonx.Object(data)
	if obj == nil {
		obj = map[string]any{}
	}
	if page == 1 && perPage == 0 {
		c.mu.Lock()
		c.dirCache[key] = dirCacheEntry{at: time.Now(), data: obj}
		c.mu.Unlock()
	}
	return obj, nil
}

// Invalidate 写操作后清掉相关目录的缓存。
func (c *Client) Invalidate(path string) {
	key := pathx.NormalizeRemote(path)
	parent, _ := pathx.SplitRemote(key)
	c.mu.Lock()
	delete(c.dirCache, key)
	delete(c.dirCache, parent)
	c.mu.Unlock()
}

// Stat 取单个路径的详情。
func (c *Client) Stat(ctx context.Context, path, password string) (map[string]any, error) {
	data, err := c.post(ctx, "/api/fs/get", map[string]any{
		"path": pathx.NormalizeRemote(path), "password": password,
	})
	if err != nil {
		return nil, err
	}
	if obj := jsonx.Object(data); obj != nil {
		return obj, nil
	}
	return map[string]any{}, nil
}

// Search 搜索文件。
func (c *Client) Search(ctx context.Context, keyword, parent string, scope, page, perPage int, password string) (map[string]any, error) {
	data, err := c.post(ctx, "/api/fs/search", map[string]any{
		"parent": pathx.NormalizeRemote(parent), "keywords": keyword, "scope": scope,
		"page": page, "per_page": perPage, "password": password,
	})
	if err != nil {
		return nil, err
	}
	if obj := jsonx.Object(data); obj != nil {
		return obj, nil
	}
	return map[string]any{}, nil
}

// Exists 判断远端路径是否存在。
//
// 不用 fs/get 判断：不少存储（如 115）对不存在的路径返回 500「object not found」，
// 靠父目录列举更可靠。
func (c *Client) Exists(ctx context.Context, path string) bool {
	parent, name := pathx.SplitRemote(path)
	if name == "" {
		return true
	}
	data, err := c.ListDir(ctx, parent, "", 1, 0, false)
	if err != nil {
		return false
	}
	for _, it := range jsonx.Objects(data["content"]) {
		if jsonx.Str(it, "name") == name {
			return true
		}
	}
	return false
}

// ------------------------------------------------------------------ 写入

// Mkdir 建目录。
func (c *Client) Mkdir(ctx context.Context, path string) error {
	_, err := c.post(ctx, "/api/fs/mkdir", map[string]any{"path": pathx.NormalizeRemote(path)})
	return err
}

// Rename 重命名。
func (c *Client) Rename(ctx context.Context, path, newName string) error {
	_, err := c.post(ctx, "/api/fs/rename", map[string]any{
		"path": pathx.NormalizeRemote(path), "name": newName,
	})
	return err
}

// Move 移动。
func (c *Client) Move(ctx context.Context, src, dstDir string, overwrite bool) error {
	srcDir, srcName := pathx.SplitRemote(src)
	_, err := c.post(ctx, "/api/fs/move", map[string]any{
		"src_dir": srcDir, "dst_dir": pathx.NormalizeRemote(dstDir),
		"names": []string{srcName}, "overwrite": overwrite,
	})
	return err
}

// Copy 复制。
func (c *Client) Copy(ctx context.Context, src, dstDir string, overwrite bool) error {
	srcDir, srcName := pathx.SplitRemote(src)
	_, err := c.post(ctx, "/api/fs/copy", map[string]any{
		"src_dir": srcDir, "dst_dir": pathx.NormalizeRemote(dstDir),
		"names": []string{srcName}, "overwrite": overwrite,
	})
	return err
}

// Remove 删除单个条目（目录必须已空）。
func (c *Client) Remove(ctx context.Context, path string) error {
	parent, name := pathx.SplitRemote(path)
	if name == "" {
		return NewApiErrorNoCode("不能删除根目录", "/api/fs/remove")
	}
	_, err := c.post(ctx, "/api/fs/remove", map[string]any{
		"dir": parent, "names": []string{name},
	})
	return err
}

// ------------------------------------------------------------------ 任务

// 任务按类型分组，没有全局 /api/task/list；tid 必须走查询参数。

// TaskList 列出某类型的任务。
func (c *Client) TaskList(ctx context.Context, taskType string, done bool) ([]any, error) {
	state := "undone"
	if done {
		state = "done"
	}
	data, err := c.get(ctx, "/api/task/"+taskType+"/"+state)
	if err != nil {
		return nil, err
	}
	return jsonx.List(data), nil
}

// TaskInfo 查任务详情。
func (c *Client) TaskInfo(ctx context.Context, taskType, taskID string) (map[string]any, error) {
	data, err := c.post(ctx, "/api/task/"+taskType+"/info?"+taskQueryPrefix+taskID, nil)
	if err != nil {
		return nil, err
	}
	if obj := jsonx.Object(data); obj != nil {
		return obj, nil
	}
	return map[string]any{}, nil
}

// TaskCancel 取消任务。
func (c *Client) TaskCancel(ctx context.Context, taskType, taskID string) error {
	_, err := c.post(ctx, "/api/task/"+taskType+"/cancel?"+taskQueryPrefix+taskID, nil)
	return err
}

// TaskDelete 删除任务记录。
func (c *Client) TaskDelete(ctx context.Context, taskType, taskID string) error {
	_, err := c.post(ctx, "/api/task/"+taskType+"/delete?"+taskQueryPrefix+taskID, nil)
	return err
}

// TaskRetry 重试任务。
func (c *Client) TaskRetry(ctx context.Context, taskType, taskID string) error {
	_, err := c.post(ctx, "/api/task/"+taskType+"/retry?"+taskQueryPrefix+taskID, nil)
	return err
}

// TaskClear 清理已完成任务。
func (c *Client) TaskClear(ctx context.Context, taskType string, succeededOnly bool) error {
	action := "clear_done"
	if succeededOnly {
		action = "clear_succeeded"
	}
	_, err := c.post(ctx, "/api/task/"+taskType+"/"+action, nil)
	return err
}

// ------------------------------------------------------------------ 存储与驱动

// StorageList 列出全部存储。
func (c *Client) StorageList(ctx context.Context) ([]map[string]any, error) {
	data, err := c.get(ctx, "/api/admin/storage/list")
	if err != nil {
		return nil, err
	}
	return jsonx.Objects(jsonx.Object(data)["content"]), nil
}

// StorageGet 看单个存储（含明文 addition，展示前必须脱敏）。
func (c *Client) StorageGet(ctx context.Context, storageID int) (map[string]any, error) {
	data, err := c.get(ctx, "/api/admin/storage/get?id="+strconv.Itoa(storageID))
	if err != nil {
		return nil, err
	}
	if obj := jsonx.Object(data); obj != nil {
		return obj, nil
	}
	return map[string]any{}, nil
}

// DriverInfo 看某个驱动的配置项定义。
func (c *Client) DriverInfo(ctx context.Context, driver string) (map[string]any, error) {
	data, err := c.get(ctx, "/api/admin/driver/info?driver="+queryEscape(driver))
	if err != nil {
		return nil, err
	}
	if obj := jsonx.Object(data); obj != nil {
		return obj, nil
	}
	return map[string]any{}, nil
}

// Drivers 返回 {驱动名: {common: [...], additional: [...]}}。
func (c *Client) Drivers(ctx context.Context) (map[string]any, error) {
	data, err := c.get(ctx, "/api/admin/driver/list")
	if err != nil {
		return nil, err
	}
	if obj := jsonx.Object(data); obj != nil {
		return obj, nil
	}
	return map[string]any{}, nil
}
