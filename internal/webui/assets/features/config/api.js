import { scopedClient } from '../../api/client.js';

export function configurationAPI(signal) {
  const call = scopedClient(signal);
  return {
    document: () => call('config'),
    validate: text => call('config/validate', { method: 'POST', body: { text } }),
    save: (text, revision) => call('config', { method: 'PUT', revision, body: { text } }),
    secret: (name, value) => call('secrets', { method: 'POST', body: { name, value } }),
    settings: () => call('config/sections'),
    saveSections: (sections, revision) => call('config/sections', { method: 'PATCH', revision, body: sections }),
  };
}
