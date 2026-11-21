import { h, reactive } from '../../shared/vue.js';
import { json, safeLink } from '../../shared/format.js';
import { help, row, grid, section, pre, field, area, select, table } from '../../components/ui.js';
import { definePage, usePage } from '../../composables/page.js';
import { featureAPI } from './api.js';

export default definePage(context => {
  const view = usePage(context);
  const { button, upload, notify } = view;
  const perform = view.run;
  const api = featureAPI(view.signal);
  const network = view.network;
  const s = reactive({
    get network() {
      return context.state.network;
    },
    get rendering() {
      return context.state.rendering;
    },
    templates:[],
    template:null,
    templateRevision:'',
    templateFiles:[],
    templateFile:'',
    templateText:'',
    templateDirty:false,
    renderData:json(context.state.previewData || {}),
    provider:'core.online',
    providerPlayer:'',
    providerServer:'',
    defaultCSS:'',
    defaultTheme:'{}',
    defaultLocale:'zh-CN',
    imageURL:'',
    defaultsOriginal:'',
  });
  context.state.previewData = null;
  const networkPicker = () => view.networkPicker(() => {
    s.providerServer = '';
    s.renderData = '{}';
  });
  async function loadTemplates() {
    s.templates = (await api.list()).items;
  }
  const defaultsDraft = () => json([s.defaultCSS,s.defaultTheme,s.defaultLocale]);
  const dirty = () => s.templateDirty || (!!s.template && defaultsDraft() !== s.defaultsOriginal);
  async function image(result) {
    const blob = await api.image(result.resource_id);
    if (s.imageURL) URL.revokeObjectURL(s.imageURL);
    s.imageURL = URL.createObjectURL(blob);
  }
  async function chooseTemplate(id) {
    if(dirty()&&!confirm('切换模板会丢弃未保存的文件与样式草稿。继续？'))return;
    s.template=await api.get(id);
    s.templateRevision=s.template.draft||s.template.published||s.template.versions[0]?.revision||'';
    s.defaultCSS=s.template.defaults?.css||'';
    s.defaultTheme=json(s.template.defaults?.theme||{});
    s.defaultLocale=s.template.defaults?.locale||'zh-CN';
    s.templateFile='';
    s.templateText='';
    s.templateDirty=false;
    s.defaultsOriginal=defaultsDraft();
    await templateFiles();
  }
  async function templateFiles() {
    if(!s.template)return;
    s.templateFiles=(await api.files(s.template.id,s.templateRevision)).items;
  }
  async function readTemplateFile(name) {
    if(s.templateDirty&&!confirm('读取文件会丢弃未保存的文件草稿。继续？'))return;
    const data=await api.file(s.template.id,s.templateRevision,name);
    s.templateFile=name;
    s.templateText=data.text;
    s.templateDirty=false;
  }
  function templatesPanel() {
    const template=s.template;
    const version=template?.versions.find(item=>item.revision===s.templateRevision);
    return [
      section(
        '模板目录',
        [
          help('ZIP 根目录需要 template.json 与 HTML 入口。导入后先预览，再选择发布；旧版本可以重新发布以回滚。'),
          upload('导入模板 ZIP','.zip',false,async files=>{
            const value=await api.import(files[0]);
            await loadTemplates();
            await chooseTemplate(value.id);
          }),
          table(
            ['模板','已发布','版本数','操作'],
            s.templates.map(item=>[
              item.id,
              item.published?item.published.slice(0,12):'未发布',
              item.versions.length,
              button('管理',()=>chooseTemplate(item.id))
            ])
          )
        ]
      ),
      template?section(
        '模板版本 · '+template.id,
        [
          select(
            '内容版本',
            s.templateRevision,
            template.versions.map(v=>({
              value:v.revision,
              label:v.manifest.version+' · '+v.revision.slice(0,12)+(v.builtin?' · 内置':'')
            })),
            value=>perform(async()=>{
              if(s.templateDirty&&!confirm('切换版本会丢弃未保存的草稿。继续？'))return;
              s.templateRevision=value;
              s.templateFile='';
              s.templateText='';
              s.templateDirty=false;
              await templateFiles();
            })
          ),
          version?help(version.manifest.name+' · 作者：'+version.manifest.author+' · 许可：'+version.manifest.license):null,
          version?h('p',[
            h('a',{href:safeLink(version.manifest.project_url),target:'_blank',rel:'noopener noreferrer'},'模板项目来源')
          ]):null,
          version?.manifest.attributions?.length?pre(version.manifest.attributions):null,
          row([
            button(
              '发布所选版本',
              async()=>{
                if(!confirm('将此内容版本设为 '+template.id+' 的发布版本？'))return;
                s.template=await api.publish(template.id,template.revision,s.templateRevision);
                await loadTemplates();
                notify('模板发布版本已更新。');
              },
              {primary:true,disabled:!s.templateRevision}
            ),
            button(
              '恢复内置版本',
              async()=>{
                if(!confirm('将发布版本恢复到最近的内置模板？'))return;
                s.template=await api.restore(template.id,template.revision);
                await loadTemplates();
                notify('已恢复内置发布版本，历史草稿仍保留。');
              },
              {disabled:!template.versions.some(v=>v.builtin)}
            )
          ])
        ]
      ):null,
      template?section(
        '文件草稿',
        [
          select('读取包内文件',s.templateFile,s.templateFiles.map(id=>({id})),value=>perform(()=>readTemplateFile(value))),
          field('文件路径（也可填写新文件路径）',s.templateFile,value=>{
            s.templateFile=value;
            s.templateDirty=true;
          }),
          area('文件内容',s.templateText,value=>{
            s.templateText=value;
            s.templateDirty=true;
          },16),
          row([
            button(
              '保存为新草稿',
              async()=>{
                if(!s.templateFile)throw new Error('请指定文件路径');
                s.template=await api.draft(template.id,template.revision,s.templateRevision,{[s.templateFile]:s.templateText});
                s.templateRevision=s.template.draft;
                s.templateDirty=false;
                await templateFiles();
                await loadTemplates();
                notify('草稿已保存，尚未改变发布版本。');
              },
              {primary:true}
            ),
            button(
              '删除草稿文件',
              async()=>{
                if(!s.templateFile||!confirm('在新草稿中删除 '+s.templateFile+'？'))return;
                s.template=await api.draft(template.id,template.revision,s.templateRevision,{[s.templateFile]:null});
                s.templateRevision=s.template.draft;
                s.templateFile='';
                s.templateText='';
                s.templateDirty=false;
                await templateFiles();
                await loadTemplates();
              },
              {danger:true}
            )
          ])
        ]
      ):null,
      template?section(
        '默认样式',
        [
          grid([
            area('主题 JSON',s.defaultTheme,v=>s.defaultTheme=v,6),
            area('自定义 CSS',s.defaultCSS,v=>s.defaultCSS=v,6),
            field('语言',s.defaultLocale,v=>s.defaultLocale=v)
          ]),
          button(
            '保存默认样式',
            async()=>{
              s.template=await api.defaults(template.id,template.revision,{theme:JSON.parse(s.defaultTheme),css:s.defaultCSS,locale:s.defaultLocale});
              s.defaultsOriginal=defaultsDraft();
              notify('样式已保存，后续渲染将使用这些默认值。');
            }
          )
        ]
      ):null,
      template?section(
        '渲染预览',
        [
          help(s.rendering.enabled?'浏览器已配置。预览使用所选内容版本和已保存默认样式。':'尚未配置 Chromium。可管理模板；配置浏览器路径和精确版本并重启后才能预览。'),
          grid([
            networkPicker(),
            select('内置数据来源',s.provider,['core.online','core.profile','core.inventory','core.performance'].map(id=>({id})),v=>s.provider=v),
            field('玩家名（资料 / 背包）',s.providerPlayer,v=>s.providerPlayer=v),
            field('服务器 ID（背包）',s.providerServer,v=>s.providerServer=v)
          ]),
          button('读取数据快照',async()=>{
            s.renderData=json(await api.provider(s.provider,{network_id:s.network,player_name:s.providerPlayer,server_id:s.providerServer}));
          }),
          area('渲染数据 JSON',s.renderData,v=>s.renderData=v,12),
          button(
            '生成预览图片',
            async()=>{
              const result=await api.render({template_id:template.id,revision:s.templateRevision,data:JSON.parse(s.renderData)});
              await image(result);
              notify('图片已生成；临时资源有效期 10 分钟。');
            },
            {primary:true,disabled:!s.rendering.enabled}
          )
        ]
      ):null
    ];
  }
  return {
    view,
    load: loadTemplates,
    dirty,
    dispose() {
      if(s.imageURL) URL.revokeObjectURL(s.imageURL);
    },
    render: () => [
      ...templatesPanel(),
      s.imageURL ? section('图片预览', [
        h('img', {class:'preview',src:s.imageURL,alt:'模板渲染结果'}),
        button('关闭预览',()=>{
          URL.revokeObjectURL(s.imageURL);
          s.imageURL='';
        })
      ]) : null
    ]
  };
});
