import { scopedClient } from '../../api/client.js';
import { esc } from '../../shared/format.js';

export function featureAPI(signal) {
  const call = scopedClient(signal);
  const path = id => 'templates/'+esc(id);
  return {
    list: () => call('templates'),
    get: id => call(path(id)),
    files: (id,revision) => call(path(id)+'/files?revision='+esc(revision)),
    file: (id,revision,name) => call(path(id)+'/file?revision='+esc(revision)+'&path='+esc(name)),
    import: file => call('templates',{method:'POST',upload:file}),
    publish: (id,revision,version) => call(path(id)+'/publish',{method:'POST',revision,body:{version_revision:version}}),
    restore: (id,revision) => call(path(id)+'/restore',{method:'POST',revision}),
    draft: (id,revision,base,files) => call(path(id)+'/draft',{method:'PUT',revision,body:{base_revision:base,files}}),
    defaults: (id,revision,body) => call(path(id)+'/defaults',{method:'PUT',revision,body}),
    provider: (id,body) => call('rendering/providers/'+esc(id),{method:'POST',body}),
    render: body => call('renders',{method:'POST',body}),
    image: id => call('renders/resources/'+esc(id),{binary:true}),
  };
}
