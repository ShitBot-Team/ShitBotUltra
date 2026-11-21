import { collectionPage } from '../../components/collection-editor.js';
import { field, grid, check, badge, section, table } from '../../components/ui.js';

export default collectionPage({
  section: 'connections',
  title: '机器人连接配置',
  singular: '连接',
  hint: '内置 onebot.v11 正向 WebSocket；其他类型需要宿主注册相应适配器。凭据请先在“配置与凭据”创建。',
  headers: ['连接', '类型', '开关', '当前状态'],
  cells: (c, context) => [
    c.id,
    c.type,
    badge(c.enabled ? '启用' : '停用', c.enabled ? 'success' : ''),
    context.state.connections.adapters.find(a => a.id === c.id)?.state || '未运行'
  ],
  create: () => ({
    id: '',
    type: 'onebot.v11',
    enabled: false,
    endpoint: 'ws://127.0.0.1:3001',
    token: '',
    account_id: '',
    allow_insecure_remote: false
  }),
  fields: (c, config, persisted) => [
    grid([
      field('连接 ID', c.id, v => c.id = v, { readonly: persisted }),
      field('适配器类型', c.type, v => c.type = v),
      field('连接地址', c.endpoint, v => c.endpoint = v),
      field('机器人账号 ID', c.account_id, v => c.account_id = v),
      field('凭据引用', c.token, v => c.token = v, { placeholder: 'file:bot.token' })
    ]),
    check('启用此连接', c.enabled, v => c.enabled = v),
    check('允许远程明文连接', c.allow_insecure_remote, v => c.allow_insecure_remote = v)
  ],
  extra: context => section('正在运行的适配器', [
    table(['连接', '状态', '丢弃事件', '最后错误'], context.state.connections.adapters.map(a => [a.id, a.state, a.dropped, a.last_error || '—']))
  ])
});
