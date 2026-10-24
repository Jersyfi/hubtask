// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Measures WCAG 2.2 contrast for every pair tokens.json declares, in both modes.
//
// Contrast is measured in CI rather than in a one-off check (design-system.md §1). A one-off check
// is true on the day it is run; this one is true on the day somebody changes a neutral step.
//
// Two properties make it worth trusting. It reads colours from tokens/tokens.json and from nowhere
// else, so it cannot drift away from the source it is checking (ADR-0029, rule 15). And
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
  'bg.raised': 'surface',
  'bg.surface-sunken': 'surface',
  'bg.surface-hover': 'surface',
  'bg.surface-pressed': 'surface',
  'bg.glass': 'surface',
  'bg.emphasis': 'fill',
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

  // Borders. `default` and `strong` draw controls - `Input` and a secondary `Button` take the
  // first, `Checkbox` the second - and a control's boundary is what says it is a control, so
  // SC 1.4.11 applies to both.
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

  // The AI treatment: a proposal has to be legible *and* visibly not the reader's own
  // content, so its surfaces are measured under every text like any other tinted surface, and
  // its own text, border and fill are measured against every surface like any other - both
  // directions, one table. The surfaces sit at the neutrals' luminance on purpose - they differ
  // by hue, and rule 3 says a hue never stands alone - so the boundary a proposal is told apart
  // by is `ai.border`, held to SC 1.4.11 on every surface including its own.
  'ai.surface': 'tinted-surface',
  'ai.surface-strong': 'tinted-surface',
  'ai.border': 'indicator',
  'ai.text': 'body-text',
  'ai.accent': 'fill',

  // A status as a surface (ADR-0061): the four roles `ai.*` established, five times. Each
  // surface is measured under every text like any tinted surface, and each text, border and
  // accent against every surface - so `text.warning` is measured on `status.info.surface` as
  // well as on its own, which is the pair a banner with a warning badge in it draws.
  ...Object.fromEntries(
    ['info', 'success', 'warning', 'danger', 'neutral'].flatMap((tone) => [
      [`status.${tone}.surface`, 'tinted-surface'],
      [`status.${tone}.border`, 'indicator'],
      [`status.${tone}.text`, 'body-text'],
      [`status.${tone}.accent`, 'fill'],
    ]),
  ),

  // Laid over the canvas as gradient stops, so they change what body text is read on rather than
  // being read themselves. Measured as a canvas variant, not as a pair of their own.
  'ambient.primary': 'canvas-tint',
  'ambient.signature': 'canvas-tint',

  // The mark (design-system.md §9). Its outline carries the mark's shape, so it is held to the
  // floor of a graphic object against everything the mark may sit on (1.4.11); the inner parts
  // are a logotype's, which 1.4.11 does not cover, and the outline already makes the mark found.
  'brand.outline': 'indicator',
  'brand.plane': ['decorative', 'a part of the logotype inside the outline, which already carries the mark at the graphic floor'],
  'brand.core': ['decorative', 'a part of the logotype inside the plane; a logotype is exempt from 1.4.11'],
  'brand.spark': ['decorative', 'a part of the logotype beside the outline; a logotype is exempt from 1.4.11'],
  'brand.tile': ['decorative', 'the square of the installed app icon, drawn on a launcher, never on a product surface'],
  'brand.tile-outline': ['decorative', 'the outline drawn on the app icon square, never on a product surface'],

  'rim.highlight': ['decorative', 'a one-pixel highlight along a raised edge; it carries no information'],
  'rim.primary': ['decorative', 'as rim.highlight, tinted with the primary hue'],
  'rim.signature': ['decorative', 'as rim.highlight, tinted with the signature hue'],
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
 * The opaque planes anything translucent can be laid on: the page, a card, what stands above a
 * card, and a recess inside one. A translucent surface is measured once over each of them, because
 * a glass panel over a raised menu is a different colour from the same panel over the page - and
 * measuring a colour with an alpha channel as if it were opaque reports a contrast nobody sees.
 */
const BASES = ['bg.canvas', 'bg.surface', 'bg.raised', 'bg.surface-sunken'];

const isTranslucent = (colour) => parse(colour)[3] < 1;

/**
 * surfaces returns everything a foreground can be measured against, each flattened to an opaque
 * colour: an opaque surface as it is, a translucent one over every base, and the ambient tints over
 * the canvas they tint.
 */
function surfaces(mode) {
  const tokens = new Map(colourTokens(mode));
  const out = [];
  for (const [dotted, value] of tokens) {
    const role = roleOf(dotted);
    if (role === 'surface' || role === 'tinted-surface') {
      if (!isTranslucent(value)) out.push([dotted, parse(value)]);
      else for (const base of BASES) out.push([`${dotted} over ${base}`, over(parse(value), parse(tokens.get(base)))]);
    }
    // The ambient gradients tint the canvas itself, so the canvas is really three surfaces.
    if (role === 'canvas-tint') out.push([`bg.canvas under ${dotted}`, over(parse(value), parse(tokens.get('bg.canvas')))]);
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

// ---------------------------------------------------------------------------------------------
// The steps between surfaces (design-system.md §6 rule 1, UC-WRK-33 check 2).
//
// WCAG has no floor for one surface against another, so these floors are the product's own. Each
// is compared unrounded, and each carries beside it what the tokens measure today, so a change that
// eats into the margin shows in the diff of this file rather than on a screen.

/** A card on the page: 1.18:1 in both modes (design-system.md §6 rule 1). Measured: light 1.1818, dark 1.1823. */
const CANVAS_TO_SURFACE = 1.18;
/** What stands above a card, in dark, where a shadow does not read (design-system.md §6 rule 1). Measured: 1.3771. */
const CANVAS_TO_RAISED_DARK = 1.36;
/**
 * A hover, a press, a recess and a neutral status surface each stay visible on the planes they are
 * drawn on. The floor is the weakest hover the product had that people could see (the dark
 * surface to its hover before this palette, 1.080:1).
 */
const VISIBLE_STEP = 1.08; // weakest measured: light 1.0842 (hover on the page), dark 1.0962 (press after the hover)

const opaque = (mode) => {
  const tokens = new Map(colourTokens(mode));
  return (dotted) => {
    const value = tokens.get(dotted);
    assert.ok(value, `semantic.${mode}.${dotted} is missing`);
    return over(parse(value), parse(tokens.get('bg.canvas')));
  };
};

/** Each step: [what, from, to, floor, modes]. */
const STEPS = [
  ['a card on the page', 'bg.canvas', 'bg.surface', CANVAS_TO_SURFACE, MODES],
  ['what floats above a card, on the page', 'bg.canvas', 'bg.raised', CANVAS_TO_RAISED_DARK, ['dark']],
  ['what floats above a card, on the page', 'bg.canvas', 'bg.raised', CANVAS_TO_SURFACE, ['light']],
  ['a hover on a card', 'bg.surface', 'bg.surface-hover', VISIBLE_STEP, MODES],
  ['a hover on what floats', 'bg.raised', 'bg.surface-hover', VISIBLE_STEP, MODES],
  ['a hover on the page', 'bg.canvas', 'bg.surface-hover', VISIBLE_STEP, MODES],
  ['a press after the hover', 'bg.surface-hover', 'bg.surface-pressed', VISIBLE_STEP, MODES],
  ['a press on the page', 'bg.canvas', 'bg.surface-pressed', VISIBLE_STEP, MODES],
  ['a recess in a card', 'bg.surface', 'bg.surface-sunken', VISIBLE_STEP, MODES],
  ['a recess in what floats', 'bg.raised', 'bg.surface-sunken', VISIBLE_STEP, MODES],
  ['a neutral status on a card', 'bg.surface', 'status.neutral.surface', VISIBLE_STEP, MODES],
  ['a neutral status on what floats', 'bg.raised', 'status.neutral.surface', VISIBLE_STEP, MODES],
];

for (const mode of MODES) {
  test(`${mode}: every surface step clears its floor`, () => {
    const colour = opaque(mode);
    const failures = [];
    for (const [what, from, to, floor, modes] of STEPS) {
      if (!modes.includes(mode)) continue;
      const value = ratio(colour(from), colour(to));
      console.log(`  ${value.toFixed(4).padStart(7)}:1  >= ${floor.toFixed(4)}  ${value >= floor ? 'pass' : 'FAIL'}  ${what}: ${to} on ${from}`);
      if (value < floor) failures.push(`${what}: ${to} on ${from} is ${value.toFixed(4)}:1, needs ${floor}:1`);
    }
    assert.deepEqual(failures, []);
  });

  test(`${mode}: raised is never below the surface it stands on`, () => {
    const colour = opaque(mode);
    // In light, white is the ceiling and raised equals the surface; the shadow carries the depth.
    // In dark, raised is a step lighter. Either way it is not a step back toward the page.
    assert.ok(
      ratio(colour('bg.canvas'), colour('bg.raised')) >= ratio(colour('bg.canvas'), colour('bg.surface')),
      'bg.raised is closer to the canvas than bg.surface',
    );
  });
}

// ---------------------------------------------------------------------------------------------
// Colour as colour: labels and the two reds. Measured in OKLab, where a distance means about the
// same to the eye whatever the hue - which relative luminance, the measure above, does not.

/** oklab converts an opaque sRGB colour to OKLab [L, a, b]. The matrices are the specification's. */
function oklab([r, g, b]) {
  const [lr, lg, lb] = [r, g, b].map(channel);
  const l = Math.cbrt(0.4122214708 * lr + 0.5363325363 * lg + 0.0514459929 * lb);
  const m = Math.cbrt(0.2119034982 * lr + 0.6806995451 * lg + 0.1073969566 * lb);
  const s = Math.cbrt(0.0883024619 * lr + 0.2817188376 * lg + 0.6299787005 * lb);
  return [
    0.2104542553 * l + 0.793617785 * m - 0.0040720468 * s,
    1.9779984951 * l - 2.428592205 * m + 0.4505937099 * s,
    0.0259040371 * l + 0.7827717662 * m - 0.808675766 * s,
  ];
}
const distance = (x, y) => {
  const [p, q] = [oklab(x), oklab(y)];
  return Math.hypot(p[0] - q[0], p[1] - q[1], p[2] - q[2]);
};
const hueOf = (c) => {
  const [, a, b] = oklab(c);
  return ((Math.atan2(b, a) * 180) / Math.PI + 360) % 360;
};
const chromaOf = (c) => Math.hypot(oklab(c)[1], oklab(c)[2]);

/**
 * A label's background has to read as its colour on a card and on what floats above one, not as a
 * slightly different grey: its OKLab distance from both is held to a floor. Measured, weakest
 * label: light 0.0944 (blue), dark 0.0804 (blue on bg.raised). The labels before this palette
 * measured 0.0621 in light and 0.0190 in dark - the grey this floor exists to refuse.
 */
const LABEL_FROM_SURFACE = { light: 0.09, dark: 0.08 };
/** The one neutral label: it is meant to read as no colour, so the distance rule does not apply. */
const NEUTRAL_LABEL = ['slate', 'the one label meant to read as no colour; its pair is still measured above'];

for (const mode of MODES) {
  test(`${mode}: every label reads as its colour on a card and above one`, () => {
    const colour = opaque(mode);
    const labels = colourTokens(mode).filter(([d]) => d.startsWith('label.') && d.endsWith('.bg'));
    assert.equal(labels.length, 10, 'the ten label tokens');
    const failures = [];
    for (const [dotted] of labels) {
      if (dotted.split('.')[1] === NEUTRAL_LABEL[0]) continue;
      for (const base of ['bg.surface', 'bg.raised']) {
        const value = distance(colour(dotted), colour(base));
        const floor = LABEL_FROM_SURFACE[mode];
        console.log(`  ${value.toFixed(4)}  >= ${floor}  ${value >= floor ? 'pass' : 'FAIL'}  ${dotted} from ${base}`);
        if (value < floor) failures.push(`${dotted} is ${value.toFixed(4)} from ${base}`);
      }
      // A dark label darker than what it sits on reads as a hole in the card, not as a colour.
      if (mode === 'dark' && oklab(colour(dotted))[0] < oklab(colour('bg.raised'))[0]) {
        failures.push(`${dotted} is darker than bg.raised and reads as a hole`);
      }
    }
    assert.deepEqual(failures, []);
  });
}

/**
 * Danger and the signature are both reds. A status is never told by hue alone (rule 3), but the
 * brand must not read as an alarm either, so the two accents keep a distance in OKLCH hue.
 */
const DANGER_FROM_SIGNATURE_HUE = 17.5; // measured: light 17.56, dark 18.97 degrees

for (const mode of MODES) {
  test(`${mode}: danger and the signature are told apart by hue`, () => {
    const colour = opaque(mode);
    let d = Math.abs(hueOf(colour('status.danger.accent')) - hueOf(colour('accent.signature'))) % 360;
    if (d > 180) d = 360 - d;
    console.log(`  ${d.toFixed(2)} degrees >= ${DANGER_FROM_SIGNATURE_HUE}`);
    assert.ok(d >= DANGER_FROM_SIGNATURE_HUE, `status.danger.accent is ${d.toFixed(2)} degrees from accent.signature`);
    assert.ok(chromaOf(colour('status.danger.accent')) > 0.1, 'danger is a vivid red, not a muted one');
  });
}
