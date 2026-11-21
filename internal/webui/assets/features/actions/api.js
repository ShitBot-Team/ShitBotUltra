import { scopedClient } from '../../api/client.js';
import { esc } from '../../shared/format.js';

export function actionsAPI(signal) {
  const call = scopedClient(signal);
  return {
    connections: () => call('connections'),
    execute: (node,type,payload) => call('nodes/'+esc(node)+'/actions',{method:'POST',body:{type,payload}})
  };
}
