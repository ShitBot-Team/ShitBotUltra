import { reactive } from '../../shared/vue.js';
import { definePage, usePage } from '../../composables/page.js';
import { actionsAPI } from './api.js';
import { section, help, select, field, area, grid, pre } from '../../components/ui.js';

const definitions = [
  {id:'chat.send',label:'发送游戏消息',effect:true},
  {id:'player.kick',label:'踢出玩家',effect:true},
  {id:'command.execute',label:'执行游戏控制台命令',effect:true},
  {id:'inventory.read',label:'读取背包快照'},
  {id:'tps.read',label:'读取游戏性能'},
  {id:'permission.check',label:'查询权限'},
  {id:'placeholder.resolve',label:'解析占位符'}
];

export default definePage(context => {
  const view=usePage(context), api=actionsAPI(view.signal);
  const s=reactive({node:'',type:'',text:'',name:'',uuid:'',reason:'管理员操作',payload:'{}',result:null});
  const node=()=>context.state.connections.nodes.find(n=>n.id===s.node);
  const available=()=>definitions.filter(d=>node()?.connected&&node().capabilities.includes(d.id)&&node().allowed_actions?.includes(d.id));
  async function load() {
    context.state.connections=await api.connections();
  }
  function body() {
    if(s.type==='chat.send') {
      if(!s.text.trim())throw new Error('请输入消息内容');
      return {
        text:s.text,
        origin_id:'webui:'+(crypto.randomUUID?.() || Date.now().toString(36)+'-'+Math.random().toString(36).slice(2))
      };
    }
    if(s.type==='command.execute') {
      if(!s.text.trim())throw new Error('请输入游戏命令');
      return {command:s.text,actor_id:'admin:webui'};
    }
    if(s.type==='tps.read')return {};
    if(s.type==='player.kick'||s.type==='inventory.read') {
      const network=context.state.networks.find(n=>n.id===node()?.network_id);
      if(!s.name.trim()||!network)throw new Error('请输入玩家名并选择有效节点');
      const player={name:s.name.trim(),identity_mode:network.identity_mode};
      if(s.uuid.trim())player.uuid=s.uuid.trim();
      if(network.identity_mode!=='offline'&&!player.uuid)throw new Error('正版 / 转发身份需要玩家 UUID');
      return s.type==='player.kick'?{player,reason:s.reason}:{player};
    }
    const data=JSON.parse(s.payload);
    if(!data||typeof data!=='object'||Array.isArray(data))throw new Error('参数必须是 JSON 对象');
    return data;
  }
  return {
    view,
    load,
    render:()=>[
      section(
        '发送节点动作',
        [
          help('仅显示节点已声明且当前运行配置允许的动作。保存但未重启的权限更改不会提前生效。'),
          grid([
            select('在线节点',s.node,context.state.connections.nodes.filter(n=>n.connected),v=>{
              s.node=v;
              s.type='';
              s.result=null;
            }),
            select('动作',s.type,available(),v=>{
              s.type=v;
              s.text='';
              s.result=null;
            })
          ]),
          ['chat.send','command.execute'].includes(s.type)?area(s.type==='chat.send'?'消息内容':'游戏控制台命令',s.text,v=>s.text=v,4):null,
          ['player.kick','inventory.read'].includes(s.type)?grid([
            field('玩家名',s.name,v=>s.name=v),
            field('玩家 UUID',s.uuid,v=>s.uuid=v),
            s.type==='player.kick'?field('踢出原因',s.reason,v=>s.reason=v):null
          ]):null,
          ['permission.check','placeholder.resolve'].includes(s.type)?section('平台参数',[help('参数字段由对应游戏平台 / MC API 扩展约定，请按该实现填写。'),area('参数 JSON',s.payload,v=>s.payload=v,8)]):null,
          view.button(
            '发送动作',
            async()=>{
              const definition=available().find(d=>d.id===s.type);
              if(!definition)throw new Error('节点已断线或不再允许此动作，请刷新状态');
              const payload=body();
              if(definition.effect&&!confirm('向 '+s.node+' 执行“'+definition.label+'”？\n'+JSON.stringify(payload)))return;
              s.result=null;
              s.result=await api.execute(s.node,s.type,payload);
              view.notify('节点已返回结果。');
            },
            {primary:true,disabled:!available().some(d=>d.id===s.type)}
          ),
          help('命令只发送到游戏控制台。请求失败不会自动重发；结果未知时先到服务器确认。')
        ]
      ),
      s.result!==null?section('动作结果',[pre(s.result)]):null
    ]
  };
});
