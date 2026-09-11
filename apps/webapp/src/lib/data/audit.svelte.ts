// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The trail: queried, verified, and exported to where it went (`audit.md` §3–§5).
 *
 * **A read is not recorded and an export is.** Nothing here has to arrange that — it is the
 * server's rule — but it is why the screen says so: an auditor is owed the knowledge that their
 * own export is in the trail they are reading.
 *
 * **An entry carries no user content, by design (ADR-0017).** So an actor is an identifier plus
 * the label that was valid at the time, and the screen resolves the identifier through the
 * accounts store the way the activity feed does. The stored label is what survives a deletion,
 * which is exactly what it is for.
 *
 * **A break is a finding, not an error.** `:verify` answers `valid: false` with the sequence
 * number where the chain stops holding; that is the thing an audit trail exists to produce, and a
 * caller that treated it as a failed request would hide it.
 */

import type { ResourceState } from '@hubtask/sync-engine';

import { engine } from './engine.ts';
import { jobs } from './jobs.svelte.ts';
import type { JobRef } from './jobs.ts';
import { AUDIT, auditPath, type Entry, type Query, type Verification } from './audit.ts';

export { auditPath, readVerification } from './audit.ts';
export type { Actor, Change, Entry, Finding, Query, Verification } from './audit.ts';

/** One page of the trail, as the listing answers it. */
interface Page {
  readonly items?: readonly Entry[];
  readonly next_cursor?: string | null;
}

class Audit {
  #pages = $state<Record<string, ResourceState<Page>>>({});
  #held = $state<Record<string, readonly Entry[]>>({});
  #cursors = $state<Record<string, string | undefined>>({});
  #verification = $state<Verification | undefined>(undefined);

  stateOf(query: Query): ResourceState<Page> {
    return this.#pages[auditPath(query)] ?? { status: 'idle' };
  }

  of(query: Query): readonly Entry[] {
    return this.#held[auditPath(query)] ?? [];
  }

  moreAfter(query: Query): string | undefined {
    return this.#cursors[auditPath(query)];
  }

  /** What the last verification found, or nothing while nobody has asked. */
  get verification(): Verification | undefined {
    return this.#verification;
  }

  /** Starts a query. **From `untrack`**, for the reason every other store records. */
  open(query: Query = {}): () => void {
    const key = auditPath(query);
    return engine.subscribe<Page>({ path: key }, (next) => {
      this.#pages = { ...this.#pages, [key]: next };
      if (next.status === 'ready') {
        this.#held = { ...this.#held, [key]: next.data.items ?? [] };
        this.#cursors = { ...this.#cursors, [key]: next.data.next_cursor ?? undefined };
      }
    });
  }

  /** The next page, appended. Cursor pagination, never page numbers. */
  async more(query: Query = {}): Promise<void> {
    const key = auditPath(query);
    const cursor = this.#cursors[key];
    if (!cursor) return;
    const next = await engine.refresh<Page>({ path: auditPath(query, cursor) });
    if (next.status !== 'ready') return;
    this.#held = { ...this.#held, [key]: [...(this.#held[key] ?? []), ...(next.data.items ?? [])] };
    this.#cursors = { ...this.#cursors, [key]: next.data.next_cursor ?? undefined };
  }

  /**
   * Checks the chain over a period.
   *
   * A clean check records nothing and a break records a critical entry of its own — the server's
   * doing, and the reason the screen says a break has been noticed rather than only shown.
   */
  async verify(from: string, to: string): Promise<Verification> {
    const answer = await engine.mutate<Verification>('POST', `${AUDIT}:verify`, { from, to });
    this.#verification = answer;
    return answer;
  }

  /**
   * Writes the period to a backup target, and hands the job to the watcher.
   *
   * **There is no filter here, deliberately**: an export narrowed by action or actor before it was
   * signed would be evidence about somebody's selection rather than about an interval. The
   * contract's `AuditExport` takes a period, a format and a target, and nothing else.
   */
  async export(request: {
    from: string;
    to: string;
    format: 'JSONL' | 'CSV';
    target_id: string;
  }): Promise<JobRef> {
    const accepted = await engine.mutate<JobRef>('POST', `${AUDIT}:export`, request, {
      idempotencyKey: crypto.randomUUID(),
    });
    jobs.watch(accepted);
    return accepted;
  }
}

export const audit = new Audit();
