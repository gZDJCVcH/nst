# 调用示例

所有示例都以 `nst openlist` 开头。需要结构化结果时在任意位置加 `--json`。

## 首次登录

```bash
nst openlist auth login --url http://192.168.1.10:5244
# 用账号密码换 token
nst openlist auth login --url http://192.168.1.10:5244 -u admin --password
# 已有 token，直接写入并校验（不会引导打开令牌页面）
nst openlist auth login --url http://192.168.1.10:5244 --token 'your-token'
# SSH / 非交互环境：不尝试打开浏览器
nst openlist auth login --url http://192.168.1.10:5244 --token 'your-token' --no-open
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
- `1` 且提示未登录时执行 `auth login`
- `3` 按接口错误码处理：401 重新登录、403 检查令牌权限、405 检查 115 令牌是否过期
- `4` 检查网络与 `--url` 是否可达
