// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// What every control in wave 1 shares, in one place so that eight components cannot disagree.
//
// The types are small on purpose. `design-system.md` §5 says states are never variants - a variant
// matrix that contains states explodes - so there is no `hover` here, no `pressed`, no `focused`
// and no `disabled`. Those are CSS states, and the only reason a component ever knows about one is
// the reason below.

import type { IconName } from './icons/index.ts';

/**
 * Two sizes, and density is deliberately not a third one of them.
 *
 * `size` is how prominent this control is beside the one next to it, which is a decision per
 * control and therefore a prop. How much air a whole region carries is a different question with a
 * different answer: `data-density` on an ancestor (design-system.md §5), so that a list of two
 * hundred rows is told once rather than two hundred times. The two multiply — `sm` in a compact
 * region is the tightest control the tokens allow, and it is still a 24 px target.
 */
export type ControlSize = 'sm' | 'md';

/** What a button is for. Not a state: the same button is hovered, pressed and focused in all four. */
export type ButtonTone = 'primary' | 'secondary' | 'subtle' | 'danger';

/**
 * The shared shape of a control that can be switched off.
 *
 * There is deliberately **no `disabled` boolean**. `design-system.md` §4 makes `CapabilityGate` a
 * component of its own so that `ErrCapabilityNotSupported` never becomes silent ignoring, and the
 * same principle applies one level down: a control the reader cannot use owes them the reason, and
 * a boolean cannot carry one. Setting `disabledReason` is what disables a control, so the two
 * cannot come apart - and every one of them then satisfies the rule by construction rather than by
 * review.
 *
 * The string is display text, which means it is a resolved message code and never a sentence a
 * component wrote (ADR-0011, `voice-and-tone.md` §0).
 */
export interface Disableable {
  /** Why this control cannot be used. Present means disabled; absent means usable. */
  disabledReason?: string;
}

/**
 * Whether the control is carrying out the action it names. Separate from `disabledReason` because
 * it is not the same fact: a busy control is not unavailable, it is working, and
 * `voice-and-tone.md` §2.4 has it keep its place and change its verb rather than disappear.
 */
export interface Busyable {
  isBusy?: boolean;
}

/**
 * What a feedback surface is saying. Not a state of the component - the state of something else,
 * which is why the same four appear on a `Badge`, a `Banner` and a `Toast` rather than each of
 * them inventing a vocabulary.
 */
export type StatusTone = 'info' | 'success' | 'warning' | 'danger';

/**
 * The mark each tone carries, because rule 3 says colour never stands alone: a red surface that is
 * only red says nothing in greyscale, in print, or to a reader with a colour vision deficiency.
 * The tone chooses the icon rather than the caller - two danger banners with different marks would
 * be two vocabularies - and a caller with a better mark for one case still passes `icon`.
 */
export const STATUS_ICON = {
  info: 'info',
  success: 'circle-check',
  warning: 'triangle-alert',
  danger: 'circle-alert',
} as const satisfies Record<StatusTone, IconName>;
