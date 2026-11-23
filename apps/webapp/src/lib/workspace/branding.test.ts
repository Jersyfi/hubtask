// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// UC-INS-22 checks 2 and 4 on the client's side: a file refused before it leaves, the logo alone
// offered only with a picture, what each surface draws, and the contrast line's verdict.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { values } from '@hubtask/design-system';

import {
  cardLookOf, cardSourcesOf, contrastOf, drawingFor, initialsOf, isSameDrawing, markDrawings, mayShowLogoAlone, pictureOf, refusalOf, shownLookOf,
} from './branding.ts';

const LIGHT = values.light.bg.surface;
const DARK = values.dark.bg.surface;

/** A picture of `count` pixels, each the same RGBA. */
const paint = (rgba: readonly number[], count = 4): number[] => Array.from({ length: count }, () => [...rgba]).flat();

test('a dark mark on the dark surface is hard to read; a light one reads', () => {
  const navy = paint([20, 50, 74, 255]);
  const white = paint([242, 245, 250, 255]);
  assert.equal(contrastOf(navy, DARK), 'hard');
  assert.equal(contrastOf(white, DARK), 'reads');
  assert.equal(contrastOf(navy, LIGHT), 'reads');
  assert.equal(contrastOf(white, LIGHT), 'hard');
});

test('a transparent picture is judged on what it paints', () => {
  // Mostly transparent, with a few white pixels: on the light surface the white is what is seen.
  const mostlyClear = [...paint([0, 0, 0, 0], 60), ...paint([255, 255, 255, 255], 4)];
  assert.equal(contrastOf(mostlyClear, LIGHT), 'hard', 'the transparent black counted');
  assert.equal(contrastOf(mostlyClear, DARK), 'reads');
  // Nothing painted at all: no verdict, rather than a guess.
  assert.equal(contrastOf(paint([0, 0, 0, 0]), LIGHT), undefined);
  // A surface that is not a colour value: no verdict.
  assert.equal(contrastOf(paint([0, 0, 0, 255]), 'var(--bg-surface)'), undefined);
});

test('the logo alone may be chosen with a logo or the symbol, never with nothing', () => {
  assert.equal(mayShowLogoAlone({}), false);
  assert.equal(mayShowLogoAlone({ LOGO_LIGHT: 'a' }), true);
  assert.equal(mayShowLogoAlone({ LOGO_DARK: 'a' }), true);
  assert.equal(mayShowLogoAlone({ SYMBOL: 'a' }), true);
});

test('a file the browser names as another type, or past the limit, is refused before it leaves', () => {
  const limit = 5_000_000;
  assert.equal(refusalOf({ type: 'image/svg+xml', size: 100 }, limit), 'type');
  assert.equal(refusalOf({ type: 'application/pdf', size: 100 }, limit), 'type');
  assert.equal(refusalOf({ type: 'image/png', size: limit + 1 }, limit), 'size');
  assert.equal(refusalOf({ type: 'image/webp', size: limit }, limit), undefined);
  // A type the browser cannot tell goes to the server, which judges the bytes.
  assert.equal(refusalOf({ type: '', size: 100 }, limit), undefined);
});

test('each surface draws its own logo, then the other, then the symbol, then the product', () => {
  const all = { LOGO_LIGHT: 'l', LOGO_DARK: 'd', SYMBOL: 's' };
  assert.deepEqual(drawingFor('LOGO', all, 'light'), { kind: 'logo', picture: 'LOGO_LIGHT' });
  assert.deepEqual(drawingFor('LOGO', all, 'dark'), { kind: 'logo', picture: 'LOGO_DARK' });
  assert.deepEqual(drawingFor('LOGO', { LOGO_LIGHT: 'l' }, 'dark'), { kind: 'logo', picture: 'LOGO_LIGHT' });
  assert.deepEqual(drawingFor('LOGO', { SYMBOL: 's' }, 'light'), { kind: 'symbol', picture: 'SYMBOL' });
  assert.deepEqual(drawingFor('LOGO', {}, 'light'), { kind: 'product' });
  assert.deepEqual(drawingFor('WORKSPACE_NAME', all, 'dark'), { kind: 'name', symbol: 'SYMBOL' });
  assert.deepEqual(drawingFor('WORKSPACE_NAME', { LOGO_LIGHT: 'l' }, 'dark'), { kind: 'name' });
  assert.deepEqual(drawingFor('PRODUCT', all, 'light'), { kind: 'product' });
});

test('the stored pictures are read by their kind', () => {
  const branding = { header: 'PRODUCT' as const, logo_light_media_id: 'l', logo_dark_media_id: null, symbol_media_id: 's', allowed: true };
  assert.equal(pictureOf(branding, 'LOGO_LIGHT'), 'l');
  assert.equal(pictureOf(branding, 'LOGO_DARK'), undefined);
  assert.equal(pictureOf(branding, 'SYMBOL'), 's');
  assert.equal(pictureOf(undefined, 'SYMBOL'), undefined);
});

test('two letters stand for a workspace without a symbol', () => {
  assert.equal(initialsOf('Nordlicht Werkstatt'), 'NW');
  assert.equal(initialsOf('  kita  '), 'K');
  assert.equal(initialsOf('Freiwillige Feuerwehr Unterlindenbach e. V.'), 'FV');
});

// UC-INS-22 checks 5, 6 and 8: what the header, the switcher and the sign-in card draw.
test('a member sees the stored choice only while the workspace may show it', () => {
  const stored = { header: 'LOGO' as const, logo_light_media_id: 'l', logo_dark_media_id: null, symbol_media_id: 's', allowed: true };
  assert.deepEqual(shownLookOf(stored), { header: 'LOGO', ids: { LOGO_LIGHT: 'l', SYMBOL: 's' } });
  // Switched off above: the product, whatever is stored (ADR-0112) - not even the symbol.
  assert.deepEqual(shownLookOf({ ...stored, allowed: false }), { header: 'PRODUCT', ids: {} });
  assert.deepEqual(shownLookOf(undefined), { header: 'PRODUCT', ids: {} });
});

test('the card draws what the rules at this address carry, and the product where they carry none', () => {
  assert.deepEqual(cardLookOf('WORKSPACE_NAME', { symbol_media_id: 's' }), { header: 'WORKSPACE_NAME', ids: { SYMBOL: 's' } });
  assert.deepEqual(cardLookOf('LOGO', { logo_dark_media_id: 'd' }), { header: 'LOGO', ids: { LOGO_DARK: 'd' } });
  assert.deepEqual(cardLookOf(undefined, { symbol_media_id: 's' }), { header: 'PRODUCT', ids: {} });
  // Each drawn from the public read at the API this page talks to, and nothing else.
  assert.deepEqual(cardSourcesOf({ SYMBOL: 's-1' }, '/api/v1'), { SYMBOL: '/api/v1/auth/workspace-pictures/s-1' });
  assert.deepEqual(cardSourcesOf({}, '/api/v1'), {});
});

test('a mark keeps the box of a picture still asked for, and falls back past one that failed', () => {
  // Asked for: the logo's box on both surfaces, never the product drawn first and swapped.
  assert.deepEqual(markDrawings('LOGO', { LOGO_LIGHT: null }, new Set()), {
    light: { kind: 'logo', picture: 'LOGO_LIGHT' },
    dark: { kind: 'logo', picture: 'LOGO_LIGHT' },
  });
  const both = { LOGO_LIGHT: 'l.png', LOGO_DARK: 'd.png', SYMBOL: 's.png' };
  assert.deepEqual(markDrawings('LOGO', both, new Set()).dark, { kind: 'logo', picture: 'LOGO_DARK' });
  // The dark logo failed to load: the light one, then the symbol, then the product (G8).
  assert.deepEqual(markDrawings('LOGO', both, new Set(['LOGO_DARK'] as const)).dark, { kind: 'logo', picture: 'LOGO_LIGHT' });
  assert.deepEqual(markDrawings('LOGO', both, new Set(['LOGO_DARK', 'LOGO_LIGHT'] as const)).dark, { kind: 'symbol', picture: 'SYMBOL' });
  assert.deepEqual(markDrawings('LOGO', both, new Set(['LOGO_DARK', 'LOGO_LIGHT', 'SYMBOL'] as const)).light, { kind: 'product' });
  // The name keeps its two letters when the symbol fails.
  assert.deepEqual(markDrawings('WORKSPACE_NAME', { SYMBOL: 's.png' }, new Set(['SYMBOL'] as const)).light, { kind: 'name' });
  assert.deepEqual(markDrawings('PRODUCT', both, new Set()).light, { kind: 'product' });
});

test('a mark alike on both surfaces is drawn once', () => {
  const drawn = markDrawings('WORKSPACE_NAME', { SYMBOL: 's' }, new Set());
  assert.equal(isSameDrawing(drawn.light, drawn.dark), true);
  const logos = markDrawings('LOGO', { LOGO_LIGHT: 'l', LOGO_DARK: 'd' }, new Set());
  assert.equal(isSameDrawing(logos.light, logos.dark), false);
});
