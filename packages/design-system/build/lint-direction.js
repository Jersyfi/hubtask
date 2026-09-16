// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Fails when a style names a physical inline side - `left`, `right`, a signed translation - where
// the document may run either way.
//
// This is design-system.md §3's "start/end only, never left/right" given teeth for every client
// tree at once. The conventions test had the rule for the components alone, and the three
// `padding-left` the RTL audit found were all in the application (F5-10): a rule that reads one
// tree is a rule the other tree does not have. The failure it catches is invisible in development
// and total in Arabic, which is why it is a gate and not a review comment.
//
// It reads the source tree, never the build output, and only what the browser is told: comments
// are stripped first, because the first thing the component rule found was its own comment saying
// "never left/right". A line that genuinely has to name a side - an override that undoes a
// physical default the platform imposes - carries `design-system-lint-ignore` with its reason, on
// the line or the line above, the way the literal lint's exemption works.

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const repositoryRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', '..', '..');

/** The trees this applies to: everything a browser renders. */
const ROOTS = ['apps', 'packages'];
const SKIP_DIRS = new Set(['node_modules', 'dist', '.vite', '.turbo', '.svelte-kit']);
/** Where a style can be written. A `.ts` file that builds a style string is not a case this has met. */
const EXTENSIONS = new Set(['.css', '.svelte', '.html']);

const EXEMPTION = /design-system-lint-ignore/;

/**
 * Each rule names what it found and what to write instead. The order is the order of the
 * message, not of precedence: a line is reported once, for the first rule that matches it.
 */
export const RULES = [
  {
    what: 'a physical margin, padding, border or scroll edge',
    pattern: /\b(?:margin|padding|border|scroll-margin|scroll-padding)-(?:left|right)(?:-[a-z]+)?\s*:/,
    instead: 'the logical side: -inline-start or -inline-end',
  },
  {
    what: 'a physical inset',
    pattern: /(?:^|[^-\w])(?:left|right)\s*:/,
    instead: 'inset-inline-start or inset-inline-end',
  },
  {
    what: 'a physical corner',
    pattern: /\bborder-(?:top|bottom)-(?:left|right)-radius\s*:/,
    instead: 'border-start-start-radius and its three siblings',
  },
  {
    what: 'a physical alignment',
    pattern: /\b(?:text-align|float|clear)\s*:\s*(?:left|right)\b/,
    instead: 'start or end; a float becomes inline-start or inline-end',
  },
  {
    what: 'a physical keyword',
    pattern: /\b(?:transform-origin|background-position|object-position|justify-content|justify-items|justify-self)\s*:[^;{}]*\b(?:left|right)\b/,
    instead: 'start, end, or a percentage',
  },
  {
    what: 'a signed translation',
    pattern: /\btranslate(?:X|3d)?\(\s*-?(?:[1-9]\d*|0?\.\d+)/,
    instead: 'a custom property the `:dir(rtl)` rule negates, or a logical inset',
  },
  {
    what: 'a physical resize cursor',
    pattern: /\bcursor\s*:\s*(?:n?[ew]|s[ew])-resize\b/,
    instead: 'ew-resize, which has no side',
  },
];

/** The text with every comment removed, so that a rule reads what the browser reads. */
export function withoutComments(text) {
  return text
    .replace(/<!--[\s\S]*?-->/g, (match) => match.replace(/[^\n]/g, ' '))
    .replace(/\/\*[\s\S]*?\*\//g, (match) => match.replace(/[^\n]/g, ' '))
    .replace(/(^|[^:\\])\/\/[^\n]*/g, (_, before) => before);
}

/**
 * Every physical side written in `text`, as `{ line, what, found, instead }`, honouring the
 * exemption marker. `text` is one file's whole content; the caller decides what a file is.
 */
export function findPhysical(text) {
  const raw = text.split('\n');
  const lines = withoutComments(text).split('\n');
  const found = [];
  lines.forEach((line, index) => {
    if (EXEMPTION.test(raw[index]) || (index > 0 && EXEMPTION.test(raw[index - 1]))) return;
    for (const rule of RULES) {
      const match = rule.pattern.exec(line);
      if (match) {
        found.push({ line: index + 1, what: rule.what, found: match[0].trim(), instead: rule.instead });
        return;
      }
    }
  });
  return found;
}

function* files(dir) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    if (entry.isDirectory()) {
      if (!SKIP_DIRS.has(entry.name)) yield* files(path.join(dir, entry.name));
    } else if (EXTENSIONS.has(path.extname(entry.name))) {
      yield path.join(dir, entry.name);
    }
  }
}

/** Every finding under the roots, as printable lines. */
export function lint(root = repositoryRoot) {
  const problems = [];
  for (const tree of ROOTS) {
    const absolute = path.join(root, tree);
    if (!fs.existsSync(absolute)) continue;
    for (const file of files(absolute)) {
      const relative = path.relative(root, file);
      for (const hit of findPhysical(fs.readFileSync(file, 'utf8'))) {
        problems.push(`${relative}:${hit.line}: ${hit.what}: ${hit.found} - write ${hit.instead}`);
      }
    }
  }
  return problems;
}

// The selftest: a checker that cannot fail proves nothing by passing. Each planted line is one
// rule's failure, and the clean lines are the logical spellings the rules must let through.
export function selftest() {
  const planted = [
    ['padding-left: var(--sp-200);', 'a physical margin, padding, border or scroll edge'],
    ['margin-right: 0;', 'a physical margin, padding, border or scroll edge'],
    ['border-left-width: var(--sp-025);', 'a physical margin, padding, border or scroll edge'],
    ['scroll-padding-left: var(--sp-100);', 'a physical margin, padding, border or scroll edge'],
    ['  left: 0;', 'a physical inset'],
    ['.x { right: var(--sp-100); }', 'a physical inset'],
    ['border-top-left-radius: var(--radius-md);', 'a physical corner'],
    ['text-align: left;', 'a physical alignment'],
    ['float: right;', 'a physical alignment'],
    ['transform-origin: left center;', 'a physical keyword'],
    ['justify-content: right;', 'a physical keyword'],
    ['transform: translateX(-100%);', 'a signed translation'],
    ['transform: translate(4px, 0);', 'a signed translation'],
    ['cursor: e-resize;', 'a physical resize cursor'],
  ];
  const clean = [
    'padding-inline-start: var(--sp-200);',
    'margin-inline-end: 0;',
    'inset-inline-start: 0;',
    'border-start-start-radius: var(--radius-md);',
    'text-align: start;',
    'transform: translateX(var(--slide-from));',
    'transform: translateX(0);',
    'cursor: ew-resize;',
    '/* never left or right */',
    '<!-- a comment that says left: 0 -->',
    'const copyright = "all rights reserved"; // right: a word, not a side',
    '/* design-system-lint-ignore: the platform default is physical */\n  left: 0;',
    'left: 0; /* design-system-lint-ignore: the platform default is physical */',
  ];

  let ok = true;
  for (const [line, what] of planted) {
    const hits = findPhysical(line);
    if (hits.length !== 1 || hits[0].what !== what) {
      console.error(`lint-direction selftest: ${JSON.stringify(line)} was not caught as ${what}`);
      ok = false;
    }
  }
  for (const line of clean) {
    const hits = findPhysical(line);
    if (hits.length !== 0) {
      console.error(`lint-direction selftest: a correct line was flagged: ${JSON.stringify(line)} as ${hits[0].what}`);
      ok = false;
    }
  }
  if (ok) console.log(`lint-direction selftest: all ${planted.length} planted violations caught, ${clean.length} clean lines passed`);
  return ok;
}

const invokedDirectly = process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url);
if (invokedDirectly) {
  if (process.argv.includes('--selftest')) {
    process.exit(selftest() ? 0 : 1);
  }
  const problems = lint();
  if (problems.length > 0) {
    for (const problem of problems) console.error(problem);
    console.error(
      `\n${problems.length} physical side(s) written where the document may run either way.\n` +
        'Write the logical property, or mark the line `design-system-lint-ignore` with the reason\n' +
        'the side has to be physical (design-system.md §3, i18n-l10n.md §6 line 6).',
    );
    process.exit(1);
  }
  console.log('design system: no physical inline side in any client tree');
}
