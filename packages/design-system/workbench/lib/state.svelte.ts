// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The workbench's state, and its address.
//
// Every axis lives in the query string, which is the point rather than a nicety: a review comment
// that says "this clips" is worth little, and a link to `?story=…&theme=dark&dir=rtl&text=long`
// is worth a screenshot. It is also what lets a pull request claim a component was seen in a
// state, in a form the reader can check.

import { AXIS_IDS, coerce, defaults, type AxisId, type AxisState } from './axes.ts';

const STORY_PARAM = 'story';

function read(): { story: string | null; axes: AxisState } {
  const query = new URLSearchParams(window.location.search);
  const axes = defaults();
  for (const id of AXIS_IDS) axes[id] = coerce(id, query.get(id));
  return { story: query.get(STORY_PARAM), axes };
}

const initial = read();

class WorkbenchState {
  story = $state<string | null>(initial.story);
  axes = $state<AxisState>(initial.axes);

  set(id: AxisId, value: string) {
    this.axes = { ...this.axes, [id]: coerce(id, value) };
    this.#write();
  }

  select(story: string) {
    this.story = story;
    this.#write();
  }

  /** Back and forward have to work, or the address is decoration rather than state. */
  adopt() {
    const next = read();
    this.story = next.story;
    this.axes = next.axes;
  }

  #write() {
    const query = new URLSearchParams();
    if (this.story) query.set(STORY_PARAM, this.story);
    // Only what differs from the default, so a shared link says what it means instead of
    // repeating the neutral state six times.
    const fallbacks = defaults();
    for (const id of AXIS_IDS) if (this.axes[id] !== fallbacks[id]) query.set(id, this.axes[id]);
    window.history.replaceState(null, '', `${window.location.pathname}?${query}`);
  }
}

export const workbench = new WorkbenchState();
