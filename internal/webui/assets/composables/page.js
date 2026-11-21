import { defineComponent, h, reactive, onMounted, onBeforeUnmount } from '../shared/vue.js';
import { controls, select } from '../components/ui.js';

export function usePage(context) {
  const controller = new AbortController();
  const task = reactive({ busy: false, message: '', error: false, code: '' });
  const notify = message => {
    task.message = message;
    task.error = false;
    task.code = '';
  };
  async function run(action) {
    if (task.busy || controller.signal.aborted) return;
    task.busy = true;
    task.message = '';
    task.code = '';
    try {
      return await action();
    }
    catch (error) {
      if (!controller.signal.aborted) {
        task.error = true;
        task.code = error.code || '';
        task.message = error.name === 'AbortError' ? '请求已取消或超时。修改操作请先刷新状态，确认结果后再操作。' : error.message || '操作未完成';
      }
    } finally {
      task.busy = false;
    }
  }
  onBeforeUnmount(() => controller.abort());
  return {
    task,
    run,
    notify,
    signal: controller.signal,
    ...controls(task, run),
    network: () => context.state.networks.find(item => item.id === context.state.network),
    networkPicker: change => select(
      '游戏网络',
      context.state.network,
      context.state.networks,
      value => run(async () => {
        context.state.network = value;
        if (change) await change();
      })
    )
  };
}
// Each page owns its state and requests. Only session/network summaries are
// shared. Unmount aborts requests; a late response cannot enter another page.

export function definePage(create) {
  return defineComponent({
    props: { context: { type: Object, required: true } },
    setup(props) {
      const page = create(props.context);
      const hooks = {
        refresh: () => page.view.run(() => page.load?.(true)),
        dirty: () => page.view.task.busy || !!page.dirty?.()
      };
      props.context.setHooks(hooks);
      onMounted(() => page.view.run(() => page.load?.()));
      onBeforeUnmount(() => {
        props.context.clearHooks(hooks);
        page.dispose?.();
      });
      return () => h(
        'div',
        { class: 'page-content', 'aria-busy': page.view.task.busy },
        [
          page.view.task.message ? h(
            'div',
            {
              class: ['notice', page.view.task.error && 'error'],
              role: page.view.task.error ? 'alert' : 'status'
            },
            [
              page.view.task.message,
              page.view.task.code === 'REVISION_CONFLICT' ? h('p', '当前草稿已保留。请先复制需要保留的内容，再重新读取最新版本合并。') : null,
              page.view.task.code === 'RESULT_UNKNOWN' ? h('p', '游戏端执行结果未知，请在服务器确认后再决定是否重试。') : null
            ]
          ) : null,
          h('fieldset', { class: 'page-fields', disabled: page.view.task.busy }, page.render())
        ]
      );
    }
  });
}
