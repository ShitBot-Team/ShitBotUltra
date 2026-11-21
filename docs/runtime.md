# 宿主与配置接入

Core 使用 Go 1.24，module 为 `github.com/ShitBot-Team/ShitBotUltra`。本仓库没有 `cmd/main` 可执行入口；CLI / 服务宿主导入公开 `runtime` 包。宿主不要导入 `internal`，不要直接修改 Core 业务数据库。

## 生命周期

`runtime.New(options)` 复制宿主选项、解析配置目录，不创建文件、不占用监听端口。`Run(ctx)` 为单次阻塞运行；再次运行返回 `ErrAlreadyRun`。宿主拥有 context、信号、日志、进程退出及重启策略。

`Run` 获取配置目录独占锁，初始化配置、凭据、数据库、模板 / 插件、监听器和连接服务。`WaitReady(ctx)` 等待就绪或启动失败；`Status()` 给出状态和实际监听 URL。取消父 context 是正常退出，`Run` 返回 nil；启动 / 监听错误会返回 error。`Shutdown(ctx)` 请求退出并等待收尾，等待期限到达不表示资源已经释放。重启需要等待旧实例结束，再新建 `Runtime`。

宿主中的基本用法（示意函数，未在本次任务编译或运行）：

```go
func serve(parent context.Context, dir, version string, log *slog.Logger) error {
    ctx, cancel := context.WithCancel(parent)
    defer cancel()
    core, err := runtime.New(runtime.Options{
        ConfigDir: dir, Version: version, Logger: log,
    })
    if err != nil {
        return err
    }
    finished := make(chan error, 1)
    go func() { finished <- core.Run(ctx) }()
    ready, stopWaiting := context.WithTimeout(ctx, 30*time.Second)
    err = core.WaitReady(ready)
    stopWaiting()
    if err != nil {
        cancel()
        <-finished
        return err
    }
    log.Info("管理入口", "url", core.Status().URL)
    return <-finished
}
```

函数需导入 `context`、`log/slog`、`time` 和本项目 `runtime` 包。如果启动时自动启用多个本地插件，宿主应按所需启动预算设置就绪等待期限。

`AdapterFactories` 注册编译进宿主的新机器人协议，不能替换内置 `onebot.v11`。`WebUI *bool` 覆盖页面初始开关，不影响 API。`Updates` 配置的是 CLI / Desktop 宿主发行资源；详见[更新交接](updates.md)。插件、渲染与管理 API 的版本不能仅从宿主 semver 推断。

## 配置目录

不指定 `ConfigDir` 时使用操作系统用户配置目录下的 `ShitBotUltra`。首次启动生成 `config.yaml` 和随机的 `secrets/admin.token`，默认监听 `127.0.0.1:17880`，WebUI 开启，没有机器人连接或游戏节点。

| 内容 | 用途 |
| --- | --- |
| `config.yaml` | 当前实现唯一的主配置，`config_version: 1` |
| `secrets/` | `file:名称` 引用的凭据；API 只创建新文件，不覆盖活动凭据 |
| `data/core.sqlite` | Core 数据，不能拿普通版数据库直接覆盖 |
| `backups/` | 配置保存、数据导入 / 撤销和更新交接前的备份 |
| `imports/` | 导入源副本；上传不等于导入 |
| `templates/`、`plugins/` | 不可变包版本与当前状态 |
| `plugin-data/` | 插件独立数据；卸载包后保留 |
| `cache/` | 临时渲染与更新下载资产 |

计划中拆分的多份 YAML 尚未实现。请使用[当前示例](../examples/config.yaml)中的字段；未知字段会被拒绝。配置内容最多 2 MiB，必须只有一个 YAML 文档。`GET /api/v1/config` 返回原文与 revision；`PUT` 必须携带带引号的 `If-Match`，校验后先备份再替换文件。活动配置快照不变，响应 `restart_required: true`，需要宿主重启。

凭据引用只允许 `env:名称` 或 `file:名称`。创建 / 读取时去掉首尾空白；非空值为 32–4096 个 UTF-8 字节，不能含 CR/LF/NUL。每个游戏节点需要独立凭据，不能复用管理凭据。管理 API 创建接口的响应会显示一次最终凭据；不要将凭据放进 URL。

## 网络与身份

每个 `network_id` 恰好有一个 `session_authority: true` 节点，负责在线快照与加入 / 离开事件。同一网络的节点必须使用同一种 `identity_mode`；`online`、`forwarded` 要求可信 UUID，`offline` 以精确大小写的玩家名为基础。代理与后端之间的身份转发与信任配置由游戏侧保证。

`login_gate` 标记处理登录拦截的节点。`actions` 是管理员允许的动作列表，执行时还必须出现在游戏节点握手的 `capabilities` 中。`command.execute` 是游戏控制台命令，不是操作系统 shell。快捷命令的参数按配置的正则逐项校验，不能把群消息直接当作任意控制台命令执行。

一条路由不能跨身份网络；一个机器人会话不能映射到不同网络，也只能有一个命令路由。OneBot v11 的数字身份使用 `qq.numeric/global`；官方 QQ openid、Discord 等应由不同的适配器身份模型表示。

## HTTP 与接入约束

管理 API 和页面使用同一监听器。`allowed_hosts` 精确匹配浏览器发送的 Host（包括非默认端口）；回环监听自动允许实际 IP:port 与 localhost:port。非回环监听默认要求 TLS 和明确的允许 Host；例外开关仅由部署者明确配置。

浏览器 Origin 必须与请求 Host 和 Core 实际接收的 HTTP/HTTPS scheme 一致。Core 不盲信 `X-Forwarded-*`；反向代理如对外提供 HTTPS，应使用 TLS 回源并保留正确 Host。只在代理终止 TLS、明文回源时，浏览器写操作会因 Origin 不一致而被拒绝。

`/bridge/v1` 使用节点 Token 和 `X-SBU-Node-ID`，不接受带 Origin 的浏览器连接；`/api/v1/` 全部要求管理 Token。SSE 客户端用带 Authorization 的 fetch 流；原生 EventSource 无法携带该 header。事件历史最多 512 条，出现 `reset` 后重新读取相关状态。

页面关闭不会关闭 API。管理接口没有提供任意远程关机 / 启动命令；进程重启与软件安装始终由宿主负责。

## 控制台管理接口补充（2026-11-21）

- `GET /api/v1/dashboard`：当前活动网络的真实玩家/绑定分类及节点、适配器计数。
- `GET /api/v1/config/sections`：读取持久化配置与共享 revision，不返回凭据原文。
- `PATCH /api/v1/config/sections`：以 `If-Match` 提交一个或多个配置分区，复用校验、备份、文件事务；保存不热切换连接。
- `/connections` 及 `runtime.Status.Nodes` 增加 `allowed_actions`，表示活动配置允许的动作；仍须与游戏端 `capabilities` 取交集。

已有 API 路径保留。原文配置保存的 `restart_required` 现在根据文件与活动配置比较，原样保存或恢复活动配置时可返回 false。页面结构和静态审查边界见 [WebUI](webui.md)。
