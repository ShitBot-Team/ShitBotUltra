let token = '';
let generation = 0;
let unauthorized = () => {};
const pending = new Set();

export function setToken(value) {
  generation++;
  token = value;
  for (const controller of pending) controller.abort();
  pending.clear();
}
export function onUnauthorized(callback) { unauthorized = callback; }

export async function api(path, options = {}) {
  if (!path.startsWith('/api/v1/')) throw new Error('管理接口路径无效');
  const epoch = generation;
  const controller = new AbortController();
  pending.add(controller);
  const timeout = setTimeout(() => controller.abort(), 150000);
  try {
    const headers = { Authorization: 'Bearer ' + token };
    let body;
    if (options.upload) body = options.upload;
    else if (options.body !== undefined) {
      headers['Content-Type'] = 'application/json';
      body = JSON.stringify(options.body);
    }
    if (options.revision) headers['If-Match'] = JSON.stringify(options.revision);
    const response = await fetch(path, { method: options.method || 'GET', body, headers,
      credentials: 'omit', redirect: 'error', cache: 'no-store', signal: controller.signal });
    if (epoch !== generation) throw new Error('会话已结束');
    if (response.status === 401) { unauthorized(); throw new Error('管理凭据已失效，请重新连接'); }
    if (!response.ok) {
      const data = await response.json().catch(() => null);
      throw new Error(data?.error?.message || ('请求失败：HTTP ' + response.status));
    }
    if (response.status === 204) return null;
    const result = options.binary ? await response.blob() : await response.json();
    if (epoch !== generation) throw new Error('会话已结束');
    return result;
  } catch (error) {
    if (error.name === 'AbortError') throw new Error('请求超时或已取消');
    throw error;
  } finally {
    clearTimeout(timeout);
    pending.delete(controller);
  }
}

// EventSource cannot send an Authorization header. Use fetch streaming so the
// credential never appears in a URL, browser history or server access log.
export function stream(onEvent, onStatus) {
  const epoch = generation;
  const controller = new AbortController();
  pending.add(controller);
  let lastID = '';
  const pause = () => new Promise(resolve => {
    const stop = () => { clearTimeout(timer); controller.signal.removeEventListener('abort', stop); resolve(); };
    const timer = setTimeout(stop, 3000);
    controller.signal.addEventListener('abort', stop, { once: true });
    if (controller.signal.aborted) stop();
  });
  (async () => {
    while (!controller.signal.aborted && epoch === generation) {
      let reader;
      try {
        onStatus('连接中');
        const headers = { Authorization: 'Bearer ' + token };
        if (lastID) headers['Last-Event-ID'] = lastID;
        const response = await fetch('/api/v1/events', { headers, credentials: 'omit', cache: 'no-store', redirect: 'error', signal: controller.signal });
        if (response.status === 401) { unauthorized(); break; }
        if (!response.ok || !response.body) throw new Error('事件连接不可用');
        onStatus('已连接');
        reader = response.body.getReader();
        const decoder = new TextDecoder();
        let buffer = '';
        while (!controller.signal.aborted) {
          const { done, value } = await reader.read();
          if (done) break;
          buffer += decoder.decode(value, { stream: true });
          if (buffer.length > 2 * 1024 * 1024) throw new Error('事件过大');
          let end;
          while ((end = buffer.indexOf('\n\n')) >= 0) {
            const block = buffer.slice(0, end); buffer = buffer.slice(end + 2);
            let data = '';
            for (const line of block.split('\n')) {
              if (line.startsWith('id: ')) lastID = line.slice(4);
              if (line.startsWith('data: ')) data += line.slice(6);
            }
            if (data && epoch === generation) onEvent(JSON.parse(data));
          }
        }
      } catch (_) {
        if (!controller.signal.aborted && epoch === generation) onStatus('等待重连');
      } finally {
        if (reader) await reader.cancel().catch(() => {});
      }
      if (!controller.signal.aborted && epoch === generation) await pause();
    }
    pending.delete(controller);
  })();
  return () => { controller.abort(); pending.delete(controller); };
}

export function downloadJSON(name, data) {
  const link = document.createElement('a');
  const url = URL.createObjectURL(new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' }));
  link.href = url; link.download = name; link.click();
  setTimeout(() => URL.revokeObjectURL(url), 5000);
}
