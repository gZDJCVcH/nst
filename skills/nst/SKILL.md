---
name: nst
description: 操作 NAS 上的 OpenList（列目录、上传下载、搜索、直链、任务、挂载点、存储管理）与 nst 工具箱。当用户提到 OpenList、AList、115、网盘文件、远程目录、挂载点，或要求用 nst / nst openlist / openlist-cli 执行命令时使用。
---

# nst

nst 是 NAS 工具箱的单一入口，一个静态二进制。先判断要操作哪个服务，再按索引读取对应文档；不要凭记忆猜命令与参数。

## 索引

| 服务 | 场景 | 文档 |
|---|---|---|
| OpenList | 列目录、上传下载、搜索、直链、任务、挂载点、存储配置、登录 | `references/openlist/README.md` |

新服务加入时在此补一行，并在 `references/` 下建同名目录。

## 通用约定

- 命令形态 `nst <服务> <命令> [参数]`，例如 `nst openlist list remote:/115`
- 以 `openlist-cli` 名字调用时等价于 `nst openlist ...`
- 路径必须带前缀：`local:<本地路径>` / `remote:/<远端路径>`；缺前缀会报错
- 默认 human 输出；需要机器可读结果加 `--json`
- 破坏性操作默认二次确认；自动化场景显式加 `-y` 前必须先确认目标路径
- 失败看退出码与 stderr：1 业务失败、2 路径/配置错误、3 接口错误、4 网络错误、130 被中断
- 全局参数：`--url`、`--token`、`--timeout`、`--rate`、`--local-rate`、`--cache-ttl`、`--no-cache`
- 首次使用先登录：`nst openlist auth login --url <地址>`；非 tty 环境必须用参数传入

## 读取顺序

1. 读 `references/openlist/README.md`，确认命令分组与前置条件
2. 需要完整参数时读同目录的 `commands.md`，需要调用示例时读 `examples.md`
3. 执行后用退出码与 `--json` 输出判断结果，不要在 skill 里重实现业务逻辑
