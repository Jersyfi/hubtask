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
import type { InvitationDelivery } from '@hubtask/sync-engine';

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

/** A choice of the invitation's role that names one of the workspace's own roles, not a shipped one. */
const CUSTOM = 'custom:';
/** A choice of the invitation's place that names a collection; a hub's is its bare id. */
const COLLECTION = 'collection:';

export interface InvitePlace {
  readonly scopeType: 'TENANT' | 'HUB' | 'COLLECTION';
  readonly scopeId?: string;
}

/**
 * The roles an invitation offers at its place: the shipped ones the reader may give there, then -
 * where the reader may give any - the workspace's own (a requester role holding `REQUEST` alone,
 * say), by their names, as the role-grant screens offer them. The server still decides the grant.
 */
export function inviteRoleChoices(
  shipped: readonly { readonly value: string; readonly label: string }[],
  custom: readonly { readonly key: string; readonly name: string }[] | undefined,
): readonly { readonly value: string; readonly label: string }[] {
  if (shipped.length === 0) return [];
  return [...shipped, ...(custom ?? []).map((role) => ({ value: `${CUSTOM}${role.key}`, label: role.name }))];
}

/** The workspace role's key a choice names, or `undefined` for a shipped role. */
export function customRoleOf(choice: string): string | undefined {
  return choice.startsWith(CUSTOM) ? choice.slice(CUSTOM.length) : undefined;
}

/** A collection's choice of place, under its hub's name. */
export const collectionChoice = (id: string): string => `${COLLECTION}${id}`;

/** The place a choice names: the workspace for none, a collection by its prefix, else a hub. */
export function invitePlaceOf(choice: string): InvitePlace {
  if (choice === '') return { scopeType: 'TENANT' };
  if (choice.startsWith(COLLECTION)) return { scopeType: 'COLLECTION', scopeId: choice.slice(COLLECTION.length) };
  return { scopeType: 'HUB', scopeId: choice };
}

/**
 * The row's line of how the last invitation mail went (user-experience.md §8.3): its state's
 * sentence with the time, and - where an attempt failed - the failure's own code and parameters,
 * rendered beside it as its reason.
 */
export function deliveryLine(
  delivery: Pick<InvitationDelivery, 'state' | 'at' | 'code' | 'params'>,
): { code: string; reason?: { code: string; params: Readonly<Record<string, string>> } } {
  const line = { code: `app.people.delivery_${delivery.state.toLowerCase()}` };
  return delivery.code ? { ...line, reason: { code: delivery.code, params: delivery.params ?? {} } } : line;
}
