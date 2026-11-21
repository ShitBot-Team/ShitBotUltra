import { collectionPage } from '../../components/collection-editor.js';
import { field, area, grid, select, check, help } from '../../components/ui.js';

export default collectionPage({
  section: 'commands',
  key: 'name',
  title: '群快捷命令',
  singular: '命令',
  hint: '命令模板中的 {0}、{1} 对应参数规则，{player} 需要开启绑定要求。目标节点必须允许 command.execute。',
  headers: ['名称', '目标节点', '模板', '角色', '冷却'],
  cells: c => [
    c.name,
    c.node_id,
    c.template,
    (c.roles || []).join(', ') || '所有角色',
    c.cooldown_seconds + ' 秒'
  ],
  create: () => ({
    name: '',
    node_id: '',
    template: '',
    arguments: [],
    roles: ['owner','admin'],
    require_binding: true,
    cooldown_seconds: 10
  }),
  fields: (c, config, persisted) => [
    grid([
      field('命令名', c.name, v => c.name = v, { readonly: persisted }),
      select('目标节点', c.node_id, (config.nodes || []).filter(n => n.actions?.includes('command.execute')), v => c.node_id = v),
      field('命令模板', c.template, v => c.template = v, { placeholder: 'say {player} {0}' }),
      field('冷却秒数', c.cooldown_seconds, v => c.cooldown_seconds = v, { type: 'number', min: 1, max: 86400 }),
      area('参数正则（每行一条，按参数顺序）', (c.arguments || []).join('\n'), v => c.arguments = v ? v.split('\n') : [], 5),
      field('角色（逗号分隔，留空允许所有角色）', (c.roles || []).join(', '), v => c.roles = v.split(',').map(x => x.trim()).filter(Boolean))
    ]),
    check('要求发送者绑定玩家', c.require_binding, v => c.require_binding = v),
    help('bind、unbind、bindings、online、help、profile、inventory、tps 为内置命令，不可重名。')
  ]
});
