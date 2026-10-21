// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Collects the faces of design-system.md §3 into dist/, so that a Hubtask contacts no foreign
// domain when it loads (ADR-0029). Nothing is fetched at build time either: the faces come from
// the packages the lockfile pins, and this only copies and rewrites them.
//
// What ships is fonts/faces.json - one list, which the tests read too. Two stylesheets come out of
// it: fonts.css with the three cuts of IBM Plex for Latin, Cyrillic and Greek, which every client
// imports, and fonts-scripts.css with the faces for the other scripts, which the web app imports
// and the English-only website does not.
//
// `unicode-range` is what keeps that cheap: a browser downloads a file only when a character on
// the page falls in its range and no family before it in the stack has it. So every script face is
// cut down to its own script's blocks - a range that also claimed an arrow or a Greek letter would
// be fetched for an English page whose Latin face happens to lack that character.

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const packageRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const modules = path.join(packageRoot, 'node_modules');

/** The faces, as fonts/faces.json lists them. */
export function readFaces() {
  return JSON.parse(fs.readFileSync(path.join(packageRoot, 'fonts', 'faces.json'), 'utf8')).faces;
}

/**
 * A document whose script the condensed cut lacks sets its headings in the interface stack
 * entirely, rather than half condensed (its digits and punctuation) and half regular (its letters).
 * The script is named on the root element by the web app, beside `lang` and `dir`.
 */
const HEADINGS_OUTSIDE_LATIN = `[data-script]:not([data-script='Latn']) {
  --font-display: var(--font-ui);
}`;

const banner = (faces) => `/**
 * The faces of design-system.md §3, self-hosted: ${faces.map((face) => face.family).join(', ')}.
 *
 * Assembled from their packages by build/fonts.js - DO NOT EDIT.
 * Every face is licensed OFL-1.1; see THIRD-PARTY-LICENSES.md.
 */`;

// ---------------------------------------------------------------------------------------------
// unicode-range as intervals

/** `U+0600-06FF, U+25CC, U+4??` as sorted, merged [from, to] pairs. */
export function parseRange(value) {
  const intervals = [];
  for (const part of value.split(',')) {
    const token = part.trim().replace(/^U\+/i, '');
    if (!token) continue;
    if (token.includes('?')) {
      intervals.push([parseInt(token.replaceAll('?', '0'), 16), parseInt(token.replaceAll('?', 'F'), 16)]);
    } else if (token.includes('-')) {
      const [from, to] = token.split('-');
      intervals.push([parseInt(from, 16), parseInt(to, 16)]);
    } else {
      const point = parseInt(token, 16);
      intervals.push([point, point]);
    }
  }
  return merge(intervals);
}

function merge(intervals) {
  const sorted = [...intervals].sort((a, b) => a[0] - b[0]);
  const merged = [];
  for (const [from, to] of sorted) {
    const last = merged.at(-1);
    if (last && from <= last[1] + 1) last[1] = Math.max(last[1], to);
    else merged.push([from, to]);
  }
  return merged;
}

/** The part of `a` that lies in `b`; both sorted and merged. */
export function intersect(a, b) {
  const out = [];
  for (const [aFrom, aTo] of a) {
    for (const [bFrom, bTo] of b) {
      const from = Math.max(aFrom, bFrom);
      const to = Math.min(aTo, bTo);
      if (from <= to) out.push([from, to]);
    }
  }
  return merge(out);
}

const hex = (point) => point.toString(16).toUpperCase().padStart(4, '0');

export function formatRange(intervals) {
  return intervals.map(([from, to]) => (from === to ? `U+${hex(from)}` : `U+${hex(from)}-${hex(to)}`)).join(', ');
}

// ---------------------------------------------------------------------------------------------
// one stylesheet's @font-face rules

/** The rules of a stylesheet, each with the comment that names it in a Fontsource package. */
function rulesOf(css) {
  const rules = [];
  const pattern = /(?:\/\*\s*([^*]*?)\s*\*\/\s*)?@font-face\s*\{([^}]*)\}/g;
  for (const match of css.matchAll(pattern)) rules.push({ name: match[1] ?? '', body: match[2] });
  return rules;
}

/** The value of one descriptor in a rule's body, or undefined. */
function descriptor(body, name) {
  return body.match(new RegExp(`(?:^|;)\\s*${name}\\s*:\\s*([^;]+);?`))?.[1].trim();
}

/**
 * One rule rewritten for dist/: renamed, woff2 only, no `local()` (a reader's own older copy of a
 * face would be a different face), `font-display: swap`, the weight declared over its range, the
 * range cut to the face's blocks, and the files referenced from ./fonts/. Undefined when nothing of
 * the range is left, so that the file is not shipped at all.
 */
function rewrite(rule, face, stylesheetDir, copy) {
  let range = descriptor(rule.body, 'unicode-range');
  if (face.blocks) {
    const kept = intersect(parseRange(range ?? 'U+0-10FFFF'), parseRange(face.blocks.join(',')));
    if (kept.length === 0) return undefined;
    range = formatRange(kept);
  }

  const weight = descriptor(rule.body, 'font-weight');
  const sources = [...(descriptor(rule.body, 'src') ?? '').matchAll(/url\((['"]?)([^'")]+)\1\)\s*format\((['"]?)(woff2(?:-variations)?)\3\)/g)];
  if (sources.length !== 1) {
    throw new Error(`${face.package}: a rule without exactly one woff2 source (${rule.name || 'unnamed'})`);
  }
  const file = copy(path.join(stylesheetDir, sources[0][2]));

  const lines = [
    `  font-family: '${face.family}';`,
    `  font-style: ${descriptor(rule.body, 'font-style') ?? 'normal'};`,
    `  font-display: swap;`,
    `  font-weight: ${face.weights?.[weight] ?? weight};`,
  ];
  const stretch = descriptor(rule.body, 'font-stretch');
  if (stretch) lines.push(`  font-stretch: ${stretch};`);
  lines.push(`  src: url(./fonts/${file}) format('${sources[0][4]}');`);
  if (range) lines.push(`  unicode-range: ${range};`);
  return `${rule.name ? `/* ${rule.name} */\n` : ''}@font-face {\n${lines.join('\n')}\n}`;
}

/**
 * Whether a Fontsource rule belongs to one of the subsets a face keeps. A rule is named
 * `<family>-<subset>-<weight or axis>-<style>`, and the family may contain the subset's name
 * (`ibm-plex-sans-arabic-latin-400-normal` is Latin), so the subset is read just before the weight.
 */
function inSubsets(rule, face) {
  if (!face.subsets) return true;
  return face.subsets.some((subset) => new RegExp(`-${subset}-(\\d+|wght|wdth|standard)-[a-z]+$`).test(rule.name));
}

// ---------------------------------------------------------------------------------------------

export function buildFonts() {
  const faces = readFaces();
  const outFiles = path.join(packageRoot, 'dist', 'fonts');
  fs.rmSync(outFiles, { recursive: true, force: true });
  fs.mkdirSync(outFiles, { recursive: true });

  const copied = new Set();
  const copy = (source) => {
    const file = path.basename(source);
    if (!copied.has(file)) {
      fs.copyFileSync(source, path.join(outFiles, file));
      copied.add(file);
    }
    return file;
  };

  const outputs = new Map();
  for (const face of faces) {
    const base = path.join(modules, face.package);
    if (!fs.existsSync(base)) {
      throw new Error(`${face.package} is not installed - run \`pnpm install\` first`);
    }
    const blocks = outputs.get(face.output) ?? [];
    for (const stylesheet of face.stylesheets) {
      let css = fs.readFileSync(path.join(base, stylesheet), 'utf8');
      if (face.rename) css = css.replaceAll(`'${face.rename}'`, `'${face.family}'`);
      for (const rule of rulesOf(css)) {
        if (!inSubsets(rule, face)) continue;
        const rewritten = rewrite(rule, face, path.dirname(path.join(base, stylesheet)), copy);
        if (rewritten) blocks.push(rewritten);
      }
    }
    outputs.set(face.output, blocks);
  }

  const scripts = outputs.get('fonts-scripts.css') ?? [];
  scripts.push(HEADINGS_OUTSIDE_LATIN);

  let count = 0;
  for (const [output, blocks] of outputs) {
    const inFile = faces.filter((face) => face.output === output);
    fs.writeFileSync(path.join(packageRoot, 'dist', output), `${banner(inFile)}\n\n${blocks.join('\n\n')}\n`);
    count += blocks.filter((block) => block.includes('@font-face')).length;
  }
  return { faces: count, files: copied.size };
}
