import { h } from '../shared/vue.js';
import { json } from '../shared/format.js';

export const help = text => h('p', { class: 'muted' }, text);

export const row = children => h('div', { class: 'actions' }, children);

export const grid = children => h('div', { class: 'form-grid' }, children);

export const section = (title, children, subtitle) => h('section', { class: 'card' }, [
  h('div', { class: 'card-heading' }, [h('h2', title), subtitle ? help(subtitle) : null]),
  ...children
]);

export const pre = value => h('pre', { class: 'output' }, typeof value === 'string' ? value : json(value));

export const badge = (label, tone = '') => h('span', { class: ['badge', tone] }, label);

export const empty = (title, text) => h('div', { class: 'empty-state' }, [h('strong', title), help(text)]);

export function field(label, value, change, options = {}) {
  return h(
    'label',
    { class: 'field' },
    [
      h('span', label),
      h(
        'input',
        {
          type: options.type || 'text',
          value,
          placeholder: options.placeholder || '',
          readonly: !!options.readonly,
          min: options.min,
          max: options.max,
          step: options.step,
          autocomplete: options.type === 'password' ? 'new-password' : 'off',
          onInput: event => change(options.type === 'number' ? (event.target.value === '' ? '' : Number(event.target.value)) : event.target.value)
        }
      ),
      options.hint ? h('small', options.hint) : null
    ]
  );
}

export function area(label, value, change, lines = 8) {
  return h('label', { class: 'field' }, [
      h('span', label),
      h('textarea', {
        value,
        rows: lines,
        spellcheck: false,
        onInput: event => change(event.target.value)
      })
  ]);
}

export function select(label, value, items, change, placeholder = '请选择') {
  return h(
    'label',
    { class: 'field' },
    [
      h('span', label),
      h(
        'select',
        {
        value, onChange: event => change(event.target.value)
        },
        [
          h('option', { value: '' }, placeholder),
          ...items.map(item => h('option', {
          key: item.value ?? item.id, value: item.value ?? item.id
          }, item.label ?? item.id))
        ]
      )
    ]
  );
}

export function check(label, value, change, disabled = false) {
  return h(
    'label',
    { class: 'check' },
    [
      h('input', {
        type: 'checkbox',
        checked: value,
        disabled,
        onChange: event => change(event.target.checked)
      }),
      h('span', label)
    ]
  );
}

export function choices(label, values, items, change) {
  return h(
    'div',
    { class: 'field' },
    [
      h('span', label),
      h(
        'div',
        { class: 'choice-list' },
        items.map(item => {
          const id = item.value ?? item.id;
          return check(item.label ?? id, (values || []).includes(id), checked => change(checked ? [...(values || []), id] : values.filter(v => v !== id)));
        })
      )
    ]
  );
}

export function table(headers, rows, noData = '暂无记录') {
  return h(
    'div',
    { class: 'table-wrap' },
    [
      h(
        'table',
        [
          h('thead', [h('tr', headers.map(label => h('th', { scope: 'col' }, label)))]),
          h(
            'tbody',
            rows.length ? rows.map((cells, index) => h('tr', { key: index }, cells.map(value => h('td', value ?? '—'))))
            : [h('tr', [h('td', { colspan: headers.length, class: 'empty' }, noData)])]
          )
        ]
      )
    ]
  );
}

export function controls(task, run) {
  function button(label, action, options = {}) {
    return h(
      'button',
      {
        type: 'button',
        class: [options.danger ? 'danger' : options.primary ? 'primary' : '', options.class],
        disabled: task.busy || !!options.disabled,
        onClick: () => run(action),
        title: options.title
      },
      label
    );
  }
  function upload(label, accept, multiple, action) {
    return h(
      'label',
      { class: 'upload' },
      [
        h('strong', label),
        h('span', '选择本地文件'),
        h(
          'input',
          {
            type: 'file',
            accept,
            multiple,
            disabled: task.busy,
            onChange: event => {
              const files = Array.from(event.target.files || []);
              event.target.value = '';
              if (files.length) run(() => action(files));
            }
          }
        )
      ]
    );
  }
  function pager(offset, length, change) {
    return row([
      button('上一页', () => change(Math.max(0, offset - 100)), { disabled: offset === 0 }),
      help('第 ' + (Math.floor(offset / 100) + 1) + ' 页'),
      button('下一页', () => change(offset + 100), { disabled: length < 100 })
    ]);
  }
  return { button, upload, pager };
}
