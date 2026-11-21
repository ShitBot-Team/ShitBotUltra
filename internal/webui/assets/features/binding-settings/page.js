import { definePage, usePage } from '../../composables/page.js';
import { useSettings } from '../../composables/settings.js';
import { section, help, field, grid, check } from '../../components/ui.js';

export default definePage(context => {
  const view = usePage(context);
  const settings = useSettings(context,view,'binding');
  return {
    view,
    load: settings.load,
    dirty: settings.dirty,
    render() {
      const b = settings.state.draft;
      return b ? [
        section(
          '绑定与准入规则',
          [
            settings.toolbar(),
            check('启用绑定检查', b.enabled, v => b.enabled = v),
            check('解绑后尝试踢出玩家', b.kick_on_unbind, v => b.kick_on_unbind = v),
            grid([
              field('验证码有效期（秒）', b.code_ttl_seconds, v => b.code_ttl_seconds = v, { type:'number',min:30,max:3600 }),
              field('时间窗口内最大尝试次数', b.max_attempts, v => b.max_attempts = v, { type:'number',min:1,max:100 }),
              field('尝试次数统计窗口（秒）', b.attempt_window_seconds, v => b.attempt_window_seconds = v, { type:'number',min:30,max:86400 }),
              field('每个身份最多绑定的玩家数', b.max_players_per_identity, v => b.max_players_per_identity = v, { type:'number',min:1,max:100 })
            ]),
            help('实际登录拦截还取决于节点的 login_gate 与游戏端实现。')
          ]
        )
      ] : [help('正在读取配置…')];
    }
  };
});
