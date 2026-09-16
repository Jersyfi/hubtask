// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Fails when a `var(--x)` names a custom property nothing defines.
//
// lint-no-literals.js refuses a value written outside tokens.json; this is its other half. A
// reference to a property the generated layer does not carry is not an error the browser reports
// - the declaration is invalid at computed-value time and the background is simply transparent -
// which is how six components came to paint nothing where they meant to paint a lift (issue 711).
//
// A property counts as defined when tokens.json generates it - the names are derived here through
// the same table the build uses, so the check needs no build to have run - or when a file in the
// scanned tree declares it itself: a `--x:` rule, or Svelte's `style:--x=` directive. Both are
// how a component carries a value it computed to a rule that reads it, and neither is a token. A
// name that is computed - `var(--bp-${width})` - is not checked, because it cannot be.

import fs from 'node:fs';
import path from 'node:path';

import { cssName } from './naming.js';

const SKIP_DIRS = new Set(['node_modules', 'dist', '.vite', '.turbo', '.svelte-kit']);
const EXTENSIONS = new Set(['.css', '.ts', '.tsx', '.js', '.jsx', '.html', '.svelte']);
const EXEMPTION = /design-system-lint-ignore/;

const REFERENCE = /var\(\s*(--[A-Za-z0-9_-]+)(?![A-Za-z0-9_$-])/g;
const DECLARATION = /(?:^|[\s;{"'])(--[A-Za-z0-9_-]+)\s*:/g;
const DIRECTIVE = /style:(--[A-Za-z0-9_-]+)/g;
/** A line that is prose: a reference quoted in a comment is not one the browser resolves. */
const COMMENT = /^\s*(?:\/\/|\/\*|\*|<!--)/;

function* files(dir) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    if (entry.isDirectory()) {
      if (!SKIP_DIRS.has(entry.name)) yield* files(path.join(dir, entry.name));
    } else if (EXTENSIONS.has(path.extname(entry.name))) {
      yield path.join(dir, entry.name);
    }
  }
}

function* matches(pattern, text) {
  pattern.lastIndex = 0;
  let match;
  while ((match = pattern.exec(text)) !== null) yield match[1];
}

/** The custom properties tokens.json generates: every leaf of the tree, named as the build names it. */
export function generatedCustomProperties(tokensFile) {
  const names = new Set();
  const walk = (node, at) => {
    for (const [key, value] of Object.entries(node)) {
      if (key.startsWith('$') || value === null || typeof value !== 'object') continue;
      if ('$value' in value) names.add(`--${cssName([...at, key])}`);
      else walk(value, [...at, key]);
    }
  };
  walk(JSON.parse(fs.readFileSync(tokensFile, 'utf8')), []);
  return names;
}

/**
 * Every `var(--x)` under `roots` that neither tokens.json nor the tree declares, as
 * `file:line: message` strings.
 */
export function undefinedCustomProperties({ repositoryRoot, roots, tokens }) {
  const defined = generatedCustomProperties(path.join(repositoryRoot, tokens));
  const problems = [];

  const sources = [];
  for (const root of roots) {
    const absolute = path.join(repositoryRoot, root);
    if (!fs.existsSync(absolute)) continue;
    for (const file of files(absolute)) {
      const text = fs.readFileSync(file, 'utf8');
      sources.push({ relative: path.relative(repositoryRoot, file), text });
      for (const name of matches(DECLARATION, text)) defined.add(name);
      for (const name of matches(DIRECTIVE, text)) defined.add(name);
    }
  }

  for (const { relative, text } of sources) {
    const lines = text.split('\n');
    lines.forEach((line, index) => {
      if (EXEMPTION.test(line) || (index > 0 && EXEMPTION.test(lines[index - 1]))) return;
      if (COMMENT.test(line)) return;
      for (const name of matches(REFERENCE, line)) {
        if (!defined.has(name)) {
          problems.push(`${relative}:${index + 1}: var(${name}) names a custom property nothing defines`);
        }
      }
    });
  }
  return problems;
}
