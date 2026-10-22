// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Fails a status told by colour alone (design-system.md §6 rule 3, P-13).
//
// A status is the standing state of a thing - health, connection, over a limit, broken, done,
// invalid. It is drawn as a tinted surface with text or an icon. A sentence about an act that was
// just refused is not a status: it is text, and red text that is a sentence is not colour standing
// alone. The gate tells the two apart by the token, not by the class name, because a list of class
// names is a list that goes stale:
//
//   1. A rule that paints with any `status.<tone>.*` token stands on that tone's surface -
//      `status.<tone>.surface`, or its `accent` as a fill - set in the same rule or in a rule whose
//      selector is a prefix of its own: the element without its state, or an element it sits in.
//      The status components themselves draw the surface their own way and are exempt by name.
//   2. `text.danger`, `text.success` and `text.warning` are for sentences: only on `color`, and
//      never under a selector that carries a state attribute. An attribute that names a kind
//      rather than a state is on a closed list below.
//   3. Both through one level of custom-property indirection, and in Svelte `style:` directives,
//      where a status token cannot be checked against a surface at all.
//
// It reads the source, comments stripped, as the other gates in this directory do.

import fs from 'node:fs';
import path from 'node:path';

/** The components that are a status: their surface is drawn their own way, through a tone. */
export const STATUS_COMPONENTS = new Set(['Badge.svelte', 'Banner.svelte', 'Toast.svelte', 'Callout.svelte', 'RunStatusBadge.svelte']);

/**
 * Attributes that name a kind rather than a state, so red text under them is a kind of action or a
 * property, not a status: [file name or '*', attribute].
 */
export const NON_STATE_ATTRIBUTES = [
  ['*', 'data-destructive'], // a destructive menu item: a kind of action
  ['Button.svelte', 'data-tone'], // a danger button: a kind of action
  ['IconButton.svelte', 'data-tone'], // as Button
  ['ParameterTable.svelte', 'data-required'], // a required parameter: a property of the parameter
];

const STATUS_TOKEN = /var\(--status-(info|success|warning|danger|neutral)-(surface|border|text|accent)\)/g;
const TEXT_TOKEN = /var\(--text-(danger|success|warning)\)/g;
const LOCAL = /var\((--[\w-]+)\)/g;

const stripComments = (text) => text.replace(/\/\*[\s\S]*?\*\//g, (m) => m.replace(/[^\n]/g, ' ')).replace(/<!--[\s\S]*?-->/g, (m) => m.replace(/[^\n]/g, ' '));

/** styleOf returns the style a file declares with its line offset: a component's block, or a whole stylesheet. */
function styleOf(file, text) {
  if (file.endsWith('.css')) return { css: text, offset: 0 };
  const start = text.indexOf('<style');
  if (start < 0) return { css: '', offset: 0 };
  const open = text.indexOf('>', start) + 1;
  const end = text.indexOf('</style>', open);
  return { css: text.slice(open, end < 0 ? undefined : end), offset: text.slice(0, open).split('\n').length - 1 };
}

/** rules flattens a stylesheet into its leaf rules: { selectors, declarations, line }. */
export function rules(css, offset = 0) {
  const out = [];
  const walk = (text, base) => {
    let i = 0;
    while (i < text.length) {
      const brace = text.indexOf('{', i);
      if (brace < 0) break;
      const prelude = text.slice(i, brace).replace(/^[\s;]+/, '');
      let depth = 1;
      let j = brace + 1;
      while (j < text.length && depth > 0) {
        if (text[j] === '{') depth += 1;
        else if (text[j] === '}') depth -= 1;
        j += 1;
      }
      const body = text.slice(brace + 1, j - 1);
      const line = base + text.slice(0, brace).split('\n').length - 1;
      if (prelude.trim().startsWith('@') || body.includes('{')) {
        if (!prelude.trim().startsWith('@keyframes')) walk(body, base + text.slice(0, brace + 1).split('\n').length - 1);
      } else {
        const declarations = body
          .split(';')
          .map((d) => d.trim())
          .filter((d) => d.includes(':'))
          .map((d) => ({ property: d.slice(0, d.indexOf(':')).trim(), value: d.slice(d.indexOf(':') + 1).trim() }));
        const selectors = prelude.split(',').map((s) => s.trim().replace(/\s+/g, ' ')).filter(Boolean);
        out.push({ selectors, declarations, line: line + 1 });
      }
      i = j;
    }
  };
  walk(css, offset);
  return out;
}

/** The status and text tokens a value reaches, directly or through one local custom property. */
function reach(value, locals) {
  const status = [...value.matchAll(STATUS_TOKEN)].map((m) => ({ tone: m[1], role: m[2] }));
  const text = [...value.matchAll(TEXT_TOKEN)].map((m) => m[1]);
  for (const [, name] of value.matchAll(LOCAL)) {
    const local = locals.get(name);
    if (local) {
      status.push(...local.status);
      text.push(...local.text);
    }
  }
  return { status, text };
}

/**
 * prefixes lists the selectors a surface may be set on for this one: the selector itself, the same
 * element with trailing classes, attributes or pseudo-classes taken away, and every element it
 * sits in.
 */
function prefixes(selector) {
  const out = [selector];
  let current = selector;
  const simple = /(\[[^\]]*\]|::?[\w-]+(\([^)]*\))?|\.[\w-]+|#[\w-]+)$/;
  const combinator = /\s*[>+~]?\s*$/;
  for (;;) {
    let next = current.replace(simple, '');
    if (next === current) break;
    if (/[\s>+~]$/.test(next)) next = next.replace(combinator, '');
    if (!next) break;
    out.push(next);
    current = next;
  }
  return out;
}

const isBackground = (property) => property === 'background' || property === 'background-color';

/** findViolations returns every status told by colour alone in one file's source. */
export function findViolations(file, source) {
  const name = path.basename(file);
  if (STATUS_COMPONENTS.has(name)) return [];
  const text = stripComments(source);
  const { css, offset } = styleOf(file, text);
  const all = rules(css, offset);
  const found = [];

  // One level of indirection: what each local custom property declared in this file reaches.
  const locals = new Map();
  for (const rule of all) {
    for (const { property, value } of rule.declarations) {
      if (!property.startsWith('--')) continue;
      const reached = reach(value.replace(LOCAL, (m) => m), new Map());
      const known = locals.get(property) ?? { status: [], text: [] };
      known.status.push(...reached.status);
      known.text.push(...reached.text);
      locals.set(property, known);
    }
  }

  // Which tones each selector gives a surface to.
  const surfaces = new Map();
  for (const rule of all) {
    for (const { property, value } of rule.declarations) {
      if (!isBackground(property)) continue;
      const tones = reach(value, locals).status.filter((s) => s.role === 'surface' || s.role === 'accent').map((s) => s.tone);
      for (const selector of rule.selectors) {
        const known = surfaces.get(selector) ?? new Set();
        tones.forEach((t) => known.add(t));
        surfaces.set(selector, known);
      }
    }
  }

  const nonState = NON_STATE_ATTRIBUTES.filter(([f]) => f === '*' || f === name).map(([, a]) => a);
  const stateAttribute = (selector) =>
    [...selector.matchAll(/\[\s*([\w-]+)/g)].map((m) => m[1]).find((a) => !nonState.includes(a));

  for (const rule of all) {
    for (const { property, value } of rule.declarations) {
      if (property.startsWith('--')) continue;
      const { status, text: words } = reach(value, locals);
      for (const selector of rule.selectors) {
        for (const { tone } of status) {
          const covered = prefixes(selector).some((s) => surfaces.get(s)?.has(tone));
          if (!covered) {
            found.push({ file, line: rule.line, what: `${selector} paints with status.${tone}.* (${property}) without the ${tone} surface under it` });
          }
        }
        if (words.length > 0 && property !== 'color') {
          found.push({ file, line: rule.line, what: `${selector} uses text.${words[0]} on ${property}; it is for sentences, on color only - a status takes status.${words[0]}.* with its surface` });
        } else if (words.length > 0) {
          const attribute = stateAttribute(selector);
          if (attribute) {
            found.push({ file, line: rule.line, what: `${selector} colours text.${words[0]} under the state attribute ${attribute}: a state is a status, drawn on its surface` });
          }
        }
      }
    }
  }

  // Svelte style directives: a status token there cannot be held to a surface.
  const markup = file.endsWith('.svelte') ? text.slice(0, text.indexOf('<style') < 0 ? undefined : text.indexOf('<style')) : '';
  for (const match of markup.matchAll(/style:([\w-]+)\s*=\s*(\{[^}]*\}|"[^"]*")/g)) {
    if (/--status-|--text-(danger|success|warning)|\bstatus\.|\btext\.(danger|success|warning)\b/.test(match[2])) {
      found.push({ file, line: markup.slice(0, match.index).split('\n').length, what: `style:${match[1]} sets a status colour no gate can hold to its surface` });
    }
  }
  return found;
}

const SKIP_DIRS = new Set(['node_modules', 'dist', '.vite', '.turbo', '.svelte-kit']);

/** lint walks one source tree and returns every violation in it. */
export function lint(root) {
  const found = [];
  const walk = (dir) => {
    for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
      if (SKIP_DIRS.has(entry.name)) continue;
      const joined = path.join(dir, entry.name);
      if (entry.isDirectory()) walk(joined);
      else if (entry.name.endsWith('.svelte') || entry.name.endsWith('.css')) found.push(...findViolations(joined, fs.readFileSync(joined, 'utf8')));
    }
  };
  walk(root);
  return found;
}

// The selftest: one planted case per shape the gate must not miss, and the clean shapes it must
// let through. A checker that cannot fail proves nothing by passing.
export function selftest() {
  const style = (css, markup = '') => `${markup}\n<style>\n${css}\n</style>\n`;
  const planted = [
    ['a status text with no surface', style(".a { color: var(--status-danger-text); }")],
    ['a status border with no surface', style(".a { border-color: var(--status-warning-border); }")],
    ['a surface of another tone', style(".a { color: var(--status-danger-text); background: var(--status-success-surface); }")],
    ['a sentence colour on a border', style(".a { border-color: var(--text-danger); }")],
    ['a sentence colour under a state attribute', style(".a[data-state='failed'] { color: var(--text-danger); }")],
    ['a sentence colour under aria state', style(".a[aria-invalid='true'] { color: var(--text-danger); }")],
    ['a status token through a local property', style(".a { --x: var(--status-success-text); }\n.b { color: var(--x); }")],
    ['a sentence colour through a local property, under a state', style(".a { --y: var(--text-warning); }\n.b[data-over] { color: var(--y); }")],
    ['a status token in a style directive', style('', '<span style:color={"var(--status-danger-text)"}></span>')],
    ['a status token from the tokens module in a style directive', style('', '<span style:color={tokens.status.danger.text}></span>')],
    ['a status inside a media query', style("@media (min-width: 1px) { .a { color: var(--status-info-text); } }")],
    ['one selector of a list without its surface', style(".a { background: var(--status-info-surface); }\n.a, .b { color: var(--status-info-text); }")],
  ];
  const clean = [
    ['a refusal sentence', style(".failure { color: var(--text-danger); }")],
    ['a status on its surface', style(".a { color: var(--status-danger-text); background: var(--status-danger-surface); }")],
    ['a dot filled with the accent', style(".a { background: var(--status-success-accent); }")],
    ['the surface on the same element, without its state', style(".t { background: var(--status-info-surface); }\n.t[data-x] { color: var(--status-info-text); }")],
    ['a destructive menu item', style(".item[data-destructive='true'] { color: var(--text-danger); }")],
    ['a status through locals on its surface', style(".a[data-tone='info'] { --s: var(--status-info-surface); --t: var(--status-info-text); }\n.a { background: var(--s); color: var(--t); }")],
    ['a hover in a sentence colour', style(".tool:hover { color: var(--text-danger); }")],
    ['text on the surface of the element it sits in', style(".line[data-kind='added'] { background: var(--status-success-surface); }\n.line[data-kind='added'] .gutter { color: var(--status-success-text); }")],
  ];
  let ok = true;
  for (const [what, source] of planted) {
    if (findViolations('Planted.svelte', source).length === 0) {
      console.error(`lint-status selftest: ${what} was not caught`);
      ok = false;
    }
  }
  for (const [what, source] of clean) {
    const hits = findViolations('Clean.svelte', source);
    if (hits.length > 0) {
      console.error(`lint-status selftest: ${what} was flagged: ${hits[0].what}`);
      ok = false;
    }
  }
  return ok;
}

// As a command: `node lint-status.js [--selftest] [dir…]` checks each directory (default `src`
// under the working directory) - how the web app's `pnpm lint` runs it over its own tree.
if (process.argv[1] && path.resolve(process.argv[1]) === path.resolve(new URL(import.meta.url).pathname)) {
  if (process.argv.includes('--selftest') && !selftest()) process.exit(1);
  const roots = process.argv.slice(2).filter((a) => !a.startsWith('--'));
  const found = (roots.length > 0 ? roots : ['src']).flatMap((root) => lint(path.resolve(root)));
  for (const v of found) console.error(`${path.relative(process.cwd(), v.file)}:${v.line}: ${v.what}`);
  if (found.length > 0) {
    console.error(`${found.length} status(es) told by colour alone (design-system.md §6 rule 3).`);
    process.exit(1);
  }
}
