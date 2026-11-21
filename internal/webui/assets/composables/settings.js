import { reactive } from '../shared/vue.js';
import { clone, json, downloadJSON } from '../shared/format.js';
import { configurationAPI } from '../features/config/api.js';
import { row, help, badge } from '../components/ui.js';

// Each editor retains the exact configuration revision it loaded. Another tab
// saving any section invalidates it; drafts are never silently rebased.

export function useSettings(context, view, section) {
  const api = configurationAPI(view.signal);
  const state = reactive({ config: {}, draft: null, original: '', revision: '', selected: -1, persisted: [] });
  const collection = ['nodes', 'connections', 'routes', 'commands'].includes(section);
  function receive(result) {
    state.config = result.config;
    state.revision = result.revision;
    state.draft = clone(result.config[section] ?? (collection ? [] : {}));
    state.original = json(state.draft);
    state.persisted = collection ? state.draft.map(() => true) : [];
    context.state.restartRequired = result.restart_required;
  }
  const dirty = () => state.draft !== null && json(state.draft) !== state.original;
  async function load() {
    if (dirty() && !confirm('重新读取将丢弃此页面的配置草稿。继续？')) return false;
    const result = await api.settings();
    receive(result);
    state.selected = -1;
    return true;
  }
  async function save() {
    const result = await api.saveSections({ [section]: clone(state.draft) }, state.revision);
    receive(result);
    view.notify(result.restart_required ? '已保存并备份，等待宿主重启后生效。' : '已保存，配置与当前运行配置一致。');
  }
  function toolbar() {
    return row([
      view.button('保存此分区', save, { primary: true, disabled: !state.revision || !dirty() }),
      view.button('重新读取', load),
      view.button('导出草稿', () => downloadJSON(section + '-draft.json', state.draft), { disabled: state.draft === null }),
      dirty() ? badge('有未保存的修改', 'warning') : badge('已与文件同步'),
      help('保存时校验引用关系并备份；运行中的连接保持到重启。')
    ]);
  }
  return { state, dirty, load, save, toolbar };
}
