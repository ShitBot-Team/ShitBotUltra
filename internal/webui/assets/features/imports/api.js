import { scopedClient } from '../../api/client.js';
import { esc } from '../../shared/format.js';

export function featureAPI(signal) {
  const call = scopedClient(signal);
  return {
    sources: () => call('imports/sources'),
    upload: file => call('imports/sources',{method:'POST',upload:file}),
    remove: id => call('imports/sources/'+esc(id),{method:'DELETE'}),
    preview: body => call('imports/previews',{method:'POST',body}),
    report: id => call('imports/'+esc(id)),
    commit: id => call('imports/'+esc(id)+'/commit',{method:'POST'}),
    restore: id => call('imports/'+esc(id)+'/restore',{method:'POST'}),
  };
}
