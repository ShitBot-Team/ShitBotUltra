let token = '';
let generation = 0;
let unauthorized = () => {};
const pending = new Set();

export class ApiError extends Error {
constructor(message, code = '', status = 0) {
  super(message);
  this.name = 'ApiError';
  this.code = code;
  this.status = status;
}
}

export function setToken(value) {
  generation++;
  token = value;
  for (const controller of pending) controller.abort();
  pending.clear();
}

export function onUnauthorized(callback) {
  unauthorized = callback;
}

export const sessionGeneration = () => generation;
// Only relative paths under the same management origin may carry credentials.

export async function request(path, options = {}) {
  if (!path.startsWith('/api/v1/') || path.includes('\\')) throw new ApiError('管理接口路径无效');
  const epoch = generation;
  const controller = new AbortController();
  const abort = () => controller.abort();
  options.signal?.addEventListener('abort', abort, { once: true });
  if (options.signal?.aborted) abort();
  pending.add(controller);
  const timeout = setTimeout(abort, 150000);
  try {
    const headers = { Authorization: 'Bearer ' + token };
    let body;
    if (options.upload) body = options.upload;
    else if (options.body !== undefined) {
      headers['Content-Type'] = 'application/json';
      body = JSON.stringify(options.body);
    }
    if (options.revision) headers['If-Match'] = JSON.stringify(options.revision);
    const response = await fetch(path, {
        method: options.method || 'GET',
        body,
        headers,
        credentials: 'omit',
        redirect: 'error',
        cache: 'no-store',
        signal: controller.signal
    });
    if (epoch !== generation || controller.signal.aborted) throw new DOMException('会话已结束', 'AbortError');
    if (response.status === 401) {
      unauthorized();
      throw new ApiError('管理凭据已失效，请重新连接', 'UNAUTHORIZED', 401);
    }
    if (!response.ok) {
      const data = await response.json().catch(() => null);
      if (epoch !== generation || controller.signal.aborted) throw new DOMException('请求已取消', 'AbortError');
      throw new ApiError(data?.error?.message || ('请求失败：HTTP ' + response.status), data?.error?.code, response.status);
    }
    const result = response.status === 204 ? null : options.binary ? await response.blob() : await response.json();
    if (epoch !== generation || controller.signal.aborted) throw new DOMException('请求已取消', 'AbortError');
    return result;
  } catch (error) {
    if (error instanceof TypeError) throw new ApiError('网络请求失败。若正在修改数据或发送动作，请先刷新状态确认结果，再决定是否重试。', 'TRANSPORT_ERROR');
    throw error;
  } finally {
    clearTimeout(timeout);
    pending.delete(controller);
    options.signal?.removeEventListener('abort', abort);
  }
}

export function scopedClient(signal) {
  return (path, options = {}) => request('/api/v1/' + path, { ...options, signal });
}
// Stream transport shares token cancellation and 401 handling with regular API
// requests. It never puts a credential into EventSource URLs or browser storage.

export async function eventResponse(signal, lastID) {
  const epoch = generation;
  const controller = new AbortController();
  const abort = () => controller.abort();
  signal.addEventListener('abort', abort, { once: true });
  if (signal.aborted) abort();
  pending.add(controller);
  const release = () => {
    controller.abort();
    pending.delete(controller);
    signal.removeEventListener('abort', abort);
  };
  try {
    const headers = { Authorization: 'Bearer ' + token };
    if (lastID) headers['Last-Event-ID'] = lastID;
    const response = await fetch('/api/v1/events', {
      headers,
      credentials: 'omit',
      cache: 'no-store',
      redirect: 'error',
      signal: controller.signal
    });
    if (epoch !== generation || controller.signal.aborted) throw new DOMException('会话已结束', 'AbortError');
    if (response.status === 401) {
      unauthorized();
      throw new ApiError('管理凭据失效', 'UNAUTHORIZED', 401);
    }
    if (!response.ok || !response.body) throw new ApiError('事件连接不可用');
    return { response, release };
  } catch (error) {
    release();
    throw error;
  }
}
