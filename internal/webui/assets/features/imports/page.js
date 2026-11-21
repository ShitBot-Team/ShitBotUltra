import { h, reactive } from '../../shared/vue.js';
import { json, date, downloadJSON } from '../../shared/format.js';
import { help, row, grid, section, field, select, check, table } from '../../components/ui.js';
import { definePage, usePage } from '../../composables/page.js';
import { featureAPI } from './api.js';

export default definePage(context => {
  const view = usePage(context);
  const { button, upload, pager, notify } = view;
  const api = featureAPI(view.signal);
  const network = view.network;
  const s = reactive({
    get network() {
      return context.state.network;
    },
    get networks() {
      return context.state.networks;
    },
    sources:[],
    report:null,
    reportPage:0,
    reportFilter:'all',
    reportFingerprint:'',
    excluded:new Set(),
    reportID:'',
  });
  const networkPicker = () => view.networkPicker();
  async function loadSources() {
    const previous = new Map(s.sources.map(item => [item.id,item]));
    const result = await api.sources();
    s.sources = result.items.map(item => ({
      ...item,
      label:previous.get(item.id)?.label || item.id,
      network_id:previous.get(item.id)?.network_id || s.network,
      selected:previous.get(item.id)?.selected || false
    }));
  }
  function importRequest() {
    return {
      sources:s.sources.filter(item=>item.selected).map(item=>({
        id:item.id,
        network_id:item.network_id,
        identity_mode:s.networks.find(n=>n.id===item.network_id)?.identity_mode||''
      })),
      exclude:Array.from(s.excluded).sort()
    };
  }
  function importsPanel() {
    const report=s.report;
    const rows=(report?.rows||[]).filter(item=>s.reportFilter==='all'||item.status===s.reportFilter);
    const current=report&&s.reportFingerprint===JSON.stringify(importRequest());
    return [
      section(
        '导入源文件',
        [
          help('选择旧 ShitBot schema 7 的一致性 SQLite 副本。单次可选择 1 至 8 个源文件；导入前必须预览，无账号白名单会保留。'),
          networkPicker(),
          upload(
            '上传 SQLite 副本',
            '.sqlite,.db',
            true,
            async files=>{
              if(files.length + s.sources.filter(item=>item.selected).length > 8)throw new Error('一次最多选择 8 个导入源，请先取消不需要的源文件。');
              for(const file of files){
                if(file.size>64*1024*1024)throw new Error(file.name+' 超过 64 MiB');
                const item=await api.upload(file);
                s.sources.push({...item,label:file.name,network_id:s.network,selected:true});
              }
              notify('源文件已上传，请选择目标网络并生成预览。');
            }
          ),
          table(
            ['选择','源文件','大小','目标网络','操作'],
            s.sources.map(item=>[
              check('纳入',item.selected,value=>item.selected=value),
              item.label,
              Math.ceil(item.size/1024)+' KiB',
              select('目标',item.network_id,s.networks,value=>item.network_id=value),
              button(
                '删除副本',
                async()=>{
                  if(!confirm('删除这个导入暂存副本？原始文件不受影响。'))return;
                  await api.remove(item.id);
                  s.sources=s.sources.filter(source=>source.id!==item.id);
                  for(const id of s.excluded)if(id.startsWith(item.id+':'))s.excluded.delete(id);
                },
                {danger:true}
              )
            ])
          ),
          row([
            button(
              '生成 / 更新预览',
              async()=>{
                const request=importRequest();
                s.report=await api.preview(request);
                s.reportFingerprint=JSON.stringify(request);
                s.reportPage=0;
                s.reportID=s.report.id;
              },
              {primary:true,disabled:!s.sources.some(item=>item.selected)}
            ),
            button('清空排除选择',()=>{
              s.excluded.clear();
            })
          ])
        ]
      ),
      section(
        '导入报告',
        [
          grid([field('任务 ID',s.reportID,value=>s.reportID=value)]),
          row([
            button('读取报告',async()=>{
              s.report=await api.report(s.reportID);
              s.reportFingerprint='';
              s.reportPage=0;
            }),
            button('下载报告',()=>downloadJSON('import-'+report.id+'.json',report),{disabled:!report})
          ]),
          report?h(
            'div',
            [
              help('任务 '+report.id+' · 新增 '+report.added+' · 重复 '+report.duplicates+' · 冲突 '+report.conflicts+' · 排除 '+report.excluded),
              help(report.committed_at?'已提交：'+date(report.committed_at)+' · 备份：'+report.backup:'预览有效期：'+date(report.expires_at)),
              ...(report.errors||[]).map(error=>h('p',{class:'notice error'},error)),
              !current&&!report.committed_at?help('源文件、范围或排除项已变化，请重新生成预览后提交。'):null,
              select('记录状态',s.reportFilter,['all','add','duplicate','conflict','excluded'].map(id=>({id})),value=>{
                s.reportFilter=value;
                s.reportPage=0;
              }),
              table(
                ['排除','玩家','状态','说明','来源'],
                rows.slice(s.reportPage,s.reportPage+100).map(item=>[
                  check('排除',s.excluded.has(item.id),value=>{
                    if(value)s.excluded.add(item.id);else s.excluded.delete(item.id);
                  },!!report.committed_at),
                  item.record.player.name,
                  item.status,
                  item.reason||'—',
                  item.sources.join(', ')
                ])
              ),
              pager(s.reportPage,Math.min(100,rows.length-s.reportPage),offset=>{
                s.reportPage=offset;
              }),
              row([
                button(
                  '提交此预览',
                  async()=>{
                    if(!confirm('确认按当前预览新增 '+report.added+' 条记录？Core 会先备份目标数据库。'))return;
                    s.report=await api.commit(report.id);
                    notify('导入已提交，报告包含备份位置。');
                  },
                  {primary:true,disabled:!current||!report.ready||!!report.committed_at}
                ),
                button(
                  '撤销此导入',
                  async()=>{
                    if(!confirm('撤销此任务新增且尚未被后续业务修改的记录？发生冲突时整批取消。'))return;
                    s.report=await api.restore(report.id);
                    notify('导入已撤销。');
                  },
                  {danger:true,disabled:!report.committed_at||!!report.restored_at}
                )
              ]),
              report.restored_at?help('已撤销：'+date(report.restored_at)):null
            ]
          ):help('预览、提交和撤销共享同一个任务 ID。保留该 ID 可在重启后查询已提交报告。')
        ]
      )
    ];
  }
  return {
    view,
    load: loadSources,
    render: importsPanel,
    dirty: () => s.sources.some(item => item.selected) && (!s.report?.committed_at || s.reportFingerprint !== JSON.stringify(importRequest()))
  };
});
