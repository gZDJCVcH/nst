# AGENTS.md

给在此仓库工作的 AI agent 的上下文。人类用户请直接看 `README.md` 或各命令的 `--help`。

## 项目定位

`nst` 是 NAS 工具箱，Go 实现，单个静态二进制。`nst openlist` 是 [OpenList](https://github.com/OpenListTeam/OpenList) 的命令行客户端，服务端对应 OpenList v4.x，API 与 AList v3 兼容。

对外依赖只有两个：`spf13/cobra`（命令树 + pflag）和 `golang.org/x/term`（密码输入、istty 判断）。**不要再引入第三方依赖**，这是刻意保持的约束。

## 目录与职责

```
cmd/nst/main.go              只做 os.Exit 转换；以 openlist-cli 名字调用时自动补 openlist 子命令
internal/cli/                组合根：命令树装配、version/upgrade
internal/app/                公共运行环境：IO、全局参数、退出码、提示、浏览器
internal/apperr/             退出码错误与统一错误分类
internal/apierr/             各服务共用的接口/网络错误
internal/auth/               登录模式、凭据解析、登录流程骨架（服务实现 hooks）
internal/config/             按服务配置（Store），0600 原子写
internal/service/openlist/   OpenList 服务：HTTP 客户端、命令、Register
  client.go transfer.go util.go
  auth.go browse.go ops.go tasks.go transfer_cmd.go admin.go verify.go
  helpers.go confighelp.go openbrowser.go register.go
internal/pathx/              local: / remote: 前缀解析
internal/output/             尺寸/时间格式化、列表渲染、进度、脱敏、确认
internal/jsonx/              任意 JSON 的宽松读取（json.Number，不丢精度）
skills/nst/                  Agent Skill：SKILL.md 索引 + references/<子命令>/ 子文档
scripts/check-skill.sh       Skill 结构校验，ci.yml 会跑
```

依赖方向：`cmd → cli → service/* → app/auth/config/...`。**服务包之间、服务包与 cli 之间禁止互相 import**。新增服务时在 `internal/service/<服务>/` 建包，提供 `Register(root, app) *cobra.Command`，再在 `cli/root.go` 的 `buildRoot()` 里注册；能力实现留在服务包，只有跨服务复用的部分才下沉到平台包。

## 命令

```bash
go build -o dist/nst ./cmd/nst
./dist/nst --help
./dist/nst openlist <命令> -h

go test ./...        # 全部用例，离线，不碰网络
go vet ./...
gofmt -l .           # 应为空
sh scripts/check-skill.sh
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

`ApiError.Render()` 里按 code 给 hint（在 `internal/apierr`，服务的提示通过 `Hints` 注入）：401 提示重新登录、403 提示权限、404 分「路径不存在」和「该功能在此存储不可用」两种、405 提示 115 令牌过期。新增错误分支时保持这个风格，不要只抛原始 message。

## OpenList / 115 的已知坑

| 现象 | 原因与对策 |
|---|---|
| 下载回来的文件是旧版本 | 115 的 `raw_url` 走 CDN，**会返回缓存的旧对象**。判断新旧一律看 `fs/get` 的 `hash_info.sha1`，不要用下载内容 |
| 直链 403 `invalid signature` | 不能用 `raw_url` 直连（UA/Referer 都救不了）。必须走 `{base}/d{path}?sign={sign}`，服务端代理。`DownloadURLs()` 已按此顺序返回候选 |
| `sign` 不能拼到 `raw_url` 后面 | `raw_url` 自带完整签名，再拼就废了。`sign` 只属于 `/d` 路由 |
| 查不存在的路径返回 500 | 115 返回 `500 object not found` 而不是 404。判断存在性用「列举父目录比对文件名」（`Exists()`） |
| 任务接口没有全局列表 | 任务是**按类型分组**的：`/api/task/{upload,copy,move,offline_download,offline_download_transfer,decompress,decompress_upload}/{undone,done,info,cancel,delete,retry,clear_done,clear_succeeded}`。没有 `/api/task/list`，访问它会被 gin 兜底成 SPA 的 HTML |
| 任务操作的 `tid` 位置 | 服务端用 `c.Query("tid")` 读，必须拼在 URL 上，放 JSON body 里无效 |
| `/api/admin/storage/list`、`/api/admin/driver/list` 只接受 GET | 用 POST 会掉进 SPA 路由，返回 HTML。`drivers` 响应是 `{驱动名: {...}}` 形式的对象 |
| 目录列表前后不一致 | 服务端也有缓存。校验以 sha1 为准，别反复重试刷列表 |

## 语言与平台相关的约束

| 坑 | 对策 |
|---|---|
| `os.UserConfigDir()` 在 macOS 返回 `~/Library/Application Support` | `config.Store.Dir()` 手工拼 `$XDG_CONFIG_HOME` → `~/.config`，配置位置不随平台变化 |
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
./dist/nst openlist mounts                       # 连通性与权限
./dist/nst openlist --url <地址> --token <token> whoami
```

**涉及写操作时，测试数据必须建在自己的目录下**（例如 `/temp/_selftest_xxx`），只对该目录增删改，结束清理干净，绝不碰用户既有数据。

`verify` 是只读的，可以用真实数据跑：

```bash
./dist/nst openlist verify "local:<目录>" "remote:/<目录>"
```

## 加一个命令要改哪几处

1. `internal/service/openlist/xxx.go` 里写 `func (a *CLI) xxxCmd() *cobra.Command`，先解析路径再 `a.newClient(true)`（顺序反了会先报“未登录”、掩盖路径错误）
2. `internal/service/openlist/register.go` 的 `Register()` 里注册
3. 在同包 `*_test.go` 补用例（用 `runCLI` + `httptest`，不要连真实服务）
4. 有写操作就调 `c.Invalidate()`
5. 更新 `README.md` 的命令表
6. 涉及新参数或触发场景时，更新 `skills/nst/references/openlist/` 下对应文档

## 登录模式约定

`internal/auth` 负责模式判定、字段优先级与登录流程骨架 `Flow`；服务通过 `Service` 接口提供 `AddressPrompt` / `NormalizeURL` / `TokenPage` / `Probe` / `Exchange` / `Validate` / `Save`。流程本身不含服务特定逻辑，OpenList 的实现见 `internal/service/openlist/auth.go`。

- 三种模式：`context`（配置 + 环境变量）、`flags`（参数一次给齐）、`interactive`（逐步询问）
- 默认 `auto` 推断，`--mode` 可强制；字段优先级 `--url` / `--token` > 环境变量 > 配置文件 > 交互询问
- 非交互模式不询问、不打开浏览器、不重试；`--json` 输出结构化 `Result`，stdout 保持纯 JSON
- 凭据只在 `flags` / `interactive` 下落盘；`context` 不重复写配置

## Skill 维护约定

- `skills/nst/SKILL.md` 只写索引与通用约定，不堆具体命令细节
- 子文档放在 `references/<子命令>/`，文件引用从 skill 根起保持一层深
- `SKILL.md` 的 `name` 必须与目录名一致
- 新增服务时在 `references/` 下建同名目录，并在主文档索引里补一行

## 代码风格

- 全部中文注释与中文 `--help`，标识符英文
- 错误用 `exitf(code, ...)` 返回 `*ExitError`，由 `App.Execute` 统一映射；不要 `os.Exit`
- 输出一律走 `a.printf/println`（`App.Out`），方便测试重定向
- 提交信息用 `feat:` / `fix:` / `refactor:` 前缀，正文说明**为什么**这么做

## 不要做

- 引入第三方依赖（cobra / x/term 之外）
- 绕过服务包的 client 直接发 HTTP 请求
- 用 `exec.Command` 调外部命令完成本可以用 API 做的事
- 在用户既有目录上做写操作测试
- 改 Release 资产命名（`nst_<ver>_<os>_<arch>`），`install.sh` 和 `upgrade` 都依赖它
- 在 `skills/` 下再嵌套 `SKILL.md` 当作子 skill
- 把服务特定逻辑（探活、换 token、保存配置）塞进 `internal/auth` 或 `internal/app`
- 让服务包互相 import，或让服务包依赖 `internal/cli`
