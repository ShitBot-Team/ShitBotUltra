export const json = value => JSON.stringify(value, null, 2);

export const clone = value => JSON.parse(JSON.stringify(value));

export const date = value => !value || String(value).startsWith('0001-') ? '—' : new Date(value).toLocaleString('zh-CN');

export const duration = seconds => seconds < 3600 ? Math.floor(seconds / 60) + ' 分钟' : (seconds / 3600).toFixed(1) + ' 小时';

export const esc = encodeURIComponent;

export function safeLink(value) {
  try {
    const url = new URL(value);
    return ['https:', 'http:'].includes(url.protocol) ? url.href : undefined;
  }
  catch {
    return undefined;
  }
}

export function downloadJSON(name, data) {
  const link = document.createElement('a');
  const url = URL.createObjectURL(new Blob([json(data)], { type: 'application/json' }));
  link.href = url;
  link.download = name;
  link.click();
  setTimeout(() => URL.revokeObjectURL(url), 5000);
}
