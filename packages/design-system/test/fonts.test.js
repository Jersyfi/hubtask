// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The faces of design-system.md §3, held to what they promise (UC-ID-23).
//
// fonts/faces.json is the one list; infrastructure/i18n/ScriptFaces_test.go holds every locale of
// the product to a script in it, and this holds every script in it to a face that really covers
// it in what the build wrote - so a face dropped from the build, a subset filtered away or a range
// that misses its script fails here rather than in a reader's browser.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

import { readFaces, parseRange, intersect } from '../build/fonts.js';

const packageRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const repositoryRoot = path.resolve(packageRoot, '..', '..');
const faces = readFaces();
const read = (...parts) => fs.readFileSync(path.join(...parts), 'utf8');

/** Every @font-face the build wrote, with its family, sources and range. */
function builtRules() {
  const rules = [];
  for (const output of new Set(faces.map((face) => face.output))) {
    const css = read(packageRoot, 'dist', output);
    for (const match of css.matchAll(/@font-face\s*\{([^}]*)\}/g)) {
      const body = match[1];
      rules.push({
        output,
        family: body.match(/font-family:\s*'([^']+)'/)?.[1],
        src: body.match(/src:\s*([^;]+);/)?.[1] ?? '',
        range: body.match(/unicode-range:\s*([^;]+);/)?.[1],
      });
    }
  }
  return rules;
}

const covers = (intervals, point) => intervals.some(([from, to]) => from <= point && point <= to);

test('every script a face is there for is covered by that face, letter by letter', () => {
  const rules = builtRules();
  for (const face of faces) {
    const ranges = rules.filter((rule) => rule.family === face.family).map((rule) => parseRange(rule.range ?? ''));
    assert.ok(ranges.length > 0, `${face.family}: the build wrote no face`);
    for (const [script, sample] of Object.entries(face.scripts)) {
      for (const letter of sample.replace(/\s/g, '')) {
        const point = letter.codePointAt(0);
        assert.ok(ranges.some((range) => covers(range, point)), `${face.family} does not cover ${letter} (U+${point.toString(16)}) of ${script}`);
      }
    }
  }
});

test('every face is loaded only for the parts its text needs, and only from this installation', () => {
  for (const rule of builtRules()) {
    assert.ok(rule.range, `${rule.family}: a face without unicode-range is downloaded for any text`);
    assert.match(rule.src, /^url\(\.\/fonts\/[^)\s]+\.woff2\) format\('woff2(-variations)?'\)$/, `${rule.family}: a source other than a shipped woff2 file: ${rule.src}`);
    const file = rule.src.match(/\.\/fonts\/([^)]+)\)/)[1];
    assert.ok(fs.existsSync(path.join(packageRoot, 'dist', 'fonts', file)), `${file} is referenced and not shipped`);
  }
});

test('a script face serves nothing outside its own script', () => {
  const rules = builtRules();
  for (const face of faces.filter((candidate) => candidate.blocks)) {
    const blocks = parseRange(face.blocks.join(','));
    for (const rule of rules.filter((candidate) => candidate.family === face.family)) {
      const range = parseRange(rule.range);
      assert.deepEqual(intersect(range, blocks), range, `${face.family} claims code points outside its blocks: ${rule.range}`);
    }
  }
  // The character that made this rule necessary: an English activity line's arrow.
  const arrow = '→'.codePointAt(0);
  for (const rule of rules.filter((candidate) => candidate.output === 'fonts-scripts.css')) {
    assert.ok(!covers(parseRange(rule.range), arrow), `${rule.family} would be fetched for an arrow on an English page`);
  }
});

test('every script face is in the stacks the interface sets text and headings in', () => {
  const tokens = JSON.parse(read(packageRoot, 'tokens', 'tokens.json'));
  const stacks = tokens.primitive.fontFamily;
  for (const face of faces.filter((candidate) => candidate.output === 'fonts-scripts.css')) {
    for (const stack of ['ui', 'display', 'mono']) {
      assert.ok(stacks[stack].$value.includes(face.family), `${face.family} is missing from the ${stack} stack`);
    }
  }
  const scripts = read(packageRoot, 'dist', 'fonts-scripts.css');
  assert.match(scripts, /\[data-script\]:not\(\[data-script='Latn'\]\)\s*\{\s*--font-display:\s*var\(--font-ui\);/, 'headings outside Latin keep the condensed stack');
});

test('every face carries its licence row and its notice', () => {
  const template = read(repositoryRoot, 'tools', 'licenses.md.tpl');
  for (const face of faces) {
    assert.ok(template.includes(`[${face.family}]`), `tools/licenses.md.tpl has no row for ${face.family}`);
    const notice = path.join(repositoryRoot, 'third-party', 'licenses', '_bundled', face.notice, 'LICENSE');
    assert.ok(fs.existsSync(notice), `no licence text at ${path.relative(repositoryRoot, notice)}`);
    assert.match(read(notice), /SIL OPEN FONT LICENSE Version 1\.1/i, `${face.notice}: not the OFL`);
  }
});

test('installing the faces runs nothing and installs no telemetry', () => {
  // Only the two sections that are the installed tree: the lockfile also repeats the workspace's
  // overrides, where the removal itself names the package.
  const lock = read(repositoryRoot, 'pnpm-lock.yaml');
  for (const section of ['packages', 'snapshots']) {
    const body = lock.match(new RegExp(`^${section}:\\n([\\s\\S]*?)(?=^\\S|(?![\\s\\S]))`, 'm'))?.[1];
    assert.ok(body, `pnpm-lock.yaml has no ${section} section`);
    assert.doesNotMatch(body, /@ibm\/telemetry-js@/, `@ibm/telemetry-js is installed (${section})`);
  }
  const workspace = read(repositoryRoot, 'pnpm-workspace.yaml');
  assert.match(workspace, /^overrides:[\s\S]*^\s+'@ibm\/telemetry-js': '-'$/m, 'the override removing @ibm/telemetry-js is gone');
  assert.match(workspace, /^allowBuilds:\n(?:\s+.*\n)*?\s+'@ibm\/plex-sans-sc': false$/m, "@ibm/plex-sans-sc's install script is no longer refused");
});
