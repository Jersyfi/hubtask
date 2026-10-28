// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// What the *Use your own domain* panel derives from the server's answer (UC-INS-15): the zone the
// records are added in, the state's badge, and which records each state shows. Kept apart from the
// component so each is tested without a DOM.

import type { OwnDomain, OwnDomainRecord } from '@hubtask/sync-engine';

export type OwnDomainState = OwnDomain['state'];

/** Where the owner adds the records: the domain without its first label - `tasks.acme.com` is managed in `acme.com`. */
export function zoneOf(domain: string): string {
  const dot = domain.indexOf('.');
  return dot < 0 ? domain : domain.slice(dot + 1);
}

/** The badge's tone for a state: waiting is information, proven is success, broken and blocked are danger. */
export function toneOf(state: OwnDomainState): 'info' | 'success' | 'danger' {
  if (state === 'PENDING') return 'info';
  if (state === 'VERIFIED' || state === 'ACTIVE') return 'success';
  return 'danger';
}

/** The records a state shows: both while waiting, the proof alone once it went missing, none otherwise. */
export function recordsShown(own: OwnDomain): readonly OwnDomainRecord[] {
  if (own.state === 'PENDING') return own.records;
  if (own.state === 'BROKEN') return own.records.filter((record) => record.type === 'TXT');
  return [];
}

/** Whether the owner is offered *Remove this domain* in a state, as the approved screens draw it. */
export function isRemovable(state: OwnDomainState): boolean {
  return state === 'PENDING' || state === 'ACTIVE' || state === 'BLOCKED';
}
