import { reactive } from '../../shared/vue.js';
import { date } from '../../shared/format.js';
import { help, row, grid, section, pre, field, select, check, table } from '../../components/ui.js';
import { definePage, usePage } from '../../composables/page.js';
import { featureAPI } from './api.js';

export default definePage(context => {
  const view = usePage(context);
  const { button, pager } = view;
  const perform = view.run;
  const api = featureAPI(view.signal);
  const network = view.network;
  const s = reactive({
    get network() {
      return context.state.network;
    },
    players:[],
    playerOffset:0,
    onlineOnly:true,
    inventoryName:'',
    inventoryServer:'',
    inventory:null,
  });
  async function loadPlayers() {
    s.players = [];
    if (s.network) s.players = (await api.list(s.network,s.onlineOnly,s.playerOffset)).items;
  }
  const networkPicker = () => view.networkPicker(async () => {
    s.playerOffset = 0;
    s.inventory = null;
    s.inventoryServer = '';
    await loadPlayers();
  });
  function playersPanel() {
    return [
      section(
        '玩家资料',
        [
          row([
            networkPicker(),
            check('只看在线玩家',s.onlineOnly,value=>{
              s.onlineOnly=value;
              s.playerOffset=0;
              perform(loadPlayers);
            })
          ]),
          table(['玩家','状态','累计在线','最后记录','节点'],s.players.map(p=>[
              p.player.name,
              p.online?'在线':'离线',
              Math.floor(p.total_seconds/60)+' 分钟',
              date(p.last_seen_at),
              p.node_id
          ])),
          pager(s.playerOffset,s.players.length,async offset=>{
            s.playerOffset=offset;
            await loadPlayers();
          })
        ]
      ),
      section(
        '背包快照',
        [
          grid([
            field('玩家名',s.inventoryName,v=>s.inventoryName=v),
            select('服务器',s.inventoryServer,(network()?.nodes||[]).map(n=>({value:n.server_id,label:n.server_id})),v=>s.inventoryServer=v)
          ]),
          button('读取快照',async()=>{
            s.inventory=null;
            s.inventory=await api.inventory(s.network,s.inventoryServer,s.inventoryName);
          }),
          s.inventory?pre(s.inventory):help('显示已保存的有效快照；游戏内最新采集通过节点 inventory.read 动作完成。')
        ]
      )
    ];
  }
  return { view, load: loadPlayers, render: playersPanel, dirty: () => false };
});
