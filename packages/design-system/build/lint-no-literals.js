// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Fails when a colour, a spacing, a radius or a duration is written outside tokens.json.
//
// This is the rule of design-system.md §0 given teeth. A design system drifts at exactly the point
// where the same value is written twice, so the useful moment to catch a second hex is before it
// is committed, not when somebody notices there are three bordeaux tones in the product.
//
// It reads the source tree, never the build output: dist/ is generated *from* tokens.json and is
// therefore full of legitimate literals.

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const repositoryRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', '..', '..');

/** The trees this applies to. The Go core is not one of them: it holds no colour at all. */
const ROOTS = ['apps', 'packages'];

/** Generated output and the one file that is allowed to contain values. */
const ALLOWED = [
  path.join('packages', 'design-system', 'tokens', 'tokens.json'),
];
const SKIP_DIRS = new Set(['node_modules', 'dist', '.vite', '.turbo', '.svelte-kit']);
const EXTENSIONS = new Set(['.css', '.ts', '.tsx', '.js', '.jsx', '.html', '.svg', '.json', '.svelte']);

// Colour is checked everywhere; length and duration only in application code.
//
// The distinction is the one design-system.md §0 draws when it says "anywhere in application
// code". A colour written twice is the failure this whole package exists to prevent, wherever it
// appears. A `620px` column in the style guide's own demo layout is not a design decision - it is
// the width that happened to look right in a mock-up, and forcing it into tokens.json would fill
// the source of truth with values no product surface ever reads.
const COLOUR_RULES = [
  { what: 'a colour', pattern: /#[0-9a-fA-F]{3,8}\b/g },
  { what: 'a colour', pattern: /\brgba?\(\s*[\d.]/g },
  { what: 'a colour', pattern: /\bhsla?\(\s*[\d.]/g },
];
const MEASURE_RULES = [
  { what: 'a length', pattern: /(?<![\w.#-])\d+(?:\.\d+)?(?:px|rem|em)\b/g },
  { what: 'a duration', pattern: /(?<![\w.#-])\d+(?:\.\d+)?m?s\b/g },
];

/** Where a bare length or duration is still a mistake: everything a user actually runs. */
const APPLICATION_CODE = [path.join('apps'), path.join('packages', 'design-system', 'src')];
const isApplicationCode = (relative) => APPLICATION_CODE.some((p) => relative.startsWith(p + path.sep));

/**
 * A line is exempt when it, or the comment directly above it, carries this marker. Putting the
 * marker on the line above is what makes the exemption readable: it has room for the reason, and
 * the reason is the only thing that stops an exemption from becoming a habit.
 */
const EXEMPTION = /design-system-lint-ignore/;

function* files(dir) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    if (entry.isDirectory()) {
      if (!SKIP_DIRS.has(entry.name)) yield* files(path.join(dir, entry.name));
    } else if (EXTENSIONS.has(path.extname(entry.name))) {
      yield path.join(dir, entry.name);
    }
  }
}

const problems = [];
for (const root of ROOTS) {
  const absolute = path.join(repositoryRoot, root);
  if (!fs.existsSync(absolute)) continue;
  for (const file of files(absolute)) {
    const relative = path.relative(repositoryRoot, file);
    if (ALLOWED.includes(relative)) continue;
    const lines = fs.readFileSync(file, 'utf8').split('\n');
    lines.forEach((line, index) => {
      if (EXEMPTION.test(line) || (index > 0 && EXEMPTION.test(lines[index - 1]))) return;
      const rules = isApplicationCode(relative) ? [...COLOUR_RULES, ...MEASURE_RULES] : COLOUR_RULES;
      for (const rule of rules) {
        rule.pattern.lastIndex = 0;
        const match = rule.pattern.exec(line);
        if (match) {
          problems.push(`${relative}:${index + 1}: ${rule.what} written outside tokens.json: ${match[0]}`);
          return;
        }
      }
    });
  }
}

if (problems.length > 0) {
  for (const problem of problems) console.error(problem);
  console.error(
    `\n${problems.length} literal value(s) outside packages/design-system/tokens/tokens.json.\n` +
      'Add the value there and use the token, or mark the line `design-system-lint-ignore` with a\n' +
      'reason if it genuinely is not a design value (ADR-0029, design-system.md §0).',
  );
  process.exit(1);
}
console.log('design system: no colour outside tokens.json, and no bare length or duration in application code');
