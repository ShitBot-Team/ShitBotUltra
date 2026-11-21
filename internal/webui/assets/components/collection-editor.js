import { definePage, usePage } from '../composables/page.js';
import { useSettings } from '../composables/settings.js';
import { h } from '../shared/vue.js';
import { section, table, row, help, pre } from './ui.js';

// Shared mechanics only; each feature supplies its own fields and domain labels.

export function collectionPage(definition) {
  return definePage(context => {
    const view = usePage(context);
    const settings = useSettings(context, view, definition.section);
    const s = settings.state;
    return {
      view,
      load: settings.load,
      dirty: settings.dirty,
      render() {
        if (s.draft === null) return [help('正在读取配置…')];
        const item = s.draft[s.selected];
        const persisted = !!s.persisted[s.selected];
        return [
          section(
            definition.title,
            [
              settings.toolbar(),
              help(definition.hint),
              view.button('添加' + definition.singular, () => {
                s.draft.push(definition.create());
                s.persisted.push(false);
                s.selected = s.draft.length - 1;
              }),
              table(
                [...definition.headers, '操作'],
                s.draft.map((record, index) => [
                  ...definition.cells(record, context),
                  row([
                    view.button(s.selected === index ? '正在编辑' : '编辑', () => {
                      s.selected = index;
                    }),
                    view.button(
                      '移除',
                      () => {
                        if (!confirm('将此记录从草稿移除？保存后才会生效，仍被其他配置引用时会拒绝保存。')) return;
                        s.draft.splice(index, 1);
                        s.persisted.splice(index, 1);
                        s.selected = -1;
                      },
                      { danger: true }
                    )
                  ])
                ])
              )
            ]
          ),
          item ? section(
            '编辑' + definition.singular,
            [
              ...definition.fields(item, s.config, persisted),
              help('已有标识不可在此更名；如需更名及同步引用，请在配置原文中一次性修改。'),
              h('details', [h('summary', '查看当前记录 JSON'), pre(item)])
            ]
          ) : null,
          definition.extra?.(context, view)
        ];
      }
    };
  });
}
