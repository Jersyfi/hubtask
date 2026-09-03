// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The axis matrix of ADR-0037.
//
// Every axis here exists because a rule in design-system.md is otherwise unobservable. That is
// the whole selection criterion: a workbench that shows a component in the one configuration its
// author developed in is a gallery, and a gallery verifies nothing. Adding an axis for any other
// reason adds a combinatorial cost to thirty-eight components at once.
//
// This file is the single declaration of the set. The axis bar renders from it, the URL codec
// reads it, and build/check-stories.js validates every story's `axes` against it - so a story
// cannot name an axis that does not exist, and an axis cannot be added without appearing
// everywhere.

/** The axes, in the order the bar shows them. */
export const AXIS_IDS = ['theme', 'dir', 'text', 'motion', 'density', 'zoom', 'width'] as const;

export type AxisId = (typeof AXIS_IDS)[number];

export interface AxisValue {
  readonly value: string;
  readonly label: string;
  /** Shown under the bar while this value is selected, when it needs saying. */
  readonly note?: string;
}

export interface Axis {
  readonly id: AxisId;
  readonly label: string;
  /** The rule this axis makes checkable. Rendered as the axis's own tooltip. */
  readonly rule: string;
  readonly values: readonly AxisValue[];
  readonly fallback: string;
}

export const AXES: readonly Axis[] = [
  {
    id: 'theme',
    label: 'Theme',
    rule: 'design-system.md §6 — every rule holds in both modes, and the stylesheet has no :root fallback',
    values: [
      { value: 'light', label: 'Light' },
      { value: 'dark', label: 'Dark' },
      { value: 'both', label: 'Both' },
    ],
    fallback: 'light',
  },
  {
    id: 'dir',
    label: 'Direction',
    rule: 'design-system.md §3 — alignment is start/end only; a `left` in a component is wrong here and nowhere else',
    values: [
      { value: 'ltr', label: 'LTR' },
      { value: 'rtl', label: 'RTL' },
      { value: 'both', label: 'Both' },
    ],
    fallback: 'ltr',
  },
  {
    id: 'text',
    label: 'Text',
    rule: 'design-system.md §6 rule 4 — everything grows by 40 %; German, Finnish and Russian break any layout measured against English',
    values: [
      { value: 'normal', label: 'Normal' },
      {
        value: 'long',
        label: '+40 %',
        note: 'Pseudo-locale: every string is accented, padded to ~140 % and bracketed. Clipping is a defect, not a rendering.',
      },
    ],
    fallback: 'normal',
  },
  {
    id: 'motion',
    label: 'Motion',
    rule: 'design-system.md §6 rule 6 and §7 — reduced motion never switches off the acknowledgement, only its extent',
    values: [
      { value: 'system', label: 'System' },
      {
        value: 'reduced',
        label: 'Reduced',
        note: 'Sets data-motion="reduced" and enforces nothing. A component that ignores it has to look wrong here, or the axis proves nothing.',
      },
    ],
    fallback: 'system',
  },
  {
    id: 'density',
    label: 'Density',
    rule: 'design-system.md §5 and §9 — density is a property of the region, not of the component, so it is only observable by setting it on one',
    values: [
      { value: 'comfortable', label: 'Comfortable' },
      {
        value: 'compact',
        label: 'Compact',
        note: 'Sets data-density="compact". Nothing here may put a target below 24 px — WCAG 2.2 SC 2.5.8 is the floor, and `sm` sits exactly on it.',
      },
    ],
    fallback: 'comfortable',
  },
  {
    id: 'zoom',
    label: 'Zoom',
    rule: 'WCAG 2.2 SC 1.4.4 — the type scale is in px, so this is the only way the question gets asked',
    values: [
      { value: '100', label: '100 %' },
      { value: '200', label: '200 %' },
    ],
    fallback: '100',
  },
  {
    id: 'width',
    label: 'Width',
    rule: 'The five primitive.breakpoint values — responsive decisions are checked at the widths the tokens declare, not at whatever the window happens to be',
    values: [
      { value: 'auto', label: 'Pane' },
      { value: 'compact', label: 'Compact' },
      { value: 'medium', label: 'Medium' },
      { value: 'expanded', label: 'Expanded' },
      { value: 'large', label: 'Large' },
      { value: 'xlarge', label: 'XLarge' },
    ],
    fallback: 'auto',
  },
];

const BY_ID = new Map(AXES.map((axis) => [axis.id, axis]));

export function axis(id: AxisId): Axis {
  const found = BY_ID.get(id);
  if (!found) throw new Error(`unknown axis: ${id}`);
  return found;
}

export type AxisState = Record<AxisId, string>;

export function defaults(): AxisState {
  return Object.fromEntries(AXES.map((a) => [a.id, a.fallback])) as AxisState;
}

/** An unknown value in a shared link falls back rather than rendering a state nobody can name. */
export function coerce(id: AxisId, value: string | null): string {
  const found = axis(id);
  return found.values.some((v) => v.value === value) ? (value as string) : found.fallback;
}

/**
 * The panes a state produces: the cartesian product of the axes that carry a `both`. Theme and
 * direction are the two that do, which caps the stage at four panes - enough to compare, few
 * enough to still see.
 */
export interface Pane {
  readonly theme: 'light' | 'dark';
  readonly dir: 'ltr' | 'rtl';
  readonly key: string;
}

export function panes(state: AxisState): Pane[] {
  const themes = state.theme === 'both' ? (['light', 'dark'] as const) : ([state.theme] as ['light' | 'dark']);
  const directions = state.dir === 'both' ? (['ltr', 'rtl'] as const) : ([state.dir] as ['ltr' | 'rtl']);
  return themes.flatMap((theme) => directions.map((dir) => ({ theme, dir, key: `${theme}-${dir}` })));
}
