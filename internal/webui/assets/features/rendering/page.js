import { definePage, usePage } from '../../composables/page.js';
import { useSettings } from '../../composables/settings.js';
import { section, help, field, grid } from '../../components/ui.js';

export default definePage(context => {
  const view = usePage(context);
  const settings = useSettings(context,view,'rendering');
  return {
    view,
    load: settings.load,
    dirty: settings.dirty,
    render() {
      const r = settings.state.draft;
      return r ? [
        section(
          'Chromium 与渲染队列',
          [
            settings.toolbar(),
            grid([
              field('浏览器可执行文件路径',r.browser_path,v=>r.browser_path=v),
              field('浏览器精确版本',r.browser_version,v=>r.browser_version=v),
              field('并发任务数',r.concurrency,v=>r.concurrency=v,{type:'number',min:1,max:4}),
              field('等待队列长度',r.queue_size,v=>r.queue_size=v,{type:'number',min:1,max:64}),
              field('单个任务超时（秒）',r.timeout_seconds,v=>r.timeout_seconds=v,{type:'number',min:1,max:120})
            ]),
            help('浏览器路径与精确版本需要同时填写；两者留空时不启用图片渲染。')
          ]
        ),
        section(
          '默认模板',
          [
            grid([
              field('在线列表模板',r.online_template,v=>r.online_template=v),
              field('玩家资料模板',r.profile_template,v=>r.profile_template=v),
              field('背包模板',r.inventory_template,v=>r.inventory_template=v),
              field('性能模板',r.performance_template,v=>r.performance_template=v)
            ]),
            help('使用已导入并发布的模板 ID。模板草稿与预览在“图片模板”中管理。')
          ]
        )
      ] : [help('正在读取配置…')];
    }
  };
});
