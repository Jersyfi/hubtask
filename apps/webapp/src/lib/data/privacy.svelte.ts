// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The cases a controller answers a person's request from (`data-protection.md` §4, §9, §12).
 *
 * **Nothing here starts work by itself.** Recording a case opens it with its deadline and collects,
 * exports and erases nothing; moving it to `IN_PROGRESS` is the controller's decision and is what
 * starts the job. So this module has a create and a patch, and the patch is where the consequence
 * lives — which is why the screen asks before it.
 *
 * **The scope is always this workspace.** `INSTALLATION` crosses the tenant boundary, needs the
 * `admin:tenants` scope and is the provider's path; it is absent from `privacy.ts`'s types rather
 * than filtered here, so nothing in this client can compose one.
 *
 * **Restriction and consent withdrawal are the two Articles with their own routes**, and neither
 * is a case: `:restrict` sets a technical state on an account, and a withdrawal is recorded rather
 * than deleted — the record that somebody consented, and when they took it back, is what shows
 * which processing was lawful when.
 */

import type { ResourceState } from '@hubtask/sync-engine';

import { engine } from './engine.ts';
import type { ErasureMode, Kind, Request, Status } from './privacy.ts';

export { byDeadline, KINDS, producesArchive, standingOf, SOON_MS } from './privacy.ts';
export type { ErasureMode, Kind, Request, Standing, Status } from './privacy.ts';

const REQUESTS = '/privacy/requests';
const WITHDRAW = '/privacy/consents:withdraw';

interface Page {
  readonly data?: readonly Request[];
  readonly page?: { readonly next_cursor?: string | null; readonly has_more?: boolean };
}

/** The listing's path. `include_closed` is a choice, because a closed case is not work. */
function path(includeClosed: boolean, cursor?: string): string {
  const written = new URLSearchParams();
  if (includeClosed) written.set('include_closed', 'true');
  if (cursor) written.set('cursor', cursor);
  const search = written.toString();
  return search ? `${REQUESTS}?${search}` : REQUESTS;
}

class Privacy {
  #pages = $state<Record<string, ResourceState<Page>>>({});
  #held = $state<Record<string, readonly Request[]>>({});
  #cursors = $state<Record<string, string | undefined>>({});

  stateOf(includeClosed: boolean): ResourceState<Page> {
    return this.#pages[path(includeClosed)] ?? { status: 'idle' };
  }

  of(includeClosed: boolean): readonly Request[] {
    return this.#held[path(includeClosed)] ?? [];
  }

  moreAfter(includeClosed: boolean): string | undefined {
    return this.#cursors[path(includeClosed)];
  }

  /** Starts the listing. **From `untrack`**, for the reason every other store records. */
  open(includeClosed = false): () => void {
    const key = path(includeClosed);
    return engine.subscribe<Page>({ path: key }, (next) => {
      this.#pages = { ...this.#pages, [key]: next };
      if (next.status === 'ready') {
        this.#held = { ...this.#held, [key]: next.data.data ?? [] };
        this.#cursors = { ...this.#cursors, [key]: next.data.page?.next_cursor ?? undefined };
      }
    });
  }

  /** The next page, appended. Cursor pagination, never page numbers. */
  async more(includeClosed: boolean): Promise<void> {
    const key = path(includeClosed);
    const cursor = this.#cursors[key];
    if (!cursor) return;
    const next = await engine.refresh<Page>({ path: path(includeClosed, cursor) });
    if (next.status !== 'ready') return;
    this.#held = { ...this.#held, [key]: [...(this.#held[key] ?? []), ...(next.data.data ?? [])] };
    this.#cursors = { ...this.#cursors, [key]: next.data.page?.next_cursor ?? undefined };
  }

  /**
   * Records a right somebody exercised, and nothing else.
   *
   * The deadline is thirty days from receipt unless one is named, and no work starts here: what
   * happens next is a decision, and a decision is a second request.
   */
  async record(draft: {
    kind: Kind;
    subject_account_id?: string;
    subject_email?: string;
    due_at?: string;
    notes?: string;
    target_id?: string;
  }): Promise<Request> {
    return engine.mutate<Request>('POST', REQUESTS, { scope: 'TENANT', ...draft }, {
      idempotencyKey: crypto.randomUUID(),
      invalidates: [REQUESTS],
    });
  }

  /**
   * Moves a case along. `RECEIVED → IN_PROGRESS → COMPLETED | REJECTED`.
   *
   * An illegitimate transition is refused by name rather than ignored, which is why the screen
   * offers the ones that exist and renders the refusal for the rest.
   */
  async change(
    requestId: string,
    patch: {
      status?: Status;
      erasure_mode?: ErasureMode;
      rejection_reason?: string;
      notes?: string;
      target_id?: string;
    },
  ): Promise<Request> {
    return engine.mutate<Request>('PATCH', `${REQUESTS}/${requestId}`, patch, {
      invalidates: [REQUESTS],
    });
  }

  /**
   * Puts an account into `RESTRICTED`, or takes it back out.
   *
   * A technical state rather than a lock: the account stays readable and its content stays where
   * it is, and what stops is processing — the automations and the AI leave the record alone.
   */
  async restrict(accountId: string, restricted: boolean, reason?: string): Promise<unknown> {
    return engine.mutate<unknown>(
      'POST',
      `/accounts/${accountId}:restrict`,
      { restricted, ...(reason ? { reason } : {}) },
      { idempotencyKey: crypto.randomUUID(), invalidates: ['/accounts'] },
    );
  }

  /** Records a withdrawal. Recorded rather than deleted: what was lawful when is the point. */
  async withdraw(purpose: string, accountId?: string, reason?: string): Promise<unknown> {
    return engine.mutate<unknown>(
      'POST',
      WITHDRAW,
      { purpose, ...(accountId ? { account_id: accountId } : {}), ...(reason ? { reason } : {}) },
      { idempotencyKey: crypto.randomUUID() },
    );
  }
}

export const privacy = new Privacy();
