import { reactive } from '../shared/vue.js';
import { setToken, onUnauthorized, request, sessionGeneration } from '../api/client.js';
import { coreAPI } from '../api/core.js';
import { stream } from '../api/events.js';

function initial() {
  return {
    authenticated: false,
    status: {},
    connections: { nodes: [], adapters: [] },
    networks: [],
    network: '',
    webui: { enabled: true },
    rendering: { enabled: false },
    updates: { enabled: false },
    events: [],
    streamState: '未连接',
    restartRequired: false,
    previewData: null
  };
}

export function createSession() {
  const state = reactive(initial());
  let stopStream = () => {};
  let hooks = null;
  let navigate = () => {};
  const api = coreAPI();
  async function refresh() {
    const epoch = sessionGeneration();
    const [status, connections, networks, webui, rendering, updates, settings] = await Promise.all([
      api.status(),
      api.connections(),
      api.networks(),
      api.webui(),
      api.rendering(),
      api.updates(),
      request('/api/v1/config/sections')
    ]);
    if (epoch !== sessionGeneration()) return;
    Object.assign(state, {
      status,
      connections,
      networks: networks.items,
      webui,
      rendering,
      updates,
      restartRequired: settings.restart_required
    });
    if (!state.networks.some(item => item.id === state.network)) state.network = state.networks[0]?.id || '';
  }
  function logout() {
    stopStream();
    setToken('');
    hooks = null;
    Object.assign(state, initial());
  }
  onUnauthorized(logout);
  async function login(credential) {
    setToken(credential.trim());
    const epoch = sessionGeneration();
    try {
      await refresh();
    }
    catch (error) {
      if (epoch === sessionGeneration()) logout();
      throw error;
    }
    if (epoch !== sessionGeneration()) return;
    state.authenticated = true;
    stopStream = stream(
      event => {
        state.events.unshift(event);
        state.events = state.events.slice(0, 100);
        if (event.type === 'connections') state.connections = event.data;
        if (event.type === 'update.changed') state.updates = event.data;
        if (event.type === 'webui.changed') state.webui = event.data;
        if (event.type === 'config.saved') state.restartRequired = !!event.data.restart_required;
        if (event.type === 'reset') refresh().catch(() => {
          state.streamState = '状态刷新失败，请手动刷新';
        });
      },
      value => {
        state.streamState = value;
      }
    );
  }
  return {
    state,
    login,
    logout,
    refresh,
    setHooks(value) {
      hooks = value;
    },
    clearHooks(value) {
      if (hooks === value) hooks = null;
    },
    isDirty: () => !!hooks?.dirty(),
    refreshPage: () => hooks?.refresh(),
    setNavigate(value) {
      navigate = value;
    },
    navigate: id => navigate(id)
  };
}
