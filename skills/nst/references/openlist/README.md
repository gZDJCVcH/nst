# OpenList 子命令

`nst openlist` 下的全部能力。服务端对应 OpenList v4.x（API 与 AList v3 兼容）。

## 前置条件

- `nst version` 能正常运行
- 已配置地址与令牌：`nst openlist auth login --url http://<地址>:5244`
  本地图形会话会顺手打开令牌页面；SSH / 非 tty 只打印页面地址，需手动取 token
- 配置位于 `~/.config/openlist-cli/config.json`（0600），可用 `--url` / `--token` 覆盖
- 也可用环境变量 `OPENLIST_URL` / `OPENLIST_TOKEN`

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
