# OpenList 子命令

`nst openlist` 下的全部能力。服务端对应 OpenList v4.x（API 与 AList v3 兼容）。

## 前置条件

- `nst version` 能正常运行
- 凭据三选一（见下方「登录模式」）：已保存配置 / 环境变量、`--url` + `--token` 参数、或人在终端交互输入
- 配置位于 `~/.config/openlist-cli/config.json`（0600），可用 `--url` / `--token` 覆盖
- 也可用环境变量 `OPENLIST_URL` / `OPENLIST_TOKEN`

## 登录模式

`auth login --mode=<模式>`，省略时 `auto` 自动推断。

| 模式 | 凭据来源 | 阻塞 | 落盘 | 用法 |
|---|---|---|---|---|
| `context` | 配置文件 + `OPENLIST_URL` / `OPENLIST_TOKEN` | 否 | 否 | 复用环境已有登录态 |
| `flags` | `--url` + `--token`（或 `--password`）一次给齐 | 否 | 是 | Agent / CI / 脚本 |
| `interactive` | 逐步询问 + 令牌页面 | 是 | 是 | 人在终端 |

`auto` 推断：参数齐全 → `flags`；已有完整上下文 → `context`；否则 tty → `interactive`，非 tty → 结构化报错。字段优先级 `--url` / `--token` > 环境变量 > 配置文件 > 交互询问。

加 `--json` 得到 `{mode, source, url, valid, saved, user, role, error}`；非交互模式 stdout 只有这一份 JSON，失败时 `valid:false` 且带 `error`。令牌页面为 `{地址}/@manage/settings/other`，本地图形会话会顺手打开浏览器，SSH / 非 tty 只打印地址。

## 命令分组

| 分组 | 命令 | 详细文档 |
|---|---|---|
| 认证 | `auth login` / `logout` / `token` / `whoami` | `commands.md` |
| 浏览 | `list`(ls) / `stat` / `tree` / `search` / `read` / `link` / `mounts` | `commands.md` |
| 传输 | `upload`(put) / `download`(get) / `verify` | `commands.md` |
| 变更 | `remove`(rm) / `move`(mv) / `copy`(cp) / `rename` / `mkdir` | `commands.md` |
| 任务 | `tasks` / `cancel-task` | `commands.md` |
| 管理 | `drivers` / `driver info` / `storage get` / `config` | `commands.md` |
| 元命令 | `version` / `upgrade` | `commands.md` |

## 关键约束

- 路径一律带前缀 `local:` / `remote:/`，绝不智能推断
- 请求按存储驱动分级限流：本地盘 50ms，网盘 1s；可用 `--rate` / `--local-rate` 调整
- 目录列表默认 60 秒缓存；`list --refresh` 穿透，写操作后自动失效
- `remove` 默认二次确认，`-r` 才能递归删除，拒绝删除 `/`
- 凭据一律脱敏；`storage get` 只在 `--show-secrets` 时输出原值
- 115 判断文件新旧一律看 `fs/get` 的 `hash_info.sha1`，不要用下载内容判断
- 115 直链必须走 `{base}/d{path}?sign={sign}`，不要用 `raw_url`

## 全局参数

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

## 示例

见 `examples.md`。
