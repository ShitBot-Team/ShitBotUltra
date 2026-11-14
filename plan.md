# ShitBotUltra Core 计划

更新日期：2026-11-14。状态：Core、CLI、Desktop、Minecraft 与独立 MC API 共五个仓库，目前仅维护计划、代理规范与许可证，尚未实现程序。本次不写代码、不运行测试或构建。

## 本仓库职责

- Go Core：账号绑定、白名单、消息路由、群命令、玩家资料、背包存储、通知与图片任务。
- 独立机器人适配器框架：首发 OneBot v11，后续 QQ 官方机器人、Discord；Minecraft Bridge 与统一管理 HTTP API / SSE。
- 本地插件开发与导入接口，不建设插件市场；Core 托管 Chromium 图片模板与自定义渲染接口。
- ShitBot 白名单单文件导入、多文件合并、冲突预览和导入记录。
- 基于 GitHub Releases 的自动更新：CLI、Desktop、WebUI 提示及检查 / 升级入口，发行资源只核对 SHA-256。
- TypeScript + Vue 3 + Vite WebUI，构建资源进入 Core 模块，由 CLI 可执行文件携带。
- 对外提供窄的 Go `runtime` 包供独立 CLI 托管，另提供适配器 SDK 等必要公开契约；业务实现保持在 `internal`。
- 保存整个项目的协议契约、公共架构、贡献规范、许可说明和 Agent 规则。

## 公共计划索引

| 文档 | 用途 |
| --- | --- |
| [总体规划](docs/plan/01-总体规划.md) | 产品目标、语言、仓库、进程、实施阶段 |
| [仓库结构与开发规范](docs/plan/02-仓库结构与开发规范.md) | 各组件目录、包名、模块依赖、提交规范 |
| [协议配置与迁移](docs/plan/03-协议配置与迁移.md) | 协议草案、配置事务、普通版迁移、统计接口 |
| [思维导图](docs/plan/04-思维导图.md) | 项目组成及职责关系 |
| [参考项目与依据](docs/plan/05-参考项目与依据.md) | 已查阅的官方项目和普通版源码基线 |
| [扩展接口与数据导入](docs/plan/06-扩展接口与数据导入.md) | 本地插件、独立适配器、Chromium 模板与署名、白名单合并、Java MC API |
| [贡献指南](CONTRIBUTING.md) | Conventional Commits 与简明 PR 约定 |
| [代理执行规范](AGENTS.md) | 五仓库共同工作约束；agent.md 仅作导航 |
| [许可证说明](docs/licensing.md) | AGPL-3.0-only 和历史许可边界 |

## 下一阶段顺序

1. 固化配置、管理 API、Bridge、平台身份和消息模型、适配器及扩展接口草案。
2. 实现窄 `runtime` 入口、实例所有权、SQLite 与配置生命周期。
3. 配合 CLI、独立 MC API 和首个平台完成 SnowLuma 连接、文本互通、绑定与登录授权闭环。
4. 增加最小 WebUI 的连接配置与状态页、单文件 / 批量白名单导入，再逐步加入 Chromium 模板、本地插件与更多适配器。
5. 在 CLI、Desktop、WebUI 加入版本提示、检查与升级入口；按对应资产的 SHA-256 校验后执行更新。

本列表是后续任务顺序，不授权立即生成骨架、运行测试或实现代码。

## 其他仓库

- [CLI 计划](https://github.com/ShitBot-Team/ShitBotUltra-CLI/blob/main/plan.md)：Go 轻量启动器。
- [Desktop 计划](https://github.com/ShitBot-Team/ShitBotUltra-Desktop/blob/main/plan.md)：C# WinUI 3 原生桌面。
- [Minecraft 计划](https://github.com/ShitBot-Team/ShitBotUltra-Minecraft/blob/main/plan.md)：Java 薄平台适配器。
- [MC API 计划](https://github.com/ShitBot-Team/ShitBotUltra-MC-API/blob/main/plan.md)：独立 Java 依赖与 Core 通信客户端，供 Minecraft 和其他 Java 扩展调用。

公共文档只在本仓库维护，各组件通过引用使用。核心组件标识为 `shitbotteam.ultra.core`，Go module 为 `github.com/ShitBot-Team/ShitBotUltra`。许可证为 **AGPL-3.0-only**。
