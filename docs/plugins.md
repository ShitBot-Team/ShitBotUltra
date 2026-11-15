# 本地插件

Core 接受根目录含 `plugin.json` 和 JavaScript 入口的 ZIP。导入不会执行插件，也不会自动授予权限；启用时明确选择版本、配置与能力范围。没有插件市场。

JavaScript 由 Goja 在进程内串行运行，每个插件有 64 项队列和 5 秒调用预算，最多同时启用 32 个插件。JS 回调和返回值 JSON 转换共用超时预算；Go 宿主服务接受同一个 context。仅导入管理员信任的代码：此模型不是操作系统隔离，也没有 Goja 堆内存硬配额。插件不能被当作运行任意不可信代码的沙箱。

## 清单、回调与调用

字段见 `sdk/extension.Manifest`。`api_version` 当前为 1，入口必须为包内 `.js` 文件。使用 `exports.init(config)`、`exports.enable()`、`exports.disable()`、`exports.dispose()`，事件入口为 `exports.onEvent(event)`，命令入口为 `exports.onCommand(request)`，定时入口为 `exports.tick()`。数据提供器注册在 `exports.providers` 对象中。

回调返回同步 JSON 值，命令可返回文本、统一消息对象或 null。异步 Promise 返回值会报错；不提供 Node.js `require`、文件系统、网络和定时器全局。应将需要的 JS 依赖打包为单入口。模板 HTML 的 `ultra.ready()` 与插件的 `ultra.call()` 是不同接口。

`ultra.call(method, request)` 的服务：

| 方法 | 所需能力 | 范围 |
| --- | --- | --- |
| `bindings.list` | `bindings.read` | 授权网络 |
| `bindings.add/remove` | `bindings.write` | 授权网络，使用统一业务规则 |
| `players.list` | `players.read` | 授权网络 |
| `inventory.get` | `inventory.read` | 授权网络 |
| `messages.send` | `messages.send` | 授权路由；图片资源还需 `render.request` |
| `game.request` | `game.<动作类型>` | 授权节点及该节点允许的动作 |
| `render.request` | `render.request` | 使用插件提供的数据渲染模板 |
| `storage.get/set` | `storage` | 插件专属键空间，单键 64 KiB、总量 256 KiB |

当前可订阅 `message`、`node.connected`、`node.disconnected`、`player.joined`、`player.left`、`players.snapshot`。事件队列满时丢弃并累计 dropped；插件不得假设所有事件都会送达。群命令复用角色、绑定、参数正则与冷却检查。命令名不能覆盖内置或已注册命令。

停用阻止新的宿主调用并回收 worker。卸载前必须停用，数据及最后配置保留在 `plugin-data/<id>`，不删除其他插件内容。修改配置、权限或版本时先停用，再凭最新 ETag 启用。

## 管理 API

全部接口要求管理 Bearer Token，前缀为 `/api/v1`。

| 方法与路径 | 请求 |
| --- | --- |
| GET / POST `/plugins` | 列表 / 上传原始 ZIP，最大 64 MiB |
| GET `/plugins/{id}` | 详情、状态、ETag |
| POST `/plugins/{id}/enable` | `{version_revision, grant: {capabilities, networks, routes, nodes}, configuration}` |
| POST `/plugins/{id}/disable` | 无请求体 |
| DELETE `/plugins/{id}` | 卸载包、保留数据 |
| POST `/plugins/{id}/providers/{name}` | 传入 JSON，返回该插件数据提供器的 JSON |

启用、停用、卸载都要带 `If-Match: "状态revision"`；启用请求中的 `version_revision` 是包内容摘要。scope 必须包含所选节点及路由所属网络。状态显示 running / disabled / failed，错误不返回插件配置或内部异常详情。

`examples/plugins/hello` 可作为开发起点；将目录内文件放到 ZIP 根目录再导入。此示例及宿主代码仅做静态审查，未执行插件或运行测试。
