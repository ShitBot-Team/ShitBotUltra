# Core 图片与模板

图片处理由 Core 托管独立 Chromium 进程。每个任务有独立页面、进程、超时和有界并发；没有配置浏览器时，群命令使用文本结果。

在 `rendering.browser_path` 指定浏览器可执行文件；`browser_version` 是 CDP `Browser.getVersion` 返回的完整 `product`，例如 `Chrome/实际版本号`，必须完全匹配。Core 不下载浏览器，也不关闭 Chromium sandbox。运行身份和系统必须能启动启用 sandbox 的该版本浏览器。本次交付未进行浏览器运行验证。

## 管理 API

所有接口均在 `/api/v1` 下，使用管理 Bearer Token。

| 方法和路径 | 内容 |
| --- | --- |
| GET `/rendering` | 渲染开关及内置数据提供器 |
| GET / POST `/templates` | 列表 / 上传原始 ZIP 请求体，最大 64 MiB |
| GET `/templates/{id}` | 模板详情和 ETag |
| GET `/templates/{id}/files?revision=...` | 指定版本文件列表 |
| GET `/templates/{id}/file?revision=...&path=...` | UTF-8 文件的 JSON 文本，不执行 HTML |
| PUT `/templates/{id}/draft` | `{base_revision, files: {文件名: 文本或null}}` |
| PUT `/templates/{id}/defaults` | `{theme, css, locale}` |
| POST `/templates/{id}/publish` | `{version_revision}`；可选择旧版本回滚 |
| POST `/templates/{id}/restore` | 发布最近的内置版本 |
| POST `/rendering/providers/{id}` | 查询 `core.online/profile/inventory/performance` 数据 |
| POST `/renders` | `sdk/render.Request`，返回图片元数据 |
| GET `/renders/resources/{id}` | 读取受认证保护的 PNG / JPEG |

草稿、默认样式、发布、恢复操作都要在 `If-Match` 中携带模板详情返回的 ETag；冲突返回 409，缺少版本返回 428。状态 revision 与文件内容 version revision 不同，不能混用。

模板 ZIP 根目录须含 `template.json` 和本地 HTML 入口。清单字段见 `sdk/render.Manifest`；保留作者、项目地址、许可证及第三方声明。HTML 在首次执行前获得 `ultra.context`；完成排版后调用 `ultra.ready()`。不提供 Node.js、任意文件读取或管理凭据。外部网络请求被限制，资源应包含在模板包中。

空的请求 `theme/css/locale` 继承模板默认值；`ignore_defaults: true` 可预览不带默认样式的结果。未指定 revision 时使用已发布版本，预览草稿必须明确指定内容 revision。模板数据通过 schema 校验，尺寸与像素总数受限。

图片保留 10 分钟，最多 128 个、合计 256 MiB。缓存已满返回 429；渲染失败返回 502；未配置浏览器返回 503。图片发送结果未知时不再自动补发文本，以避免重复消息。缓存是临时数据，不作为持久文件存储。

内置模板来源及许可保留在 `internal/rendering/builtin/templates/`。此接口不代表旧 Java2D 模板可直接导入。
