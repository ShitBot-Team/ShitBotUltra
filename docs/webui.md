# 内置管理页面

Core 默认在同一个 HTTP 监听地址提供管理页面与 API。首次启动后打开宿主输出的地址，输入管理 Token。默认凭据位于配置目录 `secrets/admin.token`；也可在 `http.admin_token` 中引用环境变量或其他凭据文件。

Token 仅在页面内存中保存，刷新或退出后需重新输入。请求使用 Authorization Bearer header；SSE 使用 fetch 流，不将 Token 放进查询参数。页面使用本地 Vue 3 runtime 和静态资源，不依赖运行时 CDN。模板源码通过 JSON 文本编辑，预览只显示渲染后的 PNG / JPEG，不在管理页面执行用户模板。

## 已实现页面

运行概览、连接状态、配置原文与校验、凭据创建、绑定与无账号白名单、玩家列表、背包快照、SQLite 单文件 / 多文件导入与冲突排除、报告下载与撤销、模板文件草稿 / 发布 / 回滚 / 默认样式 / 图片预览、插件授权与生命周期、宿主更新和事件记录。

配置文件保存有 ETag 冲突保护，写入前备份；保存后提示待重启，不宣称连接已热切换。导入的源文件、目标范围或排除选择变化后必须重新预览，不能直接提交旧报告。游戏动作仍走 Core 的权限与能力检查。

`webui: true/false` 是下次启动的默认值。`runtime.Options.WebUI` 可覆盖初始值；管理 API `PUT /api/v1/webui` 的 `{enabled:true/false}` 修改本次会话。关闭页面不关闭 `/api/`。页面与 API 共用 Host / Origin 限制；反向代理需要正确设置允许的 Host，远程部署按 HTTP 配置要求启用 TLS。

## 源码与可选打包

`internal/webui/assets/` 是手写的、浏览器可直接执行的 Vue ES modules，随 Go module 一起嵌入。它不是本次运行编译工具生成的 dist。Vue 固定为 3.5.13，上游原始 runtime、MIT LICENSE 与 npm integrity / SHA-256 记录在 `assets/vendor/`；未修改其运行时代码。

`web/` 提供 TypeScript 入口、公共 DTO、Vite 配置和锁定的依赖清单。它导入同一份组件与 API 客户端，没有第二套页面业务。主要浏览器模块当前以 JavaScript 编写，TypeScript 入口与 DTO 不等于所有页面已完成 TypeScript 严格类型校验。

维护者如需优化打包，可自行在 `web/` 执行 `npm ci` 和 `npm run build`，输出到 `internal/webui/optimized/dist/`。这两个命令未在本次任务执行。运行环境最低为 Node.js 20.19 或 22.12（按 package.json engines）。没有优化产物时，Core 直接使用完整的 authored assets；终端用户不需要 Node.js。

优化产物带 authored assets 的源码摘要；Core 只在摘要匹配时选择优化版本，防止修改页面后继续携带旧页面。发布需要优化版本的 Go module tag 时，应把经过维护者验证的整个 dist 纳入发行源码；单纯留在本地未跟踪目录不会随 Go module 分发。没有纳入时仍携带同版本原始页面。

本次只检查源文件语法、依赖清单与接口对应关系，未编译前端、未启动 Core、未进行浏览器或功能验证。
