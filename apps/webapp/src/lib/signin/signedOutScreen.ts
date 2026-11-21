// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/** What the signed-out page draws once no link screen (reset, invitation, confirmation) applies. */
export type SignedOutScreen = 'app-sign-in' | 'setup' | 'workspaces' | 'loading' | 'sign-in';

export interface SignedOutFacts {
  readonly browserSignIn: boolean;
  readonly manifestRead: boolean;
  readonly manifestFailed: boolean;
  readonly setupOpen: boolean;
  readonly deviceListHome: boolean;
}

/**
 * Whether the installation still waits for its setup is the manifest's answer, so until it is read
 * nothing is chosen: a sign-in card drawn meanwhile is the wrong screen on a fresh installation. A
 * failed read draws the sign-in card, which says why.
 */
export function signedOutScreen(facts: SignedOutFacts): SignedOutScreen {
  if (facts.browserSignIn) return 'app-sign-in';
  if (facts.setupOpen) return 'setup';
  if (facts.deviceListHome) return 'workspaces';
  if (!facts.manifestRead && !facts.manifestFailed) return 'loading';
  return 'sign-in';
}
