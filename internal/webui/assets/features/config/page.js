import { reactive, h } from '../../shared/vue.js';
import { definePage, usePage } from '../../composables/page.js';
import { section, help, area, row, grid, field, pre } from '../../components/ui.js';
import { configurationAPI } from './api.js';
import { coreAPI } from '../../api/core.js';

export default definePage(context => {
  const view = usePage(context);
  const api = configurationAPI(view.signal);
  const core = coreAPI(view.signal);
  const s = reactive({ text: '', original: '', revision: '', secretName: '', secretValue: '', secret: null });
  const dirty = () => s.text !== s.original;
  async function load() {
    if (dirty() && !confirm('重新读取将丢弃当前配置草稿。继续？')) return;
    const doc = await api.document();
    s.text = s.original = doc.text;
    s.revision = doc.revision;
  }
  return {
    view,
    load,
    dirty,
    render: () => [
      section(
        '配置原文',
        [
          help('与各功能表单共用 config.yaml。保存前校验并备份，文件版本冲突时保留当前草稿。'),
          area('config.yaml', s.text, v => s.text = v, 22),
          row([
            view.button('检查配置', async () => {
              await api.validate(s.text);
              view.notify('配置规则校验通过，尚未保存。');
            }),
            view.button(
              '保存配置',
              async () => {
                const result = await api.save(s.text,s.revision);
                s.text = s.original = result.document.text;
                s.revision = result.document.revision;
                context.state.restartRequired = result.restart_required;
                view.notify('配置已保存并备份。');
              },
              { primary: true, disabled: !s.revision || !dirty() }
            ),
            view.button('重新读取', load)
          ])
        ]
      ),
      section(
        '创建连接凭据',
        [
          help('配置只保存 file: 或 env: 引用。已存在的凭据文件不会被覆盖。'),
          grid([
            field('凭据文件名', s.secretName, v => s.secretName = v, { placeholder: 'node-lobby.token' }),
            field('凭据内容（留空生成）', s.secretValue, v => s.secretValue = v, { type: 'password' })
          ]),
          view.button('创建凭据', async () => {
            s.secret = await api.secret(s.secretName,s.secretValue);
            s.secretValue = '';
          }),
          s.secret ? h('div', [
            help('配置引用：' + s.secret.reference),
            pre(s.secret.value),
            view.button('隐藏凭据', () => s.secret = null)
          ]) : null
        ]
      ),
      section(
        '页面服务',
        [
          help('本次会话的页面开关不改变管理 API；下次启动的默认值由配置中的 webui 控制。'),
          view.button(
            context.state.webui.enabled ? '关闭页面服务' : '开启页面服务',
            async () => {
              const enabled = !context.state.webui.enabled;
              if (!enabled && !confirm('关闭后刷新页面将无法打开控制台，需要通过管理 API 或重启宿主恢复。继续？')) return;
              context.state.webui = await core.setWebUI(enabled);
              view.notify('本次会话的页面开关已更新。');
            }
          )
        ]
      )
    ]
  };
});
