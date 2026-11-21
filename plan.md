# ShitBotUltra Core 计划

更新日期：2026-11-21。状态：Core 已有源码实现并补齐管理流程，当前仅完成静态审查与补丁应用检查，未构建、测试或联调。其他四个仓库的交付状态不能从 Core 推断。[实现状态](docs/implementation-status.md)列出当前范围与后续工作，本文保留跨仓库职责及计划索引。

## 本仓库职责

- Go Core：账号绑定、白名单、消息路由、群命令、玩家资料、背包存储、通知与图片任务。
- 独立机器人适配器框架：首发 OneBot v11，后续 QQ 官方机器人、Discord；Minecraft Bridge 与统一管理 HTTP API / SSE。
- 本地插件开发与导入接口，不建设插件市场；Core 托管 Chromium 图片模板与自定义渲染接口。
- ShitBot 白名单单文件导入、多文件合并、冲突预览和导入记录。
- 基于 GitHub Releases 的自动更新：CLI、Desktop、WebUI 提示及检查 / 升级入口，发行资源只核对 SHA-256。
- Vue 3 管理页面，默认嵌入可直接执行的本地 ES modules；提供 TypeScript / Vite 优化打包入口，由 Core 宿主携带。
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

本轮继续完成 Core 控制台管理：新增统计与分区配置 API，将 WebUI 拆为独立功能模块并提供服务器、适配器、路由、命令、渲染等管理表单。实现细节见 [WebUI](docs/webui.md) 和 [开发说明](docs/webui-development.md)。依然未编译或运行测试。

## 接续工作顺序

1. 维护者审阅并应用分模块补丁，再按自己的环境完成构建与验证。
2. CLI 接入公开 runtime，提供日志、信号、就绪 URL 和符合安装方式的更新 Installer。
3. 独立 MC API 与首个平台按 Bridge v1 完成登录拦截、在线快照、游戏动作和线程调度，随后进行 OneBot / 游戏端联调。
4. Desktop 经管理 API 完成原生页面与启动 / 附加流程，复用 Core 业务与更新交接契约。
5. 根据兼容记录补齐其他平台、QQ 官方 / Discord、完整旧版迁移与统计，不将规划条目提前标为已实现。

后续工作按维护者具体授权实施；本轮没有运行测试、构建或修改其他组件仓库。

## 其他仓库

- [CLI 计划](https://github.com/ShitBot-Team/ShitBotUltra-CLI/blob/main/plan.md)：Go 轻量启动器。
- [Desktop 计划](https://github.com/ShitBot-Team/ShitBotUltra-Desktop/blob/main/plan.md)：C# WinUI 3 原生桌面。
- [Minecraft 计划](https://github.com/ShitBot-Team/ShitBotUltra-Minecraft/blob/main/plan.md)：Java 薄平台适配器。
- [MC API 计划](https://github.com/ShitBot-Team/ShitBotUltra-MC-API/blob/main/plan.md)：独立 Java 依赖与 Core 通信客户端，供 Minecraft 和其他 Java 扩展调用。

公共文档只在本仓库维护，各组件通过引用使用。核心组件标识为 `shitbotteam.ultra.core`，Go module 为 `github.com/ShitBot-Team/ShitBotUltra`。许可证为 **AGPL-3.0-only**。
