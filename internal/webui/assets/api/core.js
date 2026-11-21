import { scopedClient } from './client.js';

export function coreAPI(signal) {
  const call = scopedClient(signal);
  return {
    status: () => call('status'),
    connections: () => call('connections'),
    networks: () => call('networks'),
    dashboard: () => call('dashboard'),
    webui: () => call('webui'),
    setWebUI: enabled => call('webui', { method: 'PUT', body: { enabled } }),
    rendering: () => call('rendering'),
    updates: () => call('updates'),
  };
}
