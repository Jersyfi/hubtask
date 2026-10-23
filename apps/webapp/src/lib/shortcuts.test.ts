// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import { SHORTCUTS, ariaKeysOf, drawnKeysOf, intentOf, type Press, type Where } from './shortcuts.ts';

const press = (key: string, extra: Partial<Press> = {}): Press => ({ key, metaKey: false, ctrlKey: false, altKey: false, shiftKey: false, ...extra });
const altGraph = (key: string, extra: Partial<Press> = {}): Press =>
  press(key, { ctrlKey: true, altKey: true, getModifierState: (name) => name === 'AltGraph', ...extra });

const PAGE: Where = { isTextField: false, isInWidget: false, isLayerOpen: false, isPaletteOpen: false };
const FIELD: Where = { ...PAGE, isTextField: true };

/** One row per case: what was pressed, where, and what it means - `null` for "not ours". */
const TABLE: readonly [string, Press, Where, ReturnType<typeof intentOf>][] = [
  ['⌘K on the page', press('k', { metaKey: true }), PAGE, 'palette'],
  ['Ctrl K on the page', press('k', { ctrlKey: true }), PAGE, 'palette'],
  ['⌘K with Caps Lock', press('K', { metaKey: true }), PAGE, 'palette'],
  ['⌘K in a text field', press('k', { metaKey: true }), FIELD, 'palette'],
  ['Ctrl K in a text field is the field\'s', press('k', { ctrlKey: true }), FIELD, null],
  ['⌘K with the palette open closes it', press('k', { metaKey: true }), { ...FIELD, isPaletteOpen: true }, 'palette'],
  ['Ctrl K with the palette in its dialog, focus outside its field', press('k', { ctrlKey: true }), { ...PAGE, isLayerOpen: true, isPaletteOpen: true }, 'palette'],
  ['⌘K while a dialog is on top', press('k', { metaKey: true }), { ...PAGE, isLayerOpen: true }, null],
  ['⌘K inside a menu', press('k', { metaKey: true }), { ...PAGE, isInWidget: true }, null],
  ['Ctrl ⌘K is somebody else\'s', press('k', { metaKey: true, ctrlKey: true }), PAGE, null],
  ['Ctrl Shift K is somebody else\'s', press('k', { ctrlKey: true, shiftKey: true }), PAGE, null],
  ['Alt ⌘K is somebody else\'s', press('k', { metaKey: true, altKey: true }), PAGE, null],
  ['Russian layout: ⌘ on the K key', press('л', { code: 'KeyK', metaKey: true }), PAGE, 'palette'],
  ['Arabic layout: Ctrl on the K key', press('ن', { code: 'KeyK', ctrlKey: true }), PAGE, 'palette'],
  ['a Latin letter on the K key is that letter', press('j', { code: 'KeyK', metaKey: true }), PAGE, null],
  ['/ on the page', press('/'), PAGE, 'palette'],
  ['German layout: / is Shift 7', press('/', { shiftKey: true, code: 'Digit7' }), PAGE, 'palette'],
  ['/ in a text field types itself', press('/'), FIELD, null],
  ['/ in a listbox is the listbox\'s', press('/'), { ...PAGE, isInWidget: true }, null],
  ['/ while a dialog is open', press('/'), { ...PAGE, isLayerOpen: true }, null],
  ['/ with Ctrl is not a slash', press('/', { ctrlKey: true }), PAGE, null],
  ['? on the page', press('?', { shiftKey: true }), PAGE, 'help'],
  ['Arabic question mark', press('؟', { shiftKey: true }), PAGE, 'help'],
  ['AltGr is no modifier: German ? stays ?', altGraph('?'), PAGE, 'help'],
  ['AltGr is no modifier: a / reached with it', altGraph('/'), PAGE, 'palette'],
  ['? in a text field types itself', press('?', { shiftKey: true }), FIELD, null],
  ['? with the palette open', press('?', { shiftKey: true }), { ...PAGE, isPaletteOpen: true }, null],
  ['Q on the page means capture', press('q'), PAGE, 'capture'],
  ['Q on a Russian layout', press('й', { code: 'KeyQ' }), PAGE, 'capture'],
  ['Q in a text field types itself', press('q'), FIELD, null],
  ['a key already handled is not ours', press('k', { metaKey: true, defaultPrevented: true }), PAGE, null],
  ['a key while composing text is not ours', press('k', { metaKey: true, isComposing: true }), PAGE, null],
  ['any other key', press('x'), PAGE, null],
];

for (const [name, pressed, where, expected] of TABLE) {
  test(`intent: ${name}`, () => {
    assert.equal(intentOf(pressed, where), expected);
  });
}

test('every shortcut is listed with keys to draw and a description, and its ids are unique', () => {
  const ids = SHORTCUTS.map((each) => each.id);
  assert.equal(new Set(ids).size, ids.length);
  for (const each of SHORTCUTS) {
    assert.ok(each.drawn.length > 0, each.id);
    assert.match(each.code, /^app\.shortcuts\./, each.id);
  }
});

test('both ways to open the palette are drawn and declared', () => {
  assert.deepEqual(drawnKeysOf('palette'), ['⌘K', 'Ctrl K', '/']);
  assert.deepEqual(ariaKeysOf('palette'), ['Meta+K', 'Control+K', '/']);
  assert.deepEqual(drawnKeysOf('nothing'), []);
});

test('capture is listed, on the key that opens it, everywhere (UC-WRK-26 check 1)', () => {
  const capture = SHORTCUTS.find((each) => each.id === 'capture');
  assert.equal(capture?.scope, 'everywhere');
  assert.deepEqual(drawnKeysOf('capture'), ['Q']);
  assert.deepEqual(ariaKeysOf('capture'), ['Q']);
});
