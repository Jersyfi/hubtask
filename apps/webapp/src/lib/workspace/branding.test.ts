// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// UC-INS-22 checks 2 and 4 on the client's side: a file refused before it leaves, the logo alone
// offered only with a picture, what each surface draws, and the contrast line's verdict.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { values } from '@hubtask/design-system';

import { contrastOf, drawingFor, initialsOf, mayShowLogoAlone, pictureOf, refusalOf } from './branding.ts';

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
