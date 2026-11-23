// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import DialogDemo from './_DialogDemo.svelte';

export default {
  title: 'Wave 1 · Overlays/Dialog',
  component: DialogDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'motion', 'zoom'],
} satisfies StoryMeta;

export const confirm: Story = {
  name: 'A question that has to be answered',
  about:
    'Open it from the keyboard and Tab through it: focus is inside and stays there, everything behind it is inert, and closing it — by Escape, by the close button, or by an answer — puts focus back on the button that opened it. It is a native `<dialog>` in the top layer, so no stacking context can put anything over it.',
};

export const alert: Story = {
  name: 'An alert: a consequence that interrupts',
  about:
    '`isAlert` (APG Alert Dialog): announced as `alertdialog`, its body the description, and the focus starts on the safe answer placed first in the foot — *Keep it on* — rather than in the field below or on the destructive primary. On a phone’s full screen, where the foot goes, it starts on ✕. A click beside it does not close it; ✕ and Escape do.',
  args: { mode: 'alert' },
};

export const layers: Story = {
  name: 'With a popover inside it',
  about:
    'Escape closes one layer at a time (design-system.md §4, Wave 1): two key presses. The first Escape closes the popover and leaves the dialog standing; the second closes the dialog. The platform would have closed the dialog first, which is why `cancel` is refused and the register in layers.ts decides instead.',
  args: { mode: 'layers' },
};

export const wide: Story = {
  name: 'Wide: a dialog worked in',
  about:
    'Capture is worked in rather than answered: its chips, its field and its modes each want a line of their own, so it takes the wide measure. It still never runs past the viewport.',
  args: { mode: 'wide' },
};

export const hang: Story = {
  name: 'Hung from its button',
  about:
    'A panel worked in from the bar — *Create* — hangs under the button that opened it, its end on the button’s end, instead of standing in the middle. The button names it, so it draws no head; focus, the trap, Escape and the backdrop are the dialog’s.',
  args: { mode: 'hang' },
};

export const sizeS: Story = {
  name: 'Size S — a confirmation',
  about: 'Dialog S (`layout.dialog.s`): a confirmation or one field. Centred on every width, a phone included; the primary names the verb and stands last.',
  args: { size: 's' },
};

export const sizeM: Story = {
  name: 'Size M — two to six fields',
  about:
    'Dialog M (`layout.dialog.m`, today’s wide measure): one group of fields, no scrolling at 1280×800. Below `medium` it is the whole screen: ✕ at the start of its bar, the title, the primary at the end; the foot goes. Narrow the window to see it.',
  args: { size: 'm' },
};

export const sizeL: Story = {
  name: 'Size L — a list to pick from',
  about: 'Dialog L (`layout.dialog.l`): one list with search. The head and the foot stay; only the body scrolls. Full screen below `medium`.',
  args: { size: 'l' },
};

export const step: Story = {
  name: 'A step of the page — choose a template',
  about:
    '`isStep` (L): a choice made in it is the page’s at once, so choosing a tile keeps it open and its way out is ‹ back to the page, named in words, not ✕. Below `medium` it is the whole screen: ‹ at the start of its bar, the title, *Done* at the end. Opening puts the focus in the search; closing gives it back to the button.',
  args: { size: 'step' },
};

export const discard: Story = {
  name: 'Discard changes?',
  about:
    'Change the name, then press ✕, Escape or the backdrop: `onDismiss` answers `false`, and the caller opens its Dialog S over the first — the one dialog allowed over another. Escape then closes the confirmation first.',
  args: { size: 'discard' },
};
