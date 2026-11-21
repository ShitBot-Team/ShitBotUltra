import { reactive } from '../../shared/vue.js';
import { grid, section, field, table } from '../../components/ui.js';
import { definePage, usePage } from '../../composables/page.js';
import { featureAPI } from './api.js';

export default definePage(context => {
  const view = usePage(context);
  const { button, pager, notify } = view;
  const api = featureAPI(view.signal);
  const network = view.network;
  const s = reactive({
    get network() {
      return context.state.network;
    },
    bindings:[],
    bindingOffset:0,
    playerName:'',
    playerUUID:'',
    ownerPlatform:'qq.numeric',
    ownerScope:'global',
    ownerID:'',
  });
  async function loadBindings() {
    s.bindings = [];
    if (s.network) s.bindings = (await api.list(s.network,s.bindingOffset)).items;
  }
  const networkPicker = () => view.networkPicker(async () => {
    s.bindingOffset = 0;
    await loadBindings();
  });
  function bindingsPanel() {
    return [
      section(
        '绑定与白名单',
        [
          networkPicker(),
          table(
            ['玩家','UUID','账号身份','操作'],
            s.bindings.map(item=>[
              item.player.name,
              item.player.uuid||'—',
              item.owner?item.owner.platform+' / '+item.owner.scope+' / '+item.owner.id:'无账号白名单',
              button(
                '撤销',
                async()=>{
                  if(!confirm('撤销 '+item.player.name+' 的白名单 / 绑定？启用踢人规则时会尝试踢出玩家。'))return;
                  await api.remove(item.network_id,item.player.name);
                  await loadBindings();
                },
                {danger:true}
              )
            ])
          ),
          pager(s.bindingOffset,s.bindings.length,async offset=>{
            s.bindingOffset=offset;
            await loadBindings();
          })
        ]
      ),
      section(
        '添加记录',
        [
          grid([
            field('玩家名',s.playerName,v=>s.playerName=v),
            field('UUID（可选）',s.playerUUID,v=>s.playerUUID=v),
            field('平台',s.ownerPlatform,v=>s.ownerPlatform=v),
            field('作用域',s.ownerScope,v=>s.ownerScope=v),
            field('账号 ID（留空仅加入白名单）',s.ownerID,v=>s.ownerID=v)
          ]),
          button(
            '添加到当前网络',
            async()=>{
              if(!network())throw new Error('请先选择游戏网络');
              const body={
                network_id:s.network,
                player:{name:s.playerName,uuid:s.playerUUID,identity_mode:network().identity_mode}
              };
              if(s.ownerID)body.owner={platform:s.ownerPlatform,scope:s.ownerScope,id:s.ownerID};
              await api.add(body);
              s.playerName=s.playerUUID=s.ownerID='';
              await loadBindings();
              notify('记录已添加。');
            },
            {primary:true,disabled:!s.network}
          )
        ]
      )
    ];
  }
  return {
    view,
    load: loadBindings,
    render: bindingsPanel,
    dirty: () => !!(s.playerName || s.playerUUID || s.ownerID)
  };
});
