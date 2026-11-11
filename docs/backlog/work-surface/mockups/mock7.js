// Second feedback round: the workflow on an entry, Plan with its tabs, the view panel shared by
// collections and Plan, and the sprint views. Uses the helpers of mock.js, mock3.js and mock5.js.
(function () {
  'use strict';
  const { S, ic, H, X } = window.WB;
  const { tile, htile, key, loz, lab, av, chip, btn, note, sideHub, phone, ptop, viewbar, collHead, card } = H;
  Object.assign(H.P, {
    arrowr: '<path d="M5 12h14M13 6l6 6-6 6"/>',
    repeat: '<path d="m17 2 4 4-4 4"/><path d="M3 11v-1a4 4 0 0 1 4-4h14"/><path d="m7 22-4-4 4-4"/><path d="M21 13v1a4 4 0 0 1-4 4H3"/>',
    tag: '<path d="M20.6 13.4 13.4 20.6a2 2 0 0 1-2.8 0L3 13V3h10l7.6 7.6a2 2 0 0 1 0 2.8Z"/><circle cx="7.5" cy="7.5" r="1.5"/>',
    sliders: '<path d="M4 21v-7M4 10V3M12 21v-9M12 8V3M20 21v-5M20 12V3M1 14h6M9 8h6M17 16h6"/>',
    flow: '<rect x="3" y="3" width="6" height="6" rx="1"/><rect x="15" y="15" width="6" height="6" rx="1"/><path d="M9 6h4a2 2 0 0 1 2 2v7"/>'
  });
  const E5 = window.WB.E5;
  const neu = '<span class="newb">neu</span>';
  const sw = (on) => `<span class="sw ${on ? 'on' : ''}"><i></i></span>`;

  // ================= Workflow on an entry =================
  const STEPS = [
    ['In Prüfung', 'info', 'Prüfung anfragen', 'fragt: Wer prüft? · braucht Story Points ✓', 'ok'],
    ['Bereit', 'neu', 'Zurückstellen', 'zurück in die Planung', 'ok'],
    ['Verworfen', 'neu', 'Verwerfen', 'nur Product Owner · Clara Neumann', 'lock']
  ];
  const wfMenu = `<div class="ac wfmenu note" style="left:0;top:calc(100% + 6px);inline-size:470px">${H.pin(2)}<span class="h">Von „In Arbeit“ weiter</span>
    ${STEPS.map(([to, tn, name, meta, st], k) => `<div class="o ${k === 0 ? 'on' : ''} ${st === 'lock' ? 'off' : ''}">${ic(st === 'lock' ? 'lock' : 'arrowr', 's')}<div class="col grow" style="gap:1px"><span><b style="font-weight:600">${name}</b></span><span class="cap subtle">${meta}</span></div>${loz(to, tn)}</div>`).join('')}
    <div class="wfhint">${ic('info', 's')}<span><b>Erledigt</b> erreicht die Story über <b>In Prüfung → Abnehmen</b>; abnehmen darf Clara Neumann (Feld „Abnahme durch“).</span></div>
    <div class="ft" style="justify-content:space-between"><span>${ic('flow', 's')}<b style="color:var(--text-brand)">Workflow ansehen</b></span><span>„Story“ · gilt in Team Atlas</span></div></div>`;
  const headWith = (status, extra = '') => E5.storyHead.replace(/<span class="statusbtn t-info">In Arbeit [^]*?<\/span>\$?/, '').replace('<div class="actionbar">', `<div class="actionbar">${status}`).replace(/<span class="btn pri">Prüfung anfragen<\/span>/, extra);
  const headMenu = headWith(`${note(1, `<span style="position:relative;display:inline-flex"><span class="statusbtn t-info" style="box-shadow:0 0 0 3px var(--accent-primary-subtle)">In Arbeit ${ic('chevd', 's')}</span>${wfMenu}</span>`, 'display:inline-flex')}`, btn('Prüfung anfragen', 'pri'));
  const bodyDim = `<div style="opacity:.35;pointer-events:none;margin-block-start:var(--sp-300)">${E5.anch([['Beschreibung'], ['Teile', '2/8'], ['Verknüpfungen', 7], ['Dateien', 9], ['Unterhaltung', 21]], 'Beschreibung')}${E5.sect('d', 'Beschreibung', null, 'Bearbeiten', E5.storyDesc, { icon: 'pen' })}</div>`;

  // the diagram: one column per category, like the designer
  const NODES = { neu: ['Neu', 0, 0, 'neu'], ber: ['Bereit', 1, 0, 'neu'], arb: ['In Arbeit', 2, 0, 'info'], pru: ['In Prüfung', 2, 1, 'info'], erl: ['Erledigt', 3, 0, 'ok'], ver: ['Verworfen', 3, 1, 'neu'] };
  const CX = [120, 370, 620, 870], RY = [110, 240];
  const nx = (k) => CX[NODES[k][1]], ny = (k) => RY[NODES[k][2]];
  const wfDiagram = (o = {}) => {
    const node = (k) => { const [n, , , tn] = NODES[k]; const here = k === 'arb'; const next = !here && (k === 'pru' || k === 'ber'); return `<div class="wn t-${tn} ${here ? 'here' : ''} ${next ? 'next' : ''}" style="left:${nx(k) - 75}px;top:${ny(k) - 22}px">${n}${here ? '<em>Du bist hier</em>' : ''}</div>`; };
    const A = (d, cls, lx, ly, t, extra = '') => `<path d="${d}" class="${cls}" marker-end="url(#wa-${cls || 'n'})"/><text x="${lx}" y="${ly}" class="${cls}" ${extra}>${t}</text>`;
    const svg = `<svg class="wsvg" viewBox="0 0 1000 300" width="1000" height="300"><defs>${['n', 'go', 'lk'].map((c) => `<marker id="wa-${c}" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M0 0L10 5L0 10z" class="mk-${c}"/></marker>`).join('')}</defs>
      ${A('M195 104 L293 104', '', 244, 94, 'Einplanen', 'text-anchor="middle"')}
      ${A('M445 102 L543 102', '', 494, 92, 'Starten', 'text-anchor="middle"')}
      ${A('M545 122 L447 122', 'go', 496, 142, 'Zurückstellen', 'text-anchor="middle"')}
      ${A('M600 134 L600 216', 'go', 592, 180, 'Prüfung anfragen', 'text-anchor="end"')}
      ${A('M640 216 L640 134', '', 650, 180, 'Zurückgeben')}
      ${A('M697 236 C 760 236, 760 110, 793 110', '', 768, 168, 'Abnehmen')}
      ${A('M720 272 L793 248', 'lk', 700, 290, '🔒 Verwerfen · aus jedem Status')}</svg>`;
    const cats = ['Backlog', 'Zu erledigen', 'In Arbeit', 'Erledigt'].map((c, k) => `<div class="wcat" style="left:${k * 250}px">${c}</div>`).join('');
    return `<div class="wfd" style="${o.scale ? `zoom:${o.scale}` : ''}">${cats}${svg}${Object.keys(NODES).map(node).join('')}</div>`;
  };
  const wfLegend = `<div class="row wrap cap" style="gap:var(--sp-300);color:var(--text-secondary)"><span class="row" style="gap:6px"><span class="wn here mini"></span>dein Status</span><span class="row" style="gap:6px"><i class="lg go"></i>Schritte, die du jetzt gehen kannst</span><span class="row" style="gap:6px"><i class="lg"></i>andere Schritte</span><span class="row" style="gap:6px"><i class="lg lk"></i>gesperrt für dich</span></div>`;
  const stepList = `<div style="display:grid;grid-template-columns:repeat(5,minmax(0,1fr));gap:var(--sp-100)">${[
    ['Prüfung anfragen', 'In Arbeit → In Prüfung', 'Alle im Team', 'Fragt: Wer prüft? · Braucht: Story Points', 'go'],
    ['Zurückstellen', 'In Arbeit → Bereit', 'Alle im Team', '–', 'go'],
    ['Abnehmen', 'In Prüfung → Erledigt', 'Person im Feld „Abnahme durch“', 'Prüft: alle Teile erledigt', ''],
    ['Zurückgeben', 'In Prüfung → In Arbeit', 'Person im Feld „Abnahme durch“', 'Fragt: Grund', ''],
    ['Verwerfen', 'jeder Status → Verworfen', 'Rolle Product Owner', 'Fragt: Grund', 'lk']].map(([n, ft, who, req, c]) => `<div class="wstep ${c}"><div class="row" style="gap:6px"><b>${n}</b><span class="grow"></span>${c === 'go' ? loz('kannst du', 'ok') : c === 'lk' ? loz('gesperrt', 'neu', 'lock') : ''}</div><span class="cap sub">${ft}</span><span class="cap subtle">Wer: ${who}</span><span class="cap subtle">${req}</span></div>`).join('')}</div>`;
  const wfDialog = `<div class="scrim"></div><div class="dlg" style="left:50%;top:70px;transform:translateX(-50%);inline-size:1100px"><header class="row" style="gap:var(--sp-150)">${ic('flow')}<div class="col grow" style="gap:2px"><h2 class="h" style="font-size:var(--fs-300);margin:0">Workflow „Story“</h2><span class="cap sub">gilt in Team Atlas – Mobile App für Story · 6 Status · 7 Schritte · nur lesen</span></div>${btn('Im Designer öffnen', 'ghost', 'ext')}${btn('', 'ico', 'x')}</header>
    <div class="col" style="gap:var(--sp-200);margin-block-start:var(--sp-300)">${note(3, wfDiagram())}${wfLegend}<h3 class="lbl">Schritte</h3>${stepList}</div></div>`;

  S.wf = () => {
    const page = (head, rest = bodyDim) => H.desk(sideHub('pe', 'Team Atlas – Mobile App'), `<div style="position:relative">${head}${rest}</div>`);
    const d = page(headMenu);
    const w = X.app2(sideHub('pe', 'Team Atlas – Mobile App'), `${E5.storyHead}${bodyDim}`, { over: wfDialog });
    const freeMenu = `<div class="ac wfmenu" style="left:0;top:calc(100% + 6px);inline-size:420px"><span class="h">Status · kein Workflow, jeder ist erreichbar</span>
      ${[['Zu erledigen', 'neu'], ['In Arbeit', 'info', 1], ['Erledigt', 'ok']].map(([n, tn, on]) => `<div class="o ${on ? 'on' : ''}">${on ? ic('check', 's') : '<span style="inline-size:16px"></span>'}<span class="grow">${loz(n, tn)}</span><span class="cap subtle">${['Zu erledigen', 'In Arbeit', 'Erledigt'].indexOf(n) === 2 ? 'schließt den Eintrag ab' : ''}</span></div>`).join('')}
      <div class="ft"><span>Standard-Workflow · eigener Workflow: Verwaltung › Workflows</span></div></div>`;
    const headFree = headWith(`${note(5, `<span style="position:relative;display:inline-flex"><span class="statusbtn t-info" style="box-shadow:0 0 0 3px var(--accent-primary-subtle)">In Arbeit ${ic('chevd', 's')}</span>${freeMenu}</span>`, 'display:inline-flex')}`, `<span class="btn pri">${ic('check')}Erledigt</span>`);
    const o = page(headFree);
    const m = phone(ptop('Team Atlas', 'TAMA-18', `${ic('star')}${ic('more')}`), `<div style="position:relative;min-block-size:640px;margin:calc(-1 * var(--sp-200))"><div class="col" style="gap:var(--sp-150);opacity:.4;padding:var(--sp-200)"><div class="row" style="gap:6px">${key('work', 'TAMA-18')}${loz('Story', 'neu', 'checksq')}</div><h1 class="h" style="font-size:var(--fs-450);margin:0">Bezahlen mit Apple Pay und Google Pay im Checkout</h1><div class="row" style="gap:6px"><span class="statusbtn t-info">In Arbeit ${ic('chevd', 's')}</span></div></div>
      <div class="psheet">${note(4, `<div class="grab"></div><h3 class="lbl" style="margin-block-end:var(--sp-100)">Von „In Arbeit“ weiter</h3>${STEPS.map(([to, tn, name, meta, st]) => `<div class="prow ${st === 'lock' ? 'off' : ''}">${ic(st === 'lock' ? 'lock' : 'arrowr', 's')}<div class="col grow" style="gap:2px"><b style="font-weight:600">${name}</b><span class="cap subtle">${meta}</span></div>${loz(to, tn)}</div>`).join('')}<div class="cap sub" style="padding-block:var(--sp-100)">Erledigt über In Prüfung → Abnehmen.</div>${btn('Workflow ansehen', '', 'flow')}`)}</div></div>`, 'Durchsuchen');
    return { d, w, o, m };
  };

  // ================= the view panel (collections and Plan) =================
  const vsec = (t, body, o = {}) => `<section class="vsec"><header><h4>${t}</h4>${o.badge || ''}<span class="grow"></span>${o.right || ''}</header>${body}</section>`;
  const opts = (items, on) => `<div class="opts">${items.map((x) => `<span class="${x === on ? 'on' : ''}">${x}</span>`).join('')}</div>`;
  const toggles = (rows) => `<div class="col" style="gap:2px">${rows.map(([t, on, extra]) => `<div class="trow"><span class="grow">${t}${extra ? ` <span class="cap subtle">${extra}</span>` : ''}</span>${sw(on)}</div>`).join('')}</div>`;
  const fieldList = (rows) => `<div class="flist">${rows.map(([t, on, extra]) => `<div class="fr">${ic('grip', 's')}<span class="check ${on ? 'on' : ''}">${on ? ic('check', 's') : ''}</span><span class="grow">${t}</span>${extra || ''}</div>`).join('')}</div>`;
  const saveBox = (o = {}) => `<div class="vsave"><div class="row" style="gap:6px"><span class="dot"></span><b style="font-weight:600">${o.t || 'Geändert gegenüber „Liste“'}</span></b><span class="grow"></span><span class="btn link cap" style="padding:0">Verwerfen</span></div>
    <div class="fld"><label>Name</label><span class="field" style="color:var(--text-primary)">${o.name || 'Offene Stories nach Person'}</span></div>
    <div class="fld"><label>Wo sie lebt</label>${opts(['Nur ich', 'Diese Sammlung', 'Dieser Hub', 'Ganzer Arbeitsbereich'], o.where || 'Diese Sammlung')}</div>
    <div class="fld"><label>Wer sie sieht</label>${opts(['Nur ich', 'Alle, die den Bereich lesen'], 'Alle, die den Bereich lesen')}</div>
    <div class="row" style="gap:var(--sp-100);flex-wrap:wrap">${btn('Als neue Ansicht speichern', 'pri')}${btn('„Liste“ überschreiben', '')}</div></div>`;
  const panel = (title, body, o = {}) => `<aside class="vpanel" style="${o.style || ''}"><header class="row" style="gap:var(--sp-100)">${ic('sliders')}<h3 class="grow">${title}</h3><span class="btn link cap" style="padding:0">Wie die Vorlage</span>${btn('', 'ico', 'x')}</header><div class="vbody">${body}</div></aside>`;

  // ---------- Plan: Gantt with the panel open ----------
  S.planview = () => {
    const body = `${vsec('Darstellung', `<span class="preset"><span>${ic('timeline', 's')}Zeitleiste</span><span class="on">${ic('rows', 's')}Gantt</span></span><span class="cap subtle">Zeitleiste: leicht, ohne Spalten. Gantt: Spalten, kritischer Pfad, Baseline.</span>`)}
      ${vsec('Was zu sehen ist', `<div class="qf" style="min-block-size:0;padding:6px 8px">${X.tok('Sammlung', 'Projektplan')}${X.tok('Typ', 'nicht Risiko')}<span class="cap subtle">+ Bedingung</span></div>${opts(['Erledigtes zeigen', 'ausblenden'], 'Erledigtes zeigen')}`, { right: '<span class="cap subtle">Hubtask-Abfrage</span>' })}
      ${vsec('Zeilen', `<div class="fld"><label>Gruppieren nach</label>${opts(['Gliederung', 'Sammlung', 'Zuständig', 'Release', 'Keine'], 'Gliederung')}</div><div class="fld"><label>Sortieren nach</label>${opts(['Beginn', 'Ende', 'Eigene Reihenfolge'], 'Eigene Reihenfolge')}</div>`, { badge: neu })}
      ${vsec('Balken', `<div class="fld"><label>Farbe nach</label>${opts(['Status', 'Sammlung', 'Zuständig', 'Label'], 'Status')}</div>${toggles([['Abhängigkeiten', 1], ['Kritischer Pfad', 1], ['Puffer', 1], ['Fortschritt im Balken', 1], ['Meilensteine', 1], ['Wochenenden und Feiertage', 1, 'Arbeitskalender']])}`, { badge: neu })}
      ${vsec('Vergleichen mit', `<span class="field" style="color:var(--text-primary);justify-content:space-between">Baseline „Freigabe Sept.“ ${ic('chevd', 's')}</span><span class="cap subtle">Gleiche Auswahl wie heute; zeigt Verzug als Spalte und als grauen Strich.</span>`)}
      ${vsec('Spalten links', fieldList([['Beginn', 1], ['Ende', 1], ['Arbeitstage', 1], ['Zuständig', 1], ['Puffer', 0], ['Verzug (Tage)', 0, '<span class="cap subtle">mit Baseline</span>'], ['Fortschritt', 0]]), { badge: neu })}
      ${saveBox({ t: 'Geändert gegenüber „Zeitplan“', name: 'Gantt für den Lenkungskreis', where: 'Dieser Hub' })}
      <div class="cap subtle" style="padding:var(--sp-150) 0">${ic('gear', 's')} Toleranz und Baselines verwalten bleiben unter ⋯ › Planeinstellungen.</div>`;
    const d = X.app2(sideHub('nb', 'Plan'), `<div style="min-block-size:1520px">${X.planHead(true, { panel: 1 })}<div style="margin-inline-end:400px">${X.plan({ gantt: true, base: true, dayW: 5.2, left: 410 })}</div></div>`, { over: note(7, panel('Ansicht · Zeitplan', body, { style: 'position:relative;inset:auto' }), 'position:absolute;right:0;top:56px;inline-size:400px') });
    return { d };
  };

  // ---------- Plan: table ----------
  const TROWS = [
    [0, 'Grundlagenermittlung und Vorplanung', '14.09.', '07.10.', 18, 160, '', '', 'Von Hand', 0, 1],
    [0, 'Entwurfs- und Genehmigungsplanung', '08.10.', '06.11.', 22, 160, '', '', 'Von Hand', 0, 1],
    [0, 'Bauantrag einreichen', '06.11.', '06.11.', 0, 0, '', '', 'Automatisch', 1],
    [0, 'Ausführungsplanung Tragwerk, TGA und Fördertechnik', '09.11.', '22.01.', 54, 12, '', '+3', 'Automatisch'],
    [1, 'Statik Hallendach (Spannweite 42 m)', '12.11.', '27.11.', 12, 30, '', '', 'Von Hand'],
    [1, 'Sprinkleranlage nach VdS CEA 4001', '16.11.', '04.12.', 15, 26, '', '', 'Von Hand'],
    [0, 'Ausschreibung und Vergabe der Gewerke', '07.12.', '29.01.', 38, 0, 1, '+3', 'Automatisch', 0, 0, '10.12. → 15.12.'],
    [0, 'Rohbau Halle A und B', '01.02.', '30.04.', 64, 0, 1, '+3', 'Automatisch', 0, 0, '04.02. → 09.02.'],
    [0, 'Inbetriebnahme Fördertechnik und Probebetrieb', '03.05.', '04.06.', 24, 0, 1, '+3', 'Automatisch', 0, 0, '06.05. → 11.05.'],
    [0, 'Übergabe an den Betrieb', '18.06.', '18.06.', 0, 0, 1, '+3', 'Automatisch', 1]];
  S.plantable = () => {
    const banner = note(3, `<div class="band warn">${ic('sparkle')}<span class="grow"><b>Der Plan schlägt vor, 3 Einträge zu verschieben</b>, weil „Ausführungsplanung“ 3 Tage länger dauert. Vorschläge stehen in der Spalte Beginn.</span>${btn('Einzeln prüfen', 'ghost')}${btn('Alle 3 annehmen', 'pri')}</div>`);
    const tbl = `<div class="reg" style="overflow:hidden"><table class="tbl2 ptbl"><thead><tr><th style="inline-size:34%">Eintrag</th><th>Beginn</th><th>Ende</th><th>Arbeitstage</th><th>Puffer</th><th>Verzug</th><th>Planung</th><th></th></tr></thead><tbody>
      ${TROWS.map(([lvl, t, b, e, wd, buf, crit, delay, mode, ms, done, sug]) => `<tr class="${done ? 'done' : ''}"><td><span class="row" style="gap:6px;padding-inline-start:${lvl * 20}px">${ms ? '<span style="color:var(--label-violet-fg)">◆</span>' : lvl ? '' : ic('chevd', 's')}<span class="ell" style="${done ? 'color:var(--text-subtle)' : ''}">${t}</span>${crit ? loz('kritisch', 'bad') : ''}${done ? loz('erledigt', 'ok') : ''}</span></td>
        <td class="num">${sug ? `<span class="sug">${sug}</span>` : b}</td><td class="num">${e}</td><td class="num">${wd || '–'}</td><td class="num">${buf ? buf + ' T' : crit ? '<span style="color:var(--status-danger-text)">0</span>' : '–'}</td><td class="num">${delay ? `<span style="color:var(--status-danger-text)">${delay} T</span>` : '–'}</td><td><span class="mode ${mode === 'Automatisch' ? 'auto' : ''}">${mode === 'Automatisch' ? ic('zap', 's') : ic('hand', 's')}${mode}</span></td><td class="subtle">${ic('more', 's')}</td></tr>`).join('')}</tbody></table></div>`;
    const tools = note(2, `<div class="row" style="gap:var(--sp-100);margin-block:var(--sp-150)">${X.qline ? '' : ''}<div class="qf grow" style="min-block-size:0;padding:6px 8px">${X.tok('Sammlung', 'Projektplan')}<span class="cap subtle">Filtern …</span></div>${chip('Baseline: Freigabe Sept.', 'layers')}${btn('Spalten', 'ghost', 'cols')}${btn('Exportieren', 'ghost', 'download')}<span class="btn">${ic('sliders')}Ansicht</span></div>`);
    const d = X.app2(sideHub('nb', 'Plan'), `${X.planTop({ on: 'Tabelle', pin: 1, fig: 'Endet am <b>18. Juni 2027</b> · 4 auf dem kritischen Pfad · <span style="color:var(--status-danger-text)">3 Tage Verzug ggü. Baseline „Freigabe Sept.“</span>' })}${tools}${banner}${note(4, tbl)}<div class="cap subtle" style="margin-block-start:var(--sp-150)">◆ Meilenstein · „Von Hand“: Termine bleiben, wie gesetzt · „Automatisch“: folgt den Abhängigkeiten · Beginn und Ende sind direkt editierbar.</div>`);
    return { d };
  };

  // ---------- Plan: dependencies ----------
  S.plandeps = () => {
    const rows = [['Entwurfs- und Genehmigungsplanung', 'Grundlagenermittlung und Vorplanung', 'Ende zu Anfang', 0], ['Bauantrag einreichen', 'Entwurfs- und Genehmigungsplanung', 'Ende zu Anfang', 0], ['Ausführungsplanung Tragwerk, TGA und Fördertechnik', 'Bauantrag einreichen', 'Anfang zu Anfang', 1], ['Ausschreibung und Vergabe der Gewerke', 'Ausführungsplanung Tragwerk, TGA und Fördertechnik', 'Anfang zu Anfang', 20, 'bad'], ['Rohbau Halle A und B', 'Ausschreibung und Vergabe der Gewerke', 'Ende zu Anfang', 0], ['Inbetriebnahme Fördertechnik und Probebetrieb', 'Rohbau Halle A und B', 'Ende zu Anfang', 0], ['Übergabe an den Betrieb', 'Inbetriebnahme Fördertechnik und Probebetrieb', 'Ende zu Anfang', 0]];
    const pop = `<div class="ac note" style="right:40px;top:200px;inline-size:330px;padding:var(--sp-200)">${H.pin(3)}<span class="h">Zeitbezug</span><div class="fld"><label>Art</label><div class="opts">${['Ende zu Anfang', 'Anfang zu Anfang', 'Ende zu Ende', 'Anfang zu Ende'].map((x, k) => `<span class="${k === 1 ? 'on' : ''}">${x}</span>`).join('')}</div></div><div class="fld"><label>Versatz in Arbeitstagen</label><span class="field" style="color:var(--text-primary)">20</span><span class="help">Minus heißt Vorlauf; −999 bis 999.</span></div><div class="row" style="gap:6px;justify-content:flex-end">${btn('Entfernen', 'ghost')}${btn('Übernehmen', 'pri')}</div></div>`;
    const tbl = `<div class="reg" style="position:relative"><table class="tbl2"><thead><tr><th style="inline-size:36%">Wartet</th><th></th><th style="inline-size:36%">auf</th><th>Zeitbezug</th><th></th></tr></thead><tbody>${rows.map(([a, b, art, lag, st]) => `<tr><td><span class="ell">${a}</span></td><td class="subtle">${ic('chevl', 's')}</td><td><span class="ell">${b}</span></td><td><span class="chip ${st === 'bad' ? 'bad' : ''}">${art}${lag ? ` · ${lag} T` : ''}</span></td><td class="subtle">${ic('more', 's')}</td></tr>`).join('')}</tbody></table>${pop}</div>`;
    const band = note(2, `<div class="band bad">${ic('alert')}<span class="grow"><b>1 Abhängigkeit kann der Plan nicht halten:</b> „Ausschreibung“ soll 20 Arbeitstage nach Beginn der Ausführungsplanung starten, ist aber auf einen festen Termin „Von Hand“ gesetzt.</span>${btn('Termin lösen', 'ghost')}</div>`);
    const d = X.app2(sideHub('nb', 'Plan'), `<div style="min-block-size:880px">${X.planTop({ on: 'Abhängigkeiten', pin: 1, fig: 'Endet am <b>18. Juni 2027</b> · 4 auf dem kritischen Pfad · <span style="color:var(--status-danger-text)">3 Tage Verzug ggü. Baseline „Freigabe Sept.“</span>' })}<div class="row" style="gap:var(--sp-100);margin-block:var(--sp-150)"><span class="cap sub grow">7 Abhängigkeiten · im Gantt ziehst du sie vom Balkenende auf den wartenden Eintrag</span>${btn('Abhängigkeit hinzufügen', 'pri', 'plus')}</div>${band}${note(4, tbl)}</div>`);
    return { d };
  };

  // ---------- Plan: releases ----------
  const PE = { hub: 'Produktentwicklung', hubI: 'rocket', hubC: 'blue', sprints: 1, fig: '2 Releases geplant · nächstes: <b>App 4.2 „Schlüssellos“ am 25. Nov.</b> · Sprint 24 läuft noch 7 Tage' };
  S.planrel = () => {
    const axis = note(2, `<div class="relaxis"><div class="ax">${['Okt.', 'Nov.', 'Dez.', 'Jan.'].map((m, k) => `<span style="left:${k * 25}%">${m}</span>`).join('')}</div><i class="today" style="left:34%"></i>${[['App 4.1', 'erschienen 28. Okt.', 21.8, 'ok'], ['App 4.2 „Schlüssellos“', '25. Nov. · 1 von 3', 45, 'on'], ['API v3.8', '9. Dez. · 0 von 2', 56.5, 'low']].map(([n, s, x, c]) => `<div class="rel ${c}" style="left:${x}%"><i></i><b>${n}</b><span>${s}</span></div>`).join('')}</div>`);
    const list = `<div class="col" style="gap:var(--sp-100)">${[['App 4.2 „Schlüssellos“', 'Geplant · 25. Nov.', 33, 1], ['API v3.8', 'Geplant · 9. Dez.', 0], ['App 4.1', 'Erschienen · 28. Okt.', 100]].map(([n, s, p, on]) => `<div class="relc ${on ? 'on' : ''}">${ic('tag', 's')}<div class="col grow" style="gap:4px"><b style="font-weight:600">${n}</b><span class="cap subtle">${s}</span>${H.bar(p, p === 100 ? 'ok' : '')}</div></div>`).join('')}<span class="btn ghost" style="justify-content:flex-start">${ic('plus')}Release planen</span></div>`;
    const items = [['work', 'TAMA-1', 'Als Kundin möchte ich mich mit Passkey anmelden', 'Story', 'In Arbeit', 'info', 'Team Atlas'], ['work', 'TAMA-13', 'Dunkelmodus folgt der Systemeinstellung', 'Story', 'Erledigt', 'ok', 'Team Atlas'], ['work', 'TAMA-14', 'Screenreader liest Preise inklusive Währung vor', 'Story', 'In Prüfung', 'info', 'Team Atlas']];
    const det = `<div class="reg" style="padding:var(--sp-300)"><div class="row" style="gap:var(--sp-150);align-items:flex-start"><div class="col grow" style="gap:4px"><span class="row" style="gap:6px">${loz('Geplant', 'neu')}<span class="cap sub">soll am <b>25. Nov. 2026</b> erscheinen · in 13 Tagen</span></span><h2 class="h" style="font-size:var(--fs-400);margin:0">App 4.2 „Schlüssellos“</h2><span class="sub">Passkeys, Dunkelmodus und Preise für Screenreader.</span></div>${btn('Ändern', 'ghost', 'pen')}${note(3, btn('Veröffentlichen', 'pri', 'send'), 'display:inline-flex')}</div>
      <div class="row" style="gap:var(--sp-150);margin-block:var(--sp-200)">${H.bar(33, 'ok')}<span class="cap"><b>1 erledigt</b>, 2 offen</span><span class="cap subtle">· erscheint es, werden die Notizen eine Seite in „Inhalte“</span></div>
      <div class="row" style="margin-block-end:var(--sp-100)"><h3 class="lbl grow">Im Release · 3</h3>${note(4, `<span class="btn ghost">${ic('plus')}Einträge hinzufügen</span>`, 'display:inline-flex')}</div>
      <table class="tbl2"><thead><tr><th>Eintrag</th><th>Typ</th><th>Sammlung</th><th>Stand</th><th></th></tr></thead><tbody>${items.map(([p, k, t, ty, st, tn, c]) => `<tr><td><span class="row" style="gap:6px">${key(p, k)}<span class="ell">${t}</span></span></td><td class="cap sub">${ty}</td><td class="cap sub">${c}</td><td>${loz(st, tn)}</td><td class="subtle">${ic('x', 's')}</td></tr>`).join('')}</tbody></table>
      <div class="cap subtle" style="margin-block-start:var(--sp-150)">Einträge kommen aus allen Sammlungen des Hubs – per Auswahl oder Abfrage: <span class="code">release = "App 4.2"</span></div></div>`;
    const d = X.app2(sideHub('pe', 'Plan'), `${X.planTop(Object.assign({ on: 'Releases', pin: 1, acts: `${btn('Release planen', 'pri', 'plus')}${btn('', 'ico', 'more')}` }, PE))}${axis}<div style="display:grid;grid-template-columns:280px minmax(0,1fr);gap:var(--sp-300);margin-block-start:var(--sp-200)">${list}${det}</div>`);
    const m = phone(ptop('Produktentwicklung', 'Plan', ic('more')), `<div class="col" style="gap:var(--sp-150)"><div class="viewbar" style="overflow:hidden;flex-wrap:nowrap"><span>Zeitplan</span><span>Tabelle</span><span class="on">Releases</span><span>Sprints</span></div>${list}</div>`, 'Durchsuchen');
    return { d, m };
  };

  // ---------- burndown ----------
  const burn = (w = 220, h = 56) => { const pts = [8, 8, 8, 8, 8, 8, 8]; const x = (k) => (k / 10) * w; const y = (v) => h - 4 - (v / 9) * (h - 10); return `<svg class="burn" viewBox="0 0 ${w} ${h}" width="${w}" height="${h}"><path class="ideal" d="M0 ${y(8)} L${w} ${y(0)}"/><path class="real" d="${pts.map((v, k) => `${k ? 'L' : 'M'}${x(k)} ${y(v)}`).join(' ')}"/><circle cx="${x(6)}" cy="${y(8)}" r="3" class="dotb"/></svg>`; };

  // ---------- Plan: sprints across the hub ----------
  S.plansprints = () => {
    const runC = `<div class="reg sprc"><div class="row" style="gap:var(--sp-150);align-items:flex-start"><div class="col grow" style="gap:4px"><span class="row" style="gap:6px">${loz('läuft', 'info')}<span class="cap sub">Team Atlas – Mobile App · 10. – 24. Nov.</span></span><h3 class="h" style="font-size:var(--fs-300);margin:0">Sprint 24 – Passkeys</h3><span class="cap sub">Ziel: Passwortlose Anmeldung für alle Kundinnen und Kunden</span></div>${btn('Sprint öffnen', '', 'arrowr')}</div>
      <div class="row" style="gap:var(--sp-400);margin-block-start:var(--sp-200);align-items:flex-end">${[['Noch', '7 Tage'], ['Erledigt', '0 von 8 Punkten'], ['Einträge', '4'], ['Velocity', '8 Punkte']].map(([k, v]) => `<div class="col" style="gap:0"><span class="cap subtle">${k}</span><b style="font-size:var(--fs-200)">${v}</b></div>`).join('')}<span class="grow"></span><div class="col" style="gap:2px;align-items:flex-end">${burn(260, 64)}<span class="cap subtle">Burndown · gestrichelt = ideal</span></div></div></div>`;
    const planned = `<div class="reg sprc"><div class="row" style="gap:var(--sp-150)"><div class="col grow" style="gap:4px"><span class="row" style="gap:6px">${loz('geplant', 'neu')}<span class="cap sub">Team Atlas · 24. Nov. – 5. Dez.</span></span><h3 class="h" style="font-size:var(--fs-200);margin:0">Sprint 25 – Wallets</h3></div><div class="col" style="gap:4px;inline-size:260px"><span class="cap"><b>5</b> von ≈ 8 Punkten eingeplant</span>${H.bar(62)}</div>${btn('Im Backlog planen', 'ghost')}</div></div>`;
    const boreal = `<div class="reg sprc" style="border-style:dashed;background:transparent"><div class="row" style="gap:var(--sp-150)">${tile('work')}<span class="grow"><b style="font-weight:600">Team Boreal – Platform &amp; API</b> <span class="cap sub">arbeitet ohne Sprints (Board mit Limits)</span></span>${btn('Sprints einschalten', 'ghost')}</div></div>`;
    const closed = `<div class="reg" style="padding:var(--sp-200)"><div class="row" style="margin-block-end:var(--sp-100)"><h3 class="lbl grow">Geschlossen</h3><span class="cap sub">Velocity: 8 Punkte je Sprint</span></div><table class="tbl2"><thead><tr><th>Sprint</th><th>Sammlung</th><th style="text-align:end">Zugesagt</th><th style="text-align:end">Geschafft</th><th style="inline-size:30%"></th></tr></thead><tbody>${[['Sprint 23 – Barrierefreiheit', 10, 8], ['Sprint 22 – Checkout', 9, 9], ['Sprint 21 – Push', 8, 7]].map(([n, a, b]) => `<tr><td>${n}</td><td class="cap sub">Team Atlas</td><td class="num" style="text-align:end">${a}</td><td class="num" style="text-align:end">${b}</td><td><span class="vel"><i style="inline-size:${a * 8}%"></i><b style="inline-size:${b * 8}%"></b></span></td></tr>`).join('')}</tbody></table></div>`;
    const d = X.app2(sideHub('pe', 'Plan'), `${X.planTop(Object.assign({ on: 'Sprints', pin: 1, acts: btn('', 'ico', 'more') }, PE))}<div class="col" style="gap:var(--sp-200);margin-block-start:var(--sp-200)"><h3 class="lbl">Läuft</h3>${note(2, runC)}<h3 class="lbl">Als Nächstes</h3>${note(3, planned)}${note(4, boreal)}${note(5, closed)}</div>`);
    return { d };
  };

  // ---------- the collection's sprint view ----------
  const atlasHead = () => collHead('Produktentwicklung', htile('rocket', 'blue'), 'Team Atlas – Mobile App', 'iOS- und Android-App für Endkundinnen und Endkunden.', '<b>7 offen</b><span>· Sprint 24 noch 7 Tage</span>', `${loz('Bug', 'neu', 'bug')}${loz('Story', 'neu')}`);
  const atlasViews = (on) => viewbar([`${ic('board')}Board`, `${ic('repeat')}Sprint 24 – Passkeys`, `${ic('list')}Backlog`, `${ic('checksq')}Liste`, `${ic('timeline')}Zeitleiste`, `${ic('clock')}Zeit`, `${ic('star')}Meine`, `${ic('plus')}`], on);
  const est = (p) => p ? `<span class="estc">${p} P</span>` : '<span class="estc empty">schätzen</span>';
  const SPR = [['Zu erledigen', '3', [['TAMA-17', 'Passkey-Anmeldung: Wiederherstellung über ein zweites Gerät', '', chip('heute 18:00', 'clock', 'warn') + est(3), 'JK']]],
    ['In Arbeit', '3', [['TAMA-1', 'Als Kundin möchte ich mich mit Passkey anmelden', lab('Kundenwunsch', 'violet') + lab('Sicherheit', 'red'), chip('20. Nov.') + est(3), 'AH'], ['TAMA-15', 'App stürzt beim Öffnen eines Push-Links ab', lab('Tech-Schuld', 'orange'), est(0), 'MY']]],
    ['In Prüfung', '2', [['TAMA-14', 'Screenreader liest Preise inklusive Währung vor', lab('Barrierefreiheit', 'teal'), est(2), 'SR']]]];
  S.sprint = () => {
    const sbar = note(2, `<div class="sprbar"><span class="field" style="color:var(--text-primary);gap:6px">${ic('repeat', 's')}<b style="font-weight:600">Sprint 24 – Passkeys</b>${loz('läuft', 'info')}${ic('chevd', 's')}</span><div class="col" style="gap:2px;min-inline-size:0;flex:1"><span class="cap subtle">Ziel</span><span class="ell">Passwortlose Anmeldung für alle Kundinnen und Kunden</span></div>
      ${[['Noch', '7 Tage', '10. – 24. Nov.'], ['Erledigt', '0 / 8 P', '4 Einträge']].map(([k, v, s]) => `<div class="col" style="gap:0"><span class="cap subtle">${k}</span><b>${v}</b><span class="cap subtle">${s}</span></div>`).join('')}<div class="col" style="gap:0;align-items:flex-end">${burn(170, 44)}<span class="cap subtle">Burndown ${ic('chev', 's')}</span></div>${btn('Sprint schließen', '', 'check')}${btn('', 'ico', 'more')}</div>`);
    const cols = `<div class="board" style="grid-template-columns:repeat(3,minmax(0,1fr)) 56px">${SPR.map(([n, pts, cards]) => `<div class="bcol"><header>${n}<span class="cap subtle">${cards.length} · ${pts} P</span><span class="grow"></span><span class="subtle">${ic('more')}</span></header>${cards.map((x) => card(...x)).join('')}</div>`).join('')}<div class="bcol" style="inline-size:56px;align-items:center;writing-mode:vertical-rl;padding-block:var(--sp-200)"><span class="cap semi">Erledigt · 0</span></div></div>`;
    const tools = `<div class="toolbar"><div class="qf" style="min-block-size:0;padding:5px 8px;inline-size:340px">${X.tok('Zuständig', 'alle')}<span class="cap subtle">Filtern …</span></div>${chip('Nur meine', 'user')}<span class="grow"></span><span class="cap subtle">Spalten = Status des Workflows „Story“</span><span class="btn">${ic('sliders')}Ansicht</span></div>`;
    const d = H.desk(sideHub('pe', 'Team Atlas – Mobile App'), `${atlasHead()}${atlasViews(`${ic('repeat')}Sprint 24 – Passkeys`)}${sbar}${tools}${note(3, cols)}<div class="cap sub" style="margin-block-start:var(--sp-150)">${ic('info', 's')} Neue Arbeit kommt aus dem Backlog in den Sprint – im Reiter <b>Backlog</b>. Nach dem Start Hinzugekommenes trägt „nach dem Start“.</div>`);
    const m = phone(ptop('Team Atlas', 'Sprint 24', ic('more')), `<div class="col" style="gap:var(--sp-150)"><div class="reg" style="padding:var(--sp-150)"><div class="row" style="gap:6px">${loz('läuft', 'info')}<span class="cap sub">noch 7 Tage · 0 / 8 P</span></div><span class="cap" style="margin-block:4px">Passwortlose Anmeldung für alle</span>${burn(320, 48)}</div>
      ${note(4, `<div class="seg" style="inline-size:100%;overflow:hidden"><span>Zu erledigen 1</span><span class="on">In Arbeit 2</span><span>In Prüfung 1</span></div>`)}<div class="bcol">${SPR[1][2].map((x) => card(...x)).join('')}</div></div>`, 'Durchsuchen');
    return { d, m };
  };

  // ---------- backlog and sprint planning ----------
  const brow = (k, t, typ, p, a, o = {}) => `<div class="brow ${o.lift ? 'lift' : ''}">${ic('grip', 's')}${key('work', k)}<span class="grow ell">${t}</span>${typ ? loz(typ, 'neu') : ''}${est(p)}${a ? av(a, 's') : '<span style="inline-size:22px"></span>'}</div>`;
  S.backlog = () => {
    const sec = (title, meta, right, rows, o = {}) => `<section class="bsec ${o.cls || ''}"><header>${ic('chevd', 's')}<b>${title}</b>${meta}<span class="grow"></span>${right}</header>${rows}</section>`;
    const s24 = sec('Sprint 24 – Passkeys', `${loz('läuft', 'info')}<span class="cap sub">10. – 24. Nov. · 4 Einträge · 8 P</span>`, btn('Sprint öffnen', 'ghost', 'arrowr'), `<div class="cap subtle" style="padding:6px var(--sp-150)">eingeklappt · 4 Einträge</div>`, { cls: 'fold' });
    const s25 = sec('Sprint 25 – Wallets', `${loz('geplant', 'neu')}<span class="cap sub">24. Nov. – 5. Dez.</span>`, `<span class="col" style="gap:3px;inline-size:220px"><span class="cap"><b>5</b> von ≈ 8 P · Velocity der letzten 3</span>${H.bar(62)}</span>${btn('Ändern', 'ghost', 'pen')}${btn('Sprint starten', 'pri', 'play')}`,
      `${brow('TAMA-19', 'Rechnungen in der App als PDF ansehen', 'Story', 5, 'AH')}${brow('TAMA-10', 'Offline mode: orders placed without a connection are queued', 'Story', 0, 'SR', { lift: 1 })}<div class="drop">Hier ablegen · ${ic('plus', 's')} Eintrag direkt im Sprint anlegen</div>`);
    const bl = sec('Backlog', '<span class="cap sub">5 Einträge · 7 P geschätzt · 2 ohne Schätzung</span>', `${btn('Sprint planen', '', 'plus')}`,
      `${brow('TAMA-16', 'Checkout button hidden behind the keyboard on small screens', 'Bug', 2, 'JW')}${brow('TAMA-23', 'Bestellhistorie auch ohne Verbindung zeigen', 'Story', 3, '')}${brow('TAMA-20', 'Bestellstatus als Push-Benachrichtigung', 'Story', 0, '')}${brow('TAMA-21', 'Adressbuch: zuletzt genutzte Lieferadresse vorschlagen', 'Story', 2, 'PN')}${brow('TAMA-22', 'Barrierefreie Fehlermeldungen im Checkout', 'Story', 0, '')}<div class="drop" style="border-style:solid;color:var(--text-subtle)">${ic('plus', 's')} Eintrag – Titel tippen, ↵ legt an</div>`);
    const d = H.desk(sideHub('pe', 'Team Atlas – Mobile App'), `${atlasHead()}${atlasViews(`${ic('list')}Backlog`)}<div class="toolbar"><div class="qf" style="min-block-size:0;padding:5px 8px;inline-size:380px">${X.tok('Typ', 'Story, Bug')}<span class="cap subtle">Filtern …</span></div>${chip('Ohne Schätzung', 'filter')}<span class="grow"></span><span class="cap subtle">Ziehen ordnet und plant ein · Rangfolge oben = zuerst</span></div>
      <div class="col" style="gap:var(--sp-150)">${note(2, s24)}${note(3, s25)}${note(4, bl)}</div>`);
    return { d };
  };

  // ---------- collection: the view panel ----------
  S.viewcfg = () => {
    const LAYOUTS = [['list', 'Liste', 'Zeilen, Felder als Spalten'], ['outline', 'Gliederung', 'mit Teilen darunter'], ['board', 'Board', 'Spalten nach Status oder Feld'], ['timeline', 'Zeitleiste', 'Balken auf der Zeitachse'], ['calendar', 'Kalender', 'nach Fälligkeit']];
    const body = `${vsec('Layout', `<div class="ltiles">${LAYOUTS.map(([i, n, s], k) => `<div class="lt ${k === 0 ? 'on' : ''}">${ic(i)}<b>${n}</b><span>${s}</span>${k === 4 ? neu : ''}</div>`).join('')}</div>`)}
      ${vsec('Was zu sehen ist', `<div class="qf" style="min-block-size:0;padding:6px 8px">${X.tok('Typ', 'Story')}${X.tok('Status', 'nicht Erledigt')}<span class="cap subtle">+ Bedingung</span></div><span class="cap subtle">Als Zeile: <span class="code">type = Story AND status != Erledigt</span></span>`, { right: '<span class="cap subtle">Hubtask-Abfrage</span>' })}
      ${vsec('Ordnen', `<div class="fld"><label>Gruppieren nach</label>${opts(['Nicht gruppiert', 'Typ', 'Status', 'Zuständig', 'Fälligkeit', 'Label', 'Sprint'], 'Zuständig')}</div><div class="fld"><label>Sortieren nach</label><div class="row" style="gap:6px"><span class="field grow" style="color:var(--text-primary);justify-content:space-between">Fällig ${ic('chevd', 's')}</span><span class="seg"><span class="on">Aufsteigend</span><span>Absteigend</span></span></div><span class="help">oder „Die Reihenfolge, die du festgelegt hast“</span></div><div class="fld"><label>Erledigtes</label>${opts(['an seinem Platz', 'am Ende', 'ausblenden'], 'ausblenden')}</div>`)}
      ${vsec('Felder', fieldList([['Schlüssel', 1], ['Typ', 1], ['Status', 1], ['Zuständig', 1], ['Fällig', 1], ['Labels', 1], ['Teile', 0], ['Story Points', 1, neu], ['Sprint', 0, neu]]), { right: '<span class="cap subtle">ziehen ordnet</span>' })}
      ${vsec('Nur für Board', `<div class="col" style="gap:6px;opacity:.6"><div class="trow"><span class="grow">Spalten nach</span><span class="cap">Status</span></div><div class="trow"><span class="grow">Bahnen nach ${neu}</span><span class="cap">Zuständig</span></div><div class="trow"><span class="grow">Limits je Spalte</span>${sw(1)}</div></div>`)}
      ${saveBox()}`;
    const rows = [['AH', 'Amelia Hughes', [['TAMA-1', 'Als Kundin möchte ich mich mit Passkey anmelden', 'In Arbeit', 'info', '20. Nov.', '3'], ['TAMA-18', 'Bezahlen mit Apple Pay und Google Pay im Checkout', 'In Arbeit', 'info', '27. Nov.', '8']]], ['JK', 'Jana Koch', [['TAMA-17', 'Passkey-Anmeldung: Wiederherstellung über ein zweites Gerät', 'Zu erledigen', 'neu', 'heute', '3']]], ['SR', 'Sofia Romano', [['TAMA-14', 'Screenreader liest Preise inklusive Währung vor', 'In Prüfung', 'info', '–', '2'], ['TAMA-10', 'Offline mode: orders are queued', 'Zu erledigen', 'neu', '27. Nov.', '–']]]];
    const list = `<section class="reg">${rows.map(([a, n, items]) => `<div class="rh" style="padding:var(--sp-100) var(--sp-150);gap:6px">${av(a, 's')}<b style="font-weight:600">${n}</b><span class="cap subtle">${items.length}</span></div>${items.map(([k, t, st, tn, due, p]) => `<div class="lrow" style="padding-inline:var(--sp-150)">${key('work', k)}<span class="grow ell">${t}</span>${loz('Story', 'neu')}${loz(st, tn)}<span class="cap sub" style="inline-size:60px">${due}</span><span class="cap sub" style="inline-size:28px;text-align:end">${p}</span></div>`).join('')}`).join('')}</section>`;
    const vb = (viewbar([`${ic('board')}Board`, `${ic('repeat')}Sprint 24 – Passkeys`, `${ic('list')}Backlog`, `${ic('checksq')}Liste <span class="dot"></span>`, `${ic('timeline')}Zeitleiste`, `${ic('clock')}Zeit`, `${ic('star')}Meine`, `${ic('plus')}`], `${ic('checksq')}Liste <span class="dot"></span>`));
    const tools = `<div class="toolbar"><div class="qf" style="min-block-size:0;padding:5px 8px;inline-size:340px">${X.tok('Typ', 'Story')}${X.tok('Status', 'nicht Erledigt')}</div><span class="grow"></span>${note(2, `<span class="btn on">${ic('sliders')}Ansicht · 3 geändert</span>`, 'display:inline-flex')}</div>`;
    const d = X.app2(sideHub('pe', 'Team Atlas – Mobile App'), `<div style="margin-inline-end:400px;min-block-size:1620px">${atlasHead()}${vb}${tools}${list}</div>`, { over: note(3, panel('Ansicht · Liste', body, { style: 'position:relative;inset:auto' }), 'position:absolute;right:0;top:56px;inline-size:400px') });
    const m = phone(ptop('Team Atlas', 'Ansicht', ''), `<div class="col" style="gap:var(--sp-100)">${['Layout · Liste', 'Was zu sehen ist · 2 Bedingungen', 'Ordnen · nach Zuständig, Fällig', 'Felder · 7', 'Speichern'].map((t, k) => `<div class="prow">${ic(['list', 'filter', 'group', 'cols', 'star'][k], 's')}<span class="grow">${t}</span>${ic('chev', 's')}</div>`).join('')}<div class="cap subtle">Dieselben Abschnitte wie am Desktop, je einer als eigene Seite des Blatts.</div></div>`, 'Durchsuchen');
    return { d, m };
  };
})();
