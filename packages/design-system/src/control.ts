// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// What every control in wave 1 shares, in one place so that eight components cannot disagree.
//
// The types are small on purpose. `design-system.md` §5 says states are never variants - a variant
// matrix that contains states explodes - so there is no `hover` here, no `pressed`, no `focused`
// and no `disabled`. Those are CSS states, and the only reason a component ever knows about one is
// the reason below.

/** Two sizes, and density is deliberately not one of them (design-system.md §9's open gap). */
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
