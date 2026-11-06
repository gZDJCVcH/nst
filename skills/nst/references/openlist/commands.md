# 命令与参数

所有命令都在 `nst openlist` 下，别名见 `README.md`。全局参数（`--url`、`--token`、`--json`、`--timeout`、`--rate`、`--local-rate`、`--cache-ttl`、`--no-cache`）可放在任意位置。

## 认证

| 命令 | 主要参数 | 说明 |
|---|---|---|
| `auth login` | `--mode <模式>`、`--url <地址>`、`--token <值>`、`-u <用户名>`、`--password [值]`、`--otp <码>`、`--no-open`、`--json` | 三模式登录，见下方 |
| `auth logout` | | 清除本地令牌 |
| `auth token` | | 打印当前令牌 |
| `auth whoami` | | 查看当前身份与权限 |

`--password` 支持 `--password=secret` 与 `--password secret` 两种写法；不带值时会提示输入（仅 tty）。

`--mode` 取值 `auto`（默认）/ `context` / `flags` / `interactive`：

| 模式 | 凭据来源 | 阻塞 | 落盘 |
|---|---|---|---|
| `context` | 配置文件 + `OPENLIST_URL` / `OPENLIST_TOKEN` | 否 | 否 |
| `flags` | `--url` + `--token`（或 `--password`）一次给齐 | 否 | 是 |
| `interactive` | 逐步询问 + 令牌页面 | 是 | 是 |

`auto` 推断：参数齐全 → `flags`；已有完整上下文 → `context`；否则 tty → `interactive`、非 tty → 结构化报错。非交互模式不询问、不打开浏览器、不重试；`--json` 输出 `{mode, source, url, valid, saved, user, role, error}`，失败时 `valid:false` 带 `error`。

## 浏览

| 命令 | 别名 | 主要参数 | 说明 |
|---|---|---|---|
| `list <path>` | `ls` | `-l`、`--refresh` | 列目录 |
| `stat <path>` | | | 文件/目录详情 |
| `tree <path>` | | `-L <深度>` | 树形展开 |
| `search <关键词>` | | `--path <目录>`、`--scope` | 搜索 |
| `read <path>` | | `--max-bytes`、`--force` | 读文本文件到 stdout |
| `link <path>` | | | 打印直链 |
| `mounts` | | | 挂载点、驱动与限流间隔 |

## 传输

| 命令 | 别名 | 主要参数 | 说明 |
|---|---|---|---|
| `upload <local>... <remote>` | `put` | `-r`、`--skip-same`、`--as-task` | 上传，支持多文件与递归 |
| `download <remote> [local]` | `get` | `-r` | 下载，不写 local 时用远端文件名 |
| `verify <local> <remote>` | | | 按大小 + sha1 比对，不下载文件本体 |

## 变更

| 命令 | 别名 | 主要参数 | 说明 |
|---|---|---|---|
| `remove <path>...` | `rm` | `-r`、`-y` | 删除；默认二次确认，拒绝删除 `/` |
| `move <src> <dst>` | `mv` | | 移动/改名 |
| `copy <src> <dst>` | `cp` | | 复制 |
| `rename <path> <新名>` | | | 重命名 |
| `mkdir <path>` | | `-p` | 建目录 |

## 任务

| 命令 | 主要参数 | 说明 |
|---|---|---|
| `tasks` | `--type`、`--done`、`--clear` | 异步任务列表 |
| `cancel-task <id>` | `--action` | 取消/删除/重试任务 |

任务接口按类型分组，没有全局列表；详见 `README.md` 的关键约束。

## 管理

| 命令 | 主要参数 | 说明 |
|---|---|---|
| `drivers` | | 支持的存储驱动 |
| `driver info <驱动名>` | | 某驱动的配置项定义 |
| `storage get <id>` | `--show-secrets` | 存储详情，凭据默认打码 |
| `config show\|path\|set` | | 本地配置读写 |

## 元命令

| 命令 | 主要参数 | 说明 |
|---|---|---|
| `version` | | 显示 nst 版本 |
| `upgrade` | `--check`、`--version <tag>`、`--force`、`--current` | 检查并原地升级；`--check` 只看不装 |

`upgrade` 也可用顶层 `nst upgrade`；32 位 ARM 不支持自升级，改用 `install.sh`。

## 退出码

| 码 | 含义 |
|---|---|
| 0 | 成功 |
| 1 | 业务失败（未登录、目标不存在、用户取消、部分上传失败等） |
| 2 | 路径/配置格式错误 |
| 3 | 接口返回错误（`code != 200`） |
| 4 | 网络错误 |
| 130 | 被 Ctrl-C 中断 |
