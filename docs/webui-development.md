# WebUI 功能开发

控制台源码是 `internal/webui/assets/` 的 ES modules，使用本地 Vue runtime 的 render functions，适配 Core 的 CSP，不使用运行时模板编译、CDN 或 `eval`。`web/src/main.ts` 仅为同一套源码提供可选 Vite 入口。

## 新增一个功能

1. 在 `features/<name>/` 放置 `page.js` 与需要的 `api.js`。页面自己的状态放在 `reactive` 中，接口 URL 和请求结构放在该功能的 API 层。
2. 页面默认导出 `definePage(context => ...)`。使用 `usePage(context)` 获取 `view.signal`、忙碌状态、通知及公共交互；把信号传给 API 工厂。
3. 返回 `{view, load, render, dirty, dispose}`，后三个生命周期成员按实际需要提供。`load` 负责初次/手动读取，`dirty` 表示尚未提交的草稿，`dispose` 清理 Blob URL 等资源。
4. 在 `app/routes.js` 登记唯一 ID、分组、标题、描述、图标与 `load: () => import(...)`。导航、地址和按需加载统一使用这一条记录，不再修改主组件的 switch。
5. 若后端没有对应能力，先补 Core 服务、管理 handler 与 OpenAPI；不要增加只有按钮而无实现的页面，也不要自动执行第三方插件提供的 UI 脚本。

下面是结构示意，接口名仅为开发示例，不是已提供的接口：

```js
import { reactive } from '../../shared/vue.js';
import { definePage, usePage } from '../../composables/page.js';
import { section, table } from '../../components/ui.js';
import { featureAPI } from './api.js';

export default definePage(context => {
  const view = usePage(context);
  const api = featureAPI(view.signal);
  const state = reactive({ items: [] });
  return {
    view,
    async load() { state.items = (await api.list()).items; },
    render() {
      return [section('功能记录', [table(['名称'], state.items.map(item => [item.name]))])];
    }
  };
});
```

表单型配置优先复用 `useSettings`；列表型配置可参考 `collectionPage`，它只负责列表、草稿、添加/删除和保存，具体字段由功能模块提供。不要让共享组件引用具体的节点、绑定或插件业务。

## 生命周期与契约

- 所有请求经 `api/client.js`，只发送同源 `/api/v1/` 路径。不要把 Token 存入 localStorage、查询参数、图片 URL 或页面日志。
- `view.run` 不重入；写操作完成或失败后释放忙碌状态。导航离开会取消页面请求，但取消 HTTP 并不保证已送达的写操作没有执行。
- 保存使用读取时的 revision。收到 `REVISION_CONFLICT` 后保留草稿，不自动带上新版本重试。
- `context.state` 只承载会话、运行摘要、网络选择和有限的跨页数据交接；功能数据保留在本页。新增状态不要回到集中式大对象。
- 分区配置中保存的是文件状态。绑定网络、在线状态、远程动作权限使用运行状态，不能用未重启的草稿推断已经生效。
- 图片模板文件通过 JSON 文本编辑，预览只显示渲染后的图片，不能在管理源执行模板 HTML/JS。外部链接通过 `safeLink` 限制为 HTTP(S)。
- `styles/tokens.css` 定义颜色、间距和主题；布局改 `styles/layout.css`，通用控件改 `styles/components.css`，领域样式放独立文件并从 `style.css` 引入。不要使用内联样式绕过 CSP。
- 允许的节点动作由 Core 的配置和 Bridge 协议共同约束，新增动作需同步服务端、OpenAPI、MC API 和页面模型，不能只加一个前端选项。

## 维护者自行验收

本轮未运行以下流程；它们是维护者的后续验收建议：

- 首次登录、错误 Token、退出后换 Token、事件断线恢复和刷新页面重新登录。
- 两个页面/浏览器同时编辑不同分区：后保存者应提示冲突，并保留其草稿。
- 新增节点/路由/命令后保存但不重启：页面应显示待重启，业务 API 仍按活动配置处理。
- 无节点、无玩家、离线节点、空列表、窄屏导航、明暗主题与键盘操作。
- 模板草稿、发布/回滚、预览；插件授权/停用；导入预览变化后禁止提交旧预览；更新交接与重连。
- 节点断线时的远程动作、结果未知提示及不自动重试。

静态审查与补丁应用只证明源文件和指定基线关系，不代替这些运行验收。
