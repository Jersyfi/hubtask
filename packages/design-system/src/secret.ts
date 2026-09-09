// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The two decisions `OneTimeSecret` makes that are worth checking without a browser: what a hidden
// secret looks like, and when it may be dismissed.
//
// The same bargain `layers.ts` and `structure.ts` make. A component that answered these inline
// could only be checked by opening one and looking at it — and "does the mask leak the length" is
// not a question looking answers.

/**
 * How many characters a hidden secret is drawn as.
 *
 * Fixed, and that is the whole point: a mask as long as the value tells a shoulder-surfer how long
 * the secret is, which narrows a guess for free. Sixteen is enough to read as "something is here"
 * and short enough not to wrap on a narrow screen.
 */
export const MASK_LENGTH = 16;

/** The character. A bullet rather than an asterisk: it is what every password field draws. */
const MASK_CHARACTER = '•';

/**
 * The hidden form of a value.
 *
 * Takes the value and deliberately tells you nothing about it — not its length, not its alphabet,
 * not whether it is empty. The argument exists so that a call site cannot accidentally render the
 * mask of a secret it does not hold.
 */
export function mask(value: string): string {
  void value;
  return MASK_CHARACTER.repeat(MASK_LENGTH);
}

/** What a dismissal is waiting for. */
export interface DismissalState {
  /** Whether the caller requires an acknowledgement before the value may go. */
  readonly isRequired: boolean;
  /** Whether the reader has given it. */
  readonly hasAcknowledged: boolean;
}

/**
 * Whether the value may be dismissed.
 *
 * A caller that requires an acknowledgement is saying "this value cannot be shown again, and ten
 * codes scrolled past are ten codes nobody wrote down". Where nothing is required, dismissing is
 * always allowed: a component that made somebody tick a box to close a panel they had already read
 * would be ceremony rather than care.
 */
export function mayDismiss({ isRequired, hasAcknowledged }: DismissalState): boolean {
  return !isRequired || hasAcknowledged;
}

/**
 * Whether this browser can be asked to copy.
 *
 * The clipboard is not available on an insecure origin and not in every browser, and its absence
 * is **not an error**: the value is on screen and can be selected. So the control is offered where
 * it works and left out where it does not, rather than being offered and failing at the press.
 */
export function canCopy(clipboard: { writeText?: unknown } | undefined | null): boolean {
  return typeof clipboard?.writeText === 'function';
}
