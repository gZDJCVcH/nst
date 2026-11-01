# openlist-cli

[OpenList](https://github.com/OpenListTeam/OpenList) 的命令行客户端，Go 实现，**单个静态二进制、零运行时依赖**。

服务端对应 OpenList v4.x（实测 v4.2.9），API 与 AList v3 兼容。

## 安装

```bash
# 一键安装（推荐）：默认装到 /usr/local/bin，不可写则 ~/.local/bin
curl -fsSL https://raw.githubusercontent.com/gZDJCVcH/openlist-cli/main/install.sh | sh

# 指定版本 / 目录
curl -fsSL .../install.sh | sh -s -- -v v0.2.0 -d ~/bin

# 国内网络可换下载源
OPENLIST_CLI_DOWNLOAD_BASE=https://your-mirror.example.com/gh \
  curl -fsSL .../install.sh | sh

# raw.githubusercontent 连不上时，改用 jsDelivr 取脚本（内容相同）
curl -fsSL https://cdn.jsdelivr.net/gh/gZDJCVcH/openlist-cli@main/install.sh | sh
```

手动下载：

```bash
# 1) 拿最新 tag（跟随一次 302，不消耗 API 配额）
tag=$(curl -fsSLI -o /dev/null -w '%{url_effective}' \
      https://github.com/gZDJCVcH/openlist-cli/releases/latest | sed 's#.*/##')

# 2) 下载对应平台（linux_amd64 / linux_arm64 / darwin_arm64 ...）
curl -fsSLO "https://github.com/gZDJCVcH/openlist-cli/releases/download/$tag/openlist-cli_${tag#v}_linux_amd64.tar.gz"
tar -xzf openlist-cli_*.tar.gz && sudo install -m 0755 openlist-cli /usr/local/bin/

# 或用 gh
gh release download "$tag" -R gZDJCVcH/openlist-cli -p '*linux_amd64*'
```

有 Go 工具链时也可以 `go install github.com/gZDJCVcH/openlist-cli/cmd/openlist-cli@latest`。

自升级：

```bash
openlist-cli upgrade --check   # 只看有没有新版本
openlist-cli upgrade           # 下载校验后原地替换
```

## 快速开始

```bash
# 登录：问地址 → 探活 → 给出令牌页面（本地会话会顺手打开浏览器）→ 粘贴 token → 校验通过才保存
openlist-cli auth login --url http://192.168.1.10:5244

# 或用账号密码换 token
openlist-cli auth login --url http://192.168.1.10:5244 -u admin --password

# 日常
openlist-cli list remote:/115
openlist-cli stat remote:/115/电影/x.mkv
openlist-cli download remote:/115/x.mkv local:./x.mkv
openlist-cli upload local:./a.cue remote:/115/
openlist-cli verify local:./dir remote:/115/dir   # 按大小 + sha1 比对，不下载
```

### 路径必须带前缀

不做任何智能推断——本地绝对路径和远端路径在字符串上无法可靠区分，猜错一次就可能覆盖错文件。

```
local:<本地路径>      例如 local:/tmp/a.mkv、local:./a.cue
remote:/<远端路径>    例如 remote:/115/音乐
```

缺前缀直接报错并给出两种正确写法。

## 命令

| 命令 | 别名 | 说明 |
|---|---|---|
| `auth login/logout/token/whoami` | | 登录、登出、打印令牌、看身份 |
| `list <path>` | `ls` | 列目录（`-l` 详细、`--refresh` 强刷缓存） |
| `stat <path>` | | 文件/目录详情 |
| `tree <path>` | | 树形展开（`-L` 限深） |
| `search <关键词>` | | 搜索（`--path` 限定目录、`--scope`） |
| `read <path>` | | 读文本文件到 stdout（`--max-bytes`、`--force`） |
| `link <path>` | | 打印直链 |
| `upload <local>... <remote>` | `put` | 上传（`-r` 递归、`--skip-same`、`--as-task`） |
| `download <remote> [local]` | `get` | 下载（`-r` 递归） |
| `remove <path>...` | `rm` | 删除（`-r` 递归、`-y` 免确认） |
| `move <src> <dst>` | `mv` | 移动/改名 |
| `copy <src> <dst>` | `cp` | 复制 |
| `rename <path> <新名>` | | 重命名 |
| `mkdir <path>` | | 建目录（`-p`） |
| `tasks` | | 异步任务列表（`--type`、`--done`、`--clear`） |
| `cancel-task <id>` | | 取消/删除/重试任务（`--action`） |
| `drivers` | | 支持的存储驱动 |
| `driver info <驱动名>` | | 某驱动的配置项定义 |
| `mounts` | | 挂载点、驱动与限流间隔 |
| `storage get <id>` | | 存储详情（凭据默认打码，`--show-secrets` 看原值） |
| `verify <local> <remote>` | | 按大小 + sha1 比对，不下载文件本体 |
| `config show/path/set` | | 本地配置 |
| `version` / `upgrade` | | 版本 / 自升级 |
| `completion <shell>` | | 生成补全脚本 |

### 全局参数

```
--url <地址>          覆盖配置
--token <令牌>        覆盖配置
--json                输出原始 JSON
--timeout <秒>        请求超时（默认 30）
--rate <毫秒>         网盘请求最小间隔（默认 1000）
--local-rate <毫秒>   本地盘请求最小间隔（默认 50）
--cache-ttl <秒>      目录列表缓存（默认 60，0 表示不缓存）
--no-cache            禁用目录缓存
```

### 退出码

| 码 | 含义 |
|---|---|
| 0 | 成功 |
| 1 | 业务失败（未登录、目标不存在、用户取消、部分上传失败等） |
| 2 | 路径/配置格式错误 |
| 3 | 接口返回错误（`code != 200`） |
| 4 | 网络错误 |
| 130 | 被 Ctrl-C 中断 |

## 开发

```bash
make build     # 编译到 dist/openlist-cli
make test      # 全部单测（离线，不碰网络）
make fmt vet
make cross     # 手工交叉编译 6 个平台
```

只需要 Go（1.23+），无其他外部依赖。

### 交叉编译

纯 Go、无 cgo，所以一条命令就能产出静态二进制：

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "-s -w" ./cmd/openlist-cli
```

### 发布

三条 workflow 分工明确，**推代码不会打包**：

| workflow | 触发 | 做什么 |
|---|---|---|
| `ci.yml` | push / PR | 只跑 gofmt、vet、test、build（3 个 OS），不产出任何产物 |
| `snapshot.yml` | 手动（workflow_dispatch） | 打一次快照包，产物只作 Actions artifact，需登录下载、默认留 7 天 |
| `release.yml` | 推 tag `v*` | 跑 `goreleaser release`，把产物挂到 GitHub Release |

正式发版：

1. 仓库坐标已固定在 `install.sh`、`.goreleaser.yaml`、`internal/cli/upgrade.go` 三处，
   换账号/组织时一起改（`grep -rl gZDJCVcH --exclude-dir=.git .`）。
2. `git tag v0.1.0 && git push origin v0.1.0`
3. Actions 跑 `goreleaser`，产出 `openlist-cli_<ver>_<os>_<arch>.tar.gz`（Windows 为 zip）+ `checksums.txt`，挂在 Release 上。
4. `install.sh` 与 `upgrade` 都从 Release 下载，资产命名必须保持 `<bin>_<version>_<os>_<arch>`，不要改。

> 只想验证打包是否正常、又不想发版时，手动跑一次 `snapshot` workflow 即可。

## 已知限制

- `--encoding` 只支持 `utf-8`，其他编码会提示先用 `iconv` 转换。
- 32 位 ARM 不支持 `upgrade` 自升级，会提示改用 `install.sh` 重新安装。
- `--json` 输出中结构体字段顺序固定，`map` 类字段按 key 字母序排列。
- `auth login` 在非 tty 环境不交互询问，必须用参数传入。

## 设计约束

1. **路径必须带前缀**，绝不智能推断。
2. **请求必须限流**，且按存储驱动分级（本地盘 50ms，网盘 1s）；所有请求都走 `client.do`，不要绕过限流。
3. **目录列表默认 60 秒缓存**；写操作后必须 `Invalidate`，`--refresh` 穿透。
4. **破坏性操作二次确认**；非交互环境默认取消；`remove` 拒绝删除 `/`。
5. **凭据一律脱敏**；`storage get` 的 `addition` 里躺着明文 token，只有 `--show-secrets` 才输出原值。
6. **错误信息要能指导下一步**：`ApiError.Render()` 按 code 给出可执行提示。

## License

MIT
