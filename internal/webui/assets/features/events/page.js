import { reactive } from '../../shared/vue.js';
import { definePage, usePage } from '../../composables/page.js';
import { section, table, field, check, row, help, pre } from '../../components/ui.js';
import { date, downloadJSON } from '../../shared/format.js';

export default definePage(context => {
  const view = usePage(context);
  const s = reactive({ filter:'', paused:false, frozen:[] });
  const items = () => (s.paused?s.frozen:context.state.events).filter(e=>(e.type+' '+JSON.stringify(e.data)).toLowerCase().includes(s.filter.toLowerCase()));
  return {
    view,
    render: () => [
      section(
        '管理事件',
        [
          help('最多保留当前页面会话的 100 条摘要。不是持久审计日志；刷新或退出后清空。'),
          row([
            field('过滤类型或摘要',s.filter,v=>s.filter=v),
            check('暂停显示',s.paused,v=>{
              s.frozen=[...context.state.events];
              s.paused=v;
            }),
            view.button('导出当前结果',()=>downloadJSON('core-events.json',items())),
            view.button('清空页面记录',()=>{
              context.state.events=[];
              s.frozen=[];
            })
          ]),
          table(['时间','类型','摘要'],items().map(e=>[date(e.time),e.type,pre(e.data)]))
        ]
      )
    ]
  };
});
