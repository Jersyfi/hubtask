// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Measures WCAG 2.2 contrast for every pair tokens.json declares, in both modes.
//
// design-system.md §9 said the label tokens "are calculated for >= 4.5:1 but have not been
// measured", and that measuring them belongs in CI rather than in a one-off check. A one-off check
// is true on the day it is run; this one is true on the day somebody changes a neutral step.
//
// Two properties make it worth trusting. It reads colours from tokens/tokens.json and from nowhere
// else, so it cannot drift away from the source it is checking (ADR-0029, CLAUDE.md rule 15). And
// every semantic colour token has to carry a role in ROLES below - a token nobody has classified
// fails the suite instead of being skipped, so the check cannot quietly shrink as the token set
// grows.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const packageRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const source = JSON.parse(fs.readFileSync(path.join(packageRoot, 'tokens', 'tokens.json'), 'utf8'));

// The two floors of WCAG 2.2. SC 1.4.3 governs anything read as text; SC 1.4.11 governs the visual
// information that identifies a control, which is why a border and a focus ring are measured at
// all. Large text may sit at 3:1, and the day a token exists that is only ever set at >= 24px it
// gets the LARGE_TEXT role; until then every text pair is held to the body floor.
const BODY_TEXT = 4.5;
const NON_TEXT = 3;

/**
 * ROLES classifies every semantic colour token by what it is *for*, because that - not its name -
 * decides which pairs exist. Paths are relative to a mode, so one table serves light and dark.
 *
 * `decorative` carries a reason rather than a bare exemption. An exemption without a reason is how
 * a check shrinks: the next token to fail is the next token to be listed here.
 */
const ROLES = {
  // Backgrounds text is read on.
  'bg.canvas': 'surface',
  'bg.surface': 'surface',
  'bg.surface-sunken': 'surface',
  'bg.surface-hover': 'surface',
  'bg.surface-pressed': 'surface',
  'bg.glass': 'surface',
  'bg.scrim': ['decorative', 'a dimmer laid over content that is deliberately no longer readable'],

  // Text.
  'text.primary': 'body-text',
  'text.secondary': 'body-text',
  'text.subtle': 'body-text',
  'text.brand': 'body-text',
  'text.danger': 'body-text',
  'text.success': 'body-text',
  'text.warning': 'body-text',
  'text.inverse': 'inverse-text',

  // Borders. `default` and `strong` draw controls - foundations.html uses the first for an input
  // and a secondary button, the second for a checkbox - and a control's boundary is what says it
  // is a control, so SC 1.4.11 applies to both.
  'border.subtle': ['decorative', 'a hairline between sections and around cards, never the only thing identifying a control'],
  'border.default': 'indicator',
  'border.strong': 'indicator',
  'border.glass': ['decorative', 'the lit rim of a glass overlay; the overlay is identified by its shadow and its content'],

  // Filled controls, and the two tints text sits on.
  'accent.primary': 'fill',
  'accent.primary-hover': 'fill',
  'accent.primary-pressed': 'fill',
  'accent.signature': 'fill',
  'accent.primary-subtle': 'tinted-surface',
  'accent.signature-subtle': 'tinted-surface',

  'focus.ring': 'indicator',

  // Laid over the canvas as gradient stops, so they change what body text is read on rather than
  // being read themselves. Measured as a canvas variant, not as a pair of their own.
  'ambient.primary': 'canvas-tint',
  'ambient.signature': 'canvas-tint',

  'rim.highlight': ['decorative', 'a one-pixel highlight along a raised edge; it carries no information'],
  'rim.blue': ['decorative', 'as rim.highlight, tinted'],
  'rim.ember': ['decorative', 'as rim.highlight, tinted'],
};

// The ten label pairs are generated rather than listed: the point of the check is that it grows
// with tokens.json, and an eleventh label must not need an edit here to be measured.
const labelRole = (leaf) => (leaf === 'bg' ? 'label-bg' : leaf === 'fg' ? 'label-fg' : null);

/** at walks a dotted path into the token tree. */
const at = (dotted) => dotted.split('.').reduce((node, key) => node?.[key], source);

/** resolve follows `{a.b.c}` aliases to the literal the primitive layer holds. */
function resolve(value, depth = 0) {
  assert.ok(depth < 10, `alias chain does not terminate at ${value}`);
  if (typeof value !== 'string') return value;
  const alias = /^\{([^}]+)\}$/.exec(value.trim());
  if (!alias) return value;
  const target = at(alias[1]);
  assert.ok(target, `${value} points at a token that does not exist`);
  return resolve(target.$value, depth + 1);
}

/** parse turns the two notations tokens.json uses into straight-alpha RGB. */
function parse(colour) {
  const text = String(colour).trim();
  const long = /^#([0-9a-fA-F]{6})$/.exec(text);
  if (long) {
    const n = parseInt(long[1], 16);
    return [(n >> 16) & 255, (n >> 8) & 255, n & 255, 1];
  }
  const short = /^#([0-9a-fA-F]{3})$/.exec(text);
  if (short) return [...short[1]].map((c) => parseInt(c + c, 16)).concat(1);
  const functional = /^rgba?\(([^)]+)\)$/i.exec(text);
  assert.ok(functional, `a colour notation nothing here can measure: ${text}`);
  const parts = functional[1].split(',').map((p) => Number(p.trim()));
  assert.ok(parts.length >= 3 && parts.every(Number.isFinite), `malformed colour: ${text}`);
  return [parts[0], parts[1], parts[2], parts.length > 3 ? parts[3] : 1];
}

/** over composites a translucent colour onto an opaque one - what the eye is given, source-over. */
const over = ([r, g, b, a], base) => [
  r * a + base[0] * (1 - a),
  g * a + base[1] * (1 - a),
  b * a + base[2] * (1 - a),
  1,
];

// WCAG 2.2 relative luminance, sRGB. The numbers are the specification's, and are the one place in
// this package where a figure is written outside tokens.json - they define what "contrast" means
// rather than what the product looks like.
const channel = (v) => {
  const c = v / 255;
  return c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
};
const luminance = ([r, g, b]) => 0.2126 * channel(r) + 0.7152 * channel(g) + 0.0722 * channel(b);

function ratio(foreground, background) {
  const [lighter, darker] = [luminance(foreground), luminance(background)].sort((x, y) => y - x);
  return (lighter + 0.05) / (darker + 0.05);
}

/** colourTokens lists every `$type: color` leaf of one mode, as [dotted path, literal]. */
function colourTokens(mode) {
  const out = [];
  const walk = (node, prefix) => {
    for (const [key, value] of Object.entries(node)) {
      if (key.startsWith('$')) continue;
      if (value && typeof value === 'object' && '$value' in value) {
        if (value.$type === 'color') out.push([[...prefix, key].join('.'), resolve(value.$value)]);
      } else walk(value, [...prefix, key]);
    }
  };
  walk(source.semantic[mode], []);
  return out;
}

/** roleOf answers for every colour token or returns null, which is a failure and not a skip. */
function roleOf(dotted) {
  const segments = dotted.split('.');
  if (segments[0] === 'label' && segments.length === 3) return labelRole(segments[2]);
  const role = ROLES[dotted];
  if (!role) return null;
  return Array.isArray(role) ? role[0] : role;
}

const MODES = ['light', 'dark'];

test('every semantic colour token is classified', () => {
  for (const mode of MODES) {
    for (const [dotted] of colourTokens(mode)) {
      assert.ok(
        roleOf(dotted),
        `semantic.${mode}.${dotted} has no role in the contrast check. Give it one - a token ` +
          'nobody classified is a pair nobody measures.',
      );
    }
  }
});

test('a decorative exemption carries its reason', () => {
  for (const [dotted, role] of Object.entries(ROLES)) {
    if (!Array.isArray(role)) continue;
    assert.equal(role[0], 'decorative', `${dotted} is written as a pair but is not decorative`);
    assert.ok(role[1]?.length > 20, `${dotted} is exempt without saying why`);
  }
});

/**
 * surfaces returns everything a foreground can be measured against, each already flattened to an
 * opaque colour. Translucent tokens are composited over the canvas: it is the only thing known to
 * be behind them, and the alternative - measuring a colour with an alpha channel as if it were
 * opaque - reports a contrast nobody ever sees.
 */
function surfaces(mode) {
  const tokens = new Map(colourTokens(mode));
  const canvas = parse(tokens.get('bg.canvas'));
  const flatten = (dotted) => over(parse(tokens.get(dotted)), canvas);

  const out = [];
  for (const [dotted] of tokens) {
    const role = roleOf(dotted);
    if (role === 'surface' || role === 'tinted-surface') out.push([dotted, flatten(dotted)]);
    // The ambient gradients tint the canvas itself, so the canvas is really three surfaces.
    if (role === 'canvas-tint') out.push([`bg.canvas under ${dotted}`, flatten(dotted)]);
  }
  return out;
}

/** pairs enumerates what is measured, with the floor each pair answers to. */
function pairs(mode) {
  const tokens = new Map(colourTokens(mode));
  const canvas = parse(tokens.get('bg.canvas'));
  const flatten = (dotted) => over(parse(tokens.get(dotted)), canvas);
  const on = (dotted, background) => over(parse(tokens.get(dotted)), background);

  const out = [];
  const against = surfaces(mode);

  for (const [dotted] of tokens) {
    const role = roleOf(dotted);
    if (role === 'body-text' || role === 'indicator') {
      const floor = role === 'body-text' ? BODY_TEXT : NON_TEXT;
      for (const [name, background] of against) out.push([dotted, name, on(dotted, background), background, floor]);
    }
    if (role === 'fill') {
      // A filled control has to be findable against the page (1.4.11) and legible (1.4.3).
      for (const [name, background] of against) out.push([dotted, name, on(dotted, background), background, NON_TEXT]);
      const fill = flatten(dotted);
      out.push(['text.inverse', dotted, on('text.inverse', fill), fill, BODY_TEXT]);
    }
    if (role === 'label-fg') {
      const background = flatten(dotted.replace(/\.fg$/, '.bg'));
      out.push([dotted, dotted.replace(/\.fg$/, '.bg'), on(dotted, background), background, BODY_TEXT]);
    }
  }
  return out;
}

for (const mode of MODES) {
  test(`${mode}: every declared pair clears its WCAG 2.2 floor`, () => {
    const measured = pairs(mode);
    assert.ok(measured.length > 0, 'nothing was measured');

    const failures = [];
    for (const [foreground, background, fg, bg, floor] of measured) {
      const value = ratio(fg, bg);
      const verdict = value >= floor ? 'pass' : 'FAIL';
      // Every pair appears with its ratio, whether it passes or not: the record of the measurement
      // is the point, and a check that only speaks when it fails cannot be reviewed.
      console.log(
        `  ${value.toFixed(2).padStart(6)}:1  >= ${floor.toFixed(1)}  ${verdict}  ${foreground} on ${background}`,
      );
      if (value < floor) failures.push(`${foreground} on ${background}: ${value.toFixed(2)}:1, needs ${floor}:1`);
    }

    assert.deepEqual(failures, [], `${failures.length} of ${measured.length} pairs are below their floor in ${mode}`);
  });
}
