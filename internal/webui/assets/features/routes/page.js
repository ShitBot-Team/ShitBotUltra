import { collectionPage } from '../../components/collection-editor.js';
import { field, grid, select, check, choices } from '../../components/ui.js';

export default collectionPage({
  section: 'routes',
  title: '消息路由',
  singular: '路由',
  hint: '一条路由只能连接同一身份网络中的节点。群聊、私聊和其他平台会话使用独立的平台、作用域与 ID。',
  headers: ['路由', '机器人连接', '会话', '目标节点', '转发方向'],
  cells: r => [
    r.id,
    r.connection_id,
    r.conversation.platform + ' / ' + r.conversation.id,
    r.node_ids.join(', '),
    [r.to_game && '到游戏', r.to_bot && '到机器人'].filter(Boolean).join(' / ') || '关闭'
  ],
  create: () => ({
    id: '',
    connection_id: '',
    conversation: { platform: 'qq.numeric', scope: 'global', id: '', kind: 'group' },
    node_ids: [],
    to_game: true,
    to_bot: true,
    commands: true,
    notifications: true,
    unbind_on_leave: false,
    bot_prefix: '',
    game_prefix: ''
  }),
  fields: (r, config, persisted) => [
    grid([
      field('路由 ID', r.id, v => r.id = v, { readonly: persisted }),
      select('机器人连接', r.connection_id, config.connections || [], v => r.connection_id = v),
      field('平台', r.conversation.platform, v => r.conversation.platform = v),
      field('作用域', r.conversation.scope, v => r.conversation.scope = v),
      field('会话 ID', r.conversation.id, v => r.conversation.id = v),
      select('会话类型', r.conversation.kind, ['group','private','channel','thread'].map(id => ({ id })), v => r.conversation.kind = v),
      field('机器人消息前缀', r.bot_prefix, v => r.bot_prefix = v),
      field('游戏消息前缀', r.game_prefix, v => r.game_prefix = v)
    ]),
    choices('目标节点', r.node_ids, (config.nodes || []).map(n => ({ id: n.id, label: n.id + ' · ' + n.network_id })), v => r.node_ids = v),
    grid([
      check('转发到游戏', r.to_game, v => r.to_game = v),
      check('转发到机器人', r.to_bot, v => r.to_bot = v),
      check('允许群命令', r.commands, v => r.commands = v),
      check('发送通知', r.notifications, v => r.notifications = v),
      check('退出会话时解除绑定', r.unbind_on_leave, v => r.unbind_on_leave = v)
    ])
  ]
});
