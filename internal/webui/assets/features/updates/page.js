import { h, reactive } from '../../shared/vue.js';
import { date, safeLink } from '../../shared/format.js';
import { help, row, section, pre, check } from '../../components/ui.js';
import { definePage, usePage } from '../../composables/page.js';
import { featureAPI } from './api.js';

export default definePage(context => {
  const view = usePage(context);
  const { button, notify } = view;
  const api = featureAPI(view.signal);
  const network = view.network;
  const s = reactive({
    get updates() {
      return context.state.updates;
    },
    set updates(value) {
      context.state.updates = value;
    },
  });
  async function load() {
    s.updates = await api.status();
  }
  function updatesPanel() {
    const state=s.updates;
    const release=state.release;
    const locked=['checking','downloading','handing_off','handed_off'].includes(state.phase);
    return [
      section(
        '宿主版本更新',
        [
          help(state.enabled?'仓库：'+state.repository+' · 当前宿主版本：'+state.current_version:'当前宿主没有注册更新功能。请由 CLI / Desktop 按其安装方式管理版本。'),
          help('阶段：'+(state.phase||'disabled')+' · 最近检查：'+date(state.checked_at)),
          state.last_error?help('最后错误：'+state.last_error):null,
          state.backup?help('数据库备份：'+state.backup):null,
          row([
            button(
              '检查更新',
              async()=>{
                try {
                  s.updates=await api.check();
                }
                finally {
                  s.updates=await api.status();
                }
              },
              {disabled:!state.enabled||locked}
            ),
            button('刷新任务状态',async()=>{
              s.updates=await api.status();
            })
          ]),
          release?h(
            'div',
            [
              h('h3','候选版本 '+release.version),
              help(release.available?'有较新的宿主版本。':'当前版本无需升级。'),
              h('p',[
                h('a',{href:safeLink(release.url),target:'_blank',rel:'noopener noreferrer'},'查看 Release 与人工下载入口')
              ]),
              help('资产：'+release.asset_name+' · '+(release.size?Math.ceil(release.size/1024/1024)+' MiB':'无法确定')),
              pre('SHA-256: '+(release.sha256||'缺少可用摘要，禁止自动安装')),
              pre(release.notes||'此版本未提供更新说明。'),
              button(
                '下载、校验并交给宿主升级',
                async()=>{
                  if(!confirm('升级到 '+release.version+'？Core 将先校验并备份，宿主接管后可能退出或重启。'))return;
                  s.updates=await api.install(release.version,release.sha256);
                  notify('更新任务已受理，请观察任务状态。');
                },
                {primary:true,disabled:!state.can_install||!release.available||!release.sha256||locked}
              )
            ]
          ):null,
          state.phase==='handed_off'?help('安装资产已交给宿主，尚未确认新版本运行。请等待宿主操作完成后重新连接。'):null,
          help('自动检查不会自动安装；程序替换、协议兼容检查与失败回退由拥有当前进程的宿主执行。')
        ]
      )
    ];
  }
  return { view, load: load, render: updatesPanel, dirty: () => false };
});
