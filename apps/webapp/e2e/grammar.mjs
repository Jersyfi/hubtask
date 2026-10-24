// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The grammar's measurements (design-system.md §13), shared by the layout walk and by a report run.
// Each answers a list of findings, empty when the screen keeps the rule; a finding names what broke
// it in words a person can find on the screen.

/** Runs in the page: every rule G1-G4 measured on what is drawn now. */
export function measure() {
  const findings = [];
  const visible = (el) => {
    const box = el.getBoundingClientRect();
    const style = getComputedStyle(el);
    return box.width > 1 && box.height > 1 && style.visibility !== 'hidden' && Number(style.opacity) > 0.01;
  };
  const name = (el) => (el.getAttribute('aria-label') || el.textContent || el.tagName).replace(/\s+/g, ' ').trim().slice(0, 40);

  // G1: the trail, the title and the toolbar of the page share one inline-start edge.
  const main = document.querySelector('main');
  // The head's own box, not the heading's: a place's mark stands before its name inside the head.
  const head = main?.querySelector('header.head') ?? main?.querySelector('h1');
  if (head && visible(head)) {
    const edge = Math.round(head.getBoundingClientRect().left);
    const others = [
      ...main.querySelectorAll('nav[aria-label="Where you are"], nav[aria-label="Wo du bist"]'),
      ...main.querySelectorAll('[role="toolbar"], [data-toolbar]'),
    ].filter(visible);
    for (const other of others) {
      const left = Math.round(other.getBoundingClientRect().left);
      if (Math.abs(left - edge) > 1) findings.push(`G1: "${name(other)}" starts ${left - edge} px off the head's edge`);
    }
  }

  // G2: one control size in the bar.
  const bar = document.querySelector('header');
  if (bar) {
    const icons = [...bar.querySelectorAll('button, a')].filter((el) => visible(el) && el.textContent.trim().length <= 2 && !el.closest('form'));
    const sizes = new Map();
    for (const el of icons) {
      const box = el.getBoundingClientRect();
      const key = `${Math.round(box.width)}x${Math.round(box.height)}`;
      sizes.set(key, [...(sizes.get(key) ?? []), name(el)]);
    }
    if (sizes.size > 1) findings.push(`G2: the bar's icon controls come in ${sizes.size} sizes: ${JSON.stringify(Object.fromEntries(sizes))}`);
  }

  // G3: no word broken across lines.
  const scopes = [...document.querySelectorAll('main, aside, header, [role="dialog"]')];
  const broken = new Set();
  for (const scope of scopes) {
    const walker = document.createTreeWalker(scope, NodeFilter.SHOW_TEXT);
    for (let node = walker.nextNode(); node; node = walker.nextNode()) {
      const text = node.nodeValue ?? '';
      if (text.trim().length < 4 || !node.parentElement || !visible(node.parentElement)) continue;
      const pattern = /\S{4,}/g;
      for (let match = pattern.exec(text); match; match = pattern.exec(text)) {
        const range = document.createRange();
        range.setStart(node, match.index);
        range.setEnd(node, match.index + match[0].length);
        const lines = new Set([...range.getClientRects()].filter((rect) => rect.width > 0).map((rect) => Math.round(rect.top)));
        // A hyphenation the language chose is not a break inside a word the reader must repair.
        if (lines.size > 1 && getComputedStyle(node.parentElement).hyphens !== 'auto') broken.add(match[0]);
      }
    }
  }
  if (broken.size > 0) findings.push(`G3: words broken across lines: ${[...broken].slice(0, 8).join(', ')}`);

  // G3: a name cut without an ellipsis.
  for (const el of document.querySelectorAll('[role="treeitem"] .label, [role="tab"], [role="menuitem"], button')) {
    if (!visible(el) || el.scrollWidth <= el.clientWidth + 1) continue;
    const style = getComputedStyle(el);
    if (style.textOverflow !== 'ellipsis' && style.overflow !== 'visible' && el.children.length === 0) findings.push(`G3: "${name(el)}" is cut without an ellipsis`);
  }

  // G4: an empty state is centred in its region.
  for (const empty of document.querySelectorAll('.empty[data-kind]')) {
    if (!visible(empty) || !empty.parentElement) continue;
    const box = empty.getBoundingClientRect();
    const region = empty.parentElement.getBoundingClientRect();
    const off = Math.round(box.left + box.width / 2 - (region.left + region.width / 2));
    if (Math.abs(off) > 2) findings.push(`G4: an empty state stands ${off} px off its region's centre`);
  }
  return findings;
}
