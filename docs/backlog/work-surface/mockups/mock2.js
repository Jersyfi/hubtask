// Pages and whiteboards in depth: the scenes of chapter 08. Uses the helpers of mock.js; the frame
// (app bar, sidebar, phone bars) stays as the navigation concept built it.
(function () {
  'use strict';
  const { S, ic, PROD, H } = window.WB;
  const { P, tile, htile, key, loz, lab, av, avs, chip, btn, pin, note, nr, sideHub, sideWS, appbar, phone, ptop, productHead, viewbar, rg, prop } = H;
  Object.assign(P, {
    pointer: '<path d="m4 3 7 18 2.5-7.5L21 11Z"/>',
    hand: '<path d="M18 11V6a2 2 0 0 0-4 0v5M14 10V4a2 2 0 0 0-4 0v6M10 10.5V6a2 2 0 0 0-4 0v8"/><path d="M18 8a2 2 0 1 1 4 0v6a8 8 0 0 1-8 8h-2c-2.8 0-4.5-.9-6-2.4l-3.6-3.6a2 2 0 0 1 2.8-2.8L7 15"/>',
    sticky: '<path d="M15 3H5a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h9l7-7V5a2 2 0 0 0-2-2Z"/><path d="M14 21v-5a2 2 0 0 1 2-2h5"/>',
    shape: '<rect x="3" y="3" width="10" height="10" rx="1"/><circle cx="16" cy="16" r="5"/>',
    text: '<path d="M4 7V4h16v3M9 20h6M12 4v16"/>',
    arrow: '<path d="M5 19 19 5M9 5h10v10"/>',
    frame: '<path d="M7 3v18M17 3v18M3 7h18M3 17h18"/>',
    pen: '<path d="M12 20h9"/><path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4Z"/>',
    marker: '<path d="m9 11-6 6v3h9l3-3"/><path d="m22 12-4.6 4.6a2 2 0 0 1-2.8 0l-5.2-5.2a2 2 0 0 1 0-2.8L14 4"/>',
    eraser: '<path d="m7 21-4.3-4.3a1 1 0 0 1 0-1.4l10-10a1 1 0 0 1 1.4 0l5.6 5.6a1 1 0 0 1 0 1.4L11 21Z"/><path d="M22 21H7M5 11l9 9"/>',
    comment: '<path d="M7.9 20A9 9 0 1 0 4 16.1L2 22Z"/>',
    timer: '<circle cx="12" cy="13" r="8"/><path d="M12 9v4l2 2M10 2h4"/>',
    vote: '<path d="m9 12 2 2 4-4"/><path d="M5 7c0-1.1.9-2 2-2h10a2 2 0 0 1 2 2v12H5V7Z"/><path d="M22 19H2"/>',
    minus: '<path d="M5 12h14"/>', fit: '<path d="M8 3H5a2 2 0 0 0-2 2v3M21 8V5a2 2 0 0 0-2-2h-3M3 16v3a2 2 0 0 0 2 2h3M16 21h3a2 2 0 0 0 2-2v-3"/>',
    map: '<path d="M14.1 6.3 9.9 4.2a2 2 0 0 0-1.8 0L3.6 6.5A1 1 0 0 0 3 7.4v12.1a1 1 0 0 0 1.4.9l3.7-1.9a2 2 0 0 1 1.8 0l4.2 2.1a2 2 0 0 0 1.8 0l4.5-2.3a1 1 0 0 0 .6-.9V5.3a1 1 0 0 0-1.4-.9l-3.7 1.9a2 2 0 0 1-1.8 0Z"/>',
    layers: '<path d="m12 2 10 5-10 5L2 7Z"/><path d="m2 17 10 5 10-5M2 12l10 5 10-5"/>',
    undo: '<path d="M9 14 4 9l5-5"/><path d="M4 9h10.5a5.5 5.5 0 0 1 0 11H11"/>', redo: '<path d="m15 14 5-5-5-5"/><path d="M20 9H9.5a5.5 5.5 0 0 0 0 11H13"/>',
    textsz: '<path d="M3 7V5h10v2M8 5v14M6 19h4M14 12v-1h7v1M17.5 11v8M16 19h3"/>',
    lockic: '<rect x="4" y="11" width="16" height="10" rx="2"/><path d="M8 11V7a4 4 0 0 1 8 0v4"/>',
    upload: '<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4M17 8l-5-5-5 5M12 3v12"/>',
    copy: '<rect x="8" y="8" width="13" height="13" rx="2"/><path d="M4 16V5a1 1 0 0 1 1-1h11"/>',
    check2: '<path d="M18 6 7 17l-5-5M22 10l-7.5 7.5L13 16"/>', verified: '<path d="M12 2 4 5v6c0 5 3.4 9.3 8 11 4.6-1.7 8-6 8-11V5Z"/><path d="m9 12 2 2 4-4"/>',
    dots: '<circle cx="9" cy="6" r="1"/><circle cx="15" cy="6" r="1"/><circle cx="9" cy="12" r="1"/><circle cx="15" cy="12" r="1"/><circle cx="9" cy="18" r="1"/><circle cx="15" cy="18" r="1"/>',
    strike: '<path d="M16 4H9a3 3 0 0 0-2.8 4M14 12a4 4 0 0 1 0 8H6M4 12h16"/>', at: '<circle cx="12" cy="12" r="4"/><path d="M16 8v5a3 3 0 0 0 6 0v-1a10 10 0 1 0-4 8"/>',
    slash: '<path d="M16 3 8 21"/>', outline: '<path d="M3 6h18M7 12h14M11 18h10"/>', mobile: '<rect x="6" y="2" width="12" height="20" rx="2"/><path d="M11 18h2"/>',
    embed: '<rect x="2" y="4" width="20" height="16" rx="2"/><path d="m10 9-3 3 3 3M14 9l3 3-3 3"/>', toggle: '<path d="m9 6 6 6-6 6"/><path d="M3 4v16"/>'
  });

  const deskFull = (side, main) => `<div class="mk mk-app">${appbar()}<div class="mk-body">${side}<main class="mk-main" style="padding:0;position:relative">${main}</main></div></div>`;
  const tabFull = (main) => `<div class="mk mk-app">${appbar(true)}<main class="mk-main" style="padding:0;position:relative">${main}</main></div>`;
  const phoneFull = (top, main, extra = '') => `<div class="mk mk-phone"><header class="mk-ptop">${top}</header><main class="mk-pmain" style="padding:0;position:relative">${main}</main>${extra}</div>`;
  const phoneNav = (cur = 'Durchsuchen') => `<nav class="mk-bottom">${H.BOTTOM.map(([i, n]) => `<div class="${n === cur ? 'cur' : ''} ${n === 'Erfassen' ? 'cap' : ''}">${ic(i)}${n}</div>`).join('')}</nav>`;
  const ment = (p, k, t, extra = '') => `<span class="ment">${tile(p)}${k ? `<span class="k">${k}</span>` : ''}${t}${extra}</span>`;

  // the hub sidebar with the page tree open (as built: pages and boards in one tree)
  const sidePage = (cur) => sideHub('pe', '').replace(/<div class="mk-sh">(?:(?!<div class="mk-sh">)[^])*?Inhalte · 3<\/div>[^]*?(<div class="foot">)/, (m, foot) => `<div class="mk-sh">${ic('chevd', 's')}Inhalte · 8</div>
    ${nr('board', 'Retro Sprint 23', { cur: cur === 'Retro Sprint 23' })}${nr('board', 'Anmeldefluss mit Passkey', { cur: cur === 'Anmeldefluss' })}${nr('book', 'Handbuch Produktentwicklung')}
    ${nr('', 'Architekturüberblick Kunden-App', { sub: 1, raw: '<span style="font-size:13px;line-height:1">🧭</span>', cur: cur === 'Architektur' })}
    ${nr('', 'Onboarding für Entwickler', { sub: 1, raw: '<span style="font-size:13px;line-height:1">👋</span>' })}${nr('', 'Team-Rituale', { sub: 1, raw: '<span style="font-size:13px;line-height:1">🔁</span>' })}
    ${cur === 'new' ? nr('', 'Ohne Titel', { sub: 1, raw: ic('book'), cur: 1 }) : ''}${foot}`);

  // ============ PAGES ============
  const trail = `<span class="trail">${htile('rocket', 'blue')} Produktentwicklung ${ic('chev')} Handbuch Produktentwicklung ${ic('chev')} <span style="color:var(--text-primary)">Architekturüberblick</span></span>`;
  const pageActs = (n = 2) => `${avs(['AH', 'MY', 'JK'])}${btn(`${n}`, 'ghost', 'comment')}${btn('Teilen', '', 'users')}${btn('', 'ico', 'star')}${btn('', 'ico', 'more')}`;
  const trustLine = (compact) => `<div class="trust"><span class="who">${av('MY', 's')}<span><b style="font-weight:var(--fw-semibold)">Mehmet Yılmaz</b>${compact ? '' : ' verantwortet diese Seite'}</span></span>
    ${loz('Geprüft bis 15. Jan. 2027', 'ok', 'verified')}<span>Geändert vor 22 Min. von Amelia</span>${compact ? '' : `<span class="row" style="gap:6px">${lab('architektur', 'teal')}${lab('kunden-app', 'blue')}<span class="btn link cap" style="padding:0">${ic('plus', 's')}Label</span></span>`}</div>`;
  const pageBody = (o = {}) => `
    <span class="icon">🧭</span><h1 class="t">Architekturüberblick Kunden-App</h1>
    ${o.trust === false ? '' : note(o.pins ? 2 : 0, trustLine(o.compact), o.pins ? '' : 'outline:0').replace(o.pins ? '' : /<span class="pin">0<\/span>/, '')}
    <p>Die App spricht ausschließlich mit der <b>Public API v3</b>. ${o.hl ? '<span class="hl">Es gibt keinen direkten Zugriff auf Datenbanken.</span>' : 'Es gibt keinen direkten Zugriff auf Datenbanken.'}</p>
    <div class="callout">${ic('info')}<span>Diese Seite beschreibt den Stand ab Release 3.2. Ältere Apps sprechen noch mit der API v2 – siehe <a style="color:var(--text-brand)">Migration</a>.</span></div>
    <h2>Bausteine</h2>
    <table><tr><th>Baustein</th><th>Verantwortung</th><th>Team</th></tr><tr><td>App (iOS/Android)</td><td>Darstellung, Offline-Warteschlange</td><td>Atlas</td></tr><tr><td>Public API</td><td>Authentifizierung, Bestellungen, Webhooks</td><td>Boreal</td></tr><tr><td>Event-Pipeline</td><td>Benachrichtigungen, Auswertungen</td><td>Boreal</td></tr></table>
    <h2>Entscheidungen</h2>
    <ol><li>Passkeys vor Passwort – ${ment('work', 'TAMA-1', 'Anmeldung mit Passkey', ' ' + loz('In Arbeit', 'info'))}</li><li>Bestellungen werden offline mit einem Idempotenzschlüssel gepuffert.</li></ol>
    <h2>Offene Punkte</h2>
    <div class="chk done">${'<span class="check on">' + ic('check', 's') + '</span>'}<span class="tx">Sequenzdiagramm für die Passkey-Anmeldung ergänzen</span> ${ment('work', 'TAMA-14', '')}</div>
    <div class="chk"><span class="check"></span><span class="tx">Webhook-Wiederholungen dokumentieren</span> ${ment('work', 'TAMA-15', '')}</div>
    <div class="chk" style="margin-block-end:var(--sp-200)"><span class="check"></span><span class="tx">Grenzen der Offline-Warteschlange festlegen</span>${o.taskHover ? `<span class="hov">${btn('Aufgabe daraus', 'ghost', 'checksq')}</span>` : ''}</div>
    <h2>Was gerade läuft</h2>
    ${note(o.pins ? 4 : 0, `<div class="live"><header>${tile('work')}<span class="semi">Team Atlas – offen, Label kunden-app</span><span class="pulse">Live</span><span class="grow"></span><span class="btn link cap">In Aufgaben öffnen ${ic('chev', 's')}</span></header>
      ${[['TAMA-1', 'Als Kundin möchte ich mich mit Passkey anmelden', 'In Arbeit', 'info', 'AH'], ['TAMA-10', 'Offline-Modus: Bestellungen ohne Netz aufgeben', 'Zu erledigen', 'neu', 'SR'], ['TAMA-15', 'Webhook-Wiederholungen dokumentieren', 'Zu erledigen', 'neu', 'MY']].map(([k, t, s, tn, a]) => `<div class="r">${key('work', k)}<span class="grow ell">${t}</span>${loz(s, tn)}${av(a, 's')}</div>`).join('')}</div>`, o.pins ? '' : 'outline:0').replace(o.pins ? '' : /<span class="pin">0<\/span>/, '')}
    <h2>Ablauf</h2>
    <div class="live"><header>${tile('whiteboards')}<span class="semi">Anmeldefluss mit Passkey · Rahmen „Ablauf“</span><span class="pulse">Live</span><span class="grow"></span><span class="btn link cap">Board öffnen ${ic('chev', 's')}</span></header>
      <div class="cv" style="block-size:150px">${[[30, 50, 'Start'], [190, 50, 'Passkey?'], [350, 20, 'Anmelden'], [350, 85, 'Passwort'], [510, 50, 'Fertig']].map(([x, y, t]) => `<span class="o" style="left:${x}px;top:${y}px;padding:6px 14px;border:1.5px solid var(--border-strong);border-radius:var(--r-md);background:var(--bg-surface);font-size:var(--fs-075)">${t}</span>`).join('')}</div></div>
    <h2>Glossar</h2><p><b>Passkey</b> – FIDO2-Anmeldedaten auf dem Gerät; ersetzt das Passwort.</p>`;
  const refsBlock = `<section style="margin-block-start:var(--sp-500);padding-block-start:var(--sp-300);border-block-start:var(--bw-hairline) solid var(--border-subtle);font-size:var(--fs-090)">
      <h3 class="lbl" style="margin-block-end:var(--sp-150)">Verweist hierher · 3</h3><div class="col" style="gap:6px">${ment('pages', '', 'Onboarding für Entwickler')}${ment('work', 'TAMA-1', 'Als Kundin möchte ich mich mit Passkey anmelden')}${ment('whiteboards', '', 'Anmeldefluss mit Passkey')}</div>
      </section>`;
  const outline = (on = 'Bausteine') => `<div class="outline"><h3 class="lbl" style="margin-block-end:var(--sp-100)">Auf dieser Seite</h3>${['Bausteine', 'Entscheidungen', 'Offene Punkte', 'Was gerade läuft', 'Ablauf', 'Glossar'].map((x) => `<span class="${x === on ? 'on' : ''}">${x}</span>`).join('')}</div>`;
  const marginCmt = `<div class="mcard"><div class="row">${av('AH', 's')}<span class="cap"><b>Amelia Hughes</b> · 09:25</span></div><span class="cap sub" style="font-style:italic">„Es gibt keinen direkten Zugriff auf Datenbanken.“</span>
    <span>Gilt das auch für den Reporting-Export? Der liest heute direkt aus der Replik.</span><div class="row">${btn('Antworten', 'ghost')}${btn('Erledigt', 'ghost', 'check')}</div></div>`;

  S.pread = () => {
    const d = deskFull(sidePage('Architektur'), `<div style="padding:var(--sp-300) var(--sp-400) var(--sp-800)">
      ${note(1, `<div class="pg-top" style="margin:0">${trail}<span class="acts">${pageActs()}</span></div>`)}
      <div class="pg" style="margin-block-start:var(--sp-400)"><article class="doc">${pageBody({ pins: true, hl: true })}${refsBlock}</article>
      <aside class="col" style="gap:var(--sp-300);position:sticky;top:0">${note(3, outline())}${note(5, `<div class="col" style="gap:var(--sp-150)"><div class="row"><h3 class="lbl grow">Kommentare · 2</h3><span class="cap subtle">Offen | Erledigt</span></div><div class="mcard" style="border-inline-start:3px solid var(--accent-primary)"><span class="cap subtle">Zur ganzen Seite</span><div class="row">${av('JK', 's')}<span class="cap"><b>Jana Krüger</b> · gestern</span></div><span style="font-size:var(--fs-090)">Bitte bis zum Review am Freitag aktualisieren.</span><span class="cap"><span class="btn link cap" style="padding:0">Antworten</span> · <span class="btn link cap" style="padding:0">Erledigt</span></span></div><div style="margin-block-start:var(--sp-300)">${marginCmt}</div><div class="composer"><div class="ta">${ic('comment', 's')} Zur ganzen Seite kommentieren …</div></div></div>`)}</aside></div></div>`);
    const t = tabFull(`<div style="padding:var(--sp-300) var(--sp-400) var(--sp-600)"><div class="pg-top">${trail.replace('Handbuch Produktentwicklung ' + ic('chev') + ' ', '… ' + ic('chev') + ' ')}<span class="acts">${btn('2', 'ghost', 'comment')}${btn('', 'ico', 'more')}</span></div>
      <article class="doc">${pageBody({ compact: true, hl: true }).replace('<p>Die App', `${note(3, `<div class="row" style="margin-block-end:var(--sp-300)">${btn('Auf dieser Seite', '', 'outline')}${btn('1 Kommentar am Text', 'ghost', 'comment')}</div>`)}<p>Die App`)}${refsBlock}</article></div>`);
    const m = phoneFull(ptop('Handbuch', ''), `<div style="padding:var(--sp-200) var(--sp-200) var(--sp-300)"><article class="doc" style="font-size:var(--fs-100)">${pageBody({ compact: true, hl: true }).replace('font-size:var(--fs-700)', '').replace(/<h1 class="t">/, '<h1 class="t" style="font-size:var(--fs-500)">').replace('<table>', '<div style="overflow-x:auto"><table style="min-inline-size:460px">').replace('</table>', '</table></div>')}</article></div>`,
      `${note(6, `<div class="row" style="justify-content:space-between;padding:var(--sp-100) var(--sp-200);background:var(--bg-surface);border-block-start:var(--bw-hairline) solid var(--border-subtle)">${btn('Inhalt', 'ghost', 'outline')}${btn('2', 'ghost', 'comment')}${btn('Bearbeiten', 'pri', 'pen')}</div>`)}${phoneNav()}`);
    return { d, t, m };
  };

  S.pwrite = () => {
    const body = pageBody({ taskHover: true }).replace('Es gibt keinen direkten Zugriff auf Datenbanken.', '<span class="sel">Es gibt keinen direkten Zugriff auf Datenbanken.</span>')
      .replace('<p>Die App', `<div style="position:relative;margin-inline-start:180px">${note(1, `<div class="float" style="position:relative;inline-size:fit-content;margin-block:0 var(--sp-050)"><span class="b">Absatz ${ic('chevd', 's')}</span><i class="sep"></i><span class="b">${ic('bold')}</span><span class="b">${ic('italic')}</span><span class="b">${ic('strike')}</span><span class="b">${ic('code')}</span><span class="b">${ic('link')}</span><i class="sep"></i><span class="b on">${ic('comment')}Kommentar</span></div>`)}</div><p>Die App`)
      .replace('<h2>Entscheidungen</h2>', `<h2>Entscheidungen</h2>`)
      .replace('<li>Bestellungen werden', `<li>Bestellungen werden`)
      .replace('<h2>Glossar</h2>', `<div style="position:relative;margin-block:var(--sp-300)">${note(3, `<div class="handle" style="inset-block-start:2px"><span>${ic('plus')}</span><span class="on">${ic('dots')}</span></div>`, 'position:absolute;inset-inline-start:0;inset-block-start:0')}<p style="margin:0;color:var(--text-subtle)">/ein<span class="caret" style="background:var(--text-primary)"></span></p>
        ${note(2, `<div class="menu" style="position:relative;margin-block-start:var(--sp-100)"><h4>Hubtask</h4>
          ${[['embed', 'Ansicht einbetten', 'Eine gespeicherte Ansicht, live', ''], ['board', 'Whiteboard einbetten', 'Ein Board oder ein Rahmen, live', ''], ['checksq', 'Eintrag erwähnen', 'Mit Status, wie du ihn sehen darfst', '@']].map(([i, n, s, k], x) => `<div class="mi ${x === 0 ? 'on' : ''}"><span class="g">${ic(i)}</span><span>${n}<small>${s}</small></span><span class="kbd">${k || ''}</span></div>`).join('')}
          <h4>Blöcke</h4>${[['info', 'Hinweis', 'Info, Warnung, Erfolg, Fehler'], ['toggle', 'Aufklappbereich', 'Mit einer Zusammenfassung'], ['image', 'Bild oder Datei', 'Hochladen oder hineinziehen']].map(([i, n, s]) => `<div class="mi"><span class="g">${ic(i)}</span><span>${n}<small>${s}</small></span><span></span></div>`).join('')}
          <span class="cap subtle" style="padding:var(--sp-050) var(--sp-100)">↑↓ wählen · ↵ einfügen · Esc schließen</span></div>`)}</div><h2>Glossar</h2>`)
      .replace('Webhook-Wiederholungen dokumentieren</span>', 'Webhook-Wiederholungen dokumentieren</span>')
      .replace('<p>Die App', `<p>Die App`);
    const withCaret = body.replace('ausschließlich mit der', 'ausschließlich<span class="caret"><b>Amelia</b></span> mit der');
    const d = deskFull(sidePage('Architektur'), `<div style="padding:var(--sp-300) var(--sp-400) var(--sp-800)">
      <div class="pg-top" style="margin:0">${trail}<span class="row cap sub" style="gap:6px;margin-inline-start:var(--sp-200)">${ic('check', 's')}Gespeichert · Amelia schreibt mit</span><span class="acts">${pageActs()}</span></div>
      <div class="pg" style="margin-block-start:var(--sp-400)"><article class="doc">${withCaret.replace('<span class="hov">', '<span class="hov">' + pin(4).replace('class="pin"', 'class="pin" style="position:relative;inset:auto;display:inline-block;margin-inline-end:6px"'))}</article>
      <aside class="col" style="gap:var(--sp-300)">${outline('Offene Punkte')}<div class="mcard" style="border-color:var(--accent-primary)"><span class="cap semi">Neuer Kommentar</span><span class="cap sub" style="font-style:italic">„Es gibt keinen direkten Zugriff …“</span><div class="composer"><div class="ta" style="min-block-size:40px">Was meinst du?</div></div></div></aside></div></div>`);
    const m = phoneFull(ptop('Handbuch', '', `<span class="btn link">Fertig</span>`), `<div style="padding:var(--sp-200)"><article class="doc" style="font-size:var(--fs-100)"><h1 class="t" style="font-size:var(--fs-450)">Architekturüberblick Kunden-App</h1>
      <p>Die App spricht ausschließlich mit der <b>Public API v3</b>. Es gibt keinen direkten Zugriff auf Datenbanken.</p><h2 style="font-size:var(--fs-300)">Offene Punkte</h2>
      <div class="chk"><span class="check"></span><span class="tx">Grenzen der Offline-Warteschlange festlegen<span class="caret" style="background:var(--text-primary)"></span></span></div></article></div>`,
      `${note(5, `<div class="kbar">${['textsz', 'bold', 'italic', 'list', 'checksq', 'at', 'slash', 'image'].map((x) => `<span class="b">${ic(x)}</span>`).join('')}<span class="grow"></span><span class="b" style="color:var(--text-brand)">${ic('checksq')}</span></div>`)}<div class="kb">Bildschirmtastatur</div>`);
    return { d, m };
  };

  S.phist = () => {
    const v = (on, cmp, t, s, named) => `<div class="v ${on ? 'on' : ''} ${cmp ? 'cmp' : ''}"><span>${on ? '<span class="check on round">' + ic('check', 's') + '</span>' : cmp ? '<span class="check round"></span>' : '<span class="dot" style="margin:6px"></span>'}</span><span class="col" style="gap:2px"><span class="semi">${t}</span><span class="cap sub">${s}</span>${named ? loz(named, 'brand', 'pin') : ''}</span></div>`;
    const left = `<article class="doc"><h1 class="t" style="font-size:var(--fs-500)">Architekturüberblick Kunden-App</h1><p>Die App spricht ausschließlich mit der <b>Public API <del>v2</del></b>. Es gibt keinen direkten Zugriff auf Datenbanken.</p><h2>Bausteine</h2><table><tr><th>Baustein</th><th>Team</th></tr><tr><td>App</td><td>Atlas</td></tr><tr><td>Public API</td><td>Boreal</td></tr></table><h2>Entscheidungen</h2><ol><li><del>Passwort und SMS-Code als Standard.</del></li></ol></article>`;
    const right = `<article class="doc"><h1 class="t" style="font-size:var(--fs-500)">Architekturüberblick Kunden-App</h1><p>Die App spricht ausschließlich mit der <b>Public API <ins>v3</ins></b>. Es gibt keinen direkten Zugriff auf Datenbanken.</p><h2>Bausteine</h2><table><tr><th>Baustein</th><th>Team</th></tr><tr><td>App</td><td>Atlas</td></tr><tr><td>Public API</td><td>Boreal</td></tr><tr><td><ins>Event-Pipeline</ins></td><td><ins>Boreal</ins></td></tr></table><h2>Entscheidungen</h2><ol><li><ins>Passkeys vor Passwort.</ins></li><li><ins>Bestellungen werden offline mit einem Idempotenzschlüssel gepuffert.</ins></li></ol></article>`;
    const d = deskFull(sidePage('Architektur'), `<div style="padding:var(--sp-300) var(--sp-400) var(--sp-800)">
      <div class="pg-top">${btn('Zur Seite', 'ghost', 'chevl')}<span class="trail">Architekturüberblick Kunden-App ${ic('chev')} <span style="color:var(--text-primary)">Verlauf</span></span><span class="acts">${btn('Version benennen', '', 'pin')}<span class="btn pri">${ic('history')}Diese Version zurückholen</span></span></div>
      <div style="display:grid;grid-template-columns:260px minmax(0,1fr);gap:var(--sp-400);align-items:start">
        ${note(1, `<section class="reg"><div class="rh"><h2 class="h grow" style="font-size:var(--fs-100)">Versionen · 5</h2></div><div class="vers" style="padding:var(--sp-100)">
          ${v(true, false, 'Jetzt', 'Amelia, Mehmet · heute 10:12')}${v(false, false, 'Heute 09:40', 'Mehmet Yılmaz')}${v(false, true, '4. Nov., 16:05', 'Jana Krüger', 'An Team Boreal geschickt')}${v(false, false, '28. Okt., 11:20', 'Jana Krüger')}${v(false, false, '21. Okt., 09:02', 'Jana Krüger · angelegt')}</div>
          <div class="rb cap sub">Eine Version je Schreibsitzung. Zurückholen legt eine neue Version an; nichts geht verloren.</div></section>`)}
        <div class="col" style="gap:var(--sp-200)">${note(2, `<div class="row"><span class="seg"><span class="on">Nebeneinander</span><span>Nur Änderungen</span></span><span class="grow"></span><span class="row cap sub" style="gap:var(--sp-150)"><span class="row" style="gap:4px"><del style="background:var(--status-danger-surface);color:var(--status-danger-text)">entfernt</del></span><span class="row" style="gap:4px"><ins style="background:var(--status-success-surface);color:var(--status-success-text);text-decoration:none">hinzugefügt</ins></span><b>3 Stellen</b></span></div>`)}
          ${note(3, `<div class="diff sheet" style="padding:var(--sp-300)"><div class="col"><span class="cap semi sub">4. Nov., 16:05 · „An Team Boreal geschickt“</span>${left}</div><div class="col" style="border-inline-start:var(--bw-hairline) solid var(--border-subtle);padding-inline-start:var(--sp-300)"><span class="cap semi sub">Jetzt</span>${right}</div></div>`)}</div></div></div>`);
    return { d };
  };

  const pageCard = (em, t, ex, where, who, trust) => `<section class="pcard"><div class="row" style="justify-content:space-between"><span class="em">${em}</span>${trust}</div><span class="semi">${t}</span><span class="ex">${ex}</span><span class="row cap sub" style="margin-block-start:auto">${where} · ${av(who, 's')} ${'vor 2 Std.'}</span></section>`;
  S.phome = () => {
    const ok = loz('Geprüft', 'ok', 'verified'), due = loz('Prüfung fällig', 'warn', 'alert'), none = '';
    const head = note(1, productHead('pages', 'Seiten', '<b>31 Seiten</b> in 6 Bereichen · <b>24</b> geprüft · 3 dir zur Prüfung fällig', `<span class="btn">${ic('calendar')}Notiz von heute</span><span class="btn pri split">${ic('plus')}Neue Seite ${ic('chevd', 's')}</span>`));
    const vb = viewbar(['Für dich', 'Bereiche', 'Meine Notizen'], 'Für dich');
    const search = note(2, `<div class="col" style="gap:var(--sp-100);margin-block-end:var(--sp-400)"><div class="field" style="padding:var(--sp-150) var(--sp-200);font-size:var(--fs-100)">${ic('search')}In allen Seiten suchen – Titel, Text und Labels</div><div class="row wrap" style="gap:6px"><span class="cap sub">Labels:</span>${[['architektur', 12], ['onboarding', 7], ['protokoll', 9], ['anleitung', 6], ['entscheidung', 4]].map(([l, n]) => `<span class="chip" style="gap:6px">${l}<b style="color:var(--text-subtle)">${n}</b></span>`).join('')}</div></div>`);
    const cont = note(3, `<h3 class="lbl" style="margin-block-end:var(--sp-150)">Weiterlesen</h3><div class="tcards">${pageCard('🧭', 'Architekturüberblick Kunden-App', 'Die App spricht ausschließlich mit der Public API v3. Es gibt keinen direkten Zugriff auf Datenbanken.', 'Produktentwicklung', 'MY', ok)}${pageCard('👋', 'Onboarding für Entwickler', 'Laptop, Konten und der erste Pull Request in den ersten drei Tagen.', 'Produktentwicklung', 'JK', ok)}${pageCard('📝', 'Protokoll Lenkungskreis 10.10.', 'Teilnehmende, Beschlüsse und offene Punkte des Lenkungskreises.', 'Neubau Logistikzentrum', 'SR', none)}</div>`);
    const dueR = note(4, `<section class="reg"><div class="rh">${htile('alert', 'amber')}<h2 class="h grow" style="font-size:var(--fs-100)">Dir zur Prüfung fällig · 3</h2><span class="cap sub">Du verantwortest diese Seiten</span></div>
      ${[['🔁', 'Team-Rituale: Daily, Refinement, Review, Retro', 'Produktentwicklung', 'seit 3 Tagen'], ['🖨️', 'Drucker im 2. OG: Treiber und Warteschlangen', 'IT-Service-Desk', 'seit 12 Tagen'], ['🔐', 'How to reset your password when you are locked out', 'IT-Service-Desk', 'morgen']].map(([e, t, w, s]) => `<div class="row" style="padding:var(--sp-100) var(--sp-200);border-block-end:var(--bw-hairline) solid var(--border-subtle)"><span>${e}</span><span class="grow ell">${t}</span><span class="cap sub">${w}</span>${chip(s, 'clock', 'warn')}${btn('Prüfen', '', 'verified')}</div>`).join('')}</section>`);
    const recent = `<section class="reg"><div class="rh"><h2 class="h grow" style="font-size:var(--fs-100)">Zuletzt geändert</h2><span class="seg"><span class="on">Alle</span><span>Von mir</span></span></div>
      ${[['📝', 'Protokoll Lenkungskreis 10.10.', 'Neubau Logistikzentrum', 'SR', 'vor 39 Min.', none], ['🖨️', 'Drucker im 2. OG: Treiber und Warteschlangen', 'IT-Service-Desk', 'JW', 'vor 1 Std.', due], ['🔐', 'How to reset your password when you are locked out', 'IT-Service-Desk', 'JW', 'vor 1 Std.', ok], ['🌐', 'VPN verbindet nicht: die fünf häufigsten Ursachen', 'IT-Service-Desk', 'TB', 'gestern', ok], ['🔁', 'Team-Rituale: Daily, Refinement, Review, Retro', 'Produktentwicklung', 'AH', 'gestern', due], ['🐳', 'Fehlerbehebung: Docker startet nicht', 'Produktentwicklung', 'MY', '3. Nov.', ok]].map(([e, t, w, a, s, tr]) => `<div class="row" style="padding:0 var(--sp-200);block-size:44px;border-block-end:var(--bw-hairline) solid var(--border-subtle)"><span>${e}</span><span class="grow ell">${t}</span><span class="cap sub" style="inline-size:190px">${w}</span><span style="inline-size:130px">${tr}</span>${av(a, 's')}<span class="cap subtle" style="inline-size:72px;text-align:end">${s}</span></div>`).join('')}</section>`;
    const tpl = `<section class="reg pad col" style="gap:var(--sp-100)"><h3 class="lbl">Mit einer Vorlage beginnen</h3>${[['📋', 'Besprechungsnotizen'], ['⚖️', 'Entscheidungsprotokoll'], ['🔄', 'Retrospektive'], ['📘', 'Anleitung'], ['🎯', 'Projektsteckbrief']].map(([e, n]) => `<div class="row" style="padding:6px 0"><span>${e}</span><span class="grow">${n}</span>${ic('chev', 's')}</div>`).join('')}<span class="btn link cap" style="align-self:flex-start;padding:0">Alle Vorlagen und Bereichsvorlagen</span></section>`;
    const spaces = note(5, `<section class="reg pad col" style="gap:var(--sp-150)"><h3 class="lbl">Bereiche · geprüft</h3>${[['rocket', 'blue', 'Produktentwicklung', 11, 8], ['hub', 'teal', 'IT-Service-Desk', 9, 7], ['hub', 'orange', 'Neubau Logistikzentrum', 6, 4], ['book', 'teal', 'Team-Wiki', 5, 5]].map(([i, c, n, a, g]) => `<div class="col" style="gap:4px"><div class="row">${htile(i, c)}<span class="grow ell">${n}</span><span class="cap sub">${g} von ${a}</span></div>${H.bar(Math.round(g / a * 100), g === a ? 'ok' : '')}</div>`).join('')}</section>`);
    const d = H.desk(sideWS('pages'), `${head}${vb}${cont}<div style="display:grid;grid-template-columns:minmax(0,1fr) 300px;gap:var(--sp-300);margin-block-start:var(--sp-400);align-items:start"><div class="col" style="gap:var(--sp-300)">${dueR}${recent}</div><div class="col" style="gap:var(--sp-300)">${spaces}${tpl}</div></div>`);
    const m = phone(ptop('Durchsuchen', 'Seiten', ''), `<div class="col" style="gap:var(--sp-200)">
      <div class="row" style="gap:var(--sp-100);overflow:hidden">${viewbar(['Für dich', 'Bereiche', 'Notizen'], 'Für dich')}</div>
      <h3 class="lbl">Weiterlesen</h3><div class="row" style="gap:var(--sp-150);overflow:hidden">${pageCard('🧭', 'Architekturüberblick Kunden-App', 'Die App spricht ausschließlich mit der Public API v3.', 'Produktentwicklung', 'MY', '').replace('class="pcard"', 'class="pcard" style="min-inline-size:240px"')}${pageCard('👋', 'Onboarding für Entwickler', 'Laptop, Konten und der erste Pull Request.', 'Produktentwicklung', 'JK', '').replace('class="pcard"', 'class="pcard" style="min-inline-size:240px"')}</div>
      <h3 class="lbl">Dir zur Prüfung fällig · 3</h3><section class="reg">${[['🔁', 'Team-Rituale', 'seit 3 Tagen'], ['🖨️', 'Drucker im 2. OG', 'seit 12 Tagen']].map(([e, t, s]) => `<div class="prow"><span>${e}</span><span class="grow">${t}</span>${chip(s, 'clock', 'warn')}</div>`).join('')}</section>
      <h3 class="lbl">Zuletzt geändert</h3><section class="reg">${[['📝', 'Protokoll Lenkungskreis 10.10.', 'Neubau · vor 39 Min.'], ['🌐', 'VPN verbindet nicht', 'IT-Service-Desk · gestern']].map(([e, t, s]) => `<div class="prow"><span>${e}</span><div class="col grow" style="gap:2px"><span>${t}</span><span class="cap sub">${s}</span></div></div>`).join('')}</section></div>`);
    return { d, m };
  };

  S.pnew = () => {
    const chips = [['📋', 'Besprechungsnotizen'], ['⚖️', 'Entscheidungsprotokoll', 1], ['🔄', 'Retrospektive'], ['📘', 'Anleitung'], ['🎯', 'Projektsteckbrief']];
    const prev = `<section class="reg" style="overflow:hidden"><div class="rh"><h2 class="h grow" style="font-size:var(--fs-100)">⚖️ Entscheidungsprotokoll</h2>${btn('', 'ico', 'x')}</div>
      <div class="rb col" style="gap:var(--sp-150)"><span class="cap sub">Eine Entscheidung, warum sie fiel und was sie ändert. Hubtasks eigene Vorlage, in deiner Sprache.</span>
      <div class="sunk" style="padding:var(--sp-200);font-size:var(--fs-090);display:flex;flex-direction:column;gap:6px"><b>Entscheidungsprotokoll</b><span class="row" style="gap:6px">${loz('Offen', 'neu')}<span class="cap sub">Datum · Beteiligte</span></span><b style="margin-block-start:6px">Kontext</b><span class="sub">Was ist die Lage, welche Kräfte wirken?</span><b>Optionen</b><span class="sub">1. … 2. … 3. …</span><b>Entscheidung</b><span class="sub">Wir wählen …, weil …</span><b>Folgen</b><span class="sub">☐ Aufgaben, die daraus entstehen</span></div>
      <span class="btn pri" style="align-self:flex-start">Diese Vorlage verwenden</span></div></section>`;
    const d = deskFull(sidePage('new'), `<div style="padding:var(--sp-300) var(--sp-400) var(--sp-800)">
      <div class="pg-top" style="margin:0"><span class="trail">${htile('rocket', 'blue')} Produktentwicklung ${ic('chev')} Handbuch Produktentwicklung ${ic('chev')} <span style="color:var(--text-primary)">Ohne Titel</span></span><span class="acts">${btn('', 'ico', 'more')}</span></div>
      <div class="pg" style="grid-template-columns:minmax(0,1fr) 340px;margin-block-start:var(--sp-300)"><article class="doc">
        ${note(1, `<div class="banner">${ic('lock')}<span class="grow"><b>Entwurf.</b> Nur du siehst diese Seite, bis du sie veröffentlichst – dieser Bereich arbeitet mit Entwürfen.</span><span class="btn pri">Veröffentlichen</span>${btn('Verwerfen', 'ghost')}</div>`)}
        <span class="row cap subtle" style="gap:var(--sp-150);margin-block-end:var(--sp-100)"><span class="row" style="gap:4px">${ic('sparkle', 's')}Symbol</span><span class="row" style="gap:4px">${ic('verified', 's')}Verantwortung</span><span class="row" style="gap:4px">${ic('plus', 's')}Label</span></span>
        <h1 class="t ph">Ohne Titel<span class="caret" style="background:var(--text-primary)"></span></h1>
        ${note(2, `<div class="col" style="gap:var(--sp-100);margin-block:var(--sp-200) var(--sp-300)"><span class="cap sub">Beginnen mit</span><div class="tplchips">${chips.map(([e, n, on]) => `<span class="c ${on ? 'on' : ''}">${e} ${n}</span>`).join('')}<span class="c">${ic('more')}Alle Vorlagen</span></div><span class="cap sub" style="margin-block-start:var(--sp-100)">Ganzer Bereich: <a style="color:var(--text-brand)">Teambereich</a> · <a style="color:var(--text-brand)">Projektbereich</a> · <a style="color:var(--text-brand)">Wissensdatenbank</a></span></div>`)}
        ${note(3, `<p style="color:var(--text-subtle)">Schreib einfach los – oder tippe <span class="kbd">/</span> für Blöcke, <span class="kbd">@</span> für Personen, Seiten und Einträge.</p>`)}
      </article><aside>${note(4, prev)}</aside></div></div>`);
    return { d };
  };

  // ============ WHITEBOARDS ============
  const COLORS = ['amber', 'orange', 'red', 'magenta', 'violet', 'blue', 'teal', 'green', 'lime', 'slate'];
  const stk = (x, y, c, t, o = {}) => `<div class="stk ${o.sel ? 'sel' : ''}" data-p="${c}" style="left:${x}px;top:${y}px;inline-size:${o.w || 150}px;block-size:${o.h || 112}px">${t}${o.by ? `<span class="by">${o.by}</span>` : ''}${o.votes ? `<span class="votes">${o.votes}</span>` : ''}</div>`;
  const frm = (x, y, w, h, n) => `<div class="frm" style="left:${x}px;top:${y}px;inline-size:${w}px;block-size:${h}px"><b>${n}</b></div>`;
  const tcard = (x, y, k, t, s, tn, a, c = 'indigo', done) => `<div class="tcard" data-p="${c}" style="left:${x}px;top:${y}px;inline-size:170px"><div class="row" style="justify-content:space-between">${key('work', k)}${av(a, 's')}</div><span class="ttl" style="${done ? 'text-decoration:line-through;color:var(--text-subtle)' : ''}">${t}</span><div class="row"><span class="check ${done ? 'on' : ''}">${done ? ic('check', 's') : ''}</span>${loz(s, tn)}</div></div>`;
  const cursor = (x, y, n, c) => `<div class="lcur" data-p="${c}" style="left:${x}px;top:${y}px"><svg viewBox="0 0 16 16"><path d="M1 1l5 14 2-6 6-2z"/></svg><b>${n}</b></div>`;
  const conn = (w, h, d) => `<svg class="o" style="left:0;top:0;overflow:visible" width="${w}" height="${h}"><defs><marker id="ah" markerWidth="8" markerHeight="8" refX="7" refY="4" orient="auto"><path d="M0 0L8 4L0 8z" fill="var(--text-secondary)"/></marker></defs>${d.map((p) => `<path d="${p}" stroke="var(--text-secondary)" stroke-width="1.6" fill="none" marker-end="url(#ah)"/>`).join('')}</svg>`;
  // the retro board as drawn in the demo, tidied: three frames, notes, two task cards, one connector
  const retro = (o = {}) => `${frm(40, 70, 300, 360, 'Lief gut')}${frm(370, 70, 300, 360, 'Lief nicht gut')}${frm(700, 70, 300, 360, 'Nächstes Mal')}
    ${stk(56, 90, 'green', 'Dark Mode zwei Tage früher ausgeliefert 🎉', { w: 130, h: 104, by: 'Amelia', votes: o.votes && 2 })}${stk(196, 90, 'green', 'Pairing zwischen Atlas und Boreal hat super funktioniert', { w: 130, h: 104, by: 'Mehmet', votes: o.votes && 4 })}${stk(56, 210, 'lime', 'Review-Demo lief ohne Hänger', { w: 130, h: 104, by: 'Sven' })}
    ${stk(386, 90, 'orange', 'Review-Termin kollidierte mit dem Release', { w: 130, h: 104, by: 'Jana', votes: o.votes && 1 })}${stk(526, 90, 'orange', 'Zu viele Meetings am Mittwoch', { w: 130, h: 104, by: 'Amelia', votes: o.votes && 3 })}${stk(386, 210, 'red', 'Flaky UI-Tests kosten uns einen halben Tag', { w: 130, h: 104, by: 'Sven', sel: o.sel, votes: o.votes && 5 })}
    ${o.task ? tcard(716, 90, 'TAMA-21', 'Review eine Stunde früher legen', 'Zu erledigen', 'neu', 'JK', 'blue').replace('inline-size:170px', 'inline-size:130px') : stk(716, 90, 'blue', 'Review eine Stunde früher legen (Lukas)', { w: 130, h: 104, by: 'Jana' })}${stk(716, 226, 'blue', 'UI-Tests in einen Quarantäne-Lauf schieben', { w: 130, h: 104, by: 'Sven' })}
    ${tcard(40, 470, 'TAMA-1', 'Als Kundin möchte ich mich mit Passkey anmelden', 'In Arbeit', 'info', 'AH')}${tcard(230, 470, 'TAMA-10', 'Offline-Modus: Bestellungen ohne Netz', 'Zu erledigen', 'neu', 'SR')}
    ${conn(1100, 700, ['M516 262 C 600 262, 640 280, 712 278'])}`;
  const dock = (on = 'pointer', o = {}) => `<div class="isl dock" ${o.style ? `style="${o.style}"` : ''}>${[['pointer', 'V'], ['hand', 'H']].map(([i, k]) => `<span class="b ${on === i ? 'on' : ''}">${ic(i)}<kbd>${k}</kbd></span>`).join('')}<i class="sep"></i>
    ${[['sticky', 'N'], ['shape', 'R'], ['text', 'T'], ['arrow', 'L'], ['frame', 'F']].map(([i, k]) => `<span class="b ${on === i ? 'on' : ''}">${ic(i)}<kbd>${k}</kbd></span>`).join('')}<i class="sep"></i>
    ${[['pen', 'P'], ['marker', 'M'], ['eraser', 'E']].map(([i, k]) => `<span class="b ${on === i ? 'on' : ''}">${ic(i)}<kbd>${k}</kbd></span>`).join('')}<i class="sep"></i>
    <span class="b">${ic('comment')}<kbd>C</kbd></span><span class="b soft">${ic('plus')}Einfügen</span>${o.sw ? `<i class="sep"></i><span class="sw">${COLORS.slice(0, 5).map((c) => `<i data-p="${c}"></i>`).join('')}</span>` : ''}</div>`;
  const titleIsl = (o = {}) => `<div class="isl" style="left:var(--sp-200);top:var(--sp-200)"><span class="b">${ic('chevl')}</span>${tile('whiteboards')}<span class="semi">Retro Sprint 23</span><span class="mk-sync" style="inline-size:20px;block-size:20px;background:none">${ic('check', 's')}</span>${ic('star', 's')}<i class="sep"></i><span class="seg" style="font-size:var(--fs-075)"><span class="on">${ic('board', 's')}Fläche</span><span>${ic('list', 's')}Liste</span></span><span class="b">${ic('more')}</span></div>`;
  const rightIsl = (o = {}) => `<div class="isl" style="right:var(--sp-200);top:var(--sp-200)">${avs(['AH', 'MY', 'SR'])}<i class="sep"></i><span class="b ${o.mod ? 'soft' : ''}">${ic('timer')}${ic('vote')}Moderieren</span><span class="b">${ic('comment')}2</span><span class="b">${ic('history')}</span></div>`;
  const zoomIsl = (o = {}) => `<div class="isl" style="right:var(--sp-200);bottom:${o.bottom || 'var(--sp-200)'};${o.col ? 'flex-direction:column;align-items:stretch' : ''}">${o.map ? `<div class="mini">${[[10, 18, 26, 34, 'green'], [42, 18, 26, 34, 'orange'], [74, 18, 26, 34, 'blue'], [10, 70, 30, 14, ''], [44, 70, 30, 14, '']].map(([x, y, w, h, c]) => `<i ${c ? `data-p="${c}"` : ''} style="left:${x}px;top:${y}px;inline-size:${w}px;block-size:${h}px"></i>`).join('')}<span class="vp" style="left:6px;top:10px;inline-size:110px;block-size:80px"></span></div>` : ''}<div class="row" style="gap:2px"><span class="b">${ic('frame')}Rahmen</span><i class="sep"></i><span class="b">${ic('minus')}</span><span class="mono" style="inline-size:40px;text-align:center">90 %</span><span class="b">${ic('plus')}</span><span class="b">${ic('fit')}</span><span class="b ${o.map ? 'soft' : ''}">${ic('map')}</span></div></div>`;
  const undoIsl = `<div class="isl" style="left:var(--sp-200);bottom:var(--sp-200)"><span class="b">${ic('undo')}</span><span class="b">${ic('redo')}</span></div>`;
  const cvWrap = (h, inner, offX = 60, offY = 70) => `<div class="cv" style="block-size:${h}px"><div class="o" style="left:${offX}px;top:${offY}px;inline-size:1100px;block-size:700px">${inner}</div>`;

  S.wboard = () => {
    const canvas = (cur) => `${retro({ votes: false })}${cur ? cursor(560, 360, 'Amelia', 'violet') + cursor(250, 300, 'Mehmet', 'teal') : ''}`;
    const d = deskFull(sidePage('Retro Sprint 23'), `${cvWrap(852, canvas(true) + `<div class="cpin" style="left:660px;top:206px">1</div>`)}</div>
      ${note(1, titleIsl(), 'position:absolute;inset:0;pointer-events:none;outline:0').replace('<span class="pin">1</span>', '')}
      <div style="position:absolute;left:var(--sp-200);top:var(--sp-200)">${pin(1)}</div>
      ${rightIsl()}<div style="position:absolute;right:340px;top:var(--sp-200)">${pin(2)}</div>
      ${dock()}<div style="position:absolute;left:calc(50% - 300px);bottom:56px">${pin(3)}</div>
      ${zoomIsl({ map: true, col: true })}<div style="position:absolute;right:var(--sp-200);bottom:170px">${pin(4)}</div>${undoIsl}
      <div style="position:absolute;left:600px;top:420px">${pin(5)}</div>`);
    const t = tabFull(`<div class="cv" style="block-size:1132px"><div class="o" style="left:0;top:110px;transform:scale(.76);transform-origin:0 0;inline-size:1100px;block-size:700px">${canvas(false)}</div></div>${titleIsl()}<div class="isl" style="right:var(--sp-200);top:var(--sp-200)">${avs(['AH', 'MY'])}<span class="b">${ic('vote')}</span><span class="b">${ic('comment')}2</span></div>
      ${dock('pen').replace('Einfügen', '')}<div class="isl" style="left:50%;transform:translateX(-50%);bottom:84px">${note(6, `<span class="cap sub row" style="gap:6px;padding:2px 6px">${ic('pen', 's')}Stift zeichnet · Finger verschiebt und wählt · zwei Finger tippen = rückgängig</span>`)}</div>${zoomIsl({ bottom: '140px' })}`);
    const m = phoneFull(ptop('Produktentwicklung', 'Retro Sprint 23', `${ic('more')}`), `<div class="cv" style="block-size:700px"><div class="o" style="left:-30px;top:30px;transform:scale(1.15);transform-origin:0 0;inline-size:1100px;block-size:700px">${retro()}</div>
      ${note(7, `<div class="isl" style="left:var(--sp-150);top:var(--sp-150);font-size:var(--fs-075)">${ic('frame')}<span class="semi">Lief gut</span>${ic('chev', 's')}<span class="sub">1 von 3</span></div>`, 'position:absolute;left:12px;top:12px')}
      <div class="isl" style="right:var(--sp-150);top:var(--sp-150)">${avs(['AH', 'MY'])}</div>
      <div class="isl" style="left:50%;transform:translateX(-50%);bottom:var(--sp-150);gap:2px">${['pointer', 'sticky', 'text', 'comment'].map((i, k) => `<span class="b ${k === 0 ? 'on' : ''}" style="padding:10px">${ic(i)}</span>`).join('')}<i class="sep"></i><span class="b" style="padding:10px">${ic('more')}</span></div><div style="position:absolute;left:calc(50% - 110px);bottom:64px">${pin(8)}</div></div>`, phoneNav());
    return { d, t, m };
  };

  S.wsel = () => {
    const floatBar = `<div class="float" style="left:300px;top:150px">${['amber', 'orange', 'red', 'magenta', 'violet', 'blue', 'teal', 'green', 'lime', 'slate'].map((c) => `<span class="b" style="padding:5px 3px"><span class="sq" data-p="${c}" style="inline-size:16px;block-size:16px;border-radius:var(--r-full);${c === 'red' ? 'box-shadow:0 0 0 2px var(--bg-raised),0 0 0 4px var(--accent-primary)' : ''}"></span></span>`).join('')}<i class="sep"></i>
      <span class="b">${ic('textsz')}M ${ic('chevd', 's')}</span><i class="sep"></i><span class="b">${ic('arrow')}</span><span class="b">${ic('comment')}</span><span class="b">${ic('lockic')}</span><i class="sep"></i><span class="b on">${ic('checksq')}Aufgabe daraus</span><span class="b">${ic('more')}</span></div>`;
    const taskPop = `<div class="pop" style="left:600px;top:200px;inline-size:300px"><span class="semi">Aufgabe daraus machen</span><span class="cap sub">Der Text wird zum Titel; die Notiz wird zur Karte der Aufgabe, ihre Pfeile bleiben.</span>
      <div class="col" style="gap:4px"><span class="cap semi sub">Sammlung</span><div class="field" style="color:var(--text-primary)">${ic('coll')}Team Atlas – Mobile App ${ic('chevd', 's')}</div></div>
      <div class="col" style="gap:4px"><span class="cap semi sub">Art</span><span class="seg"><span class="on">Aufgabe</span><span>Story</span><span>Fehler</span></span></div>
      <div class="row" style="justify-content:flex-end">${btn('Abbrechen', 'ghost')}<span class="btn pri">Aufgabe anlegen</span></div></div>`;
    const handles = `<span class="hnd" style="left:511px;top:257px"></span><span class="hnd" style="left:381px;top:257px"></span><span class="hnd" style="left:446px;top:205px"></span><span class="hnd" style="left:446px;top:309px"></span>
      <span class="plus" style="left:532px;top:251px">${ic('plus', 's')}</span><span class="plus" style="left:440px;top:326px">${ic('plus', 's')}</span>`;
    const inner = `${retro({ sel: true, task: true })}<div class="o" style="left:0;top:0;inline-size:0;block-size:0">${note(1, floatBar, 'position:absolute;left:0;top:0;outline:0').replace('<span class="pin">1</span>', '')}</div>${handles}<div class="o" style="left:0;top:0">${taskPop}</div>`;
    const d = deskFull(sidePage('Retro Sprint 23'), `${cvWrap(852, inner, 60, 130)}</div>${titleIsl()}${rightIsl()}${dock('pointer')}${zoomIsl()}${undoIsl}
      <div style="position:absolute;left:350px;top:272px">${pin(1)}</div><div style="position:absolute;left:652px;top:322px">${pin(2)}</div><div style="position:absolute;left:600px;top:398px">${pin(3)}</div><div style="position:absolute;left:768px;top:206px">${pin(4)}</div>
      ${note(5, `<div class="pop" style="position:relative;inline-size:240px"><span class="cap semi sub">Mehrere gewählt · 3 Notizen</span><div class="row" style="gap:2px">${['layers', 'cols', 'group', 'copy'].map((i) => `<span class="btn ico ghost">${ic(i)}</span>`).join('')}<span class="btn" style="margin-inline-start:auto">${ic('checksq')}3 Aufgaben</span></div><span class="cap subtle">Ausrichten, verteilen, gruppieren – oder alle Notizen eines Rahmens auf einmal zu Aufgaben.</span></div>`, 'position:absolute;right:24px;bottom:84px')}`);
    return { d };
  };

  S.wsess = () => {
    const sess = `<div class="isl sess">${ic('timer')}<span class="timer">04:12</span><span class="b">${ic('pause')}</span><span class="b">${ic('plus')}1 Min.</span><i class="sep"></i>${ic('vote')}<span class="col" style="gap:0;line-height:1.2"><span class="semi">Abstimmung · anonym</span><span class="cap sub">noch 1 von 3 Stimmen · 4 von 5 fertig</span></span><span class="btn pri">Zeigen</span></div>`;
    const people = `<div class="pop" style="right:var(--sp-200);top:72px;inline-size:280px"><span class="semi">Gerade hier · 4</span>${[['AH', 'Amelia Hughes', 'moderiert', 1], ['MY', 'Mehmet Yılmaz', '', 0], ['SR', 'Sven Richter', '', 0], ['JK', 'Du', '', 0]].map(([a, n, s, f]) => `<div class="row">${av(a)}<span class="grow">${n}${s ? ` <span class="cap sub">· ${s}</span>` : ''}</span>${a !== 'JK' ? (f ? '<span class="btn pri" style="padding:3px 10px">Folgst du</span>' : btn('Folgen', 'ghost')) : ''}</div>`).join('')}<span class="cap subtle">Folgen endet, sobald du das Board selbst bewegst.</span></div>`;
    const d = deskFull(sidePage('Retro Sprint 23'), `<div style="position:relative">${cvWrap(852, retro({ votes: true }) + cursor(470, 330, 'Amelia', 'violet'), 60, 110)}</div>
      ${note(4, `<div class="follow" data-p="teal"><b>Du folgst Amelia · Board bewegen beendet das</b></div>`, 'position:absolute;inset:0;outline:0').replace('<span class="pin">4</span>', '')}</div>
      ${titleIsl()}${note(1, sess, 'position:absolute;inset:0;outline:0;pointer-events:none').replace('<span class="pin">1</span>', '')}<div style="position:absolute;left:calc(50% - 250px);top:58px">${pin(1)}</div>
      <div class="isl" style="right:var(--sp-200);top:var(--sp-200)">${avs(['AH', 'MY', 'SR'])}<i class="sep"></i><span class="b soft">${ic('timer')}${ic('vote')}Moderieren</span><span class="b">${ic('comment')}2</span></div>${people}<div style="position:absolute;right:300px;top:76px">${pin(2)}</div>
      <div style="position:absolute;left:540px;top:318px">${pin(3)}</div><div style="position:absolute;left:calc(50% + 120px);top:2px">${pin(4)}</div>
      ${dock()}${zoomIsl()}`);
    return { d };
  };

  const boardThumb = (kind) => {
    // [x, y, w, h, colour] in a 240×150 box; colour '' = a plain shape, 'f' = a frame
    const f = (x, y, w, h) => [x, y, w, h, 'f'];
    const col3 = (c1, c2, c3) => [f(8, 10, 72, 130), f(84, 10, 72, 130), f(160, 10, 72, 130), [16, 20, 26, 22, c1], [46, 20, 26, 22, c1], [16, 48, 26, 22, c1], [92, 20, 26, 22, c2], [122, 20, 26, 22, c2], [92, 48, 26, 22, c2], [168, 20, 26, 22, c3], [168, 48, 26, 22, c3]];
    const K = {
      retro: col3('green', 'orange', 'blue'), mad: col3('red', 'blue', 'green'),
      four: [f(8, 8, 110, 64), f(122, 8, 110, 64), f(8, 78, 110, 64), f(122, 78, 110, 64), [16, 18, 26, 20, 'green'], [46, 18, 26, 20, 'green'], [130, 18, 26, 20, 'blue'], [16, 88, 26, 20, 'amber'], [130, 88, 26, 20, 'violet'], [160, 88, 26, 20, 'violet']],
      kanban: [f(8, 10, 72, 130), f(84, 10, 72, 130), f(160, 10, 72, 130), [16, 22, 56, 16, ''], [16, 42, 56, 16, ''], [16, 62, 56, 16, ''], [92, 22, 56, 16, ''], [168, 22, 56, 16, '']],
      map: [[8, 10, 34, 18, 'blue'], [48, 10, 34, 18, 'blue'], [88, 10, 34, 18, 'blue'], [128, 10, 34, 18, 'blue'], f(8, 40, 224, 46), f(8, 92, 224, 46), [14, 50, 30, 18, 'amber'], [54, 50, 30, 18, 'amber'], [94, 50, 30, 18, 'amber'], [14, 102, 30, 18, 'amber']],
      flow: [[14, 64, 36, 18, ''], [66, 64, 36, 18, ''], [118, 40, 36, 18, ''], [118, 92, 36, 18, ''], [176, 64, 36, 18, '']],
      brain: [[100, 64, 40, 24, 'amber'], [40, 22, 34, 22, 'amber'], [166, 22, 34, 22, 'amber'], [40, 108, 34, 22, 'amber'], [166, 108, 34, 22, 'amber'], [103, 14, 34, 22, 'amber']]
    };
    return `<div class="thumb">${(K[kind] || []).map(([x, y, w, h, c]) => c === 'f' ? `<s style="left:${x / 2.4}%;top:${y / 1.5}%;inline-size:${w / 2.4}%;block-size:${h / 1.5}%"></s>` : `<i ${c ? `data-p="${c}"` : ''} style="left:${x / 2.4}%;top:${y / 1.5}%;inline-size:${w / 2.4}%;block-size:${h / 1.5}%;${c ? '' : 'background:var(--bg-surface);border:1px solid var(--border-strong)'}"></i>`).join('')}</div>`;
  };
  const bcard = (kind, t, w, n, s, a) => `<section class="bcard2">${boardThumb(kind)}<div class="meta"><span class="semi">${t}</span><span class="cap sub row" style="gap:6px">${w} · ${n} Objekte · ${s}</span><span class="row" style="gap:6px">${avs(a)}</span></div></section>`;
  S.whome = () => {
    const head = note(1, productHead('whiteboards', 'Whiteboards', '<b>3 Whiteboards</b> · 1 heute geändert · 2 offene Kommentare', `<span class="btn pri split">${ic('plus')}Neues Whiteboard ${ic('chevd', 's')}</span>`));
    const vb = viewbar(['Für dich', 'Alle', 'Vorlagen'], 'Für dich');
    const recent = note(2, `<h3 class="lbl" style="margin-block-end:var(--sp-150)">Zuletzt geöffnet</h3><div class="tcards">${bcard('retro', 'Retro Sprint 23', 'Produktentwicklung', 12, 'vor 39 Min.', ['AH', 'MY', 'SR'])}${bcard('flow', 'Anmeldefluss mit Passkey (Entwurf)', 'Produktentwicklung', 7, 'gestern', ['MY'])}${bcard('brain', 'Brainstorming IT-Expo Köln', 'Marketing & Kommunikation', 6, '3. Nov.', ['CN', 'JW'])}</div>`);
    const tpls = [['retro', 'Retrospektive: Start, Stop, Weiter'], ['four', 'Liked, Learned, Lacked, Longed for'], ['mad', 'Mad, Sad, Glad'], ['brain', 'Brainstorming'], ['flow', 'Flussdiagramm']];
    const tpl = note(3, `<div class="row" style="margin-block:var(--sp-400) var(--sp-150)"><h3 class="lbl grow">Mit einer Vorlage beginnen</h3><span class="btn link cap">Alle 8 Vorlagen ${ic('chev', 's')}</span></div><div style="display:grid;grid-template-columns:repeat(5,minmax(0,1fr));gap:var(--sp-150)">${tpls.map(([k, n]) => `<section class="bcard2">${boardThumb(k).replace('block-size: 150px', '')}<span class="cap semi" style="padding:0 4px 4px">${n}</span></section>`).join('')}</div>`).replace(/class="thumb"/g, 'class="thumb" style="block-size:96px"');
    const open = note(4, `<section class="reg" style="margin-block-start:var(--sp-400)"><div class="rh">${htile('comment', 'amber')}<h2 class="h grow" style="font-size:var(--fs-100)">Offene Kommentare an dich · 2</h2></div>${[['Retro Sprint 23', 'Emil: „@Jana kannst du den Raum für Mittwoch buchen?“', 'an der Notiz „Review eine Stunde früher …“'], ['Anmeldefluss mit Passkey', 'Mehmet: „Fehlt hier nicht der Fall ohne zweites Gerät?“', 'im Rahmen „Ablauf“']].map(([b, c, w]) => `<div class="row" style="padding:var(--sp-150) var(--sp-200);border-block-end:var(--bw-hairline) solid var(--border-subtle);gap:var(--sp-150)">${tile('whiteboards')}<div class="col grow" style="gap:2px"><span>${c}</span><span class="cap sub">${b} · ${w}</span></div>${btn('Am Board öffnen', 'ghost')}</div>`).join('')}</section>`);
    const d = H.desk(sideWS('whiteboards'), `${head}${vb}${recent}${tpl}${open}`);
    const m = phone(ptop('Durchsuchen', 'Whiteboards', ''), `<div class="col" style="gap:var(--sp-200)">${viewbar(['Für dich', 'Alle', 'Vorlagen'], 'Für dich')}<h3 class="lbl">Zuletzt geöffnet</h3>
      ${bcard('retro', 'Retro Sprint 23', 'Produktentwicklung', 12, 'vor 39 Min.', ['AH', 'MY', 'SR'])}<div style="display:grid;grid-template-columns:1fr 1fr;gap:var(--sp-150)">${bcard('flow', 'Anmeldefluss', 'Produktentw.', 7, 'gestern', ['MY']).replace(/class="thumb"/, 'class="thumb" style="block-size:90px"')}${bcard('brain', 'IT-Expo Köln', 'Marketing', 6, '3. Nov.', ['CN']).replace(/class="thumb"/, 'class="thumb" style="block-size:90px"')}</div>
      <h3 class="lbl">Offene Kommentare an dich · 2</h3><section class="reg"><div class="prow">${tile('whiteboards')}<div class="col grow" style="gap:2px"><span>Raum für Mittwoch buchen?</span><span class="cap sub">Retro Sprint 23</span></div></div></section></div>`);
    return { d, m };
  };

  S.wnew = () => {
    const tpls = [['', 'Leer', 'Frei zeichnen.'], ['retro', 'Start, Stop, Weiter', 'Retrospektive in drei Spalten.'], ['four', 'Liked, Learned, Lacked, Longed for', 'Retrospektive in vier Feldern.'], ['mad', 'Mad, Sad, Glad', 'Retrospektive nach Gefühl.'], ['kanban', 'Kanban-Skizze', 'Zu tun, in Arbeit, erledigt.'], ['brain', 'Brainstorming', 'Ein Thema in der Mitte.'], ['flow', 'Flussdiagramm', 'Schritte, Entscheidung, zwei Wege.'], ['map', 'User-Story-Map', 'Aktivitäten oben, Storys je Release.']];
    const sheet = `<div class="sheet2" style="left:50%;transform:translateX(-50%);bottom:var(--sp-300);inline-size:900px"><div class="row" style="margin-block-end:var(--sp-200)"><div class="col grow" style="gap:2px"><span class="semi" style="font-size:var(--fs-300)">Womit beginnt ihr?</span><span class="cap sub">Eine Vorlage legt Rahmen, Notizen und Pfeile aufs Board. Später kommt sie über ⋯ wieder.</span></div>${btn('Leer lassen', 'ghost')}</div>
      <div style="display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:var(--sp-150)">${tpls.map(([k, n, s], x) => `<section class="bcard2" style="${x === 1 ? 'box-shadow:0 0 0 2px var(--accent-primary)' : ''}">${k ? boardThumb(k).replace('class="thumb"', 'class="thumb" style="block-size:84px"') : '<div class="thumb" style="block-size:84px"></div>'}<div class="meta"><span class="cap semi">${n}</span><span class="cap sub">${s}</span></div></section>`).join('')}</div></div>`;
    const hint = `<div class="hint" style="left:50%;top:150px;transform:translateX(-50%)">${tile('whiteboards', 'l')}<span class="semi" style="color:var(--text-primary);font-size:var(--fs-200)">Ein leeres Board</span><span><span class="kbd">N</span> Haftnotiz · <span class="kbd">Doppelklick</span> Notiz an der Stelle · <span class="kbd">Leertaste</span> halten zum Verschieben</span></div>`;
    const d = deskFull(sidePage('Retro Sprint 23').replace('Retro Sprint 23', 'Retro Sprint 24'), `<div class="cv" style="block-size:852px">${note(2, hint, 'position:absolute;inset:0;outline:0').replace('<span class="pin">2</span>', '')}<div style="position:absolute;left:640px;top:140px">${pin(2)}</div></div>
      ${titleIsl().replace('Retro Sprint 23', 'Retro Sprint 24')}${note(1, sheet, 'position:absolute;inset:0;outline:0;pointer-events:none').replace('<span class="pin">1</span>', '')}<div style="position:absolute;left:150px;bottom:420px">${pin(1)}</div>${zoomIsl()}`);
    return { d };
  };
  window.WB.P2 = { deskFull, tabFull, phoneFull, phoneNav, ment, sidePage, trail, pageActs, trustLine, pageBody, refsBlock, outline, marginCmt, boardThumb, retro, cvWrap, titleIsl, rightIsl, dock, zoomIsl, undoIsl, stk, frm, tcard, cursor };
})();
