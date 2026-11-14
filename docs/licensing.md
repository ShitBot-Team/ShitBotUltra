# ShitBotUltra 许可证说明

ShitBotUltra Core、CLI、Desktop、Minecraft 与独立 MC API 共五个仓库的新版本采用 **GNU Affero General Public License version 3 only**，SPDX 标识为 **`AGPL-3.0-only`**。本次选择不包含“或任何更新版本”的自动授权。已有四仓库保留现有完整正文，MC API 本次补上相同 LICENSE，不另选一套许可。

各仓库根目录保留完整的 `LICENSE` 正文，便于独立获取和分发；本文件统一说明项目级许可约定。许可证正文以各仓库 [LICENSE](../LICENSE) 为准。

## 项目版权与署名

项目统一署名为 **ShitBot-Team and contributors**，项目名称为 **ShitBotUltra**，项目地址为 [ShitBot-Team/ShitBotUltra](https://github.com/ShitBot-Team/ShitBotUltra)。五个仓库的 LICENSE 完整保留 AGPL 正文，并在文末的项目说明区列出各自组件名称、以下版权声明、项目地址、组件源码地址及 SPDX 标识：

```text
Copyright (C) 2026 ShitBot-Team and contributors.
SPDX-License-Identifier: AGPL-3.0-only
```

文末的项目说明区用于说明项目版权与来源，与完整的 AGPL 正文分开，保留 GitHub 的许可证自动识别。授权条款仍为 AGPL-3.0-only。AGPL 正文中的 Free Software Foundation 版权声明属于许可证文本本身，完整保留。贡献者保留各自贡献的版权，第三方内容继续保留原作者的声明，不因项目统一署名而转移版权。

分发本项目或修改版本时，应保留适用的版权、许可及来源说明；修改版本应注明改动和日期，并按 AGPL 提供该版本的对应源码，不能只用上游项目地址代替修改版源码。后续 CLI 版本信息、Desktop 与 WebUI 关于页统一展示项目名称、版权、项目地址和许可入口；本次只补充文档，不实现界面。[GNU 许可使用指南](https://www.gnu.org/licenses/gpl-howto.en.html)、[AGPL 正文](https://spdx.org/licenses/AGPL-3.0-only.html)

## 许可范围

- AGPL-3.0 允许商业使用。本项目采用其源码提供与再分发要求，不额外添加“禁止商用”条款。
- 分发受许可覆盖的程序时，按 AGPL 的对应源码及许可要求执行；修改程序后通过网络与用户交互时，应按第 13 条向相关用户提供取得该修改版本对应源码的机会。
- 各组件通过已规划的 API 协作；实际引入平台 API、库、字体、图标或其他素材时，分别核对许可兼容与声明要求。本项目许可证不替代第三方组件许可证。

正文与适用解释可参阅 [GNU 官方 AGPL-3.0](https://www.gnu.org/licenses/agpl-3.0.html)和 [OSI 收录的许可正文](https://opensource.org/license/agpl-3.0)。

## 普通版与历史版本

Ultra 当前版本以各仓库根目录的 AGPL-3.0-only `LICENSE` 为准。历史版本的许可信息可在对应 Git 提交中查看。

普通版 [hutuyee/ShitBot](https://github.com/hutuyee/ShitBot) 保持自己的 MIT 许可。今后实际复用其代码时，保留对应原许可与版权声明，并记录来源；本次仅以普通版作为规划参考，没有把它的运行代码导入 Ultra 仓库。

## 第三方模板与浏览器资源

计划适配并内置 [EasyBot Image Templates](https://github.com/BaizhouziYou/easybot-image-templates) 的服务器在线列表、玩家统计资料卡和服务器性能监控模板，作者署名为 **BaiZhouziYou**。当前只写计划，没有复制其模板、字体或媒体素材。

上游自有源码标注 AGPL-3.0-only，并另提供 [NOTICE.md](https://github.com/BaizhouziYou/easybot-image-templates/blob/main/NOTICE.md)、[附加条款](https://github.com/BaizhouziYou/easybot-image-templates/blob/main/docs/ADDITIONAL_TERMS.md)及[第三方声明](https://github.com/BaizhouziYou/easybot-image-templates/blob/main/THIRD_PARTY_NOTICES.md)。实际引入时固定上游版本，保留这些文件及 LICENSE，记录适配内容和修改日期；在模板详情与关于 / 致谢页显示作者、项目名称和源码链接，不把衍生版本宣称成原作者运营或认可的项目。上游附加条款未要求每张输出图片添加水印。

实际打包的库、字体和素材各自保留许可：上游列出的 liquidGL、html2canvas 为 MIT，Noto Sans SC 为 SIL OFL 1.1；封面与预览图片另按其[素材说明](https://github.com/BaizhouziYou/easybot-image-templates/blob/main/docs/ASSET-LICENSES.md)核对，不能仅凭仓库源码开放就视为全部获准再分发。随实际引入内容建立 THIRD_PARTY_NOTICES.md 和对应许可文件，不提前写成“已经内置”。

Chromium / Chrome 二进制、浏览器随附组件及其他字体资源也按选定发行来源保留许可与声明，不能由 Ultra 的 LICENSE 替代。正式打包时登记版本、来源和声明文件；本阶段不下载或重新分发浏览器。

## 贡献

新提交的代码与文档按项目当前 AGPL-3.0-only 许可提供；包含第三方内容时，提交者应具备相应授权并保留其声明。AI 辅助贡献遵循相同要求，不因使用 AI 自动改变第三方内容的许可。

贡献流程、提交格式及 PR 说明只在 [CONTRIBUTING.md](../CONTRIBUTING.md) 维护。
