# 调用示例

所有示例都以 `nst openlist` 开头。需要结构化结果时在任意位置加 `--json`。

## 登录

```bash
# 交互式（人用）：问地址 → 探活 → 令牌页面 → 粘贴 token → 校验保存
nst openlist auth login --mode=interactive --url http://192.168.1.10:5244
nst openlist auth login --url http://192.168.1.10:5244 -u admin --password

# 参数模式（Agent 首选）：一次给齐，不阻塞、不打开浏览器
nst openlist auth login --mode=flags --url http://192.168.1.10:5244 --token 'your-token' --json
# {"mode":"flags","source":"flag","url":"...","valid":true,"saved":true,"user":"admin","role":"管理员"}

# 账号密码一次给齐
nst openlist auth login --mode=flags --url http://192.168.1.10:5244 --password=secret --json

# 上下文模式：复用已保存配置或环境变量
nst openlist auth login --mode=context --json

# 环境变量已设置时，auto 会自动走 context
OPENLIST_URL=http://192.168.1.10:5244 OPENLIST_TOKEN='your-token' nst openlist auth login --json

# 非 tty 且缺凭据：直接返回结构化错误，不会阻塞
nst openlist auth login --json
# {"valid":false,"error":"非交互环境缺少凭据：..."}
```

## 浏览与查询

```bash
nst openlist mounts
nst openlist list remote:/115
nst openlist list -l --refresh remote:/115/电影
nst openlist tree -L 2 remote:/115
nst openlist search 电影 --path remote:/115
nst openlist stat remote:/115/电影/x.mkv
nst openlist link remote:/115/x.mkv
nst openlist read remote:/115/notes.txt --max-bytes 4096
```

交给程序处理时取 JSON：

```bash
nst openlist list --json remote:/115
```

## 传输

```bash
nst openlist upload local:./a.cue remote:/115/
nst openlist upload -r --skip-same local:./dir remote:/115/dir
nst openlist download remote:/115/x.mkv local:./x.mkv
nst openlist download -r remote:/115/电影 local:./电影
# 只比对，不下载文件本体
nst openlist verify local:./dir remote:/115/dir
```

`local:` 后可以是相对路径；`remote:` 后必须是绝对路径。

## 变更操作

```bash
nst openlist mkdir -p remote:/115/新目录
nst openlist rename remote:/115/a.mkv b.mkv
nst openlist move remote:/115/a.mkv remote:/115/归档/
nst openlist copy remote:/115/a.mkv remote:/115/备份/
nst openlist remove -r -y remote:/115/临时目录
```

批量或自动化场景使用 `-y` 前，先确认目标路径；`remove` 拒绝删除 `/`。

## 任务与管理

```bash
nst openlist tasks --type upload
nst openlist cancel-task <id> --action cancel
nst openlist drivers
nst openlist driver info '115 Open'
nst openlist storage get 3
nst openlist config show
```

## 版本与升级

```bash
nst version
nst upgrade --check
nst upgrade
```

## 旧命令名

安装脚本会创建 `openlist-cli → nst` 软链，以下两种写法等价：

```bash
openlist-cli list remote:/115
nst openlist list remote:/115
```

## 失败处理

- 退出码非 0 时读 stderr，按 `commands.md` 的退出码表判断原因
- `2` 多为路径缺前缀或格式错误，检查 `local:` / `remote:/`
- `1` 且提示未登录时执行 `auth login`；加 `--json` 可读 `valid` / `error`
- `3` 按接口错误码处理：401 重新登录、403 检查令牌权限、405 检查 115 令牌是否过期
- `4` 检查网络与 `--url` 是否可达
