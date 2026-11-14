# 贡献指南

欢迎提交 PR，包括问题修复、功能改进、文档与翻译，也欢迎 AI 辅助编写的代码和文档。人工与 AI 辅助贡献按同一套标准审阅；提交者应阅读自己的改动，确认范围，并能回应评审中的问题。

本规范适用于 ShitBotUltra 系列仓库的人工作业和 AI 辅助贡献。提交信息、PR 标题与说明均可以使用中文，允许其他语言提交信息；提交类型使用下列英文前缀。

## Commit Message Format

使用 [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/) 格式：

```text
<type>: <描述>
<type>(<scope>): <描述>
```

`scope` 可省略；类型后使用英文冒号和一个空格。描述、正文和补充说明可以用中文、英文或其他语言，AI 生成的提交信息同样遵守这一格式。

| 类型 | 用途 | 示例 |
| --- | --- | --- |
| `fix:` | 修复问题 | `fix(minecraft): 修复断线后的登录提示` |
| `feat:` | 新增功能 | `feat(desktop): 添加配置文件编辑入口` |
| `refactor:` | 不改变对外行为的代码重构 | `refactor(core): 拆分消息路由逻辑` |
| `docs:` | 文档修改 | `docs: 补充首次启动说明` |
| `style:` | 不改变行为的格式调整，如缩进、空格、分号 | `style(cli): 统一源码缩进` |
| `test:` | 新增或更新测试 | `test(core): 补充绑定验证码过期用例` |
| `chore:` | 构建流程、依赖、辅助工具等维护工作 | `chore: 更新构建依赖` |

可选范围例如 `core`、`cli`、`desktop`、`minecraft`、`webui`、`protocol`。`style` 指代码格式；新增或修复界面功能时按实际改动使用 `feat` / `fix`。

描述应简短、具体，说明这次改动，例如“修复重复发送群消息”；避免只写“更新”“修改代码”或“AI 修改”。复杂改动可在标题下空一行，补充原因和影响。

破坏兼容性的修改在类型或范围后加 `!`，或使用 `BREAKING CHANGE:` 页脚，并说明受影响的配置 / API 及迁移方式。例如：

```text
feat(protocol)!: 升级游戏连接握手格式

BREAKING CHANGE: 旧版适配器无法连接，需要同时升级 Minecraft 插件。
```

## 提交与签名

- 按职责分批提交，每个提交围绕一个完整改动；无关调整分开处理。
- 使用系统安装的 Git 与实际提交者身份完成提交和推送；AI 工具不附加自身署名或 AI `Co-authored-by`。
- 使用已配置的签名密钥提交，推送到 GitHub 后确认提交显示 `Verified`；签名失败时先解决问题，不关闭签名绕过要求。本地验签和 GitHub 平台核验分别确认。
- 新规范用于后续提交，不要求为了格式重写已有历史。

提交签名与发行包签名是不同事项。签名设置和 GitHub 核验条件见 [GitHub 官方说明](https://docs.github.com/en/authentication/managing-commit-signature-verification/about-commit-signature-verification)。

## Pull Request 约定

1. **标题使用同样的 Conventional Commits 格式。** 例如 `fix(minecraft): 修复重复进服通知`；说明和讨论可使用中文。
2. **一个 PR 聚焦一个明确目的。** 相关改动分批提交，避免夹带无关重构或大范围格式化。
3. **说明问题、改动和影响。** 有关联 Issue 时附上链接；涉及配置、协议或兼容性变化时，说明受影响范围及迁移方式，相关文档同步更新。
4. **如实填写验证情况。** 做过什么、结果如何就写什么；未运行测试时明确写“未运行测试”及原因。这条不要求每个 PR 必须新增或执行测试，也不能把计划中的验证写成已完成。
5. **欢迎 AI 辅助贡献。** 可以让 AI 编写代码、提交信息和 PR 描述，内容须与实际改动一致；不要求额外填写 AI 工具声明，也不添加助手署名。

## 贡献许可与公共文档

Ultra 五个仓库（Core、CLI、Desktop、Minecraft、MC API）采用 **AGPL-3.0-only**。新贡献按项目当前许可提供，第三方内容保留原许可与必要声明；历史 MIT 版本及模板复用边界见[许可证说明](docs/licensing.md)。

本贡献指南、[代理执行规范](AGENTS.md)和公共架构文档只在 `ShitBot-Team/ShitBotUltra` 维护。组件 AGENTS.md 保留本地执行限制、职责和公共入口；agent.md 仅作导航，避免多份详细规范逐渐不一致。
