// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The reading the page-system gates share (pagekinds.test.js, dialogs.test.js): a Svelte file's
// markup with its script, style and comments blanked - so a word in a comment is no element -, the
// elements of one name with where they start and end, and the local components a file draws. Read
// from the source rather than a rendered page, because a gate over the source holds a screen that
// no walk opens yet.

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

export const SRC = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', 'src');

/** Every Svelte file under `dir`. */
export function svelteFiles(dir = SRC) {
  return fs.readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) return svelteFiles(full);
    return entry.name.endsWith('.svelte') ? [full] : [];
  });
}

const blank = (text) => text.replace(/[^\n]/g, ' ');

const cache = new Map();

/** The file's source and its markup: script, style and comments replaced by spaces, so offsets hold. */
export function read(file) {
  let entry = cache.get(file);
  if (!entry) {
    const source = fs.readFileSync(file, 'utf8');
    const markup = source
      .replace(/<script\b[\s\S]*?<\/script>/g, blank)
      .replace(/<style\b[\s\S]*?<\/style>/g, blank)
      .replace(/<!--[\s\S]*?-->/g, blank);
    entry = { source, markup };
    cache.set(file, entry);
  }
  return entry;
}

/** Where the tag opened at `start` ends, past its `>`, and whether it closes itself. */
function tagEnd(markup, start) {
  let depth = 0;
  let quote = '';
  for (let i = start + 1; i < markup.length; i += 1) {
    const char = markup[i];
    if (quote) {
      if (char === '\\') i += 1;
      else if (char === quote) quote = '';
      continue;
    }
    if (char === '"' || char === "'" || (char === '`' && depth > 0)) quote = char;
    else if (char === '{') depth += 1;
    else if (char === '}') depth -= 1;
    else if (char === '>' && depth === 0) return { end: i + 1, isSelfClosing: markup[i - 1] === '/' };
  }
  return { end: markup.length, isSelfClosing: true };
}

/** Each element named `name`: its opening tag and the range up to its closing tag. */
export function elements(markup, name) {
  const found = [];
  const open = new RegExp(`<${name}(?=[\\s/>])`, 'g');
  const first = new RegExp(`<${name}(?=[\\s/>])`);
  const close = `</${name}>`;
  for (const match of markup.matchAll(open)) {
    const { end, isSelfClosing } = tagEnd(markup, match.index);
    const tag = markup.slice(match.index, end);
    if (isSelfClosing) {
      found.push({ start: match.index, end, tag });
      continue;
    }
    // The matching close: same-named elements inside it are counted.
    let depth = 1;
    let at = end;
    while (depth > 0) {
      const nextOpen = markup.slice(at).search(first);
      const nextClose = markup.indexOf(close, at);
      if (nextClose < 0) break;
      const openAt = nextOpen < 0 ? -1 : at + nextOpen;
      if (openAt >= 0 && openAt < nextClose) {
        const inner = tagEnd(markup, openAt);
        if (!inner.isSelfClosing) depth += 1;
        at = inner.end;
      } else {
        depth -= 1;
        at = nextClose + close.length;
      }
    }
    found.push({ start: match.index, end: at, tag });
  }
  return found;
}

/** Whether an offset lies inside any of the ranges. */
export const within = (ranges, at) => ranges.some((range) => at > range.start && at < range.end);

/** The components a file imports from inside `src/`, by the name it draws them with. */
export function localComponents(file) {
  const { source } = read(file);
  const found = new Map();
  for (const match of source.matchAll(/import\s+(\w+)\s+from\s+'(\.{1,2}\/[^']+\.svelte)'/g)) {
    found.set(match[1], path.resolve(path.dirname(file), match[2]));
  }
  return found;
}

/** Each place the file draws one of its local components: the component's file and the offset. */
export function uses(file) {
  const { markup } = read(file);
  const drawn = [];
  for (const [name, target] of localComponents(file)) {
    for (const match of markup.matchAll(new RegExp(`<${name}(?=[\\s/>])`, 'g'))) drawn.push({ name, file: target, at: match.index });
  }
  return drawn;
}

/** Every file a file draws, itself included, through its local components. */
export function reachable(file, seen = new Set()) {
  if (seen.has(file)) return seen;
  seen.add(file);
  for (const use of uses(file)) if (fs.existsSync(use.file)) reachable(use.file, seen);
  return seen;
}

export const relative = (file) => path.relative(SRC, file);
