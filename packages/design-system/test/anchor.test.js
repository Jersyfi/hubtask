// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// ADR-0039's positioner, tested where it can be without a layout engine: the table that turns a
// placement into `position-area`, and the top layer. Whether an engine then draws the overlay
// where the table says is the `engines` job's question (ADR-0048), asked of each engine in CI.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { positionArea, raiseToTopLayer } from '../src/anchor.ts';

test('a placement reads as the position-area the engine expects', () => {
  // `span-inline-end` runs from the anchor's inline start towards its end, which is what
  // `align: 'start'` means here. Getting this table backwards is a bug that only shows up as
  // "the menu is left-aligned in Chrome and right-aligned in Firefox".
  assert.equal(positionArea({ side: 'block-end', align: 'start' }), 'block-end span-inline-end');
  assert.equal(positionArea({ side: 'block-end', align: 'end' }), 'block-end span-inline-start');
  assert.equal(positionArea({ side: 'block-end', align: 'center' }), 'block-end');
  assert.equal(positionArea({ side: 'block-start', align: 'center' }), 'block-start');
  assert.equal(positionArea({ side: 'inline-end', align: 'start' }), 'inline-end span-block-end');
  assert.equal(positionArea({ side: 'inline-start', align: 'end' }), 'inline-start span-block-start');
});

// ---------------------------------------------------------------------------------------------
// The top layer. No DOM here: the function touches four methods of the element it is given, so a
// test can give it four methods.
// ---------------------------------------------------------------------------------------------

function fakeOverlay({ showPopover }) {
  const element = {
    attributes: {},
    shown: 0,
    hidden: 0,
    setAttribute(name, value) {
      this.attributes[name] = value;
    },
    removeAttribute(name) {
      delete this.attributes[name];
    },
    showPopover() {
      showPopover?.();
      this.shown += 1;
    },
    hidePopover() {
      this.hidden += 1;
    },
  };
  return element;
}

test('an overlay is raised into the top layer and lowered again', () => {
  const overlay = fakeOverlay({});
  const lower = raiseToTopLayer(overlay);
  assert.equal(overlay.attributes.popover, 'manual', 'manual, so light dismiss cannot take Escape');
  assert.equal(overlay.shown, 1);

  lower();
  assert.equal(overlay.hidden, 1);
  assert.equal(overlay.attributes.popover, undefined, 'the attribute goes off with it');
});

test('an overlay that cannot be raised is left visible rather than deleted', () => {
  // The branch that matters most: a `[popover]` which was never shown is `display: none`. Leaving
  // the attribute on after a failed `showPopover` would not misplace the overlay, it would remove
  // it from the screen entirely.
  const overlay = fakeOverlay({
    showPopover() {
      throw new Error('not connected');
    },
  });
  const lower = raiseToTopLayer(overlay);
  assert.equal(overlay.attributes.popover, undefined, 'the overlay is still rendered');
  lower();
  assert.equal(overlay.hidden, 0, 'and nothing is lowered that was never raised');
});

test('a browser with no top layer is left alone', () => {
  const overlay = { setAttribute: () => assert.fail('nothing may be set'), attributes: {} };
  const lower = raiseToTopLayer(overlay);
  assert.equal(typeof lower, 'function');
  lower();
});
