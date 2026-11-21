import { scopedClient } from '../../api/client.js';
import { esc } from '../../shared/format.js';

export function featureAPI(signal) {
  const call = scopedClient(signal);
  return {
    list: (network, online, offset) => call('players?network_id='+esc(network)+'&online='+online+'&limit=100&offset='+offset),
    inventory: (network, server, name) => call('inventory?network_id='+esc(network)+'&server_id='+esc(server)+'&player_name='+esc(name)),
  };
}
