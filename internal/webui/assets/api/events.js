import { eventResponse, sessionGeneration } from './client.js';

export function stream(onEvent, onStatus) {
  const epoch = sessionGeneration();
  const controller = new AbortController();
  const current = () => !controller.signal.aborted && epoch === sessionGeneration();
  let lastID = '';
  function pause() {
    return new Promise(resolve => {
      const stop = () => {
        clearTimeout(timer);
        controller.signal.removeEventListener('abort', stop);
        resolve();
      };
      const timer = setTimeout(stop, 3000);
      controller.signal.addEventListener('abort', stop, { once: true });
      if (!current()) stop();
    });
  }
  (async () => {
    while (current()) {
      let transport, reader;
      let watchdog;
      const attempt = new AbortController();
      const abort = () => attempt.abort();
      controller.signal.addEventListener('abort', abort, { once: true });
      const resetWatchdog = () => {
        clearTimeout(watchdog);
        watchdog = setTimeout(abort, 45000);
      };
      try {
        onStatus('连接中');
        resetWatchdog();
        transport = await eventResponse(attempt.signal, lastID);
        if (!current()) break;
        onStatus('已连接');
        reader = transport.response.body.getReader();
        const decoder = new TextDecoder();
        let buffer = '';
        while (current()) {
          const { done, value } = await reader.read();
          if (done) break;
          resetWatchdog();
          buffer += decoder.decode(value, { stream: true });
          if (buffer.length > 2 * 1024 * 1024) throw new Error('事件过大');
          // Core emits LF; normalize CRLF as well, without losing partial lines.
          buffer = buffer.replace(/\r\n/g, '\n');
          let end;
          while ((end = buffer.indexOf('\n\n')) >= 0) {
            const block = buffer.slice(0, end);
            buffer = buffer.slice(end + 2);
            let id = lastID;
            const data = [];
            for (const line of block.split('\n')) {
              if (line.startsWith('id:')) id = line.slice(3).trimStart();
              if (line.startsWith('data:')) data.push(line.slice(5).trimStart());
            }
            if (data.length && current()) onEvent(JSON.parse(data.join('\n')));
            lastID = id;
          }
        }
      } catch (error) {
        if (error.code === 'UNAUTHORIZED') break;
      } finally {
        clearTimeout(watchdog);
        transport?.release();
        controller.signal.removeEventListener('abort', abort);
        if (reader) await reader.cancel().catch(() => {});
      }
      if (current()) {
        onStatus('等待重连');
        await pause();
      }
    }
  })();
  return () => controller.abort();
}
