import { scopedClient } from '../../api/client.js';
import { esc } from '../../shared/format.js';

export function featureAPI(signal) {
  const call = scopedClient(signal);
  return {
    list: () => call('plugins'),
    get: id => call('plugins/'+esc(id)),
    import: file => call('plugins',{method:'POST',upload:file}),
    enable: (id,revision,body) => call('plugins/'+esc(id)+'/enable',{method:'POST',revision,body}),
    disable: (id,revision) => call('plugins/'+esc(id)+'/disable',{method:'POST',revision}),
    remove: (id,revision) => call('plugins/'+esc(id),{method:'DELETE',revision}),
    provider: (id,name,body) => call('plugins/'+esc(id)+'/providers/'+esc(name),{method:'POST',body}),
  };
}
