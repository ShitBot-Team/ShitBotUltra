# ShitBotUltra Core 计划

状态：四个仓库已建立，目前处于规划阶段。语言和接口属于实施方案，尚未实现程序。

## 本仓库职责

- Go Core：账号绑定、白名单、消息路由、群命令、玩家资料、背包存储、通知与图片任务。
- OneBot v11 和 Minecraft Bridge 适配，以及统一管理 HTTP API / SSE。
- TypeScript + Vue 3 + Vite WebUI，构建资源进入 Core 模块，由 CLI 可执行文件携带。
- 对外提供窄的 Go `runtime` 包，供独立 CLI 仓库托管；业务实现保持在 `internal`。
- 保存整个项目的协议契约、公共架构、贡献规范、许可说明和 Agent 规则。

## 公共计划索引

| 文档 | 用途 |
| --- | --- |
| [总体规划](docs/plan/01-总体规划.md) | 产品目标、语言、仓库、进程、实施阶段 |
| [仓库结构与开发规范](docs/plan/02-仓库结构与开发规范.md) | 各组件目录、包名、模块依赖、提交规范 |
| [协议配置与迁移](docs/plan/03-协议配置与迁移.md) | 协议草案、配置事务、普通版迁移、统计接口 |
| [思维导图](docs/plan/04-思维导图.md) | 项目组成及职责关系 |
| [参考项目与依据](docs/plan/05-参考项目与依据.md) | 已查阅的官方项目和普通版源码基线 |
| [贡献指南](CONTRIBUTING.md) | Conventional Commits 与简明 PR 约定 |
| [Agent 规则](agent.md) | 四仓库共同协作要求 |
| [许可证说明](docs/licensing.md) | AGPL-3.0-only 和历史许可边界 |

## 下一阶段顺序

1. 固化配置 schema、管理 API、Bridge 握手和平台能力协商草案。
2. 实现窄 `runtime` 入口、实例所有权、SQLite 与配置生命周期。
3. 配合 CLI 和首个平台完成 SnowLuma 连接、文本互通、绑定与登录授权闭环。
4. 增加最小 WebUI 的连接配置和状态页，再按公共迁移表逐项加入功能。

本列表是后续任务顺序，不授权立即生成骨架、运行测试或实现代码。

## 其他仓库

- [CLI 计划](https://github.com/ShitBot-Team/ShitBotUltra-CLI/blob/main/plan.md)：Go 轻量启动器。
- [Desktop 计划](https://github.com/ShitBot-Team/ShitBotUltra-Desktop/blob/main/plan.md)：C# WinUI 3 原生桌面。
- [Minecraft 计划](https://github.com/ShitBot-Team/ShitBotUltra-Minecraft/blob/main/plan.md)：Java 薄平台适配器。

公共文档只在本仓库维护，各组件通过引用使用。核心组件标识为 `shitbotteam.ultra.core`，Go module 为 `github.com/ShitBot-Team/ShitBotUltra`。许可证为 **AGPL-3.0-only**。
