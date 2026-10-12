// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * How an invitation reaches the person on this installation, and what the answer to one carries.
 *
 * **The screen promises from the manifest, and shows from the answer.** `features.mail` says
 * before the press whether a mail goes out or a link is handed over, so the form can say which. What
 * follows the press is read from the answer itself: a link is shown exactly when the server answered
 * one, and never guessed from a manifest that may be a moment old.
 */

/** `unknown` until the manifest has been read: the form then promises neither. */
export type InviteWay = 'mail' | 'link' | 'unknown';

export function inviteWay(features: Readonly<Record<string, boolean>> | undefined): InviteWay {
  if (features === undefined || !('mail' in features)) return 'unknown';
  return features['mail'] === true ? 'mail' : 'link';
}

/** The link an invitation's answer carries where no mail goes out; nothing where the mail carries it. */
export function handedOverLink(answer: { readonly invitation_link?: string | null } | undefined): string | undefined {
  const link = answer?.invitation_link;
  return typeof link === 'string' && link !== '' ? link : undefined;
}
