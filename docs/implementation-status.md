# Core 实现状态

更新日期：2026-11-15。本文描述当前源码；`docs/plan/` 保留跨仓库规划，不能作为功能已交付的证明。本次按维护者要求只做源码审查、格式与契约检查、补丁应用检查，未编译、未运行测试、未启动 Core 或浏览器、未进行游戏端联调。

## 当前 Core 范围

| 部分 | 已有实现与本轮补全 | 使用边界 |
| --- | --- | --- |
| 宿主入口 | `runtime.New/Run/WaitReady/Shutdown/Status/ValidateConfig`，配置目录互斥、启动失败回收、SQLite 生命周期 | Core 是 Go module；CLI / 服务进程负责入口、信号与退出 |
| 配置与管理 | 单一 `config.yaml`、凭据引用、ETag、备份保存、Bearer API、SSE | 保存配置后重启宿主生效；没有连接热切换 |
| 机器人与群服消息 | 独立适配器 SDK、OneBot v11 正向 WebSocket、作用域身份、路由、消息去重与有界队列 | 不把 QQ openid 与数字 QQ 号混用；未做具体机器人实现的兼容性验证 |
| 绑定与白名单 | 登录授权、一次性验证码、限流、无账号白名单、多玩家绑定、解绑与退群处理 | 游戏端必须实现对应登录拦截与动作；网络身份模式由管理员配置 |
| 命令、玩家与背包 | 内置命令、配置化游戏控制台命令、权威会话源、累计时长、有效背包快照 | 游戏动作依赖节点能力；不把 Core 性能当作 Minecraft TPS |
| Bridge v1 | 鉴权、握手、心跳、重连游标、在线快照恢复、请求期限、未知执行结果 | Java 客户端、平台主线程调度及动作去重由 MC API / Minecraft 仓库实现 |
| 白名单导入 | ShitBot schema 7 SQLite 单文件 / 多文件预览、冲突排除、事务提交、报告与撤销 | 仅迁移白名单 / 绑定数据，不是旧版全库无损迁移 |
| 图片模板 | 内置模板、ZIP 导入、草稿编辑、默认样式、发布 / 回滚、数据提供器、受限渲染队列及图片资源 | 必须自行配置匹配的 Chromium 路径与精确版本；模板效果未运行验证 |
| 本地插件 | ZIP 导入、声明与授权、版本启停、事件 / 命令 / 定时 / 存储 / 数据提供器 | 同步 JavaScript；进程内可信扩展，没有操作系统级隔离或硬内存上限；不提供市场 |
| 宿主更新 | GitHub Releases 检查、资产 SHA-256、下载限额、数据库 / 配置备份、Installer 交接 | 安装、回滚、重启与兼容判断归进程宿主；`handed_off` 不表示升级成功 |
| WebUI | 概览、配置、凭据、绑定、玩家、导入、模板、插件、更新、SSE 页面；会话开关 | 本地 Vue 模块随 Core 嵌入；提供可选 TypeScript / Vite 入口，主要页面仍是 JavaScript |

本轮代码分成生命周期、模板管理、插件管理、宿主更新、内置页面及最终复核修复几个补丁；源码包中原本未提交的图片 / 插件开发改动单独保留为上传快照补丁，不冒充本轮新增。

## 后续项目工作

CLI、Windows Desktop、Java MC API、Minecraft 各平台实现与真实环境联调仍在各自仓库推进。QQ 官方机器人、Discord、bStats 项目 ID / 接入、旧版玩家时长与背包等完整数据迁移，以及原版全部定时任务 / 自定义规则的功能对齐尚不属于当前已实现集合。Core 内未放置这些功能的空实现来宣称完成。

发布前仍需维护者自行完成构建、自动化测试、Windows / Linux 检查、OneBot / 游戏端联调、Chromium 渲染与更新安装回退验证。补丁应用检查只能说明改动能在指定基线上重放，不能证明上述运行行为。

## 当前接入依据

- [runtime 与配置接入](runtime.md)，[示例配置](../examples/config.yaml)
- [管理 OpenAPI](../api/admin/v1/openapi.yaml)，[Bridge 时序与边界](bridge-v1.md)，[Bridge JSON Schema](../api/bridge/v1/envelope.schema.json)
- [模板与渲染](rendering.md)，[插件](plugins.md)，[宿主更新](updates.md)，[WebUI](webui.md)

协议 / 数据模型以这些契约及相应公开 Go 类型为准。公共协议变更需同时考虑 CLI、Desktop 和 Java 端，但本轮没有改动其他仓库。
