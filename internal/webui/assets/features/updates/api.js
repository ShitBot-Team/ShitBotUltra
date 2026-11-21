import { scopedClient } from '../../api/client.js';
import { esc } from '../../shared/format.js';

export function featureAPI(signal) {
  const call = scopedClient(signal);
  return {
    status: () => call('updates'),
    check: () => call('updates/check',{method:'POST'}),
    install: (version,sha256) => call('updates/install',{method:'POST',body:{version,sha256}}),
  };
}
