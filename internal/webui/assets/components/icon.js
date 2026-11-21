import { h } from '../shared/vue.js';

// Small original line icons; no external icon/font service is required.
const paths = {
  dashboard: 'M3 3h7v7H3z M14 3h7v7h-7z M3 14h7v7H3z M14 14h7v7h-7z',
  server: 'M3 3h18v7H3z M3 14h18v7H3z M6 6h2 M6 17h2 M16 6h2 M16 17h2',
  plug: 'M8 3v5 M16 3v5 M6 8h12v3a6 6 0 0 1-12 0z M12 17v4',
  route: 'M3 5h12v6H7v8h14 M18 16l3 3-3 3 M3 2v6',
  users: 'M8 3a4 4 0 1 0 0 8 4 4 0 0 0 0-8 M2 21v-3a6 6 0 0 1 12 0v3 M17 4a4 4 0 0 1 0 8 M17 15a5 5 0 0 1 5 5',
  link: 'M10 7l2-2a5 5 0 0 1 7 7l-2 2 M14 17l-2 2a5 5 0 0 1-7-7l2-2 M8 16l8-8',
  terminal: 'M3 4h18v16H3z M6 8l4 4-4 4 M13 16h5',
  box: 'M3 7l9-5 9 5v10l-9 5-9-5z M3 7l9 5 9-5 M12 12v10',
  image: 'M3 3h18v18H3z M3 17l6-6 4 4 3-3 5 5 M7 7h1',
  settings: 'M4 6h16 M4 12h16 M4 18h16 M8 3v6 M16 9v6 M10 15v6',
  activity: 'M2 12h5l3-8 4 16 3-8h5',
  download: 'M12 2v13 M7 10l5 5 5-5 M3 16v5h18v-5',
  info: 'M12 11v6 M12 7h.01 M12 2a10 10 0 1 0 0 20 10 10 0 0 0 0-20',
  menu: 'M3 6h18 M3 12h18 M3 18h18',
  search: 'M10 3a7 7 0 1 0 0 14 7 7 0 0 0 0-14 M15 15l6 6',
  sun: 'M12 8a4 4 0 1 0 0 8 4 4 0 0 0 0-8 M12 1v3 M12 20v3 M1 12h3 M20 12h3 M4 4l2 2 M18 18l2 2 M4 20l2-2 M18 6l2-2',
};

export function icon(name) {
  return h(
    'svg',
    {
      viewBox: '0 0 24 24',
      width: 20,
      height: 20,
      fill: 'none',
      stroke: 'currentColor',
      'stroke-width': 1.6,
      'stroke-linecap': 'round',
      'stroke-linejoin': 'round',
      'aria-hidden': 'true'
    },
    [h('path', { d: paths[name] || paths.box })]
  );
}
