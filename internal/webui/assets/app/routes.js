// Add a feature here: navigation, hash routing and the lazy loader stay in sync.
// This registry describes built-in pages, not executable third-party plugins.

export const groups = [
  { id: 'workspace', label: '工作台' },
  { id: 'connections', label: '连接与路由' },
  { id: 'data', label: '玩家与数据' },
  { id: 'automation', label: '命令与扩展' },
  { id: 'system', label: '核心设置' }
];

export const routes = [
  {
    id: 'overview',
    group: 'workspace',
    title: '运行概览',
    description: '连接、玩家与运行状态，一眼掌握。',
    icon: 'dashboard',
    load: () => import('../features/overview/page.js')
  },
  {
    id: 'events',
    group: 'workspace',
    title: '事件记录',
    description: '查看当前会话中的 Core 管理事件。',
    icon: 'activity',
    load: () => import('../features/events/page.js')
  },
  {
    id: 'nodes',
    group: 'connections',
    title: '服务器管理',
    description: '管理 Minecraft 节点、网络身份与动作权限。',
    icon: 'server',
    load: () => import('../features/nodes/page.js')
  },
  {
    id: 'adapters',
    group: 'connections',
    title: '机器人适配器',
    description: '配置机器人连接，查看连接状态。',
    icon: 'plug',
    load: () => import('../features/adapters/page.js')
  },
  {
    id: 'routes',
    group: 'connections',
    title: '消息路由',
    description: '把群聊与服务器连接起来。',
    icon: 'route',
    load: () => import('../features/routes/page.js')
  },
  {
    id: 'players',
    group: 'data',
    title: '玩家与背包',
    description: '查看玩家会话、在线时长与背包快照。',
    icon: 'users',
    load: () => import('../features/players/page.js')
  },
  {
    id: 'bindings',
    group: 'data',
    title: '绑定与白名单',
    description: '按游戏网络管理账号绑定和准入名单。',
    icon: 'link',
    load: () => import('../features/bindings/page.js')
  },
  {
    id: 'binding-settings',
    group: 'data',
    title: '绑定规则',
    description: '配置验证码、数量限制与解绑行为。',
    icon: 'settings',
    load: () => import('../features/binding-settings/page.js')
  },
  {
    id: 'imports',
    group: 'data',
    title: '白名单导入',
    description: '预览、解决冲突，再迁移旧版数据。',
    icon: 'download',
    load: () => import('../features/imports/page.js')
  },
  {
    id: 'commands',
    group: 'automation',
    title: '自定义命令',
    description: '配置命令模板、参数规则、角色与冷却。',
    icon: 'terminal',
    load: () => import('../features/commands/page.js')
  },
  {
    id: 'actions',
    group: 'automation',
    title: '远程动作',
    description: '向已连接节点发送管理动作。',
    icon: 'terminal',
    load: () => import('../features/actions/page.js')
  },
  {
    id: 'plugins',
    group: 'automation',
    title: '本地插件',
    description: '管理本地扩展的版本、配置和授权。',
    icon: 'box',
    load: () => import('../features/plugins/page.js')
  },
  {
    id: 'templates',
    group: 'automation',
    title: '图片模板',
    description: '编辑草稿、生成预览与管理发布版本。',
    icon: 'image',
    load: () => import('../features/templates/page.js')
  },
  {
    id: 'rendering',
    group: 'system',
    title: '渲染设置',
    description: '配置 Chromium、队列与默认模板。',
    icon: 'image',
    load: () => import('../features/rendering/page.js')
  },
  {
    id: 'config',
    group: 'system',
    title: '配置与凭据',
    description: '编辑完整配置，创建连接所需的凭据。',
    icon: 'settings',
    load: () => import('../features/config/page.js')
  },
  {
    id: 'updates',
    group: 'system',
    title: '版本更新',
    description: '检查宿主版本，查看更新任务。',
    icon: 'download',
    load: () => import('../features/updates/page.js')
  },
  {
    id: 'about',
    group: 'system',
    title: '关于项目',
    description: '项目来源、许可与第三方致谢。',
    icon: 'info',
    load: () => import('../features/about/page.js')
  },
];

export const findRoute = id => routes.find(route => route.id === id);
