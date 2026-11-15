import { createApp, h, reactive, ref } from './vendor/vue.runtime.esm-browser.prod.js';
import { api, setToken, onUnauthorized, stream, downloadJSON } from './api.js';

const root = '/api/v1/';
const esc = encodeURIComponent;
const json = value => JSON.stringify(value, null, 2);
const date = value => !value || value.startsWith('0001-') ? '—' : new Date(value).toLocaleString();
const pages = [['overview','运行概览'],['config','配置与凭据'],['bindings','绑定与白名单'],['players','玩家与背包'],['imports','白名单导入'],['templates','图片模板'],['plugins','本地插件'],['updates','版本更新'],['events','事件记录']];

function initial() {
  return {
    authenticated:false, credential:'', busy:0, notice:'', error:false, streamState:'未连接', status:{}, connections:{nodes:[],adapters:[]},
    networks:[], network:'', webui:{enabled:true}, rendering:{enabled:false}, updates:{enabled:false}, events:[], restartRequired:false,
    configText:'', configOriginal:'', configRevision:'', configLoaded:false, secretName:'', secretValue:'', secretResult:null,
    bindings:[], bindingOffset:0, playerName:'', playerUUID:'', ownerPlatform:'qq.numeric', ownerScope:'global', ownerID:'',
    players:[], playerOffset:0, onlineOnly:true, inventoryName:'', inventoryServer:'', inventory:null,
    sources:[], report:null, reportPage:0, reportFilter:'all', reportFingerprint:'', excluded:new Set(), reportID:'',
    templates:[], template:null, templateRevision:'', templateFiles:[], templateFile:'', templateText:'', templateDirty:false,
    renderData:'{}', provider:'core.online', providerPlayer:'', providerServer:'', defaultCSS:'', defaultTheme:'{}', defaultLocale:'zh-CN', imageURL:'',
    plugins:[], plugin:null, pluginVersion:'', pluginConfig:'{}', pluginGrant:json({capabilities:[],networks:[],routes:[],nodes:[]}), pluginProvider:'', pluginRequest:'{}', pluginResult:null
  };
}

export function mount() {
  createApp({ setup() {
    const s = reactive(initial());
    const page = ref('overview');
    let stopStream = () => {};
    const network = () => s.networks.find(n => n.id === s.network);
    function notify(message) { s.notice = message; s.error = false; }
    async function perform(action) {
      s.busy++; s.notice = '';
      try { await action(); }
      catch (error) { s.notice = error.message || '操作未完成'; s.error = true; }
      finally { s.busy = Math.max(0, s.busy - 1); }
    }
    function logout() {
      stopStream(); setToken('');
      if (s.imageURL) URL.revokeObjectURL(s.imageURL);
      Object.assign(s, initial()); page.value = 'overview';
    }
    onUnauthorized(logout);
    async function overview() {
      const [status,connections,networks,webui,updates,rendering] = await Promise.all(
        ['status','connections','networks','webui','updates','rendering'].map(path => api(root + path)));
      Object.assign(s,{status,connections,networks:networks.items,webui,updates,rendering});
      if (!s.networks.some(n => n.id === s.network)) s.network = s.networks[0]?.id || '';
    }
    async function login() {
      setToken(s.credential.trim());
      try { await overview(); } catch (error) { setToken(''); throw error; }
      s.credential = ''; s.authenticated = true;
      stopStream = stream(event => {
        s.events.unshift(event); s.events = s.events.slice(0,100);
        if (event.type === 'connections') s.connections = event.data;
        if (event.type === 'update.changed') s.updates = event.data;
        if (event.type === 'webui.changed') s.webui = event.data;
        if (event.type === 'config.saved') s.restartRequired = true;
        if (event.type === 'reset') overview().catch(() => {});
      }, value => { s.streamState = value; });
    }
    async function loadConfig(force = false) {
      if (s.configLoaded && !force) return;
      if (s.configLoaded && s.configText !== s.configOriginal && !confirm('重新读取会丢弃当前未保存的配置草稿。继续？')) return;
      const doc = await api(root + 'config');
      s.configText = s.configOriginal = doc.text; s.configRevision = doc.revision; s.configLoaded = true;
    }
    async function loadBindings() {
      if (!s.network) { s.bindings = []; return; }
      s.bindings = (await api(root+'bindings?network_id='+esc(s.network)+'&limit=100&offset='+s.bindingOffset)).items;
    }
    async function loadPlayers() {
      if (!s.network) { s.players = []; return; }
      s.players = (await api(root+'players?network_id='+esc(s.network)+'&online='+s.onlineOnly+'&limit=100&offset='+s.playerOffset)).items;
    }
    async function loadSources() {
      const previous = new Map(s.sources.map(item => [item.id,item]));
      const result = await api(root+'imports/sources');
      s.sources = result.items.map(item => ({...item,label:previous.get(item.id)?.label || item.id,network_id:previous.get(item.id)?.network_id || s.network,selected:previous.get(item.id)?.selected || false}));
    }
    async function loadTemplates() { s.templates = (await api(root+'templates')).items; }
    async function loadPlugins() { s.plugins = (await api(root+'plugins')).items; }
    async function reload(force = false) {
      switch (page.value) {
        case 'overview': await overview(); break;
        case 'config': await loadConfig(force); break;
        case 'bindings': await loadBindings(); break;
        case 'players': await loadPlayers(); break;
        case 'imports': await loadSources(); break;
        case 'templates': await loadTemplates(); break;
        case 'plugins': await loadPlugins(); break;
        case 'updates': s.updates = await api(root+'updates'); break;
      }
    }
    async function show(name) { page.value = name; await reload(); }
    async function selectNetwork(value) {
      s.network = value; s.bindingOffset = s.playerOffset = 0;
      if (page.value === 'bindings') await loadBindings();
      if (page.value === 'players') await loadPlayers();
    }
    function button(label, action, options = {}) {
      return h('button',{type:'button',class:options.danger?'danger':options.primary?'primary':'',disabled:!!s.busy||!!options.disabled,onClick:()=>perform(action)},label);
    }
    const help = text => h('p',{class:'muted'},text);
    const row = children => h('div',{class:'actions'},children);
    const grid = children => h('div',{class:'form-grid'},children);
    const section = (title,children) => h('section',{class:'card'},[h('h2',title),...children]);
    const pre = value => h('pre',{class:'output'},typeof value === 'string'?value:json(value));
    function field(label,value,change,options={}) {
      return h('label',{class:'field'},[h('span',label),h('input',{type:options.type||'text',value,placeholder:options.placeholder||'',readonly:options.readonly||false,autocomplete:options.type==='password'?'new-password':'off',onInput:event=>change(event.target.value)})]);
    }
    function area(label,value,change,lines=8) {
      return h('label',{class:'field'},[h('span',label),h('textarea',{value,rows:lines,spellcheck:false,onInput:event=>change(event.target.value)})]);
    }
    function select(label,value,items,change) {
      return h('label',{class:'field'},[h('span',label),h('select',{value,onChange:event=>change(event.target.value)},[h('option',{value:''},'请选择'),...items.map(item=>h('option',{value:item.value??item.id},item.label??item.id))])]);
    }
    function check(label,value,change,disabled=false) {
      return h('label',{class:'check'},[h('input',{type:'checkbox',checked:value,disabled,onChange:event=>change(event.target.checked)}),h('span',label)]);
    }
    function table(headers,rows) {
      return h('div',{class:'table-wrap'},[h('table',[h('thead',[h('tr',headers.map(label=>h('th',label)))]),h('tbody',rows.length?rows.map((cells,index)=>h('tr',{key:index},cells.map(value=>h('td',value??'—')))):[h('tr',[h('td',{colspan:headers.length,class:'empty'},'暂无记录')])])])]);
    }
    function upload(label,accept,multiple,action) {
      return h('label',{class:'upload'},[h('span',label),h('input',{type:'file',accept,multiple,disabled:!!s.busy,onChange:event=>{
        const files=Array.from(event.target.files||[]); event.target.value=''; if(files.length) perform(()=>action(files));
      }})]);
    }
    function pager(offset,length,change) {
      return row([button('上一页',()=>change(Math.max(0,offset-100)),{disabled:offset===0}),help('第 '+(Math.floor(offset/100)+1)+' 页'),button('下一页',()=>change(offset+100),{disabled:length<100})]);
    }
    function networkPicker() { return select('游戏网络',s.network,s.networks,value=>perform(()=>selectNetwork(value))); }
    async function image(result) {
      const blob = await api(root+'renders/resources/'+esc(result.resource_id),{binary:true});
      if(s.imageURL) URL.revokeObjectURL(s.imageURL);
      s.imageURL = URL.createObjectURL(blob);
    }

    function overviewPanel() {
      return [h('div',{class:'metrics'},[
        ['Core 状态',s.status.state||'—'],['版本',s.status.version||'—'],['在线节点',s.connections.nodes.filter(n=>n.connected).length+' / '+s.connections.nodes.length],['机器人连接',s.connections.adapters.filter(a=>a.state==='connected').length+' / '+s.connections.adapters.length]
      ].map(([label,value])=>h('div',{class:'metric'},[h('span',label),h('strong',value)]))),
      section('当前实例',[help('启动时间：'+date(s.status.started_at)+' · 实例：'+(s.status.instance_id||'—')),help('事件连接：'+s.streamState),
        row([button(s.webui.enabled?'关闭页面服务':'开启页面服务',async()=>{
          s.webui=await api(root+'webui',{method:'PUT',body:{enabled:!s.webui.enabled}}); notify('会话开关已更新；下次启动的默认值通过配置文件保存。');
        }),help('管理 API 始终独立可用。')])]),
      section('Minecraft 节点',[table(['节点','网络 / 服务器','平台','状态'],s.connections.nodes.map(n=>[n.id,n.network_id+' / '+n.server_id,n.platform||'—',n.connected?'已连接':'离线']))]),
      section('机器人连接',[table(['连接','类型','状态','丢弃事件','最后错误'],s.connections.adapters.map(a=>[a.id,a.type,a.state,a.dropped,a.last_error||'—']))])];
    }

    function configPanel() {
      return [section('配置文件',[help('保存前会校验并备份原文件。连接类配置在宿主重启后生效；版本冲突时请重新读取并合并草稿。'),
        area('config.yaml',s.configText,v=>s.configText=v,22),row([
          button('检查配置',async()=>{await api(root+'config/validate',{method:'POST',body:{text:s.configText}});notify('配置格式与规则校验通过，尚未保存。');}),
          button('保存配置',async()=>{const result=await api(root+'config',{method:'PUT',revision:s.configRevision,body:{text:s.configText}});s.configRevision=result.document.revision;s.configOriginal=result.document.text;s.restartRequired=true;notify('已保存并备份，等待宿主重启生效。');},{primary:true}),
          button('重新读取',()=>loadConfig(true))])]),
        section('创建连接凭据',[help('凭据只在创建响应中显示。配置文件使用返回的 file: 引用。已有凭据不会覆盖。'),
          grid([field('文件名',s.secretName,v=>s.secretName=v,{placeholder:'node-lobby.token'}),field('凭据内容（留空自动生成）',s.secretValue,v=>s.secretValue=v,{type:'password'})]),
          button('创建凭据',async()=>{s.secretResult=await api(root+'secrets',{method:'POST',body:{name:s.secretName,value:s.secretValue}});s.secretValue='';}),
          s.secretResult?h('div',[help('引用：'+s.secretResult.reference),pre(s.secretResult.value),button('隐藏凭据',()=>{s.secretResult=null;})]):null])];
    }

    function bindingsPanel() {
      return [section('绑定与白名单',[networkPicker(),table(['玩家','UUID','账号身份','操作'],s.bindings.map(item=>[item.player.name,item.player.uuid||'—',item.owner?item.owner.platform+' / '+item.owner.scope+' / '+item.owner.id:'无账号白名单',button('撤销',async()=>{
        if(!confirm('撤销 '+item.player.name+' 的白名单 / 绑定？启用踢人规则时会尝试踢出玩家。'))return;
        await api(root+'bindings/'+esc(item.network_id)+'/'+esc(item.player.name),{method:'DELETE'});await loadBindings();
      },{danger:true})])),pager(s.bindingOffset,s.bindings.length,async offset=>{s.bindingOffset=offset;await loadBindings();})]),
      section('添加记录',[grid([field('玩家名',s.playerName,v=>s.playerName=v),field('UUID（可选）',s.playerUUID,v=>s.playerUUID=v),field('平台',s.ownerPlatform,v=>s.ownerPlatform=v),field('作用域',s.ownerScope,v=>s.ownerScope=v),field('账号 ID（留空仅加入白名单）',s.ownerID,v=>s.ownerID=v)]),
        button('添加到当前网络',async()=>{
          if(!network())throw new Error('请先选择游戏网络');
          const body={network_id:s.network,player:{name:s.playerName,uuid:s.playerUUID,identity_mode:network().identity_mode}};
          if(s.ownerID)body.owner={platform:s.ownerPlatform,scope:s.ownerScope,id:s.ownerID};
          await api(root+'bindings',{method:'POST',body});s.playerName=s.playerUUID=s.ownerID='';await loadBindings();notify('记录已添加。');
        },{primary:true,disabled:!s.network})])];
    }

    function playersPanel() {
      return [section('玩家资料',[row([networkPicker(),check('只看在线玩家',s.onlineOnly,value=>{s.onlineOnly=value;s.playerOffset=0;perform(loadPlayers);})]),
        table(['玩家','状态','累计在线','最后记录','节点'],s.players.map(p=>[p.player.name,p.online?'在线':'离线',Math.floor(p.total_seconds/60)+' 分钟',date(p.last_seen_at),p.node_id])),
        pager(s.playerOffset,s.players.length,async offset=>{s.playerOffset=offset;await loadPlayers();})]),
        section('背包快照',[grid([field('玩家名',s.inventoryName,v=>s.inventoryName=v),select('服务器',s.inventoryServer,(network()?.nodes||[]).map(n=>({value:n.server_id,label:n.server_id})),v=>s.inventoryServer=v)]),
          button('读取快照',async()=>{s.inventory=await api(root+'inventory?network_id='+esc(s.network)+'&server_id='+esc(s.inventoryServer)+'&player_name='+esc(s.inventoryName));}),
          s.inventory?pre(s.inventory):help('显示已保存的有效快照；游戏内最新采集通过节点 inventory.read 动作完成。')])];
    }

    function importRequest() {
      return {sources:s.sources.filter(item=>item.selected).map(item=>({id:item.id,network_id:item.network_id,identity_mode:s.networks.find(n=>n.id===item.network_id)?.identity_mode||''})),exclude:Array.from(s.excluded).sort()};
    }
    function importsPanel() {
      const report=s.report;
      const rows=(report?.rows||[]).filter(item=>s.reportFilter==='all'||item.status===s.reportFilter);
      const current=report&&s.reportFingerprint===JSON.stringify(importRequest());
      return [section('导入源文件',[help('选择旧 ShitBot schema 7 的一致性 SQLite 副本。单次可选择 1 至 8 个源文件；导入前必须预览，无账号白名单会保留。'),networkPicker(),
        upload('上传 SQLite 副本','.sqlite,.db',true,async files=>{
          for(const file of files){
            if(file.size>64*1024*1024)throw new Error(file.name+' 超过 64 MiB');
            const item=await api(root+'imports/sources',{method:'POST',upload:file});
            s.sources.push({...item,label:file.name,network_id:s.network,selected:true});
          }
          notify('源文件已上传，请选择目标网络并生成预览。');
        }),
        table(['选择','源文件','大小','目标网络','操作'],s.sources.map(item=>[
          check('纳入',item.selected,value=>item.selected=value),item.label,Math.ceil(item.size/1024)+' KiB',select('目标',item.network_id,s.networks,value=>item.network_id=value),
          button('删除副本',async()=>{
            if(!confirm('删除这个导入暂存副本？原始文件不受影响。'))return;
            await api(root+'imports/sources/'+esc(item.id),{method:'DELETE'});
            s.sources=s.sources.filter(source=>source.id!==item.id);
            for(const id of s.excluded)if(id.startsWith(item.id+':'))s.excluded.delete(id);
          },{danger:true})])),
        row([button('生成 / 更新预览',async()=>{
          const request=importRequest();
          s.report=await api(root+'imports/previews',{method:'POST',body:request});s.reportFingerprint=JSON.stringify(request);s.reportPage=0;s.reportID=s.report.id;
        },{primary:true,disabled:!s.sources.some(item=>item.selected)}),button('清空排除选择',()=>{s.excluded.clear();})])]),
        section('导入报告',[grid([field('任务 ID',s.reportID,value=>s.reportID=value)]),row([
          button('读取报告',async()=>{s.report=await api(root+'imports/'+esc(s.reportID));s.reportFingerprint='';s.reportPage=0;}),
          button('下载报告',()=>downloadJSON('import-'+report.id+'.json',report),{disabled:!report})]),
          report?h('div',[
            help('任务 '+report.id+' · 新增 '+report.added+' · 重复 '+report.duplicates+' · 冲突 '+report.conflicts+' · 排除 '+report.excluded),
            help(report.committed_at?'已提交：'+date(report.committed_at)+' · 备份：'+report.backup:'预览有效期：'+date(report.expires_at)),
            ...(report.errors||[]).map(error=>h('p',{class:'notice error'},error)),
            !current&&!report.committed_at?help('源文件、范围或排除项已变化，请重新生成预览后提交。'):null,
            select('记录状态',s.reportFilter,['all','add','duplicate','conflict','excluded'].map(id=>({id})),value=>{s.reportFilter=value;s.reportPage=0;}),
            table(['排除','玩家','状态','说明','来源'],rows.slice(s.reportPage,s.reportPage+100).map(item=>[
              check('排除',s.excluded.has(item.id),value=>{if(value)s.excluded.add(item.id);else s.excluded.delete(item.id);},!!report.committed_at),
              item.record.player.name,item.status,item.reason||'—',item.sources.join(', ')])),
            pager(s.reportPage,Math.min(100,rows.length-s.reportPage),offset=>{s.reportPage=offset;}),
            row([button('提交此预览',async()=>{
              if(!confirm('确认按当前预览新增 '+report.added+' 条记录？Core 会先备份目标数据库。'))return;
              s.report=await api(root+'imports/'+esc(report.id)+'/commit',{method:'POST'});notify('导入已提交，报告包含备份位置。');
            },{primary:true,disabled:!current||!report.ready||!!report.committed_at}),
            button('撤销此导入',async()=>{
              if(!confirm('撤销此任务新增且尚未被后续业务修改的记录？发生冲突时整批取消。'))return;
              s.report=await api(root+'imports/'+esc(report.id)+'/restore',{method:'POST'});notify('导入已撤销。');
            },{danger:true,disabled:!report.committed_at||!!report.restored_at})]),report.restored_at?help('已撤销：'+date(report.restored_at)):null
          ]):help('预览、提交和撤销共享同一个任务 ID。保留该 ID 可在重启后查询已提交报告。')])];
    }

    async function chooseTemplate(id) {
      if(s.templateDirty&&!confirm('切换模板会丢弃未保存的文件草稿。继续？'))return;
      s.template=await api(root+'templates/'+esc(id));
      s.templateRevision=s.template.draft||s.template.published||s.template.versions[0]?.revision||'';
      s.defaultCSS=s.template.defaults?.css||'';s.defaultTheme=json(s.template.defaults?.theme||{});s.defaultLocale=s.template.defaults?.locale||'zh-CN';
      s.templateFile='';s.templateText='';s.templateDirty=false;
      await templateFiles();
    }
    async function templateFiles() {
      if(!s.template)return;
      s.templateFiles=(await api(root+'templates/'+esc(s.template.id)+'/files?revision='+esc(s.templateRevision))).items;
    }
    async function readTemplateFile(name) {
      if(s.templateDirty&&!confirm('读取文件会丢弃未保存的文件草稿。继续？'))return;
      const data=await api(root+'templates/'+esc(s.template.id)+'/file?revision='+esc(s.templateRevision)+'&path='+esc(name));
      s.templateFile=name;s.templateText=data.text;s.templateDirty=false;
    }
    function templatesPanel() {
      const template=s.template;
      const version=template?.versions.find(item=>item.revision===s.templateRevision);
      return [section('模板目录',[help('ZIP 根目录需要 template.json 与 HTML 入口。导入后先预览，再选择发布；旧版本可以重新发布以回滚。'),
        upload('导入模板 ZIP','.zip',false,async files=>{const value=await api(root+'templates',{method:'POST',upload:files[0]});await loadTemplates();await chooseTemplate(value.id);}),
        table(['模板','已发布','版本数','操作'],s.templates.map(item=>[item.id,item.published?item.published.slice(0,12):'未发布',item.versions.length,button('管理',()=>chooseTemplate(item.id))]))]),
        template?section('模板版本 · '+template.id,[
          select('内容版本',s.templateRevision,template.versions.map(v=>({value:v.revision,label:v.manifest.version+' · '+v.revision.slice(0,12)+(v.builtin?' · 内置':'')})),value=>perform(async()=>{
            if(s.templateDirty&&!confirm('切换版本会丢弃未保存的草稿。继续？'))return;
            s.templateRevision=value;s.templateFile='';s.templateText='';s.templateDirty=false;await templateFiles();
          })),
          version?help(version.manifest.name+' · 作者：'+version.manifest.author+' · 许可：'+version.manifest.license):null,
          version?h('p',[h('a',{href:version.manifest.project_url,target:'_blank',rel:'noopener noreferrer'},'模板项目来源')]):null,
          version?.manifest.attributions?.length?pre(version.manifest.attributions):null,
          row([button('发布所选版本',async()=>{
            if(!confirm('将此内容版本设为 '+template.id+' 的发布版本？'))return;
            s.template=await api(root+'templates/'+esc(template.id)+'/publish',{method:'POST',revision:template.revision,body:{version_revision:s.templateRevision}});await loadTemplates();notify('模板发布版本已更新。');
          },{primary:true,disabled:!s.templateRevision}),button('恢复内置版本',async()=>{
            if(!confirm('将发布版本恢复到最近的内置模板？'))return;
            s.template=await api(root+'templates/'+esc(template.id)+'/restore',{method:'POST',revision:template.revision});await loadTemplates();notify('已恢复内置发布版本，历史草稿仍保留。');
          },{disabled:!template.versions.some(v=>v.builtin)})])]):null,
        template?section('文件草稿',[select('读取包内文件',s.templateFile,s.templateFiles.map(id=>({id})),value=>perform(()=>readTemplateFile(value))),
          field('文件路径（也可填写新文件路径）',s.templateFile,value=>{s.templateFile=value;s.templateDirty=true;}),
          area('文件内容',s.templateText,value=>{s.templateText=value;s.templateDirty=true;},16),
          row([button('保存为新草稿',async()=>{
            if(!s.templateFile)throw new Error('请指定文件路径');
            s.template=await api(root+'templates/'+esc(template.id)+'/draft',{method:'PUT',revision:template.revision,body:{base_revision:s.templateRevision,files:{[s.templateFile]:s.templateText}}});
            s.templateRevision=s.template.draft;s.templateDirty=false;await templateFiles();await loadTemplates();notify('草稿已保存，尚未改变发布版本。');
          },{primary:true}),button('删除草稿文件',async()=>{
            if(!s.templateFile||!confirm('在新草稿中删除 '+s.templateFile+'？'))return;
            s.template=await api(root+'templates/'+esc(template.id)+'/draft',{method:'PUT',revision:template.revision,body:{base_revision:s.templateRevision,files:{[s.templateFile]:null}}});
            s.templateRevision=s.template.draft;s.templateFile='';s.templateText='';s.templateDirty=false;await templateFiles();await loadTemplates();
          },{danger:true})])]):null,
        template?section('默认样式',[grid([area('主题 JSON',s.defaultTheme,v=>s.defaultTheme=v,6),area('自定义 CSS',s.defaultCSS,v=>s.defaultCSS=v,6),field('语言',s.defaultLocale,v=>s.defaultLocale=v)]),
          button('保存默认样式',async()=>{s.template=await api(root+'templates/'+esc(template.id)+'/defaults',{method:'PUT',revision:template.revision,body:{theme:JSON.parse(s.defaultTheme),css:s.defaultCSS,locale:s.defaultLocale}});notify('样式已保存，后续渲染将使用这些默认值。');})]):null,
        template?section('渲染预览',[help(s.rendering.enabled?'浏览器已配置。预览使用所选内容版本和已保存默认样式。':'尚未配置 Chromium。可管理模板；配置浏览器路径和精确版本并重启后才能预览。'),
          grid([networkPicker(),select('内置数据来源',s.provider,['core.online','core.profile','core.inventory','core.performance'].map(id=>({id})),v=>s.provider=v),field('玩家名（资料 / 背包）',s.providerPlayer,v=>s.providerPlayer=v),field('服务器 ID（背包）',s.providerServer,v=>s.providerServer=v)]),
          button('读取数据快照',async()=>{s.renderData=json(await api(root+'rendering/providers/'+esc(s.provider),{method:'POST',body:{network_id:s.network,player_name:s.providerPlayer,server_id:s.providerServer}}));}),
          area('渲染数据 JSON',s.renderData,v=>s.renderData=v,12),button('生成预览图片',async()=>{
            const result=await api(root+'renders',{method:'POST',body:{template_id:template.id,revision:s.templateRevision,data:JSON.parse(s.renderData)}});await image(result);notify('图片已生成；临时资源有效期 10 分钟。');
          },{primary:true,disabled:!s.rendering.enabled})]):null];
    }

    async function choosePlugin(id) {
      s.plugin=await api(root+'plugins/'+esc(id));s.pluginVersion=s.plugin.active_revision;
      s.pluginConfig=json(s.plugin.configuration||{});
      s.pluginGrant=json(s.plugin.grant?.capabilities?.length?s.plugin.grant:{capabilities:[],networks:[],routes:[],nodes:[]});
      s.pluginProvider='';s.pluginResult=null;
    }
    function pluginsPanel() {
      const plugin=s.plugin;
      const version=plugin?.versions.find(item=>item.revision===s.pluginVersion);
      return [section('插件目录',[help('导入后默认停用。启用时按下面明确填写的能力和范围授权；修改配置或版本前先停用。'),
        upload('导入插件 ZIP','.zip',false,async files=>{const value=await api(root+'plugins',{method:'POST',upload:files[0]});await loadPlugins();await choosePlugin(value.id);}),
        table(['插件','运行状态','丢弃事件','最后错误','操作'],s.plugins.map(item=>[item.id,item.state,item.dropped,item.last_error||'—',button('管理',()=>choosePlugin(item.id))]))]),
        plugin?section('插件管理 · '+plugin.id,[select('包版本',s.pluginVersion,plugin.versions.map(v=>({value:v.revision,label:v.manifest.version+' · '+v.revision.slice(0,12)})),value=>s.pluginVersion=value),
          version?help(version.manifest.name+' · 作者：'+version.manifest.author+' · 许可：'+version.manifest.license):null,
          version?h('p',[h('a',{href:version.manifest.project_url,target:'_blank',rel:'noopener noreferrer'},'插件项目来源')]):null,
          help('插件在 Core 进程中运行，仅启用你信任的代码。配置、权限和版本通过最新状态版本提交。'),
          version?pre({declared_capabilities:version.manifest.capabilities,commands:version.manifest.commands||[],providers:version.manifest.providers||[]}):null,
          grid([area('插件配置 JSON',s.pluginConfig,value=>s.pluginConfig=value,10),area('授予权限与范围 JSON',s.pluginGrant,value=>s.pluginGrant=value,10)]),
          help('可选网络、节点与路由：'),pre(s.networks),
          row([button('按以上权限启用',async()=>{
            s.plugin=await api(root+'plugins/'+esc(plugin.id)+'/enable',{method:'POST',revision:plugin.revision,body:{version_revision:s.pluginVersion,grant:JSON.parse(s.pluginGrant),configuration:JSON.parse(s.pluginConfig)}});await loadPlugins();notify('插件已启用。');
          },{primary:true,disabled:plugin.state==='running'}),
          button('停用',async()=>{s.plugin=await api(root+'plugins/'+esc(plugin.id)+'/disable',{method:'POST',revision:plugin.revision});await loadPlugins();notify('插件已停用。');},{disabled:!plugin.enabled}),
          button('卸载包',async()=>{
            if(!confirm('卸载 '+plugin.id+' 的插件包？插件数据和最后配置会保留。'))return;
            await api(root+'plugins/'+esc(plugin.id),{method:'DELETE',revision:plugin.revision});s.plugin=null;await loadPlugins();
          },{danger:true,disabled:plugin.enabled}),button('重新读取',()=>choosePlugin(plugin.id))])]):null,
        plugin&&version?.manifest.providers?.length?section('插件数据提供器',[
          select('提供器',s.pluginProvider,version.manifest.providers.map(id=>({id})),value=>s.pluginProvider=value),area('请求 JSON',s.pluginRequest,value=>s.pluginRequest=value),
          button('读取数据',async()=>{s.pluginResult=await api(root+'plugins/'+esc(plugin.id)+'/providers/'+esc(s.pluginProvider),{method:'POST',body:JSON.parse(s.pluginRequest)});},{disabled:!s.pluginProvider||plugin.state!=='running'}),
          s.pluginResult!==null?pre(s.pluginResult):null,
          button('用于模板预览',async()=>{s.renderData=json(s.pluginResult);await show('templates');},{disabled:s.pluginResult===null})]):null];
    }
    function updatesPanel() {
      const state=s.updates;const release=state.release;
      const locked=['checking','downloading','handing_off','handed_off'].includes(state.phase);
      return [section('宿主版本更新',[help(state.enabled?'仓库：'+state.repository+' · 当前宿主版本：'+state.current_version:'当前宿主没有注册更新功能。请由 CLI / Desktop 按其安装方式管理版本。'),
        help('阶段：'+(state.phase||'disabled')+' · 最近检查：'+date(state.checked_at)),state.last_error?help('最后错误：'+state.last_error):null,
        state.backup?help('数据库备份：'+state.backup):null,
        row([button('检查更新',async()=>{
          try { s.updates=await api(root+'updates/check',{method:'POST'}); }
          finally { s.updates=await api(root+'updates'); }
        },{disabled:!state.enabled||locked}),button('刷新任务状态',async()=>{s.updates=await api(root+'updates');})]),
        release?h('div',[h('h3','候选版本 '+release.version),help(release.available?'有较新的宿主版本。':'当前版本无需升级。'),
          h('p',[h('a',{href:release.url,target:'_blank',rel:'noopener noreferrer'},'查看 Release 与人工下载入口')]),
          help('资产：'+release.asset_name+' · '+(release.size?Math.ceil(release.size/1024/1024)+' MiB':'无法确定')),
          pre('SHA-256: '+(release.sha256||'缺少可用摘要，禁止自动安装')),pre(release.notes||'此版本未提供更新说明。'),
          button('下载、校验并交给宿主升级',async()=>{
            if(!confirm('升级到 '+release.version+'？Core 将先校验并备份，宿主接管后可能退出或重启。'))return;
            s.updates=await api(root+'updates/install',{method:'POST',body:{version:release.version,sha256:release.sha256}});notify('更新任务已受理，请观察任务状态。');
          },{primary:true,disabled:!state.can_install||!release.available||!release.sha256||locked})]):null,
        state.phase==='handed_off'?help('安装资产已交给宿主，尚未确认新版本运行。请等待宿主操作完成后重新连接。'):null,
        help('自动检查不会自动安装；程序替换、协议兼容检查与失败回退由拥有当前进程的宿主执行。')])];
    }
    function eventsPanel() {
      return [section('当前页面的最近事件',[help('最多显示 100 条管理摘要，不包含 Token、聊天正文或验证码。页面刷新后不保留本地列表。'),
        table(['时间','类型','摘要'],s.events.map(event=>[date(event.time),event.type,pre(event.data)])),button('清空页面记录',()=>{s.events=[];})])];
    }
    const panels = {overview:overviewPanel,config:configPanel,bindings:bindingsPanel,players:playersPanel,imports:importsPanel,templates:templatesPanel,plugins:pluginsPanel,updates:updatesPanel,events:eventsPanel};

    return () => {
      if(!s.authenticated)return h('main',{class:'login'},[h('div',{class:'brand'},'ShitBotUltra'),h('h1','连接你的 Core'),help('使用配置目录 secrets 中的管理凭据，或管理员设置的环境变量凭据。'),field('管理 Token',s.credential,v=>s.credential=v,{type:'password'}),button('连接控制台',login,{primary:true,disabled:!s.credential.trim()}),s.notice?h('p',{role:'alert',class:'notice error'},s.notice):null,help('Token 仅保留在当前页面内存，刷新页面后重新输入。')]);
      return h('div',{class:'shell'},[
        h('aside',[h('div',{class:'brand'},'ShitBotUltra'),help('CORE CONSOLE'),h('nav',pages.map(([key,label])=>h('button',{class:page.value===key?'active':'',onClick:()=>perform(()=>show(key))},label))),button('退出连接',logout)]),
        h('main',[h('header',[h('div',[h('span',{class:'eyebrow'},'你的服务器 · 你的规则'),h('h1',pages.find(p=>p[0]===page.value)?.[1])]),row([h('span',{class:'badge'},s.busy?'处理中…':s.streamState),button('刷新',()=>reload(true))])]),
          s.restartRequired?h('p',{class:'notice'},'配置已保存，等待宿主重启后生效。'):null,
          s.notice?h('p',{class:'notice'+(s.error?' error':''),role:s.error?'alert':'status'},s.notice):null,
          ...(panels[page.value]?.()||[]),
          s.imageURL?section('图片预览',[h('img',{class:'preview',src:s.imageURL,alt:'模板渲染结果'}),button('关闭预览',()=>{URL.revokeObjectURL(s.imageURL);s.imageURL='';})]):null,
          h('footer','ShitBotUltra · AGPL-3.0-only · 管理操作通过当前 Core API 执行')])]);
    };
  }}).mount('#app');
}
