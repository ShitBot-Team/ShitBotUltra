import { h, reactive, shallowRef, onMounted, onBeforeUnmount, nextTick } from '../shared/vue.js';
import { groups, routes, findRoute } from './routes.js';
import { icon } from '../components/icon.js';
import { badge, help } from '../components/ui.js';

export default {
  props: ['context'],
  setup(props) {
    const ctx = props.context;
    const state = reactive({
      route: findRoute('overview'),
      search: '',
      collapsed: {},
      mobile: false,
      error: '',
      loading: false,
      theme: 'dark'
    });
    const page = shallowRef(null);
    let navigation = 0;
    let disposed = false;
    try {
      state.theme = localStorage.getItem('sbu.theme') === 'light' ? 'light' : 'dark';
    } catch { /* storage may be disabled */ }
    document.documentElement.dataset.theme = state.theme;
    function theme() {
      state.theme = state.theme === 'dark' ? 'light' : 'dark';
      document.documentElement.dataset.theme = state.theme;
      try {
        localStorage.setItem('sbu.theme', state.theme);
      } catch { /* theme remains session-only */ }
    }
    async function navigate(id, fromHash = false) {
      const route = findRoute(id);
      if (!route) {
        state.error = '找不到该页面，请从左侧导航选择功能。';
        history.replaceState(null, '', '#/' + state.route.id);
        return;
      }
      if (page.value && route.id === state.route.id) {
        navigation++;
        state.loading = false;
        state.mobile = false;
        if (!fromHash) history.pushState(null, '', '#/' + id);
        return;
      }
      if (ctx.isDirty() && !confirm('当前页面有未保存的修改或正在处理的操作。离开页面？')) {
        if (fromHash) history.replaceState(null, '', '#/' + state.route.id);
        return;
      }
      const ticket = ++navigation;
      state.loading = true;
      state.error = '';
      try {
        const module = await route.load();
        if (ticket !== navigation || disposed) return;
        page.value = module.default;
        state.route = route;
        state.mobile = false;
        if (!fromHash || location.hash !== '#/' + id) history.pushState(null, '', '#/' + id);
        document.title = route.title + ' · ShitBotUltra';
        await nextTick();
        document.getElementById('page-title')?.focus();
      } catch (error) {
        if (ticket === navigation && !disposed) {
          state.error = '页面加载失败：' + error.message;
          history.replaceState(null, '', '#/' + state.route.id);
        }
      } finally {
        if (ticket === navigation && !disposed) state.loading = false;
      }
    }
    const hash = () => navigate(location.hash.replace(/^#\/?/, '') || 'overview', true);
    const leaving = event => {
      if (ctx.isDirty()) {
        event.preventDefault();
        event.returnValue = '';
      }
    };
    onMounted(() => {
      ctx.setNavigate(navigate);
      hash();
      window.addEventListener('hashchange', hash);
      window.addEventListener('beforeunload', leaving);
    });
    onBeforeUnmount(() => {
      disposed = true;
      navigation++;
      window.removeEventListener('hashchange', hash);
      window.removeEventListener('beforeunload', leaving);
      ctx.setNavigate(() => {});
    });
    const logout = () => {
      if (!ctx.isDirty() || confirm('退出将丢弃未保存的页面草稿。继续？')) ctx.logout();
    };
    function navigationGroups() {
      const query = state.search.trim().toLowerCase();
      return groups.map(group => {
        const items = routes.filter(route => route.group === group.id && (!query || (route.title + route.description + route.id).toLowerCase().includes(query)));
        if (!items.length) return null;
        return h(
          'div',
          { class: 'nav-group', key: group.id },
          [
            h(
              'button',
              {
                class: 'nav-group-label',
                'aria-expanded': query ? true : !state.collapsed[group.id],
                onClick: () => state.collapsed[group.id] = !state.collapsed[group.id]
              },
              [group.label, h('span', state.collapsed[group.id] && !query ? '+' : '−')]
            ),
            !state.collapsed[group.id] || query ? items.map(route => h(
              'a',
              {
                key: route.id,
                href: '#/' + route.id,
                class: ['nav-item', state.route.id === route.id && 'active'],
                'aria-current': state.route.id === route.id ? 'page' : undefined,
                onClick: event => {
                  event.preventDefault();
                  navigate(route.id);
                }
              },
              [icon(route.icon), h('span', route.title)]
            )) : null
          ]
        );
      });
    }
    return () => h(
      'div',
      { class: ['shell', state.mobile && 'menu-open'] },
      [
        h('a', {
            href: '#main-content',
            class: 'skip-link',
            onClick: event => {
              event.preventDefault();
              document.getElementById('main-content')?.focus();
            }
          }, '跳到主要内容'),
        state.mobile ? h('button', { class: 'sidebar-backdrop', 'aria-label': '关闭导航', onClick: () => state.mobile = false }) : null,
        h(
          'aside',
          { class: 'sidebar', id: 'sidebar' },
          [
            h('div', { class: 'brand' }, [
              h('span', { class: 'brand-mark' }, 'S'),
              h('div', ['ShitBotUltra', h('small', 'CORE CONSOLE')])
            ]),
            h(
              'label',
              { class: 'nav-search' },
              [
                icon('search'),
                h('input', {
                  placeholder: '查找功能',
                  'aria-label': '搜索导航',
                  value: state.search,
                  onInput: event => state.search = event.target.value
                })
              ]
            ),
            h('nav', { 'aria-label': '主要导航' }, navigationGroups()),
            h(
              'div',
              { class: 'sidebar-bottom' },
              [
                badge(ctx.state.status.state || '运行中', 'success'),
                h('small', ctx.state.status.version || 'Core'),
                h('button', { onClick: logout }, '退出连接')
              ]
            )
          ]
        ),
        h(
          'div',
          { class: 'workspace' },
          [
            h(
              'header',
              { class: 'topbar' },
              [
                h(
                  'div',
                  { class: 'breadcrumb' },
                  [
                    h(
                      'button',
                      {
                        class: 'menu-toggle icon-button',
                        'aria-label': '切换导航',
                        'aria-controls': 'sidebar',
                        'aria-expanded': state.mobile,
                        onClick: () => state.mobile = !state.mobile
                      },
                      icon('menu')
                    ),
                    h('span', '控制台'),
                    h('span', '/'),
                    h('strong', state.route.title)
                  ]
                ),
                h(
                  'div',
                  { class: 'topbar-actions' },
                  [
                    badge(ctx.state.streamState, ctx.state.streamState === '已连接' ? 'success' : 'warning'),
                    h('button', { class: 'icon-button', title: '切换明暗主题', 'aria-label': '切换明暗主题', onClick: theme }, icon('sun')),
                    h('span', { class: 'avatar', title: '管理员' }, '管')
                  ]
                )
              ]
            ),
            h(
              'main',
              { id: 'main-content', tabindex: -1 },
              [
                h(
                  'div',
                  { class: 'page-heading' },
                  [
                    h('div', [
                      h('h1', { id: 'page-title', tabindex: -1 }, state.route.title),
                      help(state.route.description)
                    ]),
                    h('button', { disabled: state.loading, onClick: () => ctx.refreshPage() }, '刷新页面')
                  ]
                ),
                ctx.state.restartRequired ? h('p', { class: 'notice warning' }, '配置已保存，等待宿主重启后生效。连接状态与业务操作仍使用当前运行配置。') : null,
                state.error ? h('p', { class: 'notice error', role: 'alert' }, state.error) : null,
                state.loading ? h('p', { class: 'loading', role: 'status' }, '正在加载页面…') : null,
                page.value ? h('div', { inert: state.loading || undefined }, [h(page.value, { context: ctx, key: state.route.id })]) : null,
                h(
                  'footer',
                  { class: 'app-footer' },
                  [
                    h('span', 'ShitBot-Team and contributors'),
                    h('a', { href: '#/about', onClick: event => {
                        event.preventDefault();
                        navigate('about');
                      } }, 'ShitBotUltra · AGPL-3.0-only')
                  ]
                )
              ]
            )
          ]
        )
      ]
    );
  }
};
