# scherun

后台任务管理器. 托盘常驻, 管理界面在浏览器里, 任务按触发方式自动执行.

## 特点

- **托盘常驻**: 点击托盘图标弹出菜单, 内含应用名版本, 运行概况, 打开主页面与退出入口.
- **浏览器界面**: 同源 Web 界面, 深色与浅色主题, 实时推送任务状态.
- **六种触发方式**: 手动, 启动时, 保持运行, 定时重复, 定时一次, 直到成功.
- **进程组管理**: unix 下任务独占进程组, 停止时先发 SIGINT 再兜底强杀, 不会残留子进程.
- **失败保护**: 短时间内连续启动失败会暂停该任务的自动触发, 避免无限重启.
- **本地鉴权**: 进程级启动令牌换发签名 cookie, 配合 Host/Origin 围栏, 其他本机程序无法调用接口.

## 快速开始

```shell
just deps     # 安装 Go 模块与前端依赖
just run      # 构建并启动, 自动打开浏览器
```

首次启动会生成配置, 数据库与日志. 之后通过托盘菜单或浏览器页面管理任务.

常用命令:

```shell
just build      # 只构建到 dist/scherun
just run-quiet  # 启动但不打开浏览器
just dev        # 开发模式: Vite 热更新 + 后端
just debug      # 调试实例: 隔离数据目录, debug 日志
just test       # 运行测试
just fmt        # 格式化
just vet        # 静态检查
```

## 依赖

- Go 1.27+
- bun (构建前端)
- just
- uv (运行 python 脚本)
- C 工具链 (托盘在 macOS 上依赖 cgo)

## 目录与文件

路径随平台而定, `just run` 后用 `./dist/scherun paths` 可以打印实际位置:

| 内容 | macOS | Linux | Windows |
| --- | --- | --- | --- |
| 配置 | `~/Library/Application Support/scherun/config.toml` | `$XDG_CONFIG_HOME/scherun/config.toml` | `%APPDATA%\scherun\config.toml` |
| 数据 | 同上目录下的 `scherun.db` | `$XDG_DATA_HOME/scherun/` | `%LOCALAPPDATA%\scherun\` |
| 日志 | 数据目录下 `logs/scherun.log` | 同左 | 同左 |

环境变量可以覆盖, 便于隔离调试:

- `SCHERUN_CONFIG_DIR`: 配置目录
- `SCHERUN_DATA_DIR`: 数据目录
- `SCHERUN_LOG_FILE`: 主日志文件
- `SCHERUN_LOG_LEVEL`: 日志级别, 默认 `info`

## 配置

```toml
version = 2
quiet_launch = false          # 启动时不自动打开浏览器
log_retention_days = 5        # 日志保留天数, 最小 1
local_bind = "127.0.0.1:20008"  # 本地监听地址
env_import_enabled = true     # 启动时从登录 shell 导入环境变量
env_import_command = ""       # 导入命令, 留空时按 $SHELL 推断

[env_vars]                    # 全局环境变量, 任务可选择是否继承
```

配置文件带 `version` 字段. 旧版本配置会在启动时自动补齐缺失字段并回写;
文件损坏时会备份为 `config.toml.broken` 并用默认值继续运行.

## 环境变量

从 Finder 或 Dock 启动的 macOS 应用只继承 launchd 的最小环境,
`PATH` 通常只有 `/usr/bin:/bin:/usr/sbin:/sbin`, 于是 `adb`, `node`, `ffmpeg`
这类装在 homebrew 或用户目录下的命令全都找不到.

因此 scherun 启动时会跑一次登录 shell 读取它的环境:

- 默认命令按 `$SHELL` 推断, 例如 `/usr/local/bin/fish -l -c "env -0"`.
- 可以在设置页改成别的命令, 例如需要交互式初始化时用 `fish -l -i -c "env -0"`.
- 读取结果在启动时缓存一次, 设置页提供"立即重新导入", 改完 shell 配置不用重启.

环境变量按三层合并, 后者覆盖前者:

| 层 | 来源 | 任务能否关闭 |
| --- | --- | --- |
| 应用自身环境 | 启动 scherun 的进程 | 不能 |
| 登录 shell 导入 | 上面的导入命令 | 能 (`use_shell_env`) |
| 全局变量 | 设置页填写 | 能 (`use_global_env`) |
| 任务变量 | 任务编辑页填写 | 始终生效, 优先级最高 |

两个继承开关是独立的: 想用全局变量但不要 shell 那套环境时, 可以只关掉前者.

## 安全模型

管理接口只监听回环地址, 并且有三层防护:

1. **启动令牌**: 每次启动生成随机令牌, 只出现在托盘打开的链接与 `runtime.json` 里.
   浏览器首次带 `?token=` 访问时换发签名 cookie, 随后重定向到去掉令牌的地址.
2. **签名 cookie**: 用持久化的 HMAC 密钥签名, 载荷绑定 Host 权威与有效期.
   cookie 是 `HttpOnly` 与 `SameSite=Strict`, 脚本读不到, 也无法被伪造或跨站点复用.
3. **请求围栏**: `Host` 必须是回环地址 (防 DNS rebinding), 拒绝 `Sec-Fetch-Site: cross-site`,
   并校验 `Origin` 与 `Host` 同源. 响应带 CSP, 只允许同源资源.

因此本机其他进程即使知道端口也无法调用接口: 它们既拿不到进程内令牌, 也伪造不出签名 cookie.

## 文档

- [架构说明](docs/design/01-architecture.md)
- [界面与接口](docs/design/02-ui-and-api.md)

## 许可

[MIT](LICENSE).
