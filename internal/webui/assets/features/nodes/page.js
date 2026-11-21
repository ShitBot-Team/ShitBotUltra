import { collectionPage } from '../../components/collection-editor.js';
import { field, grid, select, check, choices, table, section, badge } from '../../components/ui.js';

export const actions = [
  'chat.send',
  'player.kick',
  'command.execute',
  'inventory.read',
  'tps.read',
  'permission.check',
  'placeholder.resolve'
];

export default collectionPage({
  section: 'nodes',
  title: '服务器节点',
  singular: '节点',
  hint: '同一网络必须有且仅有一个会话权威节点，网络内身份模式一致。每个节点使用独立的凭据引用。',
  headers: ['节点', '网络 / 服务器', '身份模式', '当前连接'],
  cells: (n, context) => [
    n.id,
    n.network_id + ' / ' + n.server_id,
    n.identity_mode,
    badge(
      context.state.connections.nodes.find(item => item.id === n.id)?.connected ? '在线' : '未连接',
      context.state.connections.nodes.find(item => item.id === n.id)?.connected ? 'success' : ''
    )
  ],
  create: () => ({
    id: '',
    network_id: '',
    server_id: '',
    token: '',
    identity_mode: 'offline',
    session_authority: true,
    login_gate: true,
    actions: ['chat.send','player.kick','inventory.read','tps.read']
  }),
  fields: (n, config, persisted) => [
    grid([
      field('节点 ID', n.id, v => n.id = v, { readonly: persisted, placeholder: 'lobby-node' }),
      field('网络 ID', n.network_id, v => n.network_id = v, { placeholder: 'main' }),
      field('服务器 ID', n.server_id, v => n.server_id = v, { placeholder: 'lobby' }),
      field('凭据引用', n.token, v => n.token = v, { placeholder: 'file:node-lobby.token' }),
      select(
        '身份模式',
        n.identity_mode,
        [
          { id: 'online', label: '正版 · online' },
          { id: 'forwarded', label: '代理转发 · forwarded' },
          { id: 'offline', label: '离线 · offline' }
        ],
        v => n.identity_mode = v
      ),
    ]),
    check('作为网络玩家会话的权威来源', n.session_authority, v => n.session_authority = v),
    check('启用登录准入检查', n.login_gate, v => n.login_gate = v),
    choices('允许的管理动作', n.actions, actions.map(id => ({ id })), v => n.actions = v)
  ],
  extra: context => section(
    '正在运行的节点',
    [
      table(
        ['节点', '平台', '连接状态', '游戏端声明的能力'],
        context.state.connections.nodes.map(n => [n.id, n.platform || '—', n.connected ? '在线' : '离线', n.capabilities.join(' / ') || '—'])
      )
    ]
  )
});
