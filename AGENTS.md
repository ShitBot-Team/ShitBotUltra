# ShitBotUltra 代理工作规范

本文件是 Ultra 系列的公共代理规范，也是本仓库的直接执行入口。`agent.md` 只做阅读导航；不要依赖工具自动加载小写文件名。用户当前明确指令优先于仓库约定。

## 当前任务边界

- 当前阶段只维护计划、协作规范和许可证。维护者明确要求“先写 plan，不写代码”；计划中的阶段、目录和接口名称都不是实现授权。
- 不生成程序骨架、依赖清单、构建配置或测试工作流；CLI、Desktop、Minecraft、MC API 的 README 留待维护者安排。
- 不运行测试，也不通过构建、启动程序、脚本或手动触发 CI 间接执行测试；测试由维护者自行完成。后续只有维护者明确要求才改变这条约定。
- 可以阅读源码与文档、检查差异、Git 状态和提交核验结果。不得把这些检查描述成功能测试、构建或兼容性验证。
- 只修改当前授权范围，保留用户已有改动；不自行实现后续阶段，不重置、不强推、不改写历史。

## 任务开始与按需阅读

1. 确认正在操作的仓库、分支及 `git status`，识别未提交修改。
2. 从本仓库 [plan.md](plan.md) 确认职责和相关任务；按下表只读本次需要的公共文档，不要求每次全文加载所有计划。
3. 变更涉及多个组件时，同步相应组件的职责与计划引用；公共设计只在 Core 维护。

| 本次任务 | 阅读入口 |
| --- | --- |
| 产品目标、语言、进程或阶段 | [总体规划](docs/plan/01-总体规划.md) |
| 目录、依赖、包名或跨仓库接口 | [仓库结构](docs/plan/02-仓库结构与开发规范.md) |
| Bridge、配置、普通版迁移或自动更新 | [协议配置与迁移](docs/plan/03-协议配置与迁移.md) |
| 插件、机器人适配器、图片模板、白名单导入或 Java API | [扩展接口与数据导入](docs/plan/06-扩展接口与数据导入.md) |
| 外部项目、来源或复用 | [参考依据](docs/plan/05-参考项目与依据.md)、[许可说明](docs/licensing.md) |
| commit、push 或 PR | [贡献指南](CONTRIBUTING.md) |

## 组件与接口约束

| 仓库 | 职责 | 边界 |
| --- | --- | --- |
| ShitBotUltra | Go Core、公共协议、WebUI、业务、渲染及导入 | 不依赖 CLI 或桌面 UI；业务不依赖特定机器人协议 |
| ShitBotUltra-CLI | Go 标准库 `flag`、日志、Core 宿主及更新入口 | 不引入 Cobra，不复制 Core 业务 |
| ShitBotUltra-Desktop | C#、WinUI 3、XAML，启动或附加 Core | 经管理 API 工作，不直写业务数据库；Avalonia 仅为备选 |
| ShitBotUltra-Minecraft | Java 游戏事件、快照、动作和平台线程调度 | 调用独立 MC API；不连接社交平台、不持有业务数据库、不渲染图片 |
| ShitBotUltra-MC-API | 独立 Java API 与 Core 通信客户端 | 统一连接、请求、事件、超时与异步语义；不依赖 Bukkit 等平台实现 |

- 不建设插件市场；保留插件开发接口和用户本地导入。插件、机器人适配器、图片模板是不同扩展入口。
- OneBot v11、QQ 官方机器人、未来 Discord 等适配器分别实现并经统一契约注册；业务使用带平台与作用域的身份、消息和能力，不能把所有平台 ID 当 QQ 号。
- 图片默认规划为 Core 托管 Chromium 的 HTML/CSS/JavaScript 模板；图片输出与模板管理接口保持可扩展。复用模板时保留作者、项目地址及上游许可要求。
- 白名单导入同时覆盖单文件与多文件合并，先预览、报告冲突，再由用户执行导入；不修改源库或静默覆盖已有绑定。
- 自动更新以 GitHub Releases 和对应文件的 SHA-256 为准，不增加发行包 sig、公钥或密钥轮换机制。此约定不取消 Git 提交的签名要求。
- 命名使用 `shitbotteam.ultra.<组件名>`；Minecraft 与 MC API 的公共 Java 包维持 `shitbotteam.ultra.minecraft.api`，Go module 保留仓库导入路径。

## Git 与交付

- 为新增功能或 Bug 修复提交 PR 前，先在对应组件仓库提交 Issue，让作者了解并沟通后再提 PR；PR 正文必须关联该 Issue。人工与代理辅助贡献遵守相同流程。
- 使用系统安装的 Git 和已经配置的提交者身份，按职责分批 commit；提交采用 Conventional Commits，描述可中文。不加助手署名或 AI `Co-authored-by`。
- 使用现有签名配置，失败时不关闭签名。push 后核对 GitHub 返回的 `Verified` / `verification.verified`；本地验签不能代替平台结果。
- 本次维护者已经授权 commit 后 push，授权在当前文档任务内有效，不重复请求确认。其他任务按其实际授权范围操作。
- 结束时说明变更范围、提交与推送结果、平台签名状态，以及“按维护者要求未运行测试或构建”；未完成事项据实报告。

## 规则维护与许可

公共规则只在本文件维护。组件 `AGENTS.md` 保留可直接生效的任务限制、组件边界与公共入口；组件 `agent.md` 只做导航。进入其他仓库时主动读取当地 `AGENTS.md`，不能假设工具会跨仓库自动发现 Core 规则。

同级克隆可读取 `../ShitBotUltra/AGENTS.md`；没有 Core 克隆时读取 [GitHub 上的公共规范](https://github.com/ShitBot-Team/ShitBotUltra/blob/main/AGENTS.md)。只读本次需要的链接，不能把未读取的内容当作已加载规则。

五个 Ultra 仓库各自提供完整的 **AGPL-3.0-only** `LICENSE`。第三方代码、模板、字体与素材保留原许可；普通版 ShitBot 的许可单独维护。具体边界见[许可说明](docs/licensing.md)。

本项目署名为 `ShitBot-Team and contributors`，项目地址为 `https://github.com/ShitBot-Team/ShitBotUltra`。保留 LICENSE 前置的项目版权、组件源码地址与 SPDX 标识，不修改 AGPL 正文里的 FSF 声明，不删除实际贡献者及第三方署名。后续 CLI 版本信息、Desktop / WebUI 关于页按许可说明展示项目署名与许可入口。
