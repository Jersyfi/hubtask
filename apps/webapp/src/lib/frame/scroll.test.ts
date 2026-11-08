// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { ScrollMemory, landingOf, type Scroller } from './scroll.ts';

test('a new screen starts at the top; back and forward restore; a query alone or a redirect keeps the place', () => {
  assert.equal(landingOf({ path: '/hubs/a' }, { path: '/collections/b' }, 'push'), 'top');
  assert.equal(landingOf({ path: '/collections/b' }, { path: '/hubs/a' }, 'pop'), 'restore');
  assert.equal(landingOf({ path: '/collections/b' }, { path: '/collections/b' }, 'push'), 'keep');
  assert.equal(landingOf({ path: '/collections/b/plan' }, { path: '/collections/b/timeline' }, 'replace'), 'keep');
  assert.equal(landingOf(undefined, { path: '/' }, 'push'), 'keep');
});

/** A page `height` tall that grows by `grows` each frame, frames run by `tick`. */
function fakeScroller(height: number, grows = 0) {
  let y = 0;
  let reach = height;
  const queue: (() => void)[] = [];
  const scroller: Scroller & { tick(): void; set(y: number): void } = {
    get scrollY() {
      return y;
    },
    scrollTo: (_x, to) => {
      y = to;
    },
    reach: () => reach,
    frame: (run) => {
      queue.push(run);
      return () => queue.splice(queue.indexOf(run), 1);
    },
    tick: () => {
      reach += grows;
      queue.splice(0).forEach((run) => run());
    },
    set: (to) => {
      y = to;
    },
  };
  return scroller;
}

test('the memory keeps each entry\'s position and puts it back on a return', () => {
  const scroller = fakeScroller(2000);
  const memory = new ScrollMemory(scroller);
  scroller.set(840);
  memory.left('e1');
  memory.arrived('e2', 'top');
  assert.equal(scroller.scrollY, 0);
  memory.left('e2');
  memory.arrived('e1', 'restore');
  assert.equal(scroller.scrollY, 840);
});

test('a restore waits for the screen to grow, and the reader\'s own scroll ends the wait', () => {
  const scroller = fakeScroller(100, 300);
  const memory = new ScrollMemory(scroller);
  scroller.set(700);
  memory.left('e1');
  memory.arrived('e2', 'top');
  memory.left('e2');
  memory.arrived('e1', 'restore');
  assert.equal(scroller.scrollY, 100, 'as far as the short page reaches');
  scroller.tick();
  scroller.tick();
  assert.equal(scroller.scrollY, 700, 'there once the page holds it');

  const short = fakeScroller(0, 10);
  const other = new ScrollMemory(short);
  short.set(500);
  other.left('a');
  other.arrived('a', 'restore');
  other.interrupted();
  short.set(20);
  short.tick();
  assert.equal(short.scrollY, 20, 'no restore after the reader scrolled');
});
