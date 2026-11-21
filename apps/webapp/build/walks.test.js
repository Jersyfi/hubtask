// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The rules for a walk that a count can hold (ci-cd.md §3, `engines`), against e2e/bounds.json:
// the fixed waits each file has left, and the suites that neither cite a use case check nor say
// what they hold instead. Both lists only shrink - a file or suite that is not listed has none,
// so a new suite starts clean, and an entry that got better fails until it is lowered or removed.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const E2E = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', 'e2e');
const BOUNDS = 'apps/webapp/e2e/bounds.json';

// The space between a use case and its check, as tools/checkdocs reads it: one, or a line break
// into the next line of the same comment.
const WRAP = String.raw`(?: |\n[ \t]*(?://|\*) ?)`;
// `UC-ID-12/4`, `UC-ID-12 check 4`, `UC-ID-12 checks 2, 3 and 6` (docs/usecases/README.md).
const CHECK_CITED = new RegExp(String.raw`\bUC-[A-Z]{2,3}-\d{2,3}(?:/\d+\b|:?,?${WRAP}checks? \d+\b)`);
const HOLDS_INSTEAD = /^\/\/ No use case check: \S/m;

/** A file's fixed waits: its calls of Playwright's `waitForTimeout`. */
export const fixedWaits = (source) => (source.match(/\.waitForTimeout\(/g) ?? []).length;

/** A suite's own stubs: its calls of `route(` on a context or a page. */
export const routeStubs = (source) => (source.match(/\.route\(/g) ?? []).length;

/** What stands above a suite's first import: its header comment. */
const header = (source) => source.split(/^import /m)[0];

/** Whether a suite names a use case check anywhere, or says in its header what it holds instead. */
export const saysWhatItProves = (source) => CHECK_CITED.test(source) || HOLDS_INSTEAD.test(header(source));

/**
 * What separates the files from the bounds, as sentences a worker can act on. `files` maps a file
 * name to its source; a name ending in `.test.mjs` is a suite.
 */
export function boundProblems(files, bounds) {
  const problems = [];
  for (const [name, source] of Object.entries(files)) {
    const waits = fixedWaits(source);
    const allowed = bounds.fixed_waits[name] ?? 0;
    if (waits > allowed) problems.push(`${name} has ${waits} fixed waits (waitForTimeout), at most ${allowed} may be: wait for what the person would see`);
    if (waits < allowed) problems.push(`${name} has ${waits} fixed waits, fewer than its bound: ${waits === 0 ? 'remove its entry from' : `lower it to ${waits} in`} fixed_waits in ${BOUNDS}`);
    if (!name.endsWith('.test.mjs')) continue;
    const listed = bounds.no_check_cited.includes(name);
    const says = saysWhatItProves(source);
    if (!says && !listed) problems.push(`${name} cites no use case check (UC-XX-nn check n, UC-XX-nn/n) and has no "// No use case check: …" line in its header`);
    if (says && listed) problems.push(`${name} says what it proves now: remove it from no_check_cited in ${BOUNDS}`);
  }
  for (const name of [...Object.keys(bounds.fixed_waits), ...bounds.no_check_cited]) {
    if (!(name in files)) problems.push(`${name} is listed in ${BOUNDS} and does not exist: remove its entry`);
  }
  return problems;
}

const suite = (source = '') => `// A walk.\n\nimport { test } from 'node:test';\n${source}`;
const none = { fixed_waits: {}, no_check_cited: [] };

test('a fixed wait beyond the bound is refused, in a new suite and in a listed one', () => {
  const cited = suite("test('UC-WRK-01 check 2', async () => { await page.waitForTimeout(50); });");
  assert.match(boundProblems({ 'new.test.mjs': cited }, none)[0], /new\.test\.mjs has 1 fixed waits .* at most 0/);
  assert.deepEqual(boundProblems({ 'old.test.mjs': cited }, { fixed_waits: { 'old.test.mjs': 1 }, no_check_cited: [] }), []);
  assert.match(boundProblems({ 'helper.mjs': 'await page.waitForTimeout(50);' }, none)[0], /helper\.mjs has 1 fixed waits/);
});

test('a bound that was beaten fails until it is lowered', () => {
  const cited = suite('// UC-WRK-01/2');
  assert.match(boundProblems({ 'old.test.mjs': cited }, { fixed_waits: { 'old.test.mjs': 2 }, no_check_cited: [] })[0], /remove its entry/);
  assert.match(boundProblems({ 'old.test.mjs': `${cited} x.waitForTimeout(1)` }, { fixed_waits: { 'old.test.mjs': 2 }, no_check_cited: [] })[0], /lower it to 1/);
  assert.match(boundProblems({ 'old.test.mjs': cited }, { fixed_waits: {}, no_check_cited: ['old.test.mjs'] })[0], /remove it from no_check_cited/);
  assert.match(boundProblems({}, { fixed_waits: { 'gone.test.mjs': 1 }, no_check_cited: [] })[0], /does not exist/);
});

test('a suite cites a check, or its header says what it holds instead', () => {
  for (const says of ['// UC-ID-12/4', '// UC-ID-12 check 4', '// UC-WRK-32 checks 2, 3 and 6', '// proves UC-ID-12,\n// check 4']) {
    assert.deepEqual(boundProblems({ 'a.test.mjs': suite(says) }, none), [], says);
  }
  assert.deepEqual(boundProblems({ 'a.test.mjs': `// No use case check: the bundle loads in each engine.\n${suite()}` }, none), []);
  // A use case without a check number names no check, and the line counts in the header only.
  assert.equal(boundProblems({ 'a.test.mjs': suite('// UC-ID-12') }, none).length, 1);
  assert.equal(boundProblems({ 'a.test.mjs': suite('// No use case check: below the imports') }, none).length, 1);
  assert.deepEqual(boundProblems({ 'a.mjs': '// a helper' }, none), []);
});

test('the walks keep to their bounds', (t) => {
  const files = Object.fromEntries(fs.readdirSync(E2E).filter((name) => name.endsWith('.mjs')).sort()
    .map((name) => [name, fs.readFileSync(path.join(E2E, name), 'utf8')]));
  const bounds = JSON.parse(fs.readFileSync(path.join(E2E, 'bounds.json'), 'utf8'));
  const suites = Object.entries(files).filter(([name]) => name.endsWith('.test.mjs'));
  const waits = Object.values(bounds.fixed_waits);
  const stubs = suites.map(([, source]) => routeStubs(source)).filter(Boolean);
  t.diagnostic(`${suites.length} suites; ${waits.reduce((sum, n) => sum + n, 0)} fixed waits in ${waits.length} files; ${bounds.no_check_cited.length} suites cite no check`);
  // Reported, not bounded: whether a stub belongs in the fixture is the reviewer's question.
  t.diagnostic(`${stubs.reduce((sum, n) => sum + n, 0)} route stubs in ${stubs.length} suites`);
  assert.deepEqual(boundProblems(files, bounds), []);
});
