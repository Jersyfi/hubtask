// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Who else has an entry open, and who is writing a reply (UC-SVC-03 check 4): the entry's
// presence document carries awareness alone, and this reads it into one row per person.

/** The entry's presence document; the server stores nothing of it. */
export const presencePath = (itemId: string) => `/realtime/documents/entry/${encodeURIComponent(itemId)}`;

/** What one open entry tells the others about its reader. */
export interface PresenceState {
  account_id?: string;
  name?: string;
  replying?: boolean;
}

export interface Present {
  accountId: string | undefined;
  name: string | undefined;
  replying: boolean;
}

/**
 * The other people here, one row per person however many tabs they have open, replying if any of
 * their tabs is; the reader themselves is never among them. A state that names no account is
 * kept by its name, so an unnamed connection still counts as somebody.
 */
export function othersHere(
  states: ReadonlyMap<number, unknown>, ownClient: number, ownAccount: string | undefined,
): Present[] {
  const byPerson = new Map<string, Present>();
  states.forEach((raw, client) => {
    if (client === ownClient || !raw || typeof raw !== 'object') return;
    const state = raw as PresenceState;
    const accountId = typeof state.account_id === 'string' && state.account_id ? state.account_id : undefined;
    if (accountId && accountId === ownAccount) return;
    const name = typeof state.name === 'string' && state.name ? state.name : undefined;
    const key = accountId ?? `client:${client}`;
    const known = byPerson.get(key);
    byPerson.set(key, {
      accountId, name: known?.name ?? name, replying: (known?.replying ?? false) || state.replying === true,
    });
  });
  return [...byPerson.values()].sort((a, b) => Number(b.replying) - Number(a.replying));
}
