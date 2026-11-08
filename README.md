# ShitBotUltra

ShitBotUltra 是基于 [ShitBot](https://github.com/hutuyee/ShitBot) 功能重新规划的、可自部署的开源群服互通主程序：由独立 Core 处理机器人连接、业务规则、数据存储、图片渲染和管理功能，Minecraft JAR 负责游戏事件、数据采集及游戏侧动作执行。

**当前阶段：架构规划。本文中的程序、命令、接口和其他仓库名称是拟定方案，尚未实现或创建。**

## 规划文档

| 文档 | 内容 |
| --- | --- |
| [总体规划](docs/plan/01-总体规划.md) | 需求、语言选择、四仓库拆分、进程架构、实施顺序 |
| [仓库结构与开发规范](docs/plan/02-仓库结构与开发规范.md) | Core、CLI、Desktop、Minecraft 的目录、依赖、命名及提交规范 |
| [协议配置与迁移](docs/plan/03-协议配置与迁移.md) | 双向通信、桌面配置管理、功能迁移、bStats 和待提供接口 |
| [思维导图](docs/plan/04-思维导图.md) | Mermaid 思维导图与纯文本版本 |
| [参考项目与依据](docs/plan/05-参考项目与依据.md) | 当前 ShitBot 依据、官方资料、借鉴方式和取舍 |

## 推荐组成

| 组件 | 语言与形式 | 主要职责 |
| --- | --- | --- |
| Core | Go | 统一业务引擎、协议、数据库、管理 API、内置 WebUI 资源 |
| CLI | Go 轻量启动器 | 参考 LuckyLillia.CLI 的使用形态，直接运行即可启动 Core、显示日志和本机 WebUI 地址 |
| Desktop | C# + WinUI 3 + XAML | Windows 原生桌面界面，内置 CLI 宿主运行同一份 Core，可选开启 WebUI；Avalonia 为备选 |
| Minecraft | Java + Maven 多模块 | 薄插件，适配 Bukkit/Paper/Folia、Velocity、BungeeCord、Nukkit-MOT |
| WebUI | TypeScript + Vue 3 + Vite | 浏览器调试与管理页面，构建后随 Core 嵌入 CLI 产物 |

推荐将本仓库 `ShitBot-Team/ShitBotUltra` 用作 **Core 仓库与项目入口**，另设 `ShitBotUltra-CLI`、`ShitBotUltra-Desktop`、`ShitBotUltra-Minecraft`。普通版继续留在个人仓库 `hutuyee/ShitBot`。

源码命名采用 `shitbotteam.ultra.<组件名>`：Minecraft 根包为 `shitbotteam.ultra.minecraft`，C# Desktop 根命名空间为 `shitbotteam.ultra.desktop`；Core、CLI、WebUI 对应 `core`、`cli`、`webui`。Go 的模块导入路径仍遵循 Go 规则，具体映射见[命名规范](docs/plan/02-仓库结构与开发规范.md#6-命名与中文信息)。

Core 集中处理业务；CLI、桌面端、浏览器只是不同操作入口。CLI 采用 [LuckyLillia.CLI](https://github.com/LLOneBot/LuckyLillia.CLI) 这种“启动程序、看日志、用少量参数控制”的体验，日常管理主要使用内置 WebUI。Desktop 默认使用 C# + WinUI 3 原生界面，通过同一个 Core 管理配置，按需开启浏览器 WebUI。若以后需要跨平台或 WinUI 的部署条件不合适，再改用 C# + Avalonia；Avalonia 使用自绘控件，不称为 Windows 原生控件。具体取舍见[语言选型](docs/plan/01-总体规划.md#2-编程语言与技术选型)。

## 机器人实现与支持边界

新用户推荐使用 [SnowLuma](https://github.com/SnowLuma/SnowLuma)，首期通过 OneBot v11 正向 WebSocket 对接。其他兼容实现也可作为候选；具体能力以之后的兼容记录为准。

按项目维护者提供的信息，LLBot 已不再开放新用户注册；已有可用环境的用户仍可将 [LLBot / LuckyLilliaBot](https://github.com/LLOneBot/LuckyLilliaBot) 作为兼容选项。Core 以协议适配，避免把某个机器人实现写死在业务中。

> 请勿在 SnowLuma、LLBot 等第三方项目的社交群中讨论 ShitBot / ShitBotUltra 或寻求使用、排障支持，以免违反群规。请通过对应项目的 GitHub Issues 反馈：[ShitBot](https://github.com/hutuyee/ShitBot/issues)、[ShitBotUltra](https://github.com/ShitBot-Team/ShitBotUltra/issues)。

## 开源与统计

本仓库现有许可证为 [MIT](LICENSE)。计划公开 Core、CLI、Desktop、Minecraft 插件、WebUI、协议及构建说明；正常自部署不依赖项目方账号或付费授权服务。第三方机器人服务的条件由各自项目决定。

bStats 计划接入受支持的 Minecraft 平台插件。Ultra 使用独立项目 ID，不沿用普通版统计 ID；ID 和后续接口由维护者提供。具体边界见[统计与外部接口](docs/plan/03-协议配置与迁移.md#6-bstats-与后续接口)。
