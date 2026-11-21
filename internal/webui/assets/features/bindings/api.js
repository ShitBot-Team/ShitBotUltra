import { scopedClient } from '../../api/client.js';
import { esc } from '../../shared/format.js';

export function featureAPI(signal) {
  const call = scopedClient(signal);
  return {
    list: (network, offset) => call('bindings?network_id='+esc(network)+'&limit=100&offset='+offset),
    add: body => call('bindings',{method:'POST',body}),
    remove: (network, name) => call('bindings/'+esc(network)+'/'+esc(name),{method:'DELETE'}),
  };
}
