// Mockups of the work-surface concept. Plain functions returning HTML strings; every
// value is a token class from mock.css. The frame (app bar, sidebar, phone bars) is drawn as the
// navigation concept built it and is not changed by this concept.
(function () {
  'use strict';
  const P = {
    sun: '<circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/>',
    inbox: '<path d="M22 12h-6l-2 3h-4l-2-3H2"/><path d="M5.5 5.1 2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.5-6.9A2 2 0 0 0 16.8 4H7.2a2 2 0 0 0-1.7 1.1Z"/>',
    shapes: '<circle cx="7" cy="7" r="4"/><rect x="13" y="13" width="8" height="8" rx="1"/><path d="M14 4h7v7h-7z"/><path d="M3 21l4-7 4 7z"/>',
    usercheck: '<circle cx="9" cy="7" r="4"/><path d="M3 21v-2a4 4 0 0 1 4-4h4"/><path d="m16 19 2 2 4-4"/>',
    search: '<circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/>',
    plus: '<path d="M12 5v14M5 12h14"/>',
    star: '<path d="m12 3 2.8 5.7 6.2.9-4.5 4.4 1 6.2L12 17.3 6.5 20.2l1-6.2L3 9.6l6.2-.9Z"/>',
    chev: '<path d="m9 6 6 6-6 6"/>', chevd: '<path d="m6 9 6 6 6-6"/>', chevl: '<path d="m15 6-6 6 6 6"/>',
    more: '<circle cx="5" cy="12" r="1"/><circle cx="12" cy="12" r="1"/><circle cx="19" cy="12" r="1"/>',
    calendar: '<rect x="3" y="5" width="18" height="16" rx="2"/><path d="M3 10h18M8 3v4M16 3v4"/>',
    book: '<path d="M2 4h7a3 3 0 0 1 3 3v14a2 2 0 0 0-2-2H2z"/><path d="M22 4h-7a3 3 0 0 0-3 3v14a2 2 0 0 1 2-2h8z"/>',
    board: '<rect x="3" y="3" width="18" height="18" rx="2"/><path d="M3 9h18M9 21V9"/>',
    clock: '<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>',
    flag: '<path d="M4 22V4M4 4h13l-2 4 2 4H4"/>',
    zap: '<path d="M13 2 4 14h7l-1 8 9-12h-7z"/>',
    shield: '<path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10Z"/>',
    gear: '<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.8.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.8V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1Z"/>',
    archive: '<rect x="2" y="4" width="20" height="5" rx="1"/><path d="M4 9v10a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9M10 13h4"/>',
    trash: '<path d="M3 6h18M8 6V4h8v2M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6"/>',
    history: '<path d="M3 12a9 9 0 1 0 3-6.7L3 8"/><path d="M3 3v5h5M12 7v5l4 2"/>',
    rocket: '<path d="M4.5 16.5c-1.5 1.3-2 5-2 5s3.7-.5 5-2c.7-.8.7-2.1-.1-2.9a2.2 2.2 0 0 0-2.9-.1Z"/><path d="m12 15-3-3a22 22 0 0 1 2-4A13 13 0 0 1 22 2c0 2.7-.8 7.5-6 11a22 22 0 0 1-4 2Z"/>',
    hub: '<rect x="3" y="3" width="18" height="18" rx="3"/><rect x="8" y="8" width="8" height="8" rx="1"/>',
    home: '<path d="m3 10 9-7 9 7v10a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2Z"/>',
    brief: '<rect x="2" y="7" width="20" height="14" rx="2"/><path d="M16 7V5a2 2 0 0 0-2-2h-4a2 2 0 0 0-2 2v2"/>',
    bug: '<rect x="8" y="6" width="8" height="14" rx="4"/><path d="M19 7l-3 2M5 7l3 2M19 19l-3-2M5 19l3-2M20 13h-4M4 13h4M10 4l1 2M14 4l-1 2"/>',
    party: '<path d="M5.8 11.3 2 22l10.7-3.8"/><path d="M4 3h.01M22 8h.01M15 2h.01M22 20h.01M22 2l-2.2.8a3 3 0 0 0-1.8 3.4v0a1 1 0 0 1-1.2 1.2v0A3 3 0 0 0 13.4 9L12 11"/><path d="m11 13 4.5 4.5"/>',
    coll: '<rect x="8" y="8" width="13" height="13" rx="2"/><path d="M4 16V5a1 1 0 0 1 1-1h11"/>',
    check: '<path d="M20 6 9 17l-5-5"/>',
    checksq: '<rect x="3" y="3" width="18" height="18" rx="2"/><path d="m8 12 3 3 5-6"/>',
    msg: '<path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/>',
    link: '<path d="M10 13a5 5 0 0 0 7.5.5l3-3a5 5 0 0 0-7-7l-1.7 1.7"/><path d="M14 11a5 5 0 0 0-7.5-.5l-3 3a5 5 0 0 0 7 7l1.7-1.7"/>',
    clip: '<path d="m21 11-8.6 8.6a5.7 5.7 0 0 1-8-8L13 3a3.8 3.8 0 0 1 5.4 5.4l-8.5 8.5a1.9 1.9 0 0 1-2.7-2.7l7.9-7.9"/>',
    filter: '<path d="M3 4h18l-7 8v7l-4 2v-9z"/>',
    group: '<rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="18" height="7" rx="1"/>',
    cols: '<rect x="3" y="3" width="18" height="18" rx="2"/><path d="M9 3v18M15 3v18"/>',
    sort: '<path d="m3 8 4-4 4 4M7 4v16M21 16l-4 4-4-4M17 20V4"/>',
    list: '<path d="M8 6h13M8 12h13M8 18h13M3 6h.01M3 12h.01M3 18h.01"/>',
    timeline: '<path d="M3 6h10M7 12h12M5 18h8"/>',
    play: '<path d="m6 3 14 9-14 9z"/>', pause: '<path d="M7 4h3v16H7zM14 4h3v16h-3z"/>',
    alert: '<path d="M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0Z"/><path d="M12 9v4M12 17h.01"/>',
    info: '<circle cx="12" cy="12" r="9"/><path d="M12 16v-4M12 8h.01"/>',
    user: '<circle cx="12" cy="8" r="4"/><path d="M4 21a8 8 0 0 1 16 0"/>',
    users: '<circle cx="9" cy="8" r="4"/><path d="M2 21a7 7 0 0 1 14 0M16 3.1a4 4 0 0 1 0 7.8M22 21a7 7 0 0 0-5-6.7"/>',
    side: '<rect x="3" y="3" width="18" height="18" rx="2"/><path d="M9 3v18"/>',
    expand: '<path d="M15 3h6v6M9 21H3v-6M21 3l-7 7M3 21l7-7"/>',
    x: '<path d="M18 6 6 18M6 6l12 12"/>',
    send: '<path d="m22 2-7 20-4-9-9-4z"/><path d="M22 2 11 13"/>',
    lock: '<rect x="4" y="11" width="16" height="10" rx="2"/><path d="M8 11V7a4 4 0 0 1 8 0v4"/>',
    eye: '<path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12Z"/><circle cx="12" cy="12" r="3"/>',
    snippet: '<path d="M4 6h16M4 12h10M4 18h7"/>', macro: '<path d="M12 2v4M12 18v4M4.9 4.9l2.8 2.8M16.3 16.3l2.8 2.8M2 12h4M18 12h4"/>',
    bold: '<path d="M6 4h8a4 4 0 0 1 0 8H6zM6 12h9a4 4 0 0 1 0 8H6z"/>', italic: '<path d="M19 4h-9M14 20H5M15 4 9 20"/>',
    h: '<path d="M4 12h8M4 18V6M12 18V6M17 12l3-2v8"/>', quote: '<path d="M3 21c3 0 7-1 7-8V5H3v7h4M14 21c3 0 7-1 7-8V5h-7v7h4"/>',
    code: '<path d="m16 18 6-6-6-6M8 6l-6 6 6 6"/>', table: '<rect x="3" y="3" width="18" height="18" rx="2"/><path d="M3 9h18M3 15h18M12 3v18"/>',
    image: '<rect x="3" y="3" width="18" height="18" rx="2"/><circle cx="9" cy="9" r="2"/><path d="m21 15-5-5L5 21"/>',
    server: '<rect x="2" y="3" width="20" height="8" rx="2"/><rect x="2" y="13" width="20" height="8" rx="2"/><path d="M6 7h.01M6 17h.01"/>',
    globe: '<circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3a14 14 0 0 1 0 18M12 3a14 14 0 0 0 0 18"/>',
    key: '<circle cx="7.5" cy="15.5" r="4.5"/><path d="m10.7 12.3 9.8-9.8M17 6l3 3M14 9l2 2"/>',
    laptop: '<rect x="4" y="4" width="16" height="12" rx="1"/><path d="M2 20h20"/>',
    help: '<circle cx="12" cy="12" r="9"/><path d="M9.1 9a3 3 0 0 1 5.8 1c0 2-3 3-3 3M12 17h.01"/>',
    sparkle: '<path d="m12 3 1.9 5.8L20 11l-6.1 2.2L12 19l-1.9-5.8L4 11l6.1-2.2Z"/>',
    target: '<circle cx="12" cy="12" r="9"/><circle cx="12" cy="12" r="5"/><circle cx="12" cy="12" r="1"/>',
    trend: '<path d="m22 7-8.5 8.5-5-5L2 17"/><path d="M16 7h6v6"/>',
    swipe: '<path d="M5 12h14M13 6l6 6-6 6"/>', grip: '<circle cx="9" cy="6" r="1"/><circle cx="15" cy="6" r="1"/><circle cx="9" cy="12" r="1"/><circle cx="15" cy="12" r="1"/><circle cx="9" cy="18" r="1"/><circle cx="15" cy="18" r="1"/>',
    prio: '<path d="M12 19V5M5 12l7-7 7 7"/>', prio2: '<path d="M5 12h14"/>', prio3: '<path d="M12 5v14M5 12l7 7 7-7"/>',
    pin: '<path d="M12 17v5M9 10.8V4h6v6.8l3 3.2H6z"/>'
  };
  const ic = (n, cls = '') => `<svg class="ic ${cls}" viewBox="0 0 24 24" aria-hidden="true">${P[n] || ''}</svg>`;
  const PROD = {
    plans: { n: 'Pläne', i: 'calendar', p: 'orange' }, pages: { n: 'Seiten', i: 'book', p: 'teal' },
    whiteboards: { n: 'Whiteboards', i: 'board', p: 'amber' }, desk: { n: 'Servicedesk', i: 'inbox', p: 'magenta' },
    assets: { n: 'Inventar', i: 'shield', p: 'slate' }, time: { n: 'Zeit', i: 'clock', p: 'lime' },
    goals: { n: 'Ziele', i: 'flag', p: 'green' }, agents: { n: 'Agenten', i: 'zap', p: 'violet' },
    work: { n: 'Aufgaben', i: 'checksq', p: 'indigo' }
  };
  const tile = (p, size = '') => { const x = PROD[p] || { i: p, p: 'blue' }; return `<span class="tile ${size}" data-p="${x.p}">${ic(x.i)}</span>`; };
  const htile = (icon, color, size = '') => `<span class="tile ${size}" data-p="${color}">${ic(icon)}</span>`;
  const key = (p, k) => `<span class="key">${tile(p)}${k}</span>`;
  const loz = (t, tone = 'neu', icon = '', extra = '') => `<span class="loz t-${tone} ${extra}">${icon ? ic(icon) : ''}${t}</span>`;
  const lab = (t, c) => `<span class="lab" data-p="${c}">${t}</span>`;
  const PEOPLE = { JK: 'blue', AH: 'violet', MY: 'teal', SR: 'orange', PN: 'green', JW: 'amber', TB: 'slate', CN: 'magenta', LB: 'lime', HA: 'red' };
  const av = (i, s = '') => `<span class="av ${s}" data-p="${PEOPLE[i] || 'slate'}">${i}</span>`;
  const avs = (list, s = 's') => `<span class="avs">${list.map((i) => av(i, s)).join('')}</span>`;
  const chip = (t, icon = 'calendar', cls = '') => `<span class="chip ${cls}">${icon ? ic(icon, 's') : ''}${t}</span>`;
  const btn = (t, cls = '', icon = '') => `<span class="btn ${cls}">${icon ? ic(icon) : ''}${t}</span>`;
  const pin = (n) => `<span class="pin">${n}</span>`;
  const note = (n, html, style = '') => `<div class="note" style="${style}">${pin(n)}${html}</div>`;
  const bar = (pct, cls = '') => `<span class="bar ${cls}"><i style="inline-size:${pct}%"></i></span>`;

  // ---------- frame, as built by the navigation concept ----------
  const HUBS = [['Allgemein', 'hub', 'blue'], ['Produktentwicklung', 'rocket', 'blue'], ['Marketing & Kommunikation', 'hub', 'violet'], ['Neubau Logistikzentrum Hamburg-Billbrook', 'hub', 'orange'],
    ['IT-Service-Desk', 'hub', 'teal'], ['Zuhause', 'home', 'green'], ['Team-Wiki', 'book', 'teal'], ['Kundenprojekt Stadtwerke', 'brief', 'orange'], ['Fehlerverfolgung', 'bug', 'red'], ['Onboarding', 'hub', 'blue'], ['Sommerfest 2027', 'party', 'magenta']];
  const nr = (icon, name, opts = {}) => `<div class="mk-nr ${opts.cur ? 'cur' : ''} ${opts.sub ? 'sub' : ''}">${opts.raw || ic(icon)}<span class="n">${name}</span>${opts.fig ? `<em class="${opts.figc || ''}">${opts.fig}</em>` : ''}${opts.chev ? ic('chev', 's') : ''}</div>`;
  function sideWS(cur) {
    const prods = ['plans', 'pages', 'whiteboards', 'desk', 'assets', 'time', 'goals', 'agents'];
    return `<nav class="mk-side">${nr('sun', 'Heute', { cur: cur === 'today', fig: '2' })}${nr('shapes', 'Eingang', { cur: cur === 'inbox' })}${nr('usercheck', 'Wartet auf dein Ja')}<hr>
      ${nr('history', 'Zuletzt', { chev: 1 })}<div class="mk-sh">${ic('chevd', 's')}Produkte</div>
      ${prods.map((p) => nr('', PROD[p].n, { raw: tile(p), cur: cur === p })).join('')}
      <div class="mk-sh">${ic('chevd', 's')}Hubs</div>${HUBS.map(([n, i, c]) => nr('', n, { raw: htile(i, c), chev: 1 })).join('')}
      <div class="foot"><hr>${nr('gear', 'Verwaltung')}${nr('archive', 'Archiv')}${nr('trash', 'Papierkorb')}</div></nav>`;
  }
  const HUBNAV = {
    pe: { name: 'Produktentwicklung', i: 'rocket', c: 'blue', tools: [['hub', 'Übersicht'], ['calendar', 'Plan', 'Sprint 24'], ['shield', 'Inventar'], ['clock', 'Zeit'], ['flag', 'Ziele', '1 gefährdet']],
      colls: [['Team Atlas – Mobile App', '7 T'], ['Team Boreal – Platform & API', '4 offen']], contents: ['Retro Sprint 23', 'Anmeldefluss mit Passkey', 'Handbuch Produktentwicklung'] },
    it: { name: 'IT-Service-Desk', i: 'hub', c: 'teal', queues: [['Meine', '1', 'd'], ['Nicht zugewiesen', '6'], ['Kurz vor Verletzung', '1', 'd'], ['Unzufrieden', '0']],
      tools: [['hub', 'Übersicht'], ['calendar', 'Plan'], ['shield', 'Inventar', '2 !', 'd'], ['clock', 'Zeit'], ['flag', 'Ziele']],
      colls: [['Störungen & Anfragen', '1 SLA', 'd'], ['Changes', '3 offen'], ['Probleme', '1 offen']], contents: ['So arbeiten wir mit diesem Desk', 'Wissensdatenbank', 'Wissensdatenbank Servicedesk'] },
    nb: { name: 'Neubau Logistikzentrum', i: 'hub', c: 'orange', tools: [['hub', 'Übersicht'], ['calendar', 'Plan'], ['shield', 'Inventar'], ['clock', 'Zeit'], ['flag', 'Ziele', '1 gefährdet']],
      colls: [['Projektplan', '21 %'], ['Risiken & Offene Punkte', '16 %']], contents: ['Protokoll Lenkungskreis 12.09.'] }
  };
  function sideHub(h, cur) {
    const H = HUBNAV[h];
    const tools = H.tools.map(([i, n, f, fc], k) => {
      let s = nr(i, n, { cur: cur === n, fig: f, figc: fc });
      if (k === 0 && H.queues) s += nr('inbox', 'Warteschlangen') + H.queues.map(([q, f2, c2]) => nr('board', q, { sub: 1, fig: f2, figc: c2, cur: cur === q })).join('');
      return s;
    }).join('');
    return `<nav class="mk-side">${nr('sun', 'Heute', { fig: '2' })}${nr('shapes', 'Eingang')}${nr('usercheck', 'Wartet auf dein Ja')}<hr>
      ${nr('chevl', 'Alle Hubs')}<div class="mk-nr semi">${htile(H.i, H.c)}<span class="n">${H.name}</span>${ic('sort', 's')}${ic('star', 's')}${ic('more', 's')}</div>${tools}
      <div class="mk-sh">${ic('chevd', 's')}Sammlungen</div>${H.colls.map(([n, f, fc]) => nr('coll', n, { cur: cur === n, fig: f, figc: fc })).join('')}
      <div class="mk-sh">${ic('chevd', 's')}Inhalte · ${H.contents.length}</div>${H.contents.map((n) => nr('book', n, { cur: cur === n })).join('')}
      <div class="foot"><hr>${nr('gear', 'Verwaltung')}${nr('archive', 'Archiv')}${nr('trash', 'Papierkorb')}</div></nav>`;
  }
  const appbar = (menu = false) => `<header class="mk-bar">${ic(menu ? 'list' : 'side')}<span class="mk-logo"><i></i>hubtask</span><span class="mk-search">${ic('search')}Suchen oder einen Befehl ausführen<kbd>⌘K</kbd></span><span class="mk-capture">${ic('plus')}Erfassen</span><span class="mk-barend">${ic('help')}<span class="mk-sync">${ic('check')}</span>${av('JK')}</span></header>`;
  const desk = (side, main) => `<div class="mk mk-app">${appbar()}<div class="mk-body">${side}<main class="mk-main">${main}</main></div></div>`;
  const tab = (main) => `<div class="mk mk-app">${appbar(true)}<main class="mk-main" style="padding:var(--sp-300)">${main}</main></div>`;
  const BOTTOM = [['sun', 'Heute'], ['board', 'Durchsuchen'], ['plus', 'Erfassen'], ['shapes', 'Eingang'], ['search', 'Suche']];
  const phone = (top, main, cur = 'Durchsuchen', extra = '') => `<div class="mk mk-phone"><header class="mk-ptop">${top}</header><main class="mk-pmain">${main}${extra}</main>
    <nav class="mk-bottom">${BOTTOM.map(([i, n]) => `<div class="${n === cur ? 'cur' : ''} ${n === 'Erfassen' ? 'cap' : ''}">${ic(i)}${n}</div>`).join('')}</nav></div>`;
  const ptop = (back, title, end = `${ic('star')}${ic('more')}`) => `${back ? `<span class="back">${ic('chevl')}${back}</span>` : av('JK')}<span class="t">${title}</span><span class="mk-sync" style="inline-size:28px;block-size:28px">${ic('check', 's')}</span>${end}`;

  // ---------- shared compositions of the concept ----------
  const productHead = (p, title, fig, acts, trail) => `<div class="ph"><div class="trail">${trail || `Produkte ${ic('chev')} ${PROD[p].n}`}</div>
    <div class="top"><div class="band">${tile(p, 'l')}<div class="col" style="gap:2px"><h1 class="h">${title}</h1><div class="fig">${fig}</div></div></div><div class="acts">${acts}</div></div></div>`;
  const viewbar = (items, on) => `<div class="viewbar">${items.map((x) => `<span class="${x === on ? 'on' : ''}">${x}</span>`).join('')}</div>`;
  const wg = (p, title, body, foot = '', fig = '', cls = '') => `<section class="wg ${cls}"><header>${p ? tile(p) : ''}<h2>${title}</h2>${fig}</header>${body}${foot ? `<footer>${foot}</footer>` : ''}</section>`;
  const kpi = (label, value, sub = '', tone = '') => `<div class="kpi"><span class="cap sub">${label}</span><span class="big ${tone}">${value}</span>${sub ? `<span class="cap subtle">${sub}</span>` : ''}</div>`;

  // SLA cell: bar with words, never colour alone
  const sla = (pct, words, tone) => `<span class="row" style="gap:6px;min-inline-size:150px">${bar(pct, tone)}<span class="cap ${tone === 'bad' ? '' : 'sub'}" style="${tone === 'bad' ? 'color:var(--status-danger-text);font-weight:var(--fw-semibold)' : ''}">${tone === 'bad' ? ic('alert', 's') + ' ' : ''}${words}</span></span>`;
  const prio = (p) => ({ P1: `<span class="row cap" style="gap:4px;color:var(--status-danger-text)">${ic('prio', 's')}P1 Kritisch</span>`, P2: `<span class="row cap" style="gap:4px;color:var(--status-warning-text)">${ic('prio', 's')}P2 Hoch</span>`, P3: `<span class="row cap sub" style="gap:4px">${ic('prio2', 's')}P3 Mittel</span>`, P4: `<span class="row cap subtle" style="gap:4px">${ic('prio3', 's')}P4 Niedrig</span>` })[p];
  const ST = { neu: ['Neu', 'neu', ''], arbeit: ['In Arbeit', 'info', ''], wartet: ['Wartet auf Rückmeldung', 'warn', ''], geloest: ['Gelöst', 'ok', 'check'], todo: ['Zu erledigen', 'neu', ''], review: ['In Prüfung', 'brand', ''], done: ['Erledigt', 'ok', 'check'] };
  const st = (k, extra = '') => loz(ST[k][0], ST[k][1], ST[k][2], extra);

  const TICKETS = [
    ['SA-7', 'Geschäftsführung erreicht das Board-Portal nicht', 'Störung', 'Clara Neumann', 'arbeit', 'P1', [92, 'noch 3 Min.', 'bad'], 'JK'],
    ['SA-4', 'VPN-Gateway Frankfurt antwortet sporadisch nicht (Paketverlust 30 %)', 'Störung', 'Thomas Becker', 'arbeit', 'P2', [61, '1 Std. 40 Min.', 'warn'], 'MY'],
    ['SA-1', 'Mails an Kunden kommen eine Stunde zu spät an', 'Störung', 'Jonas Weber', 'neu', 'P2', [40, '3 Std. 10 Min.', ''], ''],
    ['SA-2', 'Der Drucker im zweiten Stock ist offline', 'Störung', 'Priya Natarajan', 'neu', 'P3', [22, '6 Std.', ''], ''],
    ['SA-3', 'Das VPN bricht alle paar Minuten ab', 'Störung', 'Sofia Rossi', 'neu', 'P3', [18, '7 Std.', ''], ''],
    ['SA-5', "Shared mailbox 'invoices@' stopped receiving mail from the ERP", 'Störung', 'Lukas Brandt', 'wartet', 'P2', [0, 'Angehalten', ''], 'SR'],
    ['SA-8', 'Seit dem Update lässt sich das Kundenportal in Firefox nicht mehr öffnen', 'Störung', 'Hannah Albers', 'arbeit', 'P3', [35, '1 Tag 2 Std.', ''], 'MY'],
    ['SA-9', 'Neuer Zugang für Auszubildende ab 1. Dezember (3 Personen)', 'Serviceanfrage', 'Amelia Hughes', 'neu', 'P4', [10, '4 Tage', ''], ''],
    ['SA-11', 'Lizenz für das Planungswerkzeug auf eine weitere Person übertragen', 'Serviceanfrage', 'Felix Hartmann', 'neu', 'P4', [8, '4 Tage', ''], ''],
    ['SA-10', 'Delivery notes are printed without the barcode since this morning', 'Störung', 'Grace O’Neill', 'geloest', 'P3', [100, 'Eingehalten', 'ok'], 'SR']
  ];
  const typeIcon = (t) => t === 'Störung' ? `<span class="row cap sub" style="gap:4px">${ic('alert', 's')}${t}</span>` : `<span class="row cap sub" style="gap:4px">${ic('inbox', 's')}${t}</span>`;

  // ======================================================================================
  // Scenes. Each returns { d, t?, m? } HTML. Pins (n) point at the numbered changes in the text.
  // ======================================================================================
  const S = {};

  // ---------- Heute ----------
  const todayFocus = (phoneMode) => {
    const r = (t, place, chipHtml, opts = {}) => `<div class="${phoneMode ? 'prow' : 'row'}" style="${phoneMode ? '' : 'padding:var(--sp-100) var(--sp-150);border-block-end:var(--bw-hairline) solid var(--border-subtle);gap:var(--sp-150)'}">
      <span class="check round"></span><div class="col grow" style="gap:2px"><span class="${phoneMode ? '' : 'ell'}" style="font-weight:var(--fw-medium)">${t}</span><span class="row cap sub" style="gap:6px">${place}</span></div>${chipHtml}${phoneMode ? '' : `<span class="subtle">${ic('more')}</span>`}</div>`;
    const g = (name, n, rows, tone = '') => `<div class="row" style="padding:var(--sp-150) var(--sp-150) var(--sp-050)"><h3 class="lbl" style="${tone}">${name}</h3><span class="cap subtle">${n}</span></div>${rows}`;
    return `${g('Überfällig', '1', r('Pressemitteilung zur Herbstmesse freigeben', `${htile('hub', 'violet')} Marketing & Kommunikation · Content-Board Q4`, chip('Seit Mo, 9. Nov.', 'alert', 'bad')), 'color:var(--status-danger-text)')}
      ${g('Heute', '1', r('Antwort an die Geschäftsführung zum Board-Portal', `${tile('desk')} SA-7 · Störungen & Anfragen`, chip('16:00', 'clock', 'warn')))}
      ${g('Demnächst', '1', r('Lenkungskreis: Freigabe der Ausführungsplanung', `${htile('hub', 'orange')} Neubau Logistikzentrum · Projektplan`, chip('Sa, 14. Nov.')))}
      ${g('Ohne Datum', '2', r('Passkey-Registrierung im Profil', `${htile('rocket', 'blue')} Produktentwicklung · Team Atlas`, '') + r('Webinar: Running ITIL change management', `${htile('hub', 'violet')} Marketing & Kommunikation`, ''))}`;
  };
  const sprintSpark = `<svg class="spark" viewBox="0 0 240 40" preserveAspectRatio="none"><path d="M0 4 L240 38" stroke="var(--border-default)" stroke-dasharray="3 3" fill="none"/><path d="M0 4 L40 4 L80 6 L120 6 L150 14" stroke="var(--label-orange-fg)" stroke-width="2" fill="none"/><circle cx="150" cy="14" r="3" fill="var(--label-orange-fg)"/></svg>`;
  const weekBars = (vals, today = 2) => `<div class="cap-bar">${vals.map((v, k) => `<i style="block-size:${Math.max(6, v * 12)}%;${k === today ? 'background:var(--label-lime-fg)' : ''}" title=""></i>`).join('')}</div><div class="row cap subtle" style="justify-content:space-between">${['Mo', 'Di', 'Mi', 'Do', 'Fr', 'Sa', 'So'].map((d) => `<span>${d}</span>`).join('')}</div>`;
  const todayWidgets = `
    ${note(4, wg('desk', 'Braucht deine Antwort', `<div class="row" style="gap:var(--sp-100)">${key('desk', 'SA-7')}<span class="ell grow">Geschäftsführung erreicht das Board-Portal nicht</span></div>${sla(92, 'Erste Antwort in 3 Min.', 'bad')}`, btn('Antworten', '', 'send') + btn('Warteschlange', 'ghost'), loz('1', 'bad')))}
    ${wg('time', 'Zeit diese Woche', `<div class="row" style="align-items:baseline;gap:var(--sp-100)"><span class="big">6:00</span><span class="cap sub">von 40:00 · heute 2:00</span></div>${weekBars([2, 2, 2, 0, 0, 0, 0])}`, btn('Timer starten', '', 'play') + btn('Meine Woche', 'ghost'))}
    ${wg('plans', 'Sprint 24 – Passkeys', `<div class="row cap sub" style="justify-content:space-between"><span>noch 6 Tage · bis 17. Nov.</span><span>0 von 8 Punkten</span></div>${sprintSpark}`, btn('Zum Sprint', 'ghost'))}
    ${wg('goals', 'Ziele H2 2026', `<div class="col" style="gap:var(--sp-100)"><div class="row cap"><span class="grow ell">Kundinnen lösen ihr Anliegen selbst</span>${loz('Im Plan', 'ok')}</div><div class="row cap"><span class="grow ell">App-Bewertung 4,6 Sterne</span>${loz('Gefährdet', 'warn', 'alert')}</div></div>`, btn('Alle Ziele', 'ghost'))}`;
  S.today = () => {
    const head = `<div class="ph"><div class="top"><div class="col" style="gap:var(--sp-050)"><h1 class="h">Guten Morgen, Jana</h1>
      ${note(1, `<div class="fig" style="gap:var(--sp-100)"><span>Mittwoch, 11. November ·</span>${chip('<b>1</b> überfällig', 'alert', 'bad')}${chip('<b>1</b> heute', 'clock')}${chip('<b>1</b> demnächst', 'calendar')}${chip('<b>2</b> offen bei dir', 'user')}${chip('<b>0</b> im Eingang', 'shapes')}</div>`)}</div></div></div>`;
    const focus = `<section class="reg">${note(2, `<div class="row" style="padding:var(--sp-150) var(--sp-150)"><div class="field grow">${ic('plus')}Was steht an? Ein Satz genügt – „morgen 10 Uhr Angebot an Stadtwerke“<span class="kbd" style="margin-inline-start:auto">Q</span></div></div>`)}
      ${note(3, todayFocus(false))}<div class="row cap subtle" style="padding:var(--sp-150)">${ic('check', 's')}Alles Weitere ist erledigt oder liegt bei anderen.</div></section>`;
    const coll = wg('', 'Deine Sammlungen', `<div class="col" style="gap:0">${[['rocket', 'blue', 'Team Atlas – Mobile App', '6 offen', 30], ['rocket', 'blue', 'Team Boreal – Platform & API', '4 offen', 55], ['inbox', 'magenta', 'Störungen & Anfragen', '10 offen · 1 SLA', 10], ['hub', 'orange', 'Projektplan', '21 % · 3 von 14', 21]].map(([i, c, n, f, pct]) => `<div class="row" style="padding:6px 0;gap:var(--sp-100)">${htile(i, c)}<span class="grow ell">${n}</span><span class="cap sub">${f}</span></div>`).join('')}</div>`, '');
    const d = desk(sideWS('today'), `${head}<div style="display:grid;grid-template-columns:minmax(0,1.25fr) minmax(0,1fr);gap:var(--sp-300);align-items:start"><div class="col" style="gap:var(--sp-300)">${focus}${coll}</div><div class="wgrid" style="grid-template-columns:1fr">${todayWidgets}</div></div>`);
    const t = tab(`${head}<div class="col" style="gap:var(--sp-300)">${focus}<div class="wgrid">${todayWidgets}</div></div>`);
    const m = phone(ptop('', 'Heute', ic('help')), `<div class="col" style="gap:var(--sp-150)"><h1 class="h" style="font-size:var(--fs-450)">Guten Morgen, Jana</h1>
      <div class="row wrap" style="gap:6px">${chip('<b>1</b> überfällig', 'alert', 'bad')}${chip('<b>1</b> heute', 'clock')}${chip('<b>2</b> bei dir', 'user')}</div>
      ${note(5, `<div class="seg" style="inline-size:100%"><span class="on" style="flex:1;justify-content:center">Dein Tag</span><span style="flex:1;justify-content:center">Überblick ${loz('1', 'bad')}</span></div>`)}
      <section class="reg">${todayFocus(true)}</section></div>`, 'Heute');
    return { d, t, m };
  };

  // ---------- Desk queue (a collection's list) ----------
  const queueTable = (opts = {}) => {
    const groups = [['In Arbeit', 'arbeit'], ['Neu', 'neu'], ['Wartet auf Rückmeldung', 'wartet'], ['Gelöst', 'geloest']];
    let rows = '';
    for (const [gname, gk] of groups) {
      const items = TICKETS.filter((x) => x[4] === gk);
      rows += `<tr class="grp"><td colspan="8"><span class="row" style="gap:6px">${ic('chevd', 's')}${st(gk)}<span class="subtle">${items.length}</span></span></td></tr>`;
      items.forEach(([k, t, ty, req, s, p, sl, as], ix) => {
        rows += `<tr class="${s === 'geloest' ? 'done' : ''} ${opts.hover && k === 'SA-4' ? 'hov' : ''}"><td style="inline-size:28px"><span class="check"></span></td><td class="mono" style="inline-size:64px">${k}</td>
          <td class="t"><div class="row" style="gap:var(--sp-100)"><span class="ell ttl" style="font-weight:var(--fw-medium)">${t}</span>${opts.hover && k === 'SA-4' ? `<span class="row subtle" style="margin-inline-start:auto;gap:6px">${btn('Öffnen', 'ghost')}${ic('more')}</span>` : ''}</div></td>
          <td>${typeIcon(ty)}</td><td class="cap">${req}</td><td>${prio(p)}</td><td>${sla(...sl)}</td><td>${as ? av(as, 's') : `<span class="cap subtle">—</span>`}</td></tr>`;
      });
      rows += `<tr class="add"><td></td><td colspan="7"><span class="row" style="gap:6px">${ic('plus', 's')}Eintrag in „${gname}“</span></td></tr>`;
    }
    return `<table class="tbl"><thead><tr><th></th><th>Schlüssel</th><th>Anfrage</th><th>Typ</th><th>Anfragende Person</th><th>Priorität</th><th>SLA</th><th>Bei</th></tr></thead><tbody>${rows}</tbody></table>`;
  };
  const collHead = (hub, hubTile, name, desc, fig, types) => `<div class="ph"><div class="trail">${hubTile} ${hub} ${ic('chev')} ${name}</div><div class="top"><div class="col" style="gap:var(--sp-050)"><div class="row"><h1 class="h">${name}</h1>${ic('star')}</div><span class="sub">${desc}</span>
    ${note(1, `<div class="fig">${fig}<span class="subtle">·</span><span class="row" style="gap:6px">${types}</span></div>`)}</div><div class="acts">${avs(['TB', 'CN', 'PN', 'JW', 'AH'])}<span class="cap subtle">+5</span>${btn('Teilen', '', 'users')}${btn('', 'ico', 'more')}</div></div></div>`;
  const deskTools = note(2, `<div class="toolbar"><div class="field" style="inline-size:260px">${ic('search')}In dieser Warteschlange suchen</div>${btn('Filtern', '', 'filter')}${chip('Priorität: P1, P2 ×', '')}${btn('Gruppiert: Status', '', 'group')}${btn('Spalten', '', 'cols')}<span class="grow"></span><span class="seg"><span class="on">Bequem</span><span>Kompakt</span></span>${btn('Auswählen', 'ghost', 'checksq')}</div>`);
  S.queue = () => {
    const head = collHead('IT-Service-Desk', htile('hub', 'teal'), 'Störungen & Anfragen', 'Was über das Portal, per Mail oder am Telefon ankommt.', `<b>10 offen</b><span>· 1 kurz vor Verletzung</span><span>· 6 nicht zugewiesen</span>`, `${loz('Störung', 'neu', 'alert')}${loz('Serviceanfrage', 'neu', 'inbox')}${loz('Change', 'neu')}${loz('Problem', 'neu')}`);
    const vb = viewbar([`${ic('list')}Liste`, `${ic('board')}Board`, `${ic('star')}Kurz vor Verletzung`, `${ic('star')}Meine`, `${ic('star')}Nicht zugewiesen`, `${ic('star')}Unzufrieden`, `${ic('plus')}`], `${ic('list')}Liste`);
    const d = desk(sideHub('it', 'Störungen & Anfragen'), `${head}${vb}${deskTools}${note(3, `<section class="reg">${queueTable({ hover: true })}<div class="row cap subtle" style="padding:var(--sp-150) var(--sp-200);border-block-start:var(--bw-hairline) solid var(--border-subtle)">10 von 10 gezeigt</div></section>`)}`);
    const pr = (k, t, ty, req, s, p, sl, as) => `<div class="prow"><span class="check" style="margin-block-start:2px"></span><div class="col grow" style="gap:4px"><span style="font-weight:var(--fw-medium)">${t}</span><div class="meta">${key('desk', k)}${st(s)}${prio(p)}</div>${sl[1] !== 'Eingehalten' ? sla(...sl) : ''}</div>${as ? av(as, 's') : ''}</div>`;
    const m = phone(ptop('IT-Service-Desk', 'Störungen & Anfragen'), `<div class="col" style="gap:var(--sp-150)"><div class="row"><h1 class="h" style="font-size:var(--fs-450)">Störungen &amp; Anfragen</h1></div><span class="cap sub"><b>10 offen</b> · 1 kurz vor Verletzung · 6 nicht zugewiesen</span>
      <div class="row" style="gap:var(--sp-100)"><span class="btn">${ic('list')}Liste ${ic('chevd', 's')}</span><div class="field grow">${ic('search')}Suchen</div><span class="btn">${ic('filter')}Filter (1)</span></div>
      ${note(4, `<section class="reg"><div class="row" style="padding:var(--sp-100) var(--sp-200);background:var(--bg-surface-sunken);gap:6px">${st('arbeit')}<span class="cap subtle">3</span></div>${TICKETS.filter((x) => x[4] === 'arbeit').map((x) => pr(x[0], x[1], x[2], x[3], x[4], x[5], x[6], x[7])).join('')}
      <div class="row" style="padding:var(--sp-100) var(--sp-200);background:var(--bg-surface-sunken);gap:6px">${st('neu')}<span class="cap subtle">5</span></div>${TICKETS.filter((x) => x[4] === 'neu').slice(0, 3).map((x) => pr(x[0], x[1], x[2], x[3], x[4], x[5], x[6], x[7])).join('')}</section>`)}
      <div class="row cap subtle" style="gap:6px">${ic('swipe', 's')}Nach rechts wischen: erledigen · nach links: Menü</div></div>`, 'Durchsuchen', `<div class="mk-fab">${ic('plus')}Neue Anfrage</div>`);
    const t = tab(`${head}${vb}<div class="toolbar"><div class="field grow">${ic('search')}Suchen</div>${btn('Filtern (1)', '', 'filter')}${btn('Gruppiert: Status', '', 'group')}</div><section class="reg">${queueTable().replace(/<th>Typ<\/th>|<th>Anfragende Person<\/th>/g, '')}</section>`);
    return { d, t: t.replace(/<td>(<span class="row cap sub" style="gap:4px">.*?<\/span>)<\/td><td class="cap">[^<]*<\/td>/g, ''), m };
  };

  // ---------- Board ----------
  const card = (k, t, labs, foot, a, extra = '') => `<div class="bcard ${extra}">${`<div class="row" style="justify-content:space-between">${key('work', k)}${a ? av(a, 's') : ''}</div>`}<span class="ttl">${t}</span>${labs ? `<div class="row wrap" style="gap:4px">${labs}</div>` : ''}${foot ? `<div class="ft cap sub">${foot}</div>` : ''}</div>`;
  const BOARD = [
    ['Backlog', 2, '', [['TAMA-16', 'Checkout button hidden behind the keyboard on small screens', '', '', 'JW'], ['TAMA-17', 'Passkey-Anmeldung: Wiederherstellung über ein zweites Gerät', '', chip('heute 18:00', 'clock', 'warn'), 'JK']]],
    ['Zu erledigen', 1, '', [['TAMA-10', 'Offline mode: orders placed without a connection are queued', lab('Kundenwunsch', 'violet'), chip('27. Nov.') + `<span class="row" style="gap:3px">${ic('checksq', 's')}0/2</span>`, 'SR']]],
    ['In Arbeit', 2, '2 / 3', [['TAMA-1', 'Als Kundin möchte ich mich mit Passkey anmelden', lab('Kundenwunsch', 'violet') + lab('Sicherheit', 'red'), chip('20. Nov.') + `<span class="row" style="gap:3px">${ic('checksq', 's')}0/2</span><span class="row" style="gap:3px">${ic('msg', 's')}2</span>`, 'AH'], ['TAMA-15', 'App stürzt beim Öffnen eines Push-Links ab', lab('Tech-Schuld', 'orange'), `<span class="row" style="gap:3px">${ic('msg', 's')}1</span>`, 'MY']]],
    ['In Prüfung', 1, '', [['TAMA-14', 'Screenreader liest Preise inklusive Währung vor', lab('Barrierefreiheit', 'teal'), '', 'SR']]]
  ];
  const boardHtml = (cols = BOARD, drag = true) => `<div class="board">${cols.map(([n, c, wip, cards], ci) => `<div class="bcol"><header>${n}<span class="cap subtle">${c}${wip ? ` · WIP ${wip}` : ''}</span><span class="grow"></span><span class="subtle">${ic('more')}</span></header>
    ${cards.map((x, k) => card(...x, drag && ci === 2 && k === 1 ? 'lift' : '')).join('')}<div class="row cap subtle" style="padding:var(--sp-050) var(--sp-100);gap:6px">${ic('plus', 's')}Eintrag</div></div>`).join('')}
    <div class="bcol" style="inline-size:56px;align-items:center;writing-mode:vertical-rl;padding-block:var(--sp-200)"><span class="cap semi">Erledigt · 3</span></div></div>`;
  S.board = () => {
    const head = collHead('Produktentwicklung', htile('rocket', 'blue'), 'Team Atlas – Mobile App', 'iOS- und Android-App für Endkundinnen und Endkunden.', `<b>6 offen</b><span>· Sprint 24 noch 6 Tage</span>`, `${loz('Bug', 'neu', 'bug')}${loz('Story', 'neu')}`);
    const vb = viewbar([`${ic('board')}Board`, `${ic('timeline')}Sprint 24 – Passkeys`, `${ic('list')}Backlog`, `${ic('checksq')}Liste`, `${ic('calendar')}Zeitleiste`, `${ic('clock')}Zeit`, `${ic('plus')}`], `${ic('board')}Board`);
    const tools = `<div class="toolbar"><div class="field" style="inline-size:220px">${ic('search')}Karten suchen</div>${btn('Filtern', '', 'filter')}${chip('Nur meine', 'user')}${chip('Labels', 'chevd')}${btn('Gruppen: keine', '', 'group')}<span class="grow"></span>${btn('', 'ico', 'more')}</div>`;
    const d = desk(sideHub('pe', 'Team Atlas – Mobile App'), `${head}${vb}${tools}${note(3, boardHtml())}`);
    const m = phone(ptop('Produktentwicklung', 'Team Atlas – Mobile App'), `<div class="col" style="gap:var(--sp-150)"><span class="cap sub"><b>6 offen</b> · Sprint 24 noch 6 Tage</span>
      <div class="row" style="gap:var(--sp-100)"><span class="btn">${ic('board')}Board ${ic('chevd', 's')}</span><span class="grow"></span><span class="btn">${ic('filter')}Filter</span></div>
      ${note(4, `<div class="seg" style="inline-size:100%;overflow:hidden"><span>Backlog 2</span><span>Zu erledigen 1</span><span class="on">In Arbeit 2</span><span>In Prüfung 1</span></div>`)}
      <div class="bcol">${BOARD[2][3].map((x) => card(...x)).join('')}<div class="row cap subtle" style="padding:var(--sp-050) var(--sp-100);gap:6px">${ic('plus', 's')}Eintrag</div></div>
      <div class="row cap subtle" style="justify-content:center;gap:var(--sp-100)">${ic('chevl', 's')} wischen für die Nachbarspalte ${ic('chev', 's')} · lange drücken: Verschieben nach …</div></div>`, 'Durchsuchen', `<div class="mk-fab">${ic('plus')}Eintrag</div>`);
    const t = tab(`${head}${vb}${boardHtml(BOARD.slice(0, 3), false)}`);
    return { d, t, m };
  };

  // ---------- The entry: one object grammar (a desk ticket and a story) ----------
  const rg = (title, body, opts = {}) => `<section class="rg ${opts.fold ? 'fold' : ''}"><header>${opts.tile || ''}<h3 class="lbl">${title}</h3>${opts.count != null ? `<span class="cap subtle">${opts.count}</span>` : ''}${opts.act ? `<span class="btn link cap">${ic('plus', 's')}${opts.act}</span>` : ''}${opts.fold ? `<span class="subtle">${ic('chev', 's')}</span>` : `<span class="subtle">${ic('chevd', 's')}</span>`}</header>${opts.fold ? '' : body}</section>`;
  const prop = (k, v) => `<div class="prop"><span>${k}</span><span>${v}</span></div>`;
  const ticketRail = (mobile) => `${rg('Details', prop('Zuständig', `<span class="row">${av('JK', 's')}Jana Krüger</span>`) + prop('Gruppe', 'Service Desk 1st Level') + prop('Fällig', chip('Mi, 11. Nov., 16:00', 'clock', 'warn')) + prop('Labels', `<span class="row" style="gap:4px">${lab('Mail', 'blue')}${lab('Kunden', 'teal')}</span>`) + prop('Typ', loz('Störung', 'neu', 'alert')))}
    ${rg('Servicedesk', prop('Anfragend', `<span class="row">${av('JW', 's')}Jonas Weber · Nordlicht</span>`) + prop('Auswirkung', 'Mehrere Personen') + prop('Dringlichkeit', 'Hoch') + prop('Priorität', prio('P2')) + prop('Erste Antwort', sla(62, '1 Std. 40 Min.', 'warn')) + prop('Lösung', sla(18, '5 Tage 22 Std.', '')), { tile: tile('desk') })}
    ${rg('Zeit', `<div class="row" style="gap:var(--sp-100)"><span class="field grow" style="color:var(--text-primary)">Entwicklung ${ic('chevd', 's')}</span>${btn('Timer', '', 'play')}</div>`, { tile: tile('time') })}
    ${rg('Betroffene Elemente', `<div class="row" style="gap:var(--sp-100)">${tile('assets')}<span class="grow">Exchange Online Connector</span>${loz('In Betrieb', 'ok')}</div>`, { tile: tile('assets'), count: 1, act: 'Nennen' })}
    ${rg('Verknüpfungen', '', { fold: 1, count: 0, act: 'Verknüpfen' })}${rg('Dateien', '', { fold: 1, count: 0, act: 'Datei' })}`;
  const conversation = (desk_) => `<div class="tabs"><span class="on">Unterhaltung · 3</span><span>Verlauf</span></div>
    ${note(7, `<div class="composer" style="margin-block:var(--sp-200)"><div class="row" style="padding:var(--sp-100) var(--sp-150);border-block-end:var(--bw-hairline) solid var(--border-subtle)">${desk_ ? `<span class="seg"><span class="on">${ic('send', 's')}Antwort an Jonas</span><span>${ic('lock', 's')}Interne Notiz</span></span>` : '<span class="cap sub">Kommentar</span>'}</div><div class="ta">${desk_ ? 'Antworten … Textbausteine mit /' : 'Etwas sagen …'}</div><div class="tb">${ic('clip')}${desk_ ? ic('snippet') + ic('macro') : ''}<span class="grow"></span>${btn(desk_ ? 'Senden' : 'Kommentieren', 'pri')}</div></div>`)}
    <div class="cmt">${av('JW', 'l')}<div class="col" style="gap:4px"><span class="cap"><b>Jonas Weber</b> <span class="subtle">· über das Portal · 09:12</span></span><span>Seit gestern Abend kommen Bestätigungsmails an unsere Kunden etwa eine Stunde verspätet an.</span></div></div>
    <div class="cmt internal" style="border-radius:var(--r-md);padding-inline:var(--sp-100)">${av('MY', 'l')}<div class="col" style="gap:4px"><span class="cap"><b>Mehmet Yılmaz</b> ${loz('Intern', 'warn', 'lock')} <span class="subtle">· 09:25</span></span><span>Warteschlange im Connector staut sich, Drosselung durch den Anbieter?</span></div></div>
    <div class="cmt">${av('JK', 'l')}<div class="col" style="gap:4px"><span class="cap"><b>Jana Krüger</b> <span class="subtle">· an Jonas · 09:31</span></span><span>Danke für die Meldung, wir schauen uns den Connector gerade an.</span></div></div>`;
  const ticketHead = (compact) => `<div class="obj-head">
    ${compact ? '' : `<div class="row"><div class="trail grow">${htile('hub', 'teal')} IT-Service-Desk ${ic('chev')} Störungen &amp; Anfragen</div><span class="seg"><span>${ic('side', 's')}Bereich</span><span class="on">${ic('expand', 's')}Seite</span></span>${btn('', 'ico', 'more')}</div>`}
    ${note(1, `<div class="row" style="gap:var(--sp-100)">${key('desk', 'SA-1')}${loz('Störung', 'neu', 'alert')}<span class="cap subtle">· erstellt heute 09:12 über das Portal</span></div><h1 class="h m" style="margin-block-start:var(--sp-100)">Mails an Kunden kommen eine Stunde zu spät an</h1>`)}
    ${note(2, `<div class="actionbar"><span class="statusbtn t-neu">Neu ${ic('chevd', 's')}</span>${btn('Beginnen', 'pri')}${btn('Lösen')}<span class="subtle" style="margin-inline:var(--sp-100)">|</span><span class="row">${av('JK', 's')}<span class="cap">Jana Krüger</span></span>${chip('Erste Antwort in 1 Std. 40 Min.', 'clock', 'warn')}${prio('P2')}<span class="grow"></span>${btn('', 'ico', 'star')}${btn('', 'ico', 'link')}</div>`)}</div>`;
  S.entry = () => {
    const main = `<div class="col" style="gap:var(--sp-300)">${note(3, `<section class="reg pad"><div class="row" style="margin-block-end:var(--sp-100)"><h3 class="lbl grow">Beschreibung</h3><span class="cap subtle">Klicken zum Bearbeiten</span></div><div class="notes"><p>Seit gestern Abend kommen Bestätigungsmails an Kundinnen und Kunden etwa eine Stunde verspätet an. Betroffen sind alle Mails über den Exchange-Connector.</p><p class="sub">Gemeldet über das Portal von Jonas Weber.</p></div></section>`)}
      <section class="reg"><div class="rh"><h2 class="h grow" style="font-size:var(--fs-100)">Unter diesem Eintrag</h2><span class="cap subtle">0 von 0</span>${btn('Arbeitspaket', 'ghost', 'plus')}</div><div class="rb cap subtle">Teile die Arbeit auf, wenn mehrere daran arbeiten.</div></section>
      <section>${conversation(true)}</section></div>`;
    const d = desk(sideHub('it', 'Störungen & Anfragen'), `${ticketHead(false)}<div class="obj">${main}${note(4, `<aside class="rail">${ticketRail()}</aside>`)}</div>`);
    const m = phone(ptop('Störungen & Anfragen', ''), `<div class="col" style="gap:var(--sp-200)"><div class="row" style="gap:var(--sp-100)">${key('desk', 'SA-1')}${loz('Störung', 'neu', 'alert')}</div><h1 class="h m" style="font-size:var(--fs-300)">Mails an Kunden kommen eine Stunde zu spät an</h1>
      <div class="row" style="gap:var(--sp-100)"><span class="statusbtn t-neu" style="flex:1;justify-content:space-between">Neu ${ic('chevd', 's')}</span>${btn('Beginnen', 'pri')}</div>
      ${note(5, `<section class="reg">${prop('Zuständig', `<span class="row">${av('JK', 's')}Jana Krüger</span>`).replace('class="prop"', 'class="prop" style="padding-inline:var(--sp-200)"')}${prop('Priorität', prio('P2')).replace('class="prop"', 'class="prop" style="padding-inline:var(--sp-200)"')}<div style="padding:var(--sp-050) var(--sp-200) var(--sp-150)">${sla(62, 'Erste Antwort in 1 Std. 40 Min.', 'warn')}</div><div class="row" style="padding:var(--sp-150) var(--sp-200);border-block-start:var(--bw-hairline) solid var(--border-subtle)"><span class="grow semi">Alle Eigenschaften</span><span class="cap subtle">12</span>${ic('chev')}</div></section>`)}
      <div class="seg" style="inline-size:100%"><span style="flex:1;justify-content:center">Beschreibung</span><span class="on" style="flex:1;justify-content:center">Unterhaltung 3</span><span style="flex:1;justify-content:center">Teile</span></div>
      <div class="cmt">${av('JW', 'l')}<div class="col" style="gap:4px"><span class="cap"><b>Jonas Weber</b> <span class="subtle">· 09:12</span></span><span>Seit gestern Abend kommen Bestätigungsmails etwa eine Stunde verspätet an.</span></div></div>
      <div class="cmt internal" style="border-radius:var(--r-md);padding-inline:var(--sp-100)">${av('MY', 'l')}<div class="col" style="gap:4px"><span class="cap"><b>Mehmet</b> ${loz('Intern', 'warn', 'lock')}</span><span>Warteschlange im Connector staut sich.</span></div></div>
      ${note(6, `<div class="composer"><div class="row" style="padding:var(--sp-100) var(--sp-150);gap:var(--sp-100)"><span class="grow subtle">Antworten …</span>${ic('lock')}${ic('clip')}${btn('Senden', 'pri')}</div></div>`)}</div>`, 'Durchsuchen');
    // panel beside the board (expanded) and the tablet's sheet
    const panel = `<aside class="sheet" style="box-shadow:var(--shadow-overlay);padding:var(--sp-300);border-radius:var(--r-lg) 0 0 var(--r-lg)"><div class="row" style="margin-block-end:var(--sp-200)"><span class="seg"><span class="on">${ic('side', 's')}Bereich</span><span>${ic('expand', 's')}Seite</span></span><span class="grow"></span>${ic('more')}${ic('x')}</div>
      <div class="row" style="gap:var(--sp-100)">${key('work', 'TAMA-1')}${loz('Story', 'neu')}</div><h1 class="h m" style="margin-block:var(--sp-100) var(--sp-150);font-size:var(--fs-400)">Als Kundin möchte ich mich mit Passkey anmelden, damit ich mir kein Passwort mehr merken muss</h1>
      <div class="actionbar"><span class="statusbtn t-info">In Arbeit ${ic('chevd', 's')}</span>${btn('Prüfung anfragen', 'pri')}<span class="row">${av('AH', 's')}<span class="cap">Amelia Hughes</span></span>${chip('20. Nov.')}</div>
      <div style="display:grid;grid-template-columns:minmax(0,1fr) 220px;gap:var(--sp-300);margin-block-start:var(--sp-300)"><div class="col" style="gap:var(--sp-200)"><section class="reg pad notes" style="white-space:pre-line">## Hintergrund
38 % der Support-Tickets im letzten Quartal betrafen vergessene Passwörter.

## Akzeptanzkriterien
- Passkey lässt sich in den Einstellungen hinzufügen
- Anmeldung ohne Passwort auf iOS 17+ und Android 14+</section>
      <section class="reg"><div class="rh"><h2 class="h grow" style="font-size:var(--fs-100)">Unter diesem Eintrag</h2><span class="cap subtle">0 von 2</span></div>${['Passkey-Registrierung im Profil', 'Anmeldung mit Passkey'].map((x, k) => `<div class="row" style="padding:var(--sp-100) var(--sp-200);border-block-end:var(--bw-hairline) solid var(--border-subtle)"><span class="check"></span><span class="grow">${x}</span><span class="cap subtle">0/${k ? 2 : 4}</span>${av(k ? 'MY' : 'AH', 's')}</div>`).join('')}</section></div>
      <aside class="rail">${rg('Details', prop('Labels', lab('Kundenwunsch', 'violet')) + prop('Sprint', 'Sprint 24') + prop('Schätzung', '5 Punkte'))}${rg('Ziel', `<span class="cap">App-Bewertung 4,6 Sterne</span>${bar(39, 'warn')}`, { tile: tile('goals') })}${rg('Verknüpfungen', '', { fold: 1, count: 1 })}</aside></div></aside>`;
    const p = desk(sideHub('pe', 'Team Atlas – Mobile App'), `<div style="display:grid;grid-template-columns:minmax(0,0.9fr) minmax(0,1.6fr);gap:0;margin-inline-end:calc(-1 * var(--sp-400))"><div style="opacity:.55;overflow:hidden">${boardHtml(BOARD.slice(0, 2), false)}</div>${note(8, panel)}</div>`);
    const t = tab(`<div style="position:relative">${boardHtml(BOARD.slice(0, 3), false)}<div style="position:absolute;inset-block-start:0;inset-inline-end:calc(-1 * var(--sp-300));inline-size:88%;">${panel.replace('grid-template-columns:minmax(0,1fr) 220px', 'grid-template-columns:minmax(0,1fr)')}</div></div>`);
    return { d, m, p, t };
  };

  // ---------- Hub overview ----------
  S.hub = () => {
    const tc = (p, title, big, sub, body, foot) => `<section class="wg"><header>${tile(p)}<h2>${title}</h2><span class="subtle">${ic('chev')}</span></header><div class="row" style="align-items:baseline;gap:var(--sp-100)"><span class="big">${big}</span><span class="cap sub">${sub}</span></div>${body}${foot ? `<footer class="cap sub">${foot}</footer>` : ''}</section>`;
    const head = `<div class="ph"><div class="trail">Hubs ${ic('chev')} Produktentwicklung</div><div class="top"><div class="band">${htile('rocket', 'blue', 'l')}<div class="col" style="gap:2px"><div class="row"><h1 class="h">Produktentwicklung</h1>${ic('star')}</div><span class="sub">Zwei Scrum-Teams bauen die Kunden-App und die Plattform dahinter.</span></div></div><div class="acts">${avs(['TB', 'CN', 'PN', 'JW', 'AH'])}${btn('Teilen', '', 'users')}<span class="btn pri split">${ic('plus')}Neu ${ic('chevd', 's')}</span>${btn('', 'ico', 'more')}</div></div></div>`;
    const tools = note(1, `<div style="display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:var(--sp-200)">
      ${tc('plans', 'Plan', '6 Tage', 'bis Sprintende', `<div class="cap">Sprint 24 – Passkeys</div>${bar(10, 'p" data-p="orange')}<span class="cap sub">Nächstes Release: App 4.2 „Schlüssellos“ · 25. Nov.</span>`)}
      ${tc('time', 'Zeit', '6:00', 'diese Woche', weekBars([2, 2, 2, 0, 0, 0, 0]))}
      ${tc('goals', 'Ziele', '1', 'gefährdet', `<div class="row cap"><span class="grow ell">App-Bewertung 4,6 Sterne</span>${loz('Gefährdet', 'warn', 'alert')}</div>${bar(39, 'warn')}`)}
      ${tc('assets', 'Inventar', '0', 'Elemente', '<span class="cap sub">Noch nichts erfasst. Elemente dieses Hubs erscheinen hier.</span>')}</div>`);
    const colls = note(2, `<div style="display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:var(--sp-200)">${[['Team Atlas – Mobile App', 'iOS- und Android-App für Endkundinnen und Endkunden.', '6 offen', 'Sprint 24', 25, ['AH', 'MY', 'SR', 'JK']], ['Team Boreal – Platform & API', 'Public REST API, billing and the event pipeline.', '4 offen', 'Kanban', 55, ['PN', 'TB', 'JW']]].map(([n, dsc, f, k, pct, a]) => `<section class="item"><div class="row">${htile('coll', 'blue', 'm')}<div class="col grow" style="gap:0"><span class="semi">${n}</span><span class="cap sub ell">${dsc}</span></div>${avs(a)}</div><div class="row cap sub" style="gap:var(--sp-100)"><b style="color:var(--text-primary)">${f}</b><span>· ${k}</span><span class="grow"></span>${pct} % erledigt</div>${bar(pct)}</section>`).join('')}</div>`);
    const feed = [['MY', 'Mehmet Yılmaz', 'hat', 'Passkey-Anmeldung: Wiederherstellung über ein zweites Gerät', 'angelegt', 'vor 22 Min.', 'work'], ['PN', 'Priya Natarajan', 'hat', 'Pagination cursor breaks when two entries share a timestamp', 'auf „In Arbeit“ gesetzt', 'vor 40 Min.', 'work'], ['AH', 'Amelia Hughes', 'hat die Seite', 'Architekturüberblick Kunden-App', 'geändert', 'vor 1 Std.', 'pages'], ['SR', 'Sofia Rossi', 'hat das Whiteboard', 'Retro Sprint 23', 'kommentiert', 'gestern', 'whiteboards']];
    const act = note(3, `<section class="reg"><div class="rh"><h2 class="h grow" style="font-size:var(--fs-100)">Was sich getan hat</h2>${btn('Alles', 'ghost')}</div>${feed.map(([i, n, v, o, w, t, p]) => `<div class="row" style="padding:var(--sp-100) var(--sp-200);border-block-end:var(--bw-hairline) solid var(--border-subtle);gap:var(--sp-150)">${av(i)}<span class="grow ell"><b>${n}</b> ${v} ${tile(p)} <span style="font-weight:var(--fw-medium)">${o}</span> ${w}</span><span class="cap subtle">${t}</span></div>`).join('')}</section>`);
    const d = desk(sideHub('pe', 'Übersicht'), `${head}${tools}<h3 class="lbl" style="margin:var(--sp-300) 0 var(--sp-150)">Sammlungen</h3>${colls}<div style="margin-block-start:var(--sp-300)">${act}</div>`);
    const m = phone(ptop('Alle Hubs', 'Produktentwicklung'), `<div class="col" style="gap:var(--sp-200)"><div class="row">${htile('rocket', 'blue', 'l')}<div class="col" style="gap:0"><h1 class="h" style="font-size:var(--fs-450)">Produktentwicklung</h1><span class="cap sub">2 Sammlungen · 10 offen</span></div></div>
      <div style="display:grid;grid-template-columns:1fr 1fr;gap:var(--sp-150)">${[['plans', 'Plan', '6 Tage'], ['time', 'Zeit', '6:00'], ['goals', 'Ziele', '1 gefährdet'], ['assets', 'Inventar', '0']].map(([p, n, f]) => `<section class="wg" style="padding:var(--sp-150)"><header>${tile(p)}<h2>${n}</h2></header><span class="semi">${f}</span></section>`).join('')}</div>
      <h3 class="lbl">Sammlungen</h3><section class="reg">${['Team Atlas – Mobile App|6 offen', 'Team Boreal – Platform & API|4 offen'].map((x) => `<div class="prow">${htile('coll', 'blue')}<span class="grow">${x.split('|')[0]}</span><span class="cap sub">${x.split('|')[1]}</span>${ic('chev', 's')}</div>`).join('')}</section>
      <h3 class="lbl">Was sich getan hat</h3><section class="reg">${feed.slice(0, 3).map(([i, n, v, o, w, t]) => `<div class="prow">${av(i, 's')}<div class="col grow" style="gap:2px"><span class="cap"><b>${n}</b> ${w}</span><span class="cap sub ell">${o}</span></div><span class="cap subtle">${t}</span></div>`).join('')}</section></div>`, 'Durchsuchen');
    return { d, m };
  };

  // ---------- Product home: Servicedesk, and the eight bands ----------
  S.deskhome = () => {
    const head = note(1, productHead('desk', 'Servicedesk', '<b>14 offen</b> · 1 kurz vor Verletzung · 6 nicht zugewiesen', `<span class="btn pri split">${ic('plus')}Neue Anfrage ${ic('chevd', 's')}</span>${btn('', 'ico', 'more')}`));
    const vb = viewbar(['Für dich', 'Desks', 'Berichte', 'Einstellungen'], 'Für dich');
    const kp = note(2, `<div class="kpis">${kpi('Meine offenen', '3', '1 kurz vor Verletzung')}${kpi('Nicht zugewiesen', '6', 'in deinen Gruppen')}${kpi('Erste Antwort eingehalten', '50 %', 'letzte 30 Tage · 1 verletzt')}${kpi('Lösung eingehalten', '100 %', 'letzte 30 Tage')}</div>`);
    const mine = `<section class="reg"><div class="rh">${loz('Meine', 'brand', 'user')}<span class="cap subtle">3</span><span class="grow"></span>${btn('Alle meine', 'ghost')}</div>${queueTable().replace(/<tr class="grp">[\s\S]*?<\/tr>|<tr class="add">[\s\S]*?<\/tr>/g, '').replace(/<tr class="[^"]*"><td style="inline-size:28px">[\s\S]*?<\/tr>/g, (r) => /SA-7|SA-4|SA-8/.test(r) ? r : '')}</section>`;
    const un = `<section class="reg" style="margin-block-start:var(--sp-300)"><div class="rh">${loz('Nicht zugewiesen', 'neu')}<span class="cap subtle">6</span><span class="grow"></span>${btn('Mir zuweisen', '', 'user')}</div>${queueTable().replace(/<thead>[\s\S]*?<\/thead>/, '').replace(/<tr class="grp">[\s\S]*?<\/tr>|<tr class="add">[\s\S]*?<\/tr>/g, '').replace(/<tr class="[^"]*"><td style="inline-size:28px">[\s\S]*?<\/tr>/g, (r) => /SA-1<|SA-2<|SA-3<|SA-9</.test(r) ? r : '')}</section>`;
    const d = desk(sideWS('desk'), `${head}${vb}${kp}${note(3, mine)}${un}`);
    const m = phone(ptop('Durchsuchen', 'Servicedesk', ic('more')), `<div class="col" style="gap:var(--sp-200)"><div class="row">${tile('desk', 'l')}<div class="col" style="gap:0"><h1 class="h" style="font-size:var(--fs-450)">Servicedesk</h1><span class="cap sub"><b>14 offen</b> · 1 kurz vor Verletzung</span></div></div>
      <div class="seg" style="inline-size:100%"><span class="on" style="flex:1;justify-content:center">Für dich</span><span style="flex:1;justify-content:center">Desks</span><span style="flex:1;justify-content:center">Berichte</span></div>
      <div style="display:grid;grid-template-columns:1fr 1fr;gap:var(--sp-150)">${kpi('Meine offenen', '3')}${kpi('Nicht zugewiesen', '6')}</div>
      <section class="reg">${TICKETS.filter((x) => ['SA-7', 'SA-4', 'SA-8'].includes(x[0])).map((x) => `<div class="prow"><div class="col grow" style="gap:4px"><span style="font-weight:var(--fw-medium)">${x[1]}</span><div class="meta">${key('desk', x[0])}${st(x[4])}${prio(x[5])}</div>${sla(...x[6])}</div></div>`).join('')}</section></div>`, 'Durchsuchen', `<div class="mk-fab">${ic('plus')}Neue Anfrage</div>`);
    return { d, m };
  };
  S.bands = () => {
    const B = [['plans', 'Pläne', '<b>1 Sprint läuft</b> · 2 Releases geplant · 1 Roadmap', 'Sprint planen', ['Läuft', 'Sprint 24 – Passkeys', 'noch 6 Tage'], ['Nächstes Release', 'App 4.2', '25. Nov.'], ['Kritischer Pfad', '4', 'Einträge · Neubau']],
      ['pages', 'Seiten', '<b>10 Seiten</b> · 3 Bereiche · 2 heute geändert', 'Neue Seite', ['Zuletzt geändert', 'Protokoll Lenkungskreis', 'vor 22 Min.'], ['Wartet auf dich', '0', 'Prüfungen'], ['Meine Notizen', '0', '']],
      ['whiteboards', 'Whiteboards', '<b>3 Whiteboards</b> · 1 heute geändert', 'Neues Whiteboard', ['Zuletzt', 'Retro Sprint 23', 'gestern'], ['Offene Kommentare', '2', ''], ['Vorlagen', '6', '']],
      ['assets', 'Inventar', '<b>12 Elemente</b> · 2 brauchen Aufmerksamkeit', 'Neues Element', ['Gesund', '10', 'von 12'], ['Prüfung fällig', '2', 'diesen Monat'], ['Alarme', '0', 'aktiv']],
      ['time', 'Zeit', '<b>6:00 diese Woche</b> · offen', 'Zeit erfassen', ['Heute', '2:00', ''], ['Zu entscheiden', '5', 'Wochen'], ['Abwesenheiten', '0', 'diesen Monat']],
      ['goals', 'Ziele', '<b>4 Ziele</b> · 2 brauchen einen Blick · bis 31.12.2026', 'Neues Ziel', ['Im Plan', '2', ''], ['Gefährdet', '1', ''], ['Nicht im Plan', '1', '']],
      ['agents', 'Agenten', '<b>2 Agenten</b> · 0 aktiv · 1 Entwurf', 'Agent hinzufügen', ['Warten auf dein Ja', '0', ''], ['Läufe heute', '0', ''], ['Budget', '0 %', 'genutzt']]];
    const d = `<div class="mk" style="padding:var(--sp-400);display:flex;flex-direction:column;gap:var(--sp-400)">${B.map(([p, n, f, a, ...k]) => `<div>${productHead(p, n, f, `<span class="btn pri split">${ic('plus')}${a} ${ic('chevd', 's')}</span>`)}<div class="kpis" style="grid-template-columns:repeat(3,minmax(0,1fr));margin:0">${k.map(([l, v, s]) => kpi(l, v, s)).join('')}</div></div>`).join('')}</div>`;
    return { d };
  };

  // ---------- Pages (wiki) ----------
  S.page = () => {
    const head = `<div class="ph"><div class="trail">${htile('rocket', 'blue')} Produktentwicklung ${ic('chev')} Handbuch Produktentwicklung ${ic('chev')} Architekturüberblick</div>
      <div class="top"><div class="row" style="gap:var(--sp-100)">${note(1, `<span class="row cap sub" style="gap:var(--sp-100)">${avs(['AH', 'MY'])} Amelia schreibt gerade · geändert vor 22 Min. von Mehmet · ${loz('Live', 'ok')}</span>`)}</div><div class="acts">${btn('Kommentare 1', '', 'msg')}${btn('Teilen', '', 'users')}${btn('', 'ico', 'star')}${btn('', 'ico', 'more')}</div></div></div>`;
    const body = `<div class="read"><div class="row wrap" style="gap:6px;margin-block-end:var(--sp-150)">${lab('Architektur', 'teal')}${lab('Kunden-App', 'blue')}<span class="btn link cap">${ic('plus', 's')}Label</span></div><h1>Architekturüberblick Kunden-App</h1>
      <p>Die App spricht ausschließlich mit der <b>Public API v3</b>. Es gibt keinen direkten Zugriff auf Datenbanken.</p><h2>Bausteine</h2>
      <table><tr><th>Baustein</th><th>Verantwortung</th><th>Team</th></tr><tr><td>App (iOS/Android)</td><td>Darstellung, Offline-Warteschlange</td><td>Atlas</td></tr><tr><td>Public API</td><td>Authentifizierung, Bestellungen, Webhooks</td><td>Boreal</td></tr><tr><td>Event-Pipeline</td><td>Benachrichtigungen, Auswertungen</td><td>Boreal</td></tr></table>
      <h2>Entscheidungen</h2><ol><li>Passkeys vor Passwort (siehe ${key('work', 'TAMA-1')}).</li><li>Bestellungen werden offline mit einem Idempotenzschlüssel gepuffert.</li></ol><h2>Glossar</h2><p><b>Passkey</b> – FIDO2-Anmeldedaten auf dem Gerät.</p></div>`;
    const d = desk(sideHub('pe', 'Handbuch Produktentwicklung'), `${head}<div style="display:grid;grid-template-columns:minmax(0,1fr) 200px;gap:var(--sp-400);align-items:start"><div class="col" style="gap:var(--sp-200)">
      ${note(2, `<div class="edbar" style="position:sticky;top:0">${['h', 'bold', 'italic', 'list', 'checksq', 'quote', 'code'].map((x) => `<span class="btn">${ic(x)}</span>`).join('')}<i class="sep"></i><span class="btn">${ic('link')}</span><span class="btn">${ic('table')}</span><span class="btn">${ic('image')}</span><i class="sep"></i><span class="btn" style="gap:4px">${ic('plus')}Einfügen ${ic('chevd', 's')}</span><span class="kbd" style="margin-inline:var(--sp-100)">/</span></div>`)}
      ${note(3, `<article class="sheet" style="padding:var(--sp-600) var(--sp-500)">${body}</article>`)}</div>
      ${note(4, `<nav class="toc"><h3 class="lbl">Auf dieser Seite</h3><span class="on">Bausteine</span><span>Entscheidungen</span><span>Glossar</span><h3 class="lbl" style="margin-block-start:var(--sp-200)">Verweist hierher</h3><span class="cap">${key('work', 'TAMA-1')}</span><span class="cap">Onboarding für Entwickler</span></nav>`)}</div>`);
    const m = phone(ptop('Handbuch', ''), `<div class="col" style="gap:var(--sp-150)"><span class="row cap sub" style="gap:6px">${avs(['AH', 'MY'])} geändert vor 22 Min.</span>${body.replace('class="read"', 'class="read" style="font-size:var(--fs-100)"').replace('<table>', '<div style="overflow-x:auto"><table style="min-inline-size:420px">').replace('</table>', '</table></div>')}</div>`, 'Durchsuchen', `<div class="mk-fab" style="inset-block-end:var(--sp-200)">${ic('bold')}Bearbeiten</div>`);
    return { d, m };
  };

  // ---------- Portal (a requester) ----------
  S.portal = () => {
    const kinds = [['alert', 'Etwas funktioniert nicht', 'Störung melden: Gerät, Programm oder Zugang streikt.'], ['key', 'Zugang beantragen', 'Für neue Kolleginnen, Projekte oder Systeme.'], ['laptop', 'Hardware bestellen', 'Laptop, Bildschirm, Headset und Zubehör.'], ['help', 'Eine Frage stellen', 'Wenn nichts davon passt – wir helfen weiter.']];
    const hero = note(1, `<section class="hero"><span class="cap semi" style="color:var(--label-magenta-fg)">IT-Service-Desk · Nordlicht</span><h1 class="h">Wie können wir helfen?</h1><div class="field">${ic('search')}Beschreibe dein Anliegen – z. B. „VPN verbindet nicht“</div><span class="cap sub">Antworten aus der Wissensdatenbank erscheinen, während du tippst.</span></section>`);
    const tiles = note(2, `<div class="kinds">${kinds.map(([i, n, s]) => `<section class="item" style="gap:var(--sp-100)">${htile(i, 'magenta', 'm')}<span class="semi">${n}</span><span class="cap sub">${s}</span></section>`).join('')}</div>`);
    const mine = note(3, `<section class="reg"><div class="rh"><h2 class="h grow" style="font-size:var(--fs-100)">Deine Anfragen</h2>${btn('Alle', 'ghost')}</div>${[['SA-8', 'Seit dem Update lässt sich das Kundenportal in Firefox nicht mehr öffnen', 'In Bearbeitung', 'info', 'Mehmet hat heute 09:40 geantwortet'], ['SA-3', 'Das VPN bricht alle paar Minuten ab', 'Eingegangen', 'neu', 'Wir melden uns bis heute 14:00'], ['SA-12', 'Neuer Bildschirm für den Empfang', 'Erledigt', 'ok', 'Abgeschlossen am 2. Nov.']].map(([k, t, s, tn, w]) => `<div class="row" style="padding:var(--sp-150) var(--sp-200);border-block-end:var(--bw-hairline) solid var(--border-subtle);gap:var(--sp-150)"><div class="col grow" style="gap:2px"><span style="font-weight:var(--fw-medium)">${t}</span><span class="cap sub">${k} · ${w}</span></div>${loz(s, tn, tn === 'ok' ? 'check' : '')}</div>`).join('')}</section>`);
    const arts = `<section class="reg"><div class="rh"><h2 class="h grow" style="font-size:var(--fs-100)">Oft gesucht</h2></div>${['VPN verbindet nicht: die fünf häufigsten Ursachen', 'How to reset your password when you are locked out', 'Drucker im 2. OG: Treiber und Warteschlangen'].map((x) => `<div class="row" style="padding:var(--sp-100) var(--sp-200);gap:var(--sp-100)">${tile('pages')}<span class="grow">${x}</span></div>`).join('')}</section>`;
    const d = desk(sideWS(''), `${hero}<h3 class="lbl" style="margin:var(--sp-400) 0 var(--sp-150)">Was brauchst du?</h3>${tiles}<div style="display:grid;grid-template-columns:minmax(0,1.6fr) minmax(0,1fr);gap:var(--sp-300);margin-block-start:var(--sp-400)">${mine}${arts}</div>`).replace(/<nav class="mk-side">[\s\S]*?<\/nav>/, `<nav class="mk-side">${nr('sun', 'Heute')}${nr('usercheck', 'Wartet auf dein Ja')}<hr>${nr('inbox', 'Störungen & Anfragen', { cur: 1 })}<div class="foot"><hr>${nr('archive', 'Archiv')}${nr('trash', 'Papierkorb')}</div></nav>`);
    const m = phone(ptop('', 'Um Hilfe bitten', ''), `<div class="col" style="gap:var(--sp-200)"><section class="hero" style="padding:var(--sp-300) var(--sp-200)"><h1 class="h" style="font-size:var(--fs-450)">Wie können wir helfen?</h1><div class="field">${ic('search')}Anliegen beschreiben</div></section>
      <div style="display:grid;grid-template-columns:1fr 1fr;gap:var(--sp-150)">${kinds.map(([i, n]) => `<section class="item" style="padding:var(--sp-150)">${htile(i, 'magenta', 'm')}<span class="semi cap">${n}</span></section>`).join('')}</div>
      <h3 class="lbl">Deine Anfragen</h3><section class="reg">${[['Kundenportal in Firefox', 'In Bearbeitung', 'info'], ['VPN bricht ab', 'Eingegangen', 'neu']].map(([t, s, tn]) => `<div class="prow"><span class="grow">${t}</span>${loz(s, tn)}</div>`).join('')}</section></div>`, 'Heute');
    return { d, m };
  };

  // ---------- Time: my week ----------
  S.time = () => {
    const days = ['Mo 9.', 'Di 10.', 'Mi 11.', 'Do 12.', 'Fr 13.', 'Sa 14.', 'So 15.'];
    const rows = [['work', 'TAMA-17', 'Passkey-Anmeldung: Wiederherstellung über ein zweites Gerät', 'Planung', [2, 2, 2, 0, 0, 0, 0]], ['desk', 'SA-7', 'Geschäftsführung erreicht das Board-Portal nicht', 'Support', [0, 0, 0, 0, 0, 0, 0]]];
    const f = (v) => v ? `${v}:00` : '';
    const head = note(1, productHead('time', 'Meine Woche', '<b>6:00</b> von 40:00 · 9.–15. November · ' + loz('Offen', 'neu'), `<span class="btn" style="background:var(--label-lime-bg);border-color:transparent;color:var(--label-lime-fg)">${ic('play')}Timer starten</span><span class="btn pri">${ic('plus')}Zeit erfassen</span>${btn('', 'ico', 'more')}`, `Produkte ${ic('chev')} Zeit`));
    const vb = viewbar(['Meine Woche', 'Zu entscheiden 5', 'Auswertung', 'Abwesenheiten', 'Meine Kapazität', 'Einstellungen'], 'Meine Woche');
    const nav = `<div class="toolbar">${btn('', 'ico', 'chevl')}<span class="semi">9.–15. November 2026</span>${btn('', 'ico', 'chev')}${btn('Diese Woche', 'ghost')}<span class="grow"></span>${btn('Vorwoche übernehmen', '', 'history')}</div>`;
    const grid = note(2, `<section class="reg"><table class="tbl wk"><thead><tr><th>Eintrag</th><th>Tätigkeit</th>${days.map((x, k) => `<th class="d ${k === 2 ? 'today' : ''}">${x}</th>`).join('')}<th class="d">Summe</th></tr></thead><tbody>
      ${rows.map(([p, k, t, a, v]) => `<tr><td class="t"><div class="row">${key(p, k)}<span class="ell" style="max-inline-size:300px">${t}</span></div></td><td class="cap sub">${a}</td>${v.map((x, i) => `<td class="d ${i === 2 ? 'today' : ''}"><span class="cell ${x ? '' : 'e'}">${f(x) || '–'}</span></td>`).join('')}<td class="d semi">${f(v.reduce((s, x) => s + x, 0)) || '0:00'}</td></tr>`).join('')}
      <tr class="add"><td colspan="10"><span class="row" style="gap:6px">${ic('plus', 's')}Eintrag hinzufügen – suchen oder zuletzt bearbeitete</span></td></tr>
      <tr class="grp"><td>Summe</td><td></td>${[2, 2, 2, 0, 0, 0, 0].map((x, i) => `<td class="d ${i === 2 ? 'today' : ''}">${x ? x + ':00' : '0:00'}</td>`).join('')}<td class="d">6:00</td></tr>
      <tr><td class="cap sub">Kapazität</td><td></td>${[2, 2, 2, 0, 0, 0, 0].map((x, i) => `<td class="d">${i < 5 ? `<span class="bar" style="display:inline-block;inline-size:56px;vertical-align:middle"><i style="inline-size:${x / 8 * 100}%;background:var(--label-lime-fg)"></i></span>` : ''}</td>`).join('')}<td></td></tr></tbody></table></section>`);
    const foot = note(3, `<div class="row" style="margin-block-start:var(--sp-200)"><span class="cap sub grow">Einreichen schickt die Woche an Lukas Brandt zur Freigabe. Danach ist sie gesperrt, bis er sie zurückgibt.</span>${btn('Woche einreichen', 'pri', 'send')}</div>`);
    const d = desk(sideWS('time'), `${head}${vb}${nav}${grid}${foot}`);
    const m = phone(ptop('Durchsuchen', 'Zeit', ic('more')), `<div class="col" style="gap:var(--sp-200)">${note(4, `<section class="wg" style="background:var(--label-lime-bg);border-color:transparent"><header>${tile('time')}<h2>Timer</h2></header><div class="row"><span class="big">0:00</span><span class="grow"></span>${btn('Starten', 'pri', 'play')}</div><span class="cap sub">Auf: zuletzt Passkey-Anmeldung (TAMA-17)</span></section>`)}
      <div class="row">${ic('chevl')}<span class="grow semi" style="text-align:center">9.–15. Nov. · 6:00</span>${ic('chev')}</div>
      <div style="display:grid;grid-template-columns:repeat(7,1fr);gap:4px">${days.map((x, k) => `<div class="col" style="align-items:center;gap:2px;padding:6px 0;border-radius:var(--r-md);${k === 2 ? 'background:var(--accent-primary-subtle);' : ''}"><span class="cap sub">${x.split(' ')[0]}</span><span class="cap semi">${k < 3 ? '2:00' : '–'}</span></div>`).join('')}</div>
      <h3 class="lbl">Mittwoch, 11. November</h3><section class="reg"><div class="prow">${key('work', 'TAMA-17')}<span class="grow ell">Passkey-Anmeldung</span><span class="semi">2:00</span></div></section>${btn('Woche einreichen', 'pri lg', 'send')}</div>`, 'Durchsuchen', `<div class="mk-fab">${ic('plus')}Zeit erfassen</div>`);
    return { d, m };
  };

  // ---------- Goals and a dashboard ----------
  S.goals = () => {
    const conf = (n) => `<span class="row cap sub" style="gap:2px" title="Zuversicht">${Array.from({ length: 10 }, (_, k) => `<i style="inline-size:5px;block-size:10px;border-radius:1px;background:${k < n ? 'var(--label-green-fg)' : 'var(--bg-surface-sunken)'}"></i>`).join('')}<span style="margin-inline-start:4px">${n}/10</span></span>`;
    const goal = (t, place, owner, st_, prog, c, krs, sub = '') => `<section class="reg"><div class="rb col" style="gap:var(--sp-150)"><div class="row" style="gap:var(--sp-150)">${tile('goals', 'm')}<div class="col grow" style="gap:2px"><span class="semi">${t}</span><span class="cap sub">${place} · ${owner}${sub}</span></div>${st_}</div>
      <div class="row" style="gap:var(--sp-200)"><span class="cap sub" style="inline-size:70px">Fortschritt</span>${bar(prog, 'p" data-p="green')}<span class="cap semi num" style="inline-size:40px;text-align:end">${prog} %</span><span class="cap sub" style="margin-inline-start:var(--sp-200)">Zuversicht</span>${conf(c)}</div>
      ${krs.map(([n, now, tgt, pct]) => `<div class="row cap" style="gap:var(--sp-150);padding-inline-start:var(--sp-500)"><span class="grow ell">${n}</span><span class="sub num">${now} von ${tgt}</span><span style="inline-size:120px">${bar(pct)}</span></div>`).join('')}</div></section>`;
    const head = productHead('goals', 'Ziele', '<b>4 Ziele</b> · 2 brauchen einen Blick · H2 2026 bis 31. Dezember', `${btn('H2 2026', '', 'calendar')}<span class="btn pri">${ic('plus')}Neues Ziel</span>`);
    const d = desk(sideWS('goals'), `${head}${viewbar(['Ziele', 'Portfolios', 'Dashboards'], 'Ziele')}${note(1, `<div class="col" style="gap:var(--sp-200)">${goal('Kundinnen und Kunden lösen ihr Anliegen selbst – ohne Anruf', 'Arbeitsbereich', 'Lukas Brandt', loz('Im Plan', 'ok', 'check'), 38, 7, [['Anteil Anfragen über das Portal', '41 %', '60 %', 68], ['Erste Antwortzeit (Std.)', '4,5', '2', 44], ['Passkey-Anmeldung live', 'Nein', 'Ja', 0]], ' · 2 Ziele darunter')}
      ${goal('App-Bewertung von 4,6 Sternen in beiden Stores', 'Produktentwicklung', 'Amelia Hughes', loz('Gefährdet', 'warn', 'alert'), 39, 5, [['Absturzfreie Sitzungen', '98,2 %', '99,5 %', 39]])}
      ${goal('Logistikzentrum im Zeit- und Kostenrahmen (± 5 %)', 'Neubau Logistikzentrum', 'Priya Natarajan', loz('Nicht im Plan', 'bad', 'alert'), 75, 3, [])}</div>`)}`);
    return { d };
  };
  S.dashboard = () => {
    const bars = [['Aufgabe', 17, 'c1'], ['Störung', 8, 'c2'], ['Story', 8, 'c3'], ['Arbeitspaket', 8, 'c4'], ['Karte', 7, 'c5'], ['Lieferergebnis', 6, 'c6'], ['Weitere (7)', 12, 'c7']];
    const max = 18;
    const barChart = `<svg class="chart" viewBox="0 0 420 210">${[0, 6, 12, 18].map((v) => `<line class="grid" x1="90" x2="410" y1="${180 - v * 9}" y2="${180 - v * 9}"/><text x="${90 + 0}" y="198">${''}</text>`).join('')}
      ${bars.map(([n, v, c], k) => `<text x="84" y="${16 + k * 24}" text-anchor="end">${n}</text><rect class="${c}" x="90" y="${5 + k * 24}" width="${v / max * 300}" height="16" rx="3"/><text x="${96 + v / max * 300}" y="${17 + k * 24}">${v}</text>`).join('')}</svg>`;
    const line = `<svg class="chart" viewBox="0 0 420 150"><line class="grid" x1="30" x2="410" y1="120" y2="120"/><line class="grid" x1="30" x2="410" y1="70" y2="70"/><line class="grid" x1="30" x2="410" y1="20" y2="20"/><text x="24" y="124" text-anchor="end">0</text><text x="24" y="74" text-anchor="end">5</text><text x="24" y="24" text-anchor="end">10</text>
      <path d="M40 110 L110 96 L180 100 L250 80 L320 60 L390 40" class="s1" stroke-width="2.5" fill="none"/><path d="M40 110 L110 96 L180 100 L250 80 L320 60 L390 40 L390 120 L40 120Z" fill="var(--accent-primary-subtle)" opacity=".6"/><circle cx="390" cy="40" r="4" class="c1"/>
      ${['5. Okt.', '12. Okt.', '19. Okt.', '26. Okt.', '2. Nov.', '9. Nov.'].map((x, k) => `<text x="${40 + k * 70}" y="140" text-anchor="middle">${x}</text>`).join('')}<text x="390" y="32" text-anchor="middle" style="font-weight:600;fill:var(--text-primary)">8</text></svg>`;
    const donut = `<svg class="chart" viewBox="0 0 200 140"><g transform="translate(70 70)">${[[2, 'c6'], [2, 'c4'], [1, 'c2'], [10, 'c7']].reduce((a, [v, c]) => { const tot = 15, s = a.s, e = s + v / tot * Math.PI * 2; const r = 50, r2 = 32; const p = (ang, rr) => `${(Math.sin(ang) * rr).toFixed(1)} ${(-Math.cos(ang) * rr).toFixed(1)}`; a.h += `<path class="${c}" d="M${p(s, r)} A${r} ${r} 0 ${e - s > Math.PI ? 1 : 0} 1 ${p(e, r)} L${p(e, r2)} A${r2} ${r2} 0 ${e - s > Math.PI ? 1 : 0} 0 ${p(s, r2)}Z"/>`; a.s = e; return a; }, { s: 0, h: '' }).h}<text x="0" y="5" text-anchor="middle" style="font-size:16px;font-weight:600;fill:var(--text-primary)">15</text></g>
      ${[['P1', 2, 'c6'], ['P2', 2, 'c4'], ['P4', 1, 'c2'], ['ohne', 10, 'c7']].map(([n, v, c], k) => `<rect class="${c}" x="140" y="${30 + k * 22}" width="10" height="10" rx="2"/><text x="156" y="${39 + k * 22}">${n} · ${v}</text>`).join('')}</svg>`;
    const tileW = (title, body, span = 1, extra = '') => `<section class="wg" style="grid-column:span ${span}"><header><h2>${title}</h2>${extra}<span class="subtle">${ic('table')}</span><span class="subtle">${ic('more')}</span></header>${body}</section>`;
    const d = desk(sideWS('goals'), `<div class="ph"><div class="trail">Ziele ${ic('chev')} Dashboards</div><div class="top"><div class="col" style="gap:2px"><h1 class="h">Geschäftsführung – Wochenüberblick</h1><span class="cap sub">Geteilt mit dem Arbeitsbereich · berechnet 11. Nov., 09:49</span></div><div class="acts">${btn('Widget', '', 'plus')}${btn('Bearbeiten')}${btn('', 'ico', 'more')}</div></div></div>
      ${note(1, `<div style="display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:var(--sp-200)">${kpi('Offene Einträge', '77', 'über alle Hubs')}${kpi('Erledigt diese Woche', '8', '▲ 3 zur Vorwoche')}${kpi('SLA eingehalten', '50 %', 'Erste Antwort · 30 Tage')}${kpi('Ziele im Plan', '2 von 4', '1 gefährdet')}</div>`)}
      ${note(2, `<div style="display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:var(--sp-200);margin-block-start:var(--sp-200)">${tileW('Offene Einträge nach Typ', barChart, 2)}${tileW('Erledigte Einträge je Woche', line, 2)}${tileW('Störungen nach Priorität', donut, 1)}
      ${tileW('Ziele H2', `<div class="col" style="gap:var(--sp-150)">${[['Anliegen selbst lösen', 38, 'ok'], ['App-Bewertung 4,6', 39, 'warn'], ['Logistikzentrum im Rahmen', 75, 'bad']].map(([n, v, t]) => `<div class="col" style="gap:4px"><div class="row cap"><span class="grow ell">${n}</span><span class="num">${v} %</span></div>${bar(v, t)}</div>`).join('')}</div>`, 1)}
      ${tileW('Kurz vor Verletzung', `${TICKETS.slice(0, 2).map((x) => `<div class="row cap" style="gap:var(--sp-100)">${key('desk', x[0])}<span class="grow ell">${x[1]}</span></div>${sla(...x[6])}`).join('')}`, 2)}</div>`)}`);
    const m = phone(ptop('Dashboards', 'Wochenüberblick'), `<div class="col" style="gap:var(--sp-150)"><div style="display:grid;grid-template-columns:1fr 1fr;gap:var(--sp-150)">${kpi('Offene Einträge', '77')}${kpi('Erledigt (Woche)', '8')}${kpi('SLA eingehalten', '50 %')}${kpi('Ziele im Plan', '2 / 4')}</div>${tileW('Offene Einträge nach Typ', barChart)}${tileW('Erledigte je Woche', line)}</div>`, 'Durchsuchen');
    return { d, m };
  };

  // ---------- Plan (Gantt) ----------
  S.plan = () => {
    const W = 860, start = 0; const rows = [
      ['Grundlagenermittlung und Vorplanung', 0, 8, 'done', 0], ['Entwurfs- und Genehmigungsplanung', 6, 18, 'done', 0], ['Bauantrag einreichen', 18, 0, 'ms', 0], ['Ausführungsplanung Tragwerk, TGA und Fördertechnik', 19, 34, 'sum', 0],
      ['Statik Hallendach (Spannweite 42 m)', 19, 26, 'bar', 1], ['Sprinkleranlage nach VdS CEA 4001', 22, 30, 'bar', 1], ['Hochregallager 24 m – Regalbediengeräte', 24, 34, 'crit', 1], ['Lenkungskreis: Freigabe der Ausführungsplanung', 34, 0, 'ms', 0],
      ['Ausschreibung und Vergabe der Gewerke', 34, 50, 'crit', 0], ['Rohbau Halle A und B', 50, 74, 'crit', 0], ['Inbetriebnahme Fördertechnik und Probebetrieb', 74, 86, 'crit', 0], ['Übergabe an den Betrieb', 87, 0, 'ms', 0]];
    const x = (wk) => wk / 90 * W;
    const months = ['Jul 26', 'Aug', 'Sep', 'Okt', 'Nov', 'Dez', 'Jan 27', 'Feb', 'Mrz', 'Apr', 'Mai', 'Jun'];
    const gl = rows.map(([n, , , k, ind]) => `<div class="grow-r" style="padding-inline-start:calc(var(--sp-150) + ${ind * 16}px)">${k === 'sum' ? ic('chevd', 's') : k === 'ms' ? `<span style="inline-size:8px;block-size:8px;transform:rotate(45deg);background:var(--text-primary);flex:none"></span>` : `<span class="check" style="inline-size:14px;block-size:14px"></span>`}<span class="ell ${k === 'sum' ? 'semi' : ''}">${n}</span>${k === 'crit' ? `<span class="cap" style="color:var(--status-danger-text)">kritisch</span>` : ''}</div>`).join('');
    const gr = rows.map(([n, a, b, k], i) => k === 'ms' ? `<span class="gms" style="inset-inline-start:${x(a) - 6}px;inset-block-start:${i * 33 + 10}px"></span>` : `<span class="gbar ${k}" style="inset-inline-start:${x(a)}px;inline-size:${x(b - a)}px;inset-block-start:${i * 33 + (k === 'sum' ? 12 : 8)}px"></span>`).join('');
    const today = x(18.5);
    const deps = `<svg style="position:absolute;inset:0;inline-size:100%;block-size:100%;overflow:visible" viewBox="0 0 ${W} ${rows.length * 33}" preserveAspectRatio="none">${[[1, 2], [2, 3], [6, 7], [7, 8], [8, 9], [9, 10], [10, 11]].map(([f, t]) => { const xa = x(rows[f][2] || rows[f][1]) + 2, ya = f * 33 + 16, xb = x(rows[t][1]), yb = t * 33 + 16; return `<path d="M${xa} ${ya} H${xa + 8} V${yb} H${xb}" fill="none" stroke="var(--text-subtle)" stroke-width="1.2"/>`; }).join('')}</svg>`;
    const head = `<div class="ph"><div class="trail">${htile('hub', 'orange')} Neubau Logistikzentrum ${ic('chev')} Plan</div><div class="top"><div class="col" style="gap:2px"><h1 class="h">Plan</h1>${note(1, `<div class="fig"><b>Ende 18. Juni 2027</b>${loz('4 Tage über Toleranz', 'warn', 'alert')}<span>· 4 auf dem kritischen Pfad · 2 ohne Datum</span></div>`)}</div><div class="acts">${btn('Als Baseline festhalten', '', 'pin')}${btn('', 'ico', 'more')}</div></div></div>`;
    const vb = viewbar([`${ic('timeline')}Zeitplan`, `${ic('table')}Tabelle`, `${ic('link')}Abhängigkeiten`, 'Releases', 'Sprints'], `${ic('timeline')}Zeitplan`);
    const tools = `<div class="toolbar">${btn('', 'ico', 'chevl')}${btn('Heute')}${btn('', 'ico', 'chev')}${btn('Ganzer Plan', '', 'expand')}<span class="seg"><span>Wochen</span><span class="on">Monate</span><span>Quartale</span></span><span class="grow"></span>${chip('Baseline „Freigabe 1. Okt.“ zeigen', 'eye')}${btn('Ohne Datum · 2', '', 'calendar')}</div>`;
    const chart = note(2, `<section class="reg" style="overflow:hidden"><div class="gantt"><div class="gl"><div class="grow-r" style="block-size:36px"><h3 class="lbl">Eintrag</h3></div>${gl}</div><div class="gr" style="overflow:hidden"><div style="display:grid;grid-template-columns:repeat(12,1fr);block-size:36px;border-block-end:var(--bw-hairline) solid var(--border-subtle)">${months.map((m) => `<span class="cap sub" style="padding:var(--sp-100);border-inline-start:var(--bw-hairline) solid var(--border-subtle)">${m}</span>`).join('')}</div><div style="position:relative;block-size:${rows.length * 33}px;inline-size:${W}px">${deps}${gr}<span style="position:absolute;inset-block:0;inset-inline-start:${today}px;inline-size:2px;background:var(--accent-signature)"></span><span class="cap" style="position:absolute;inset-block-start:-2px;inset-inline-start:${today + 4}px;color:var(--accent-signature);font-weight:var(--fw-semibold)">Heute</span></div></div></div></section>`);
    const legend = note(3, `<div class="row cap sub wrap" style="gap:var(--sp-300);margin-block-start:var(--sp-150)"><span class="row" style="gap:6px"><span class="gbar" style="position:static;inline-size:24px;block-size:10px"></span>geplant</span><span class="row" style="gap:6px"><span class="gbar crit" style="position:static;inline-size:24px;block-size:10px"></span>kritischer Pfad</span><span class="row" style="gap:6px"><span style="inline-size:9px;block-size:9px;transform:rotate(45deg);background:var(--text-primary)"></span>Meilenstein</span><span class="row" style="gap:6px">${ic('link', 's')}Abhängigkeit – Zeile anfahren zeigt sie in Worten</span></div>`);
    const d = desk(sideHub('nb', 'Plan'), `${head}${vb}${tools}${chart}${legend}`);
    return { d };
  };

  // ---------- CI ----------
  S.ci = () => {
    const node = (t, s, cur = false) => `<div class="item" style="padding:var(--sp-100) var(--sp-150);gap:2px;${cur ? 'border-color:var(--accent-primary);box-shadow:0 0 0 1px var(--accent-primary)' : ''}"><span class="row cap" style="gap:6px">${ic('server', 's')}<b>${t}</b></span><span class="cap sub">${s}</span></div>`;
    const head = `<div class="obj-head"><div class="trail">${htile('hub', 'teal')} IT-Service-Desk ${ic('chev')} Inventar ${ic('chev')} Anwendungsdienst</div>${note(1, `<div class="row" style="gap:var(--sp-100)">${key('assets', 'Anwendungsdienst')}${loz('In Betrieb', 'ok', 'check')}<span class="cap subtle">· zuletzt bestätigt 11. Nov., 09:25</span></div><div class="row"><h1 class="h m grow">Portal-API</h1>${btn('Verweis kopieren', '', 'link')}${btn('Ändern')}${btn('', 'ico', 'more')}</div>`)}</div>`;
    const graph = note(2, `<section class="reg pad"><div class="row" style="margin-block-end:var(--sp-200)"><h2 class="h grow" style="font-size:var(--fs-100)">Beziehungen</h2><span class="seg"><span class="on">Bild</span><span>Liste</span></span></div>
      <div style="display:grid;grid-template-columns:1fr auto 1fr auto 1fr;gap:var(--sp-150);align-items:center"><div class="col">${node('Kundenportal', 'Geschäftsdienst · nutzt')}</div>${ic('chev')}<div>${node('Portal-API', 'Anwendungsdienst', true)}</div>${ic('chev')}<div class="col">${node('app-01.hh.intern', 'läuft auf')}${node('app-02.hh.intern', 'läuft auf')}${node('db-01.hh.intern', 'hängt ab von')}</div></div></section>`);
    const impact = note(3, `<section class="reg"><div class="rh"><h2 class="h grow" style="font-size:var(--fs-100)">Was ein Ausfall erreicht</h2><span class="field" style="padding-block:3px">Was leidet, wenn es ausfällt ${ic('chevd', 's')}</span></div><div class="rb row" style="gap:var(--sp-150)">${tile('assets')}<span class="grow">Kundenportal</span><span class="cap sub">Geschäftsdienst · 1 Schritt entfernt</span>${loz('Dienst', 'info')}</div></section>`);
    const rail = `<aside class="rail">${rg('Lebenszyklus', prop('Verantwortlich', '<span class="empty">Niemand – bitte nennen</span>') + prop('Gruppe', 'Plattform-Team') + prop('Nächste Prüfung', '9. Feb. 2027') + prop('Anzahl', '1'))}${rg('Attribute', prop('Webadresse', '<span class="mono">api.nordlicht.example.org/v3</span>'))}${rg('Schutzbedarf', prop('Vertraulichkeit', 'Nicht festgelegt') + prop('Integrität', 'Nicht festgelegt') + prop('Verfügbarkeit', 'Nicht festgelegt'))}${rg('Störungen und Changes', '', { fold: 1, count: 0, tile: tile('desk') })}${rg('Verweist hierher', '', { fold: 1, count: 0 })}</aside>`;
    const d = desk(sideHub('it', 'Inventar'), `${head}<div class="obj"><div class="col" style="gap:var(--sp-300)">${graph}${impact}</div>${note(4, rail)}</div>`);
    return { d };
  };

  // ---------- Search ----------
  S.search = () => {
    const grp = (t, n, rows) => `<section class="reg"><div class="rh"><h3 class="lbl grow">${t}</h3><span class="cap subtle">${n}</span></div>${rows}</section>`;
    const r = (p, k, t, sub, end = '') => `<div class="row" style="padding:var(--sp-100) var(--sp-200);border-block-end:var(--bw-hairline) solid var(--border-subtle);gap:var(--sp-150)">${k ? key(p, k) : tile(p)}<div class="col grow" style="gap:0"><span class="ell" style="font-weight:var(--fw-medium)">${t}</span><span class="cap sub">${sub}</span></div>${end}</div>`;
    const d = desk(sideWS(''), `<div class="ph"><h1 class="h">Suche</h1></div>${note(1, `<div class="field" style="padding:var(--sp-150) var(--sp-200);font-size:var(--fs-200);color:var(--text-primary);margin-block-end:var(--sp-150)">${ic('search', 'l')}Passkey<span class="grow"></span><span class="cap subtle">7 Treffer</span></div>`)}
      <div class="toolbar"><span class="seg"><span class="on">Alles 7</span><span>Einträge 4</span><span>Seiten 2</span><span>Whiteboards 1</span></span>${chip('Hub', 'chevd')}${chip('Zuständig', 'chevd')}${chip('Geändert', 'chevd')}</div>
      ${note(2, `<div class="col" style="gap:var(--sp-200)">${grp('Einträge', 4, r('work', 'TAMA-1', 'Als Kundin möchte ich mich mit <mark>Passkey</mark> anmelden', 'Produktentwicklung · Team Atlas · In Arbeit', av('AH', 's')) + r('work', 'TAMA-17', '<mark>Passkey</mark>-Anmeldung: Wiederherstellung über ein zweites Gerät', 'Produktentwicklung · Team Atlas · Backlog', av('JK', 's')) + r('plans', '', 'Sprint 24 – <mark>Passkeys</mark>', 'Sprint · noch 6 Tage'))}
      ${grp('Seiten', 2, r('pages', '', 'Architekturüberblick Kunden-App', '„… Passkeys vor Passwort (siehe Sprint 24) …“ · Handbuch Produktentwicklung') + r('pages', '', 'Onboarding for new developers', '„… set up a <mark>passkey</mark> for the staging app …“'))}
      ${grp('Whiteboards', 1, r('whiteboards', '', 'Anmeldefluss mit <mark>Passkey</mark> (Entwurf)', 'Produktentwicklung · geändert gestern'))}</div>`)}`);
    return { d };
  };

  // ---------- Empty states ----------
  S.empty = () => {
    const tpl = (i, c, n, s) => `<section class="item" style="text-align:start">${htile(i, c, 'm')}<span class="semi">${n}</span><span class="cap sub">${s}</span><span class="btn" style="align-self:flex-start">Verwenden</span></section>`;
    const d = desk(sideWS('desk'), `${productHead('desk', 'Servicedesk', 'Anfragen von außen, Warteschlangen und Antwortzeiten.', `<span class="btn pri split">${ic('plus')}Desk anlegen ${ic('chevd', 's')}</span>`)}
      ${note(1, `<div class="empty" data-p="magenta"><span class="halo">${tile('desk', 'xl')}</span><h2 class="h" style="font-size:var(--fs-400)">Noch nimmt kein Desk Anfragen an</h2><span class="sub">Ein Desk sammelt, was über das Portal, per Mail oder am Telefon ankommt, und misst, wie schnell ihr antwortet.</span><div class="row">${btn('Desk mit ITIL-Vorlage anlegen', 'pri lg')}${btn('Leer beginnen', 'lg')}</div><span class="cap subtle">${ic('book', 's')} So arbeitet ein Desk – 3 Minuten Lesezeit</span></div>`)}
      ${note(2, `<h3 class="lbl" style="margin-block:var(--sp-200) var(--sp-150)">Oder mit einer Vorlage starten</h3><div style="display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:var(--sp-200)">${tpl('inbox', 'magenta', 'IT-Servicedesk (ITIL)', 'Störungen, Serviceanfragen, Probleme und Changes mit SLAs und Portal.')}${tpl('users', 'teal', 'Personalanfragen', 'Bescheinigungen, Urlaub, Onboarding – mit Formularen fürs Team.')}${tpl('brief', 'orange', 'Einkauf', 'Bestellanfragen mit Freigabe durch die Kostenstelle.')}</div>`)}`);
    const m = phone(ptop('Durchsuchen', 'Servicedesk', ''), `<div class="empty" data-p="magenta" style="padding:var(--sp-400) 0"><span class="halo">${tile('desk', 'xl')}</span><h2 class="h" style="font-size:var(--fs-300)">Noch nimmt kein Desk Anfragen an</h2><span class="sub cap">Ein Desk sammelt Anfragen und misst, wie schnell ihr antwortet.</span>${btn('Desk mit ITIL-Vorlage anlegen', 'pri lg')}${btn('Leer beginnen', 'lg')}</div>`, 'Durchsuchen');
    return { d, m };
  };

  // ---------- Foundations: the object grammar across products ----------
  S.grammar = () => {
    const h = (p, k, ty, t, status, act, meta) => `<section class="reg pad col" style="gap:var(--sp-100)"><div class="row" style="gap:var(--sp-100)">${key(p, k)}${ty}</div><span class="semi" style="font-size:var(--fs-200)">${t}</span><div class="actionbar">${status}${act}${meta}</div></section>`;
    const d = `<div class="mk" style="padding:var(--sp-400);display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:var(--sp-200)">
      ${h('desk', 'SA-1', loz('Störung', 'neu', 'alert'), 'Mails an Kunden kommen eine Stunde zu spät an', `<span class="statusbtn t-neu">Neu ${ic('chevd', 's')}</span>`, btn('Beginnen', 'pri'), chip('Erste Antwort in 1 Std. 40 Min.', 'clock', 'warn'))}
      ${h('work', 'TAMA-1', loz('Story', 'neu'), 'Als Kundin möchte ich mich mit Passkey anmelden', `<span class="statusbtn t-info">In Arbeit ${ic('chevd', 's')}</span>`, btn('Prüfung anfragen', 'pri'), av('AH', 's') + chip('20. Nov.'))}
      ${h('assets', 'Anwendungsdienst', '', 'Portal-API', `<span class="statusbtn t-ok">${ic('check', 's')}In Betrieb ${ic('chevd', 's')}</span>`, btn('Bestätigen', 'pri'), chip('Prüfung 9. Feb. 2027', 'calendar'))}
      ${h('goals', 'Ziel · H2 2026', '', 'App-Bewertung von 4,6 Sternen in beiden Stores', `<span class="statusbtn t-warn">${ic('alert', 's')}Gefährdet ${ic('chevd', 's')}</span>`, btn('Check-in', 'pri'), av('AH', 's') + `<span class="cap sub">Zuversicht 5/10</span>`)}
      ${h('agents', 'Agent', '', 'Ticket-Vorsortierung', `<span class="statusbtn t-ok">${ic('check', 's')}Freigegeben ${ic('chevd', 's')}</span>`, btn('Live schalten', 'pri'), `<span class="cap sub">200 000 Token am Tag · 0 % genutzt</span>`)}
      ${h('plans', 'Release', '', 'App 4.2 „Schlüssellos“', `<span class="statusbtn t-neu">Geplant ${ic('chevd', 's')}</span>`, btn('Veröffentlichen', 'pri'), chip('25. Nov.') + `<span class="cap sub">1 von 3 erledigt</span>`)}</div>`;
    return { d };
  };

  window.WB = { S, ic, PROD, H: { P, tile, htile, key, loz, lab, av, avs, chip, btn, pin, note, bar, nr, sideWS, sideHub, appbar, desk, tab, phone, ptop, productHead, viewbar, wg, kpi, rg, prop, BOTTOM, collHead, card } };
})();
