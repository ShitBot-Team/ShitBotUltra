import { h } from '../../shared/vue.js';
import { definePage, usePage } from '../../composables/page.js';
import { section, help, table } from '../../components/ui.js';

const link = (name,path) => h('a',{
  href:'https://github.com/ShitBot-Team/ShitBotUltra/'+path,
  target:'_blank',
  rel:'noopener noreferrer'
},name);

export default definePage(context => ({
  view: usePage(context),
  render: () => [
    section(
      'ShitBotUltra',
      [
        h('p','Copyright (C) 2026 ShitBot-Team and contributors.'),
        help('Core 管理控制台 · '+(context.state.status.version||'版本由宿主提供')),
        h('p',[
          link('项目源码',''),
          ' · ',
          link('AGPL-3.0-only 许可','blob/main/LICENSE'),
          ' · ',
          link('第三方声明','blob/main/THIRD_PARTY_NOTICES.md')
        ]),
        help('若你使用修改后的发行版，请向发行者获取该版本对应的源码。')
      ]
    ),
    section(
      '第三方致谢',
      [
        table(
          ['组件','作者 / 项目','许可'],
          [
            ['Vue 3 runtime','Evan You 与 Vue contributors','MIT'],
            [
              'baizhou.online / profile / performance 模板',
              h('a',{
                href:'https://github.com/BaizhouziYou/easybot-image-templates',
                target:'_blank',
                rel:'noopener noreferrer'
              },'BaiZhouziYou / EasyBot Image Templates'),
              'AGPL-3.0-only；附加说明随模板保留'
            ]
          ]
        ),
        help('控制台界面由 ShitBotUltra 自行实现。各图片模板的来源、许可与署名可在模板详情中查看。')
      ]
    )
  ]
}));
