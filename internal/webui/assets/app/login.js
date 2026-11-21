import { h, reactive } from '../shared/vue.js';
import { field, help } from '../components/ui.js';

export default {
  props: ['context'],
  setup(props) {
    const form = reactive({ credential: '', busy: false, error: '' });
    async function login(event) {
      event.preventDefault();
      if (form.busy || !form.credential.trim()) return;
      form.busy = true;
      form.error = '';
      try {
        await props.context.login(form.credential);
      }
      catch (error) {
        form.error = error.name === 'AbortError' ? '连接超时，请检查 Core 地址。' : error.message;
      }
      finally {
        form.credential = '';
        form.busy = false;
      }
    }
    return () => h(
      'main',
      { class: 'login-layout' },
      [
        h(
          'div',
          { class: 'login-story' },
          [
            h('div', { class: 'brand' }, [h('span', { class: 'brand-mark' }, 'S'), 'ShitBotUltra']),
            h('span', { class: 'eyebrow' }, 'CORE CONSOLE'),
            h('h1', ['让连接有序，', h('br'), '让管理简单。']),
            help('服务器、机器人、玩家与扩展，汇聚在一个属于你的控制台。'),
            h('div', { class: 'login-topology', 'aria-hidden': 'true' }, [h('span', 'Minecraft'), h('strong', 'ULTRA'), h('span', '机器人 · 扩展')])
          ]
        ),
        h(
          'form',
          { class: 'login-card', onSubmit: login },
          [
            h('span', { class: 'eyebrow' }, '欢迎回来'),
            h('h2', '连接你的 Core'),
            help('输入管理凭据以进入控制台。'),
            h(
              'fieldset',
              { class: 'page-fields', disabled: form.busy },
              [
                field('管理 Token', form.credential, value => form.credential = value, { type: 'password', placeholder: '粘贴管理凭据' }),
                h('button', { type: 'submit', class: 'primary', disabled: !form.credential.trim() || form.busy }, form.busy ? '正在连接…' : '进入控制台 →')
              ]
            ),
            form.error ? h('p', { class: 'notice error', role: 'alert' }, form.error) : null,
            h('div', { class: 'login-note' }, [help('默认凭据文件：secrets/admin.token'), help('凭据仅保留在页面内存中，刷新后需重新登录。')]),
            h('footer', 'ShitBot-Team and contributors · AGPL-3.0-only')
          ]
        )
      ]
    );
  }
};
