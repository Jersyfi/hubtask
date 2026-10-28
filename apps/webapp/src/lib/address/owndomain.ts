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

/** A move of the main address: to the own domain, or back to the subdomain, from one host to the other. */
export interface MainAddressMove {
  readonly direction: 'own' | 'back';
  readonly from: string;
  readonly to: string;
}

/** Whether the owner is offered *Make it the main address*: an active domain that is not main yet. */
export function canBecomeMain(own: OwnDomain): boolean {
  return own.state === 'ACTIVE' && !own.is_main;
}

/**
 * The warning after the main own domain was lost (UC-INS-15 check 9): it broke and the subdomain
 * became main by itself, or the operator blocked it. Stays until the owner makes the domain main
 * again or removes it; absent otherwise.
 */
export function fallBackOf(own: OwnDomain | undefined): { readonly cause: 'broken' | 'blocked'; readonly at: string } | undefined {
  if (!own) return undefined;
  if (own.state === 'BLOCKED') return own.is_main ? { cause: 'blocked', at: own.since } : undefined;
  return own.fell_back_at ? { cause: 'broken', at: own.fell_back_at } : undefined;
}
