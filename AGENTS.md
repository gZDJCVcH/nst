# AGENTS.md

给在此仓库工作的 AI agent 的上下文。人类用户请直接看 `README.md` 或各命令的 `--help`。

## 项目定位

`openlist-cli` 是 [OpenList](https://github.com/OpenListTeam/OpenList) 的命令行客户端，Go 实现。

由同目录下的 Python 版（`../openlist-cli`）**逐命令移植**而来。移植原则是**行为等价优先**：命令名、别名、参数、输出格式、错误提示、退出码都尽量与 Python 版一致；只有在语言能力确实不支持时才偏离，并在 README 的差异表里登记。

对外依赖只有两个：`spf13/cobra`（命令树 + pflag）和 `golang.org/x/term`（密码输入、istty 判断）。**不要再引入第三方依赖**，这是刻意保持的约束。

## 目录与职责

```
cmd/openlist-cli/main.go   只做 os.Exit 转换
internal/cli/              命令定义与渲染
  app.go                   App：IO、全局参数、退出码映射、newClient
  root.go                  命令树注册 + 全局 flag
  auth.go browse.go transfer.go ops.go tasks.go admin.go verify.go version.go upgrade.go
  helpers.go               本地文件枚举、sha1、脱敏、mkdirP
  confighelp.go            配置读写薄封装
internal/client/           HTTP 层
  client.go                _request 等价物、鉴权、错误映射、限流、目录缓存
  transfer.go              上传/下载（流式 + 进度）
  util.go                  URL 编码、挂载点排序
internal/pathx/            local: / remote: 前缀解析
internal/config/           ~/.config/openlist-cli/config.json 读写（0600 原子写）
internal/output/           尺寸/时间格式化、列表渲染、进度、脱敏、确认
internal/jsonx/            任意 JSON 的宽松读取（json.Number，不丢精度）
```

新增功能通常**不动** `pathx` / `output`，只改 `client`（加接口封装）和 `cli`（加命令 + 参数 + 在 `root.go` 注册）。

## 命令

```bash
go build -o dist/openlist-cli ./cmd/openlist-cli
./dist/openlist-cli --help
./dist/openlist-cli <命令> -h

go test ./...        # 全部用例，离线，不碰网络
go vet ./...
gofmt -l .           # 应为空
```

`make cross` 可一次产出 6 个平台。

## 必须遵守的设计约束

**1. 路径一律带前缀，绝不智能推断。**

`local:<路径>` / `remote:/<路径>`，`remote:` 后必须绝对。缺前缀直接返回 `*pathx.PathError`，错误信息要给出两种正确写法。理由：本地绝对路径和远端路径在字符串上无法可靠区分，猜错一次就可能覆盖错文件。

**2. 请求必须限流，且按存储驱动分级。**

`/api/admin/storage/list` 拿到「挂载点 → 驱动」，按最长前缀匹配决定间隔：

- `Local` / `S3` / `WebDAV` / `FTP` / `SFTP` / `SMB` → 50ms（`--local-rate`）
- 其余（115、夸克等网盘）→ 1s（`--rate`）

115 有硬性限流，**任何新命令都必须走 `client.do`**，不要自己 `http.NewRequest` 绕过 `Throttle()`。拿不到存储列表时兜底为全部 1s。

**3. 目录列表默认走 60 秒内存缓存。**

`ListDir` 有 TTL 缓存，写操作后必须调 `Invalidate(path)`，否则同一次命令里会读到旧目录。需要穿透时用 `refresh=true`（`list --refresh`）。

**4. 破坏性操作默认二次确认。**

`remove` 不带 `-y` 时必须 `output.Confirm()`；非交互环境（stdin 非 tty）默认取消。递归删除非空目录必须显式 `-r`，且 `remove` 拒绝删除 `/`。

**5. 凭据一律脱敏。**

`storage get` 的 `addition` 是 JSON 字符串，里面躺着 115 的 `access_token` / `refresh_token` **明文**。展示时必须过 `output.IsSensitive()` + `output.MaskSecret()`，只有显式 `--show-secrets` 才输出原值。

**6. 错误信息要能指导下一步。**

`ApiError.Render()` 里按 code 给 hint：401 提示重新登录、403 提示权限、404 分「路径不存在」和「该功能在此存储不可用」两种、405 提示 115 令牌过期。新增错误分支时保持这个风格，不要只抛原始 message。

## OpenList / 115 的已知坑

| 现象 | 原因与对策 |
|---|---|
| 下载回来的文件是旧版本 | 115 的 `raw_url` 走 CDN，**会返回缓存的旧对象**。判断新旧一律看 `fs/get` 的 `hash_info.sha1`，不要用下载内容 |
| 直链 403 `invalid signature` | 不能用 `raw_url` 直连（UA/Referer 都救不了）。必须走 `{base}/d{path}?sign={sign}`，服务端代理。`DownloadURLs()` 已按此顺序返回候选 |
| `sign` 不能拼到 `raw_url` 后面 | `raw_url` 自带完整签名，再拼就废了。`sign` 只属于 `/d` 路由 |
| 查不存在的路径返回 500 | 115 返回 `500 object not found` 而不是 404。判断存在性用「列举父目录比对文件名」（`Exists()`） |
| 任务接口没有全局列表 | 任务是**按类型分组**的：`/api/task/{upload,copy,move,offline_download,offline_download_transfer,decompress,decompress_upload}/{undone,done,info,cancel,delete,retry,clear_done,clear_succeeded}`。没有 `/api/task/list`，访问它会被 gin 兜底成 SPA 的 HTML |
| 任务操作的 `tid` 位置 | 服务端用 `c.Query("tid")` 读，必须拼在 URL 上，放 JSON body 里无效 |
| `/api/admin/storage/list`、`/api/admin/driver/list` 只接受 GET | 用 POST 会掉进 SPA 路由，返回 HTML。`drivers` 响应是 `{驱动名: {...}}` 的 dict |
| 目录列表前后不一致 | 服务端也有缓存（他们为了防限流特意加的）。校验以 sha1 为准，别反复重试刷列表 |

## Go 特有的坑（本仓库已踩过）

| 坑 | 对策 |
|---|---|
| `os.UserConfigDir()` 在 macOS 返回 `~/Library/Application Support` | `config.Dir()` 手工拼 `$XDG_CONFIG_HOME` → `~/.config`，保 Python 版老配置 |
| `filepath` 用在远端路径上，Windows 会把 `/` 变成 `\` | 远端一律 `internal/pathx`（内部用 `path`），只有本地路径用 `filepath` |
| `path.Clean` 会折叠开头的 `//`，与 `posixpath.normpath` 不同 | `NormalizeRemote` 先手工折叠再 `path.Clean`，有单测覆盖 |
| `encoding/json` 默认把数字解成 float64，`id`/`size` 回显成科学计数法 | 统一用 `jsonx.Decode`（`UseNumber`），读值走 `jsonx.Int/Str/Bool` |
| 上传时手写 `Content-Length` 头无效 | 用 `req.ContentLength`（见 `Upload`） |
| 全局 `http.Client.Timeout` 会掐断大文件下载/上传 | 传文件传 `Timeout: -1`（不设总超时），靠 context 取消 |
| 中文按 rune 计数、显示占两列，`%-16s` 会错行 | 表格用固定列宽 + `[]rune` 计数；`verify` 的输出已按此处理 |
| cobra 的 `--flag` 默认值会打印 `(default -1)` | 用 `Changed()` 判断是否显式指定，默认值保持零值 |

## 验证方式

```bash
go test ./...          # 纯逻辑 + httptest 假服务端，不碰真实网络
./dist/openlist-cli mounts                       # 连通性与权限
./dist/openlist-cli --url <地址> --token <token> whoami
```

**涉及写操作时，测试数据必须建在自己的目录下**（例如 `/temp/_selftest_xxx`），只对该目录增删改，结束清理干净，绝不碰用户既有数据。

`verify` 是只读的，可以用真实数据跑：

```bash
./dist/openlist-cli verify "local:<目录>" "remote:/<目录>"
```

## 加一个命令要改哪几处

1. `cli/xxx.go` 里写 `func (a *App) xxxCmd() *cobra.Command`，先解析路径再 `a.newClient(true)`（顺序反了会先报"未登录"、掩盖路径错误）
2. `root.go` 的 `AddCommand(...)` 里注册
3. 在 `internal/cli/*_test.go` 补用例（用 `runCLI` + `httptest`，不要连真实服务）
4. 有写操作就调 `c.Invalidate()`
5. 更新 `README.md` 的命令表

## 代码风格

- 全部中文注释与中文 `--help`，标识符英文
- 错误用 `exitf(code, ...)` 返回 `*ExitError`，由 `App.Execute` 统一映射；不要 `os.Exit`
- 输出一律走 `a.printf/println`（`App.Out`），方便测试重定向
- 提交信息用 `feat:` / `fix:` / `refactor:` 前缀，正文说明**为什么**这么做

## 不要做

- 引入第三方依赖（cobra / x/term 之外）
- 绕过 `client` 直接发 HTTP 请求
- 用 `exec.Command` 调外部命令完成本可以用 API 做的事
- 在用户既有目录上做写操作测试
- 改 Release 资产命名（`openlist-cli_<ver>_<os>_<arch>`），`install.sh` 和 `upgrade` 都依赖它
