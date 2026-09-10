// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What an audit entry is, and how a query is written down.
 *
 * The pure half, for the reason `jobs.ts` is separate from its store: a query string is the one
 * thing about this screen that can be wrong silently — a filter that is dropped answers a wider
 * trail than somebody asked for, and a wider trail reads as an answer rather than as a mistake.
 *
 * **An entry carries no user content, by design (ADR-0017).** An actor is an identifier plus the
 * label that was valid at the time, stored denormalised so that a deletion cannot make the trail
 * unreadable.
 */

/** Where the trail is read. One constant, so the store and the path builder cannot disagree. */
export const AUDIT = '/audit';

/** Who acted, as the entry recorded them. */
export interface Actor {
  readonly type?: string;
  readonly id?: string | null;
  /** The label that was valid at the time. Denormalised, so a deletion cannot make it unreadable. */
  readonly label?: string | null;
  readonly on_behalf_of?: string | null;
}

/** One changed field, masked per its classification. A `SECRET` one is not here at all. */
export interface Change {
  readonly field?: string;
  readonly from?: unknown;
  readonly to?: unknown;
  readonly changed?: boolean;
  readonly from_hash?: string | null;
  readonly to_hash?: string | null;
}

/** One entry. */
export interface Entry {
  readonly id: string;
  readonly seq?: number;
  readonly occurred_at: string;
  readonly action: string;
  readonly outcome: 'SUCCESS' | 'DENIED' | 'FAILED';
  readonly severity?: 'INFO' | 'NOTICE' | 'WARNING' | 'CRITICAL';
  readonly actor: Actor;
  readonly target?: { readonly type?: string | null; readonly id?: string | null; readonly label?: string | null };
  readonly changes?: readonly Change[];
  readonly context?: {
    readonly request_id?: string;
    readonly ip_prefix?: string | null;
    readonly user_agent_class?: string;
    readonly channel?: string;
  };
  readonly legal_basis?: string | null;
  readonly hash?: string;
}

/** What `:verify` answers. Both outcomes are facts; neither is an error. */
export interface Verification {
  readonly valid?: boolean;
  readonly checked?: number;
  /** Where the first entry that does not hold sits — where an investigation starts. */
  readonly first_broken_seq?: number | null;
  /** The missing sequence numbers, cut at a hundred. */
  readonly gaps?: readonly number[];
  readonly gap_count?: number;
  /** When the chain was last anchored outside the database, and null when it never was. */
  readonly sealed_until?: string | null;
}

/** Every filter the contract declares. Absent means unfiltered. */
export interface Query {
  readonly from?: string;
  readonly to?: string;
  readonly action?: string;
  readonly actorId?: string;
  readonly targetType?: string;
  readonly targetId?: string;
  readonly outcome?: string;
}

/** The listing's path for a query. Built here so the store and its callers agree on one string. */
export function auditPath(query: Query = {}, cursor?: string): string {
  const written = new URLSearchParams();
  if (query.from) written.set('from', query.from);
  if (query.to) written.set('to', query.to);
  if (query.action) written.set('action', query.action);
  if (query.actorId) written.set('actor_id', query.actorId);
  if (query.targetType) written.set('target_type', query.targetType);
  if (query.targetId) written.set('target_id', query.targetId);
  if (query.outcome) written.set('outcome', query.outcome);
  if (cursor) written.set('cursor', cursor);
  const search = written.toString();
  return search ? `${AUDIT}?${search}` : AUDIT;
}
