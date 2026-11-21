import { h, reactive } from '../../shared/vue.js';
import { json, safeLink } from '../../shared/format.js';
import { help, row, grid, section, pre, area, select, table } from '../../components/ui.js';
import { definePage, usePage } from '../../composables/page.js';
import { featureAPI } from './api.js';

export default definePage(context => {
  const view = usePage(context);
  const { button, upload, notify } = view;
  const api = featureAPI(view.signal);
  const s = reactive({
    get networks() {
      return context.state.networks;
    },
    plugins:[],
    plugin:null,
    pluginVersion:'',
    pluginConfig:'{}',
    pluginGrant:json({capabilities:[],networks:[],routes:[],nodes:[]}),
    pluginProvider:'',
    pluginRequest:'{}',
    pluginResult:null,
    original:'',
  });
  async function loadPlugins() {
    s.plugins = (await api.list()).items;
  }
  const draft = () => json([s.pluginVersion,s.pluginConfig,s.pluginGrant]);
  const dirty = () => !!s.plugin && draft() !== s.original;
  async function choosePlugin(id) {
    if(dirty()&&!confirm('重新读取或切换插件会丢弃未保存的配置与授权。继续？'))return;
    s.plugin=await api.get(id);
    s.pluginVersion=s.plugin.active_revision;
    s.pluginConfig=json(s.plugin.configuration||{});
    s.pluginGrant=json(s.plugin.grant?.capabilities?.length?s.plugin.grant:{capabilities:[],networks:[],routes:[],nodes:[]});
    s.pluginProvider='';
    s.pluginResult=null;
    s.original=draft();
  }
  function pluginsPanel() {
    const plugin=s.plugin;
    const version=plugin?.versions.find(item=>item.revision===s.pluginVersion);
    return [
      section(
        '插件目录',
        [
          help('导入后默认停用。启用时按下面明确填写的能力和范围授权；修改配置或版本前先停用。'),
          upload('导入插件 ZIP','.zip',false,async files=>{
            const value=await api.import(files[0]);
            await loadPlugins();
            await choosePlugin(value.id);
          }),
          table(['插件','运行状态','丢弃事件','最后错误','操作'],s.plugins.map(item=>[
            item.id,
            item.state,
            item.dropped,
            item.last_error||'—',
            button('管理',()=>choosePlugin(item.id))
          ]))
        ]
      ),
      plugin?section(
        '插件管理 · '+plugin.id,
        [
          select('包版本',s.pluginVersion,plugin.versions.map(v=>({value:v.revision,label:v.manifest.version+' · '+v.revision.slice(0,12)})),value=>s.pluginVersion=value),
          version?help(version.manifest.name+' · 作者：'+version.manifest.author+' · 许可：'+version.manifest.license):null,
          version?h('p',[
            h('a',{href:safeLink(version.manifest.project_url),target:'_blank',rel:'noopener noreferrer'},'插件项目来源')
          ]):null,
          help('插件在 Core 进程中运行，仅启用你信任的代码。配置、权限和版本通过最新状态版本提交。'),
          version?pre({
            declared_capabilities:version.manifest.capabilities,
            commands:version.manifest.commands||[],
            providers:version.manifest.providers||[]
          }):null,
          grid([
            area('插件配置 JSON',s.pluginConfig,value=>s.pluginConfig=value,10),
            area('授予权限与范围 JSON',s.pluginGrant,value=>s.pluginGrant=value,10)
          ]),
          help('可选网络、节点与路由：'),
          pre(s.networks),
          row([
            button(
              '按以上权限启用',
              async()=>{
                s.plugin=await api.enable(plugin.id,plugin.revision,{
                  version_revision:s.pluginVersion,
                  grant:JSON.parse(s.pluginGrant),
                  configuration:JSON.parse(s.pluginConfig)
                });
                s.original=draft();
                await loadPlugins();
                notify('插件已启用。');
              },
              {primary:true,disabled:plugin.state==='running'}
            ),
            button('停用',async()=>{
              s.plugin=await api.disable(plugin.id,plugin.revision);
              await loadPlugins();
              notify('插件已停用。');
            },{disabled:!plugin.enabled}),
            button(
              '卸载包',
              async()=>{
                if(!confirm('卸载 '+plugin.id+' 的插件包？插件数据和最后配置会保留。'))return;
                await api.remove(plugin.id,plugin.revision);
                s.plugin=null;
                await loadPlugins();
              },
              {danger:true,disabled:plugin.enabled}
            ),
            button('重新读取',()=>choosePlugin(plugin.id))
          ])
        ]
      ):null,
      plugin&&version?.manifest.providers?.length?section(
        '插件数据提供器',
        [
          select('提供器',s.pluginProvider,version.manifest.providers.map(id=>({id})),value=>s.pluginProvider=value),
          area('请求 JSON',s.pluginRequest,value=>s.pluginRequest=value),
          button(
            '读取数据',
            async()=>{
              s.pluginResult=await api.provider(plugin.id,s.pluginProvider,JSON.parse(s.pluginRequest));
            },
            {disabled:!s.pluginProvider||plugin.state!=='running'}
          ),
          s.pluginResult!==null?pre(s.pluginResult):null,
          button('用于模板预览',async()=>{
            context.state.previewData=s.pluginResult;
            setTimeout(()=>context.navigate('templates'),0);
          },{disabled:s.pluginResult===null})
        ]
      ):null
    ];
  }
  return { view, load: loadPlugins, render: pluginsPanel, dirty: dirty };
});
