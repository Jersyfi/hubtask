// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The marks a general icon set cannot draw, because they name things only this domain has.
//
// Every one of them is a noun from `domain-model.md` - the levels of the hierarchy, the bucket,
// the jumble, the capability - rather than a shape somebody liked. Where Lucide already says a
// domain noun well it is used instead and no mark is drawn here: a label is a `tag`, a comment a
// `message-square`, a reminder a `bell`, a recurrence a `repeat`. Drawing a second version of an
// icon everybody already recognises is how a set stops being recognisable.
//
// They are hand-written on the same terms as the base set (ADR-0041): a 24x24 grid, a stroke the
// wrapper sets to 1.5, `currentColor` inherited from the text they sit in, and no attribute that
// names a colour - so `lint-no-literals` has nothing to find and a mark cannot disagree with the
// theme it is rendered in.
//
// The family resemblance is deliberate and is what makes them read as one set with the base:
// containers are rounded rectangles on the 3..21 box, contents are drawn inside them, and a
// relationship is a line that leaves one shape and enters another.

import type { IconNode } from './node.ts';

export const CUSTOM_ICONS = {
  // The hierarchy, outermost first. Three nested planes, which is the wordmark's idea: a hub is
  // the thing everything else sits inside.
  hub: [
    ['rect', { x: '3', y: '3', width: '18', height: '18', rx: '3' }],
    ['rect', { x: '7', y: '7', width: '10', height: '10', rx: '2' }],
    ['rect', { x: '10.5', y: '10.5', width: '3', height: '3', rx: '1' }],
  ],

  // Two planes offset: a collection is a stack of things, not a thing.
  collection: [
    ['path', { d: 'M8 3h11a2 2 0 0 1 2 2v11' }],
    ['rect', { x: '3', y: '8', width: '13', height: '13', rx: '2' }],
  ],

  // The three item types share one box and differ in what is inside it, because they share one
  // aggregate and differ in their capability profile (domain-model.md §1).
  task: [
    ['rect', { x: '3', y: '3', width: '18', height: '18', rx: '3' }],
    ['path', { d: 'm8 12 2.5 2.5L16 9' }],
  ],
  'work-package': [
    ['rect', { x: '3', y: '3', width: '18', height: '18', rx: '3' }],
    ['path', { d: 'M7.5 10h9' }],
    ['path', { d: 'M7.5 14h9' }],
  ],
  // No box: an activity is the leaf, the one level that holds nothing.
  activity: [
    ['circle', { cx: '6.5', cy: '12', r: '2.5' }],
    ['path', { d: 'M12 12h9' }],
  ],

  // Three of the same thing, at no particular angle. The jumble is the one place in the product
  // where order is explicitly not promised, so the mark promises none either - a tray would say
  // "inbox", which is a place things are ordered on arrival.
  jumble: [
    ['rect', { x: '2.5', y: '9', width: '8', height: '8', rx: '2', transform: 'rotate(-13 6.5 13)' }],
    ['rect', { x: '11.5', y: '3.5', width: '8', height: '8', rx: '2', transform: 'rotate(12 15.5 7.5)' }],
    ['rect', { x: '13', y: '13', width: '8', height: '8', rx: '2', transform: 'rotate(-7 17 17)' }],
  ],

  // A column with a head: a bucket is a named place inside a collection, and its name is the part
  // people point at.
  bucket: [
    ['rect', { x: '5', y: '3', width: '14', height: '18', rx: '2' }],
    ['path', { d: 'M5 8h14' }],
  ],

  // A switch. A capability is on or off for a type, and `ErrCapabilityNotSupported` is what makes
  // that a visible fact rather than a silent one (domain-model.md §2).
  capability: [
    ['rect', { x: '2', y: '8', width: '20', height: '8', rx: '4' }],
    ['circle', { cx: '8', cy: '12', r: '2' }],
  ],

  // Three uprights on a common ground: a workspace is where several things stand together, and it
  // is the boundary every query is scoped to.
  workspace: [
    ['path', { d: 'M3 21h18' }],
    ['path', { d: 'M6 21V9' }],
    ['path', { d: 'M12 21V4' }],
    ['path', { d: 'M18 21V12' }],
  ],

  // A funnel that was kept: a saved view is a filter with a name.
  'saved-view': [
    ['path', { d: 'M3 4h15l-6 7v4l-3 2v-6z' }],
    ['path', { d: 'm14 18 2 2 5-5' }],
  ],

  // Something enters a rule and the rule acts. The arrow is on the outside because a rule is
  // triggered rather than run.
  automation: [
    ['path', { d: 'M2 12h5' }],
    ['path', { d: 'm5 9 3 3-3 3' }],
    ['rect', { x: '10', y: '5', width: '12', height: '14', rx: '2' }],
    ['path', { d: 'M14 10h4' }],
    ['path', { d: 'M14 14h4' }],
  ],

  // One box waits for another. The line leaves the first and enters the second, which is the
  // direction a dependency actually points.
  dependency: [
    ['rect', { x: '3', y: '3', width: '8', height: '6', rx: '2' }],
    ['rect', { x: '13', y: '15', width: '8', height: '6', rx: '2' }],
    ['path', { d: 'M7 9v7a2 2 0 0 0 2 2h4' }],
    ['path', { d: 'm11 15 2 3-2 3' }],
  ],

  // The same box, not yet filled in.
  template: [
    ['rect', { x: '3', y: '3', width: '18', height: '18', rx: '3', 'stroke-dasharray': '4 3' }],
    ['path', { d: 'M8 10h8' }],
    ['path', { d: 'M8 14h5' }],
  ],

  // A container in the abstract, for the places the type is not yet known: the shape both a hub
  // and a collection reduce to.
  container: [
    ['rect', { x: '3', y: '3', width: '18', height: '18', rx: '3' }],
    ['path', { d: 'M3 9h18' }],
  ],
} as const satisfies Record<string, readonly IconNode[]>;
