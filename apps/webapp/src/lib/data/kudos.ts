// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What an entry's page offers about kudos and milestones (UC-WRK-29 checks 4, 5 and 7), decided
 * here and nowhere else:
 *
 * * **Off means nothing.** The account's `celebrations` preference switches the moments, the
 *   milestones and the kudos off together (`design-system.md` §7): nothing is offered, read or
 *   drawn while it is off.
 * * **Kudos are for somebody else's finished work.** An open entry offers none, an entry the
 *   reader completed shows what they received, any other completed one offers to give.
 * * **Nobody sees a count.** The person who completed the entry sees who gave kudos - names,
 *   never a number - and everybody else sees only their own.
 */

/** The fields of an entry the decision reads. */
export interface Completed {
  readonly completion?: { readonly is_completed?: boolean; readonly completed_by?: string | null } | null;
}

/** What the entry's page draws: the kudos received, the control to give them, or nothing. */
export type KudosOffer = 'received' | 'give' | 'none';

export function kudosOffer(item: Completed, me: string | undefined, isCelebrating: boolean): KudosOffer {
  if (!isCelebrating || !me) return 'none';
  const completedBy = item.completion?.is_completed ? item.completion.completed_by : undefined;
  if (!completedBy) return 'none';
  return completedBy === me ? 'received' : 'give';
}

/** The givers' names as the reader's language lists them: "Ben, Clara and Dana". */
export function namesOf(names: readonly string[], locale: string): string {
  try {
    return new Intl.ListFormat(locale, { type: 'conjunction' }).format(names);
  } catch {
    return names.join(', ');
  }
}

/** Whether a container's milestone is drawn: only where it has one and the reader celebrates. */
export function showsMilestone(milestone: { readonly reached_at?: string } | null | undefined, isCelebrating: boolean): boolean {
  return isCelebrating && typeof milestone?.reached_at === 'string' && milestone.reached_at !== '';
}
