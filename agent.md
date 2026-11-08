# ShitBotUltra Agent 协作规则

本文件是 Core、CLI、Desktop、Minecraft 四个仓库的共享协作规则。其他仓库的 `agent.md` 只保留职责和引用；公共规则只在这里维护。用户当前明确指令优先于本文，仓库内更具体的约定不得取消用户要求。

## 开始工作

1. 阅读当前仓库的 `plan.md`，确认本次任务属于哪个组件。
2. 阅读本文件、[贡献指南](CONTRIBUTING.md)与[许可证说明](docs/licensing.md)。
3. 按任务阅读[总体规划](docs/plan/01-总体规划.md)、[项目结构](docs/plan/02-仓库结构与开发规范.md)或[协议与迁移](docs/plan/03-协议配置与迁移.md)，不重复复制公共文档。
4. 查看当前 Git 状态和已有修改，保留用户修改；只实现用户授权的范围，不把规划中的后续功能当作本次实现指令。

当前四仓库只有规划和协作文件，尚未交付运行程序。CLI、Desktop、Minecraft 的 README 后续再写，未获新指令时不提前生成。

## 验证与工作范围

- 按维护者要求，默认不运行测试，也不通过构建、脚本、CI 手动触发等方式间接执行测试；测试由维护者自行完成。只有用户后续明确要求时才改变此约定。
- 本阶段只做文档和仓库初始化，不运行构建、启动应用或新增自动测试工作流。
- 可以阅读文件和差异、核对 Git 状态及提交签名；这些检查不等于已通过功能测试。最终报告如实说明做了什么。
- 不覆盖用户未提交的改动，不重置、不强推、不改写历史，除非用户明确要求对应操作。
- 不为范围外的功能增加依赖、框架、配置或在线服务；缺少后续接口时在计划中记录待提供信息。

## Git 与协作

- 使用系统安装的 Git 执行 commit 和 push，使用实际提交者已配置的身份；不添加助手署名或 AI `Co-authored-by`。
- 提交遵循[贡献指南](CONTRIBUTING.md)中的 Conventional Commits，按职责分批提交。类型用 `fix`、`feat`、`refactor`、`docs`、`style`、`test`、`chore`；描述和正文允许中文、英文或其他语言。
- 使用现有签名密钥签名，签名失败不得关闭签名绕过。用户授权推送后，通过 GitHub 的提交核验结果确认 `Verified`；本地验签成功不能代替平台状态。
- 用户的推送授权在当前任务范围内持续有效，不重复请求确认。未获得推送授权时只完成本地工作。
- PR 标题和说明遵循贡献指南，欢迎 AI 辅助贡献；不伪造检查、测试、构建或兼容性结果。

## 组件边界

| 仓库 | 负责内容 | 关键限制 |
| --- | --- | --- |
| [ShitBotUltra](https://github.com/ShitBot-Team/ShitBotUltra) | Go Core、公共协议、内置 WebUI、共享文档 | Core 不依赖 CLI 或桌面 UI |
| [ShitBotUltra-CLI](https://github.com/ShitBot-Team/ShitBotUltra-CLI) | Go + 标准库 `flag`，托管 Core 的轻量启动器 | 不引入 Cobra，不重复实现业务 |
| [ShitBotUltra-Desktop](https://github.com/ShitBot-Team/ShitBotUltra-Desktop) | C# + WinUI 3 + XAML，启动或附加同一个 Core | Avalonia 是备选；不以 WebView 替代默认桌面 UI |
| [ShitBotUltra-Minecraft](https://github.com/ShitBot-Team/ShitBotUltra-Minecraft) | Java 平台适配器，事件、快照和游戏动作 | 不连接 QQ、不持有业务数据库、不渲染业务图片 |

命名使用 `shitbotteam.ultra.<组件名>`，按所用语言映射。Minecraft 根包为 `shitbotteam.ultra.minecraft`，Desktop 根命名空间为 `shitbotteam.ultra.desktop`；Go 模块保留仓库导入路径，不能改成 Java 点分包名。

## 文档与许可

- 四个仓库采用 **AGPL-3.0-only**，完整许可正文在各自 `LICENSE` 中；历史 MIT 材料及第三方许可边界见[许可证说明](docs/licensing.md)。
- 公共架构、目录规范、协议、迁移方案、思维导图和贡献规则只存放于本仓库；各组件计划只描述本组件任务并引用公共文档。
- 同级克隆目录中可读取 `../ShitBotUltra/` 下的公共文件；未同时克隆 Core 时使用 `https://github.com/ShitBot-Team/ShitBotUltra/blob/main/` 对应文件。无法读取时说明缺失，不把臆测内容当作已读规则。
- 不把计划中的命令、接口、平台或统计声明为已实现；bStats 项目 ID 和外部接口等待维护者提供，不编造地址、凭据或兼容结果。
