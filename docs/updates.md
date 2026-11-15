# 宿主更新

Core 是 Go module，随拥有进程的 CLI、服务或 Desktop 升级。Core 不替换当前 EXE，也不推断宿主安装目录。宿主通过 `runtime.Options.Updates` 注册 `sdk/update.Options`，明确自己的仓库、当前版本、当前 OS / 架构 / 安装格式对应的完整资产名，以及 Installer 回调。

CLI 的官方仓库为 `ShitBot-Team/ShitBotUltra-CLI`，Desktop 为 `ShitBot-Team/ShitBotUltra-Desktop`。资产名由实际发行约定决定，不猜测 release 第一个文件。`AssetName` 可包含 `{version}`，展开为不带开头 v 的 tag。未注册 Updates 时不发更新网络请求，管理 API 返回 disabled。

检查读取所选仓库最新一页最多 100 个 Releases，忽略 draft，稳定通道忽略预发行，选择其中最高语义化版本。非语义化 tag 不参与比较；不把 dev 当作可自动升级的正式宿主版本。非常久远且不在这一页中的 Release 不参与选择。

下载必须精确匹配一个资产，优先使用该资产 `digest` 中的 SHA-256，缺失时读取同一 Release 的 `SHA256SUMS` 并精确匹配完整文件名。拒绝缺少摘要、重复目标项、未知格式和过大资产。只走受限 GitHub HTTPS 下载及其资产重定向；不使用 sig、公钥或密钥轮换机制。接口依据：[GitHub Release assets](https://docs.github.com/en/rest/releases/assets)。

下载到配置目录内私有缓存后校验实际大小和 SHA-256，失败删除下载文件。校验成功先以 SQLite 一致性快照备份数据库，再备份配置文件，最后调用宿主 Installer。返回的 `sdk/update.Staged` 包含文件、Release 和备份路径。宿主负责解包校验、Bridge / 管理 API 兼容检查、权限检查、替换、重启与失败回退；不能只按版本号推断协议兼容。

Installer 返回 nil 仅表示宿主接受了交接，管理状态为 `handed_off`，不声称新版本已运行。该状态禁止重复安装，重启后用实际宿主版本核对。Installer 必须响应 context，不得在回调内部同步等待 Core.Shutdown；关闭流程会等待回调结束。需要退出的宿主应在回调返回后安排退出。宿主消费完安装资产后删除缓存；缓存上限为四个文件，防止反复升级累积下载。

## API 与自动检查

全部接口使用管理 Bearer Token：

| 方法和路径 | 行为 |
| --- | --- |
| GET `/api/v1/updates` | 能力、当前版本、任务阶段、候选版本、错误码、备份名 |
| POST `/api/v1/updates/check` | 检查版本，30 秒期限；失败不影响 Core 业务 |
| POST `/api/v1/updates/install` | `{version, sha256}`；校验与最近一小时的候选信息一致后返回 202 |

并发检查 / 升级返回 BUSY。升级是后台单任务，客户端通过状态接口或 `update.changed` SSE 观察。下载与交接总预算为 20 分钟。无法匹配资产或摘要时，状态保留可确定的 Release 链接供人工查看，但不允许安装。

`CheckInterval` 为 0 时只有手动检查；设置为 1 小时至 7 天时，在启动和指定间隔检查。自动检查不等于自动安装。维护窗口和无人值守安装由进程宿主自行配置，本 Core 不自动替换其他安装，也不更新 Minecraft JAR 或 Java API 依赖。

本次仅做代码与契约审查，未下载真实项目发行包执行升级，未启动宿主或进行运行验证。
