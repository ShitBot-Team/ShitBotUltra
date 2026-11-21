import { h, reactive } from '../../shared/vue.js';
import { definePage, usePage } from '../../composables/page.js';
import { coreAPI } from '../../api/core.js';
import { date } from '../../shared/format.js';
import { icon } from '../../components/icon.js';
import { section, table, help, badge, empty } from '../../components/ui.js';

export default definePage(context => {
  const view = usePage(context);
  const api = coreAPI(view.signal);
  const s = reactive({ summary: null });
  async function load() {
    const [summary] = await Promise.all([api.dashboard(),context.refresh()]);
    s.summary = summary;
  }
  const jump = (id,title,detail,kind) => h('button', { class:'shortcut',type:'button',onClick:()=>context.navigate(id) }, [icon(kind),h('span',[h('strong',title),h('small',detail)]),h('span','↗')]);
  return {
    view,
    load,
    render() {
      const data = s.summary;
      const live = context.state;
      const metrics = data ? [
        ['已知玩家',data.counts.players,'当前运行网络内的玩家记录','users'],
        ['在线玩家',data.counts.online,'来自权威节点的会话','activity'],
        ['已绑定账号',data.counts.bound,'已关联社交身份的玩家','link'],
        [
          '在线服务器',
          live.connections.nodes.filter(n=>n.connected).length+' / '+live.connections.nodes.length,
          '已连接节点 / 配置节点',
          'server'
        ]
      ] : [];
      return [
        h(
          'section',
          {class:'overview-hero'},
          [
            h('div',[
              h('span',{class:'eyebrow'},'YOUR CORE, CONNECTED'),
              h('h2','一处管理，每个连接。'),
              help('从服务器到群聊，把日常管理安排得井井有条。')
            ]),
            badge(live.status.state || '—','success')
          ]
        ),
        data ? h(
          'div',
          {class:'metrics'},
          metrics.map(([label,value,detail,kind])=>h('article',{class:'metric'},[
            h('div',{class:'metric-label'},[h('span',label),icon(kind)]),
            h('strong',String(value)),
            h('small',detail)
          ]))
        ) : help('正在读取统计…'),
        h(
          'div',
          {class:'dashboard-grid'},
          [
            h(
              'div',
              [
                section(
                  '游戏网络',
                  data?.networks.length ? [
                    table(
                      ['网络','玩家总数','在线','已绑定','无账号白名单','节点'],
                      data.networks.map(n=>[
                        n.id,
                        n.counts.players,
                        n.counts.online,
                        n.counts.bound,
                        n.counts.whitelist_only,
                        n.connected+' / '+n.nodes
                      ])
                    ),
                    help('玩家按“网络 + 玩家名”计数；跨网络不合并。无账号白名单独立统计，未加入名单的历史玩家为 '+data.counts.unlisted+'。'),
                    help('玩家数据快照：'+date(data.generated_at))
                  ] : [
                    empty('还没有游戏网络','添加服务器节点并配置网络，即可开始连接 Minecraft。'),
                    jump('nodes','添加服务器','设置网络与凭据','server')
                  ]
                ),
                section(
                  '连接状态',
                  [
                    table(['Minecraft 节点','平台','状态'],live.connections.nodes.map(n=>[n.id,n.platform||'—',badge(n.connected?'在线':'离线',n.connected?'success':'')])),
                    table(['机器人','类型','状态'],live.connections.adapters.map(a=>[a.id,a.type,badge(a.state,a.state==='connected'?'success':'warning')])),
                    data ? help('已配置适配器 '+data.adapters+' 个，当前运行 '+data.enabled_adapters+' 个。') : null
                  ]
                )
              ]
            ),
            h(
              'div',
              [
                section(
                  '快捷入口',
                  [
                    jump('adapters','连接机器人','OneBot 与宿主扩展适配器','plug'),
                    jump('routes','管理消息路由','设置群服转发与通知','route'),
                    jump('commands','自定义命令','模板、角色与冷却','terminal'),
                    jump('plugins','本地扩展','导入并授权插件','box')
                  ]
                ),
                section(
                  '实例信息',
                  [
                    h(
                      'dl',
                      {class:'detail-list'},
                      [
                        h('dt','Core 版本'),
                        h('dd',live.status.version||'—'),
                        h('dt','启动时间'),
                        h('dd',date(live.status.started_at)),
                        h('dt','渲染服务'),
                        h('dd',live.rendering.enabled?'已配置':'未配置'),
                        h('dt','消息路由'),
                        h('dd',data?.routes??'—'),
                        h('dt','自定义命令'),
                        h('dd',data?.commands??'—')
                      ]
                    ),
                    help('实例 ID：'+(live.status.instance_id||'—'))
                  ]
                ),
                section(
                  '最近动态',
                  [
                    ...(live.events.slice(0,4).map(event=>h('div',{class:'event-item'},[h('strong',event.type),h('small',date(event.time))]))),
                    live.events.length ? jump('events','查看全部事件','最多保留当前会话 100 条','activity') : help('收到 Core 事件后会显示在这里。')
                  ]
                )
              ]
            )
          ]
        )
      ];
    }
  };
});
