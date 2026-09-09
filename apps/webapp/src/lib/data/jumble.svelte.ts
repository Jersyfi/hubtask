// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The jumble: things arrive, and become work or do not (G-10, `automation.md` §4).
 *
 * **An entry is decided about exactly once.** `NEW` becomes `PROCESSED` or `DISMISSED` and
 * `settled_at` says when. A dismissal is a state and not a deletion — the row stays readable and
 * ages out by retention rule — so nothing here removes anything from the list.
 *
 * **The intake token is a credential in a URL.** That is why it is *minted* rather than displayed
 * on demand: `POST /jumble/intake:rotate-token` answers the whole address once, and every later
 * read says only when it was last rotated. Rotating retires the previous one, which is what makes
 * it worth a sentence at the call site.
 *
 * **No AI.** `:suggest` exists and this client does not call it (decision 11).
 */

import type { ResourceState } from '@hubtask/sync-engine';

import { engine } from './engine.ts';

const ENTRIES = '/jumble/entries';
const ROTATE = '/jumble/intake:rotate-token';
/** A conversion creates an entry, so the containers a board reads are stale afterwards. */
const TOUCHES = [ENTRIES, '/items', '/containers'];

/** One arrival, as `JumbleEntry` answers it. */
export interface JumbleEntry {
  readonly id: string;
  readonly channel: string;
  readonly sender?: string | null;
  readonly raw_subject?: string | null;
  readonly raw_body?: string | null;
  readonly attachments: readonly string[];
  readonly status: string;
  readonly target_item_id?: string | null;
  readonly received_at: string;
  readonly settled_at?: string | null;
}

interface EntryPage {
  readonly data?: readonly JumbleEntry[];
  readonly next_cursor?: string | null;
}

/** What a conversion needs: where it lands, and what it is called. */
export interface Conversion {
  readonly collectionId: string;
  readonly bucketId?: string;
  readonly title?: string;
  readonly type?: string;
}

/** The freshly minted address, for the only time it exists outside the server's hash. */
export interface IntakeToken {
  readonly token: string;
  readonly rotated_at: string;
}

/** The listing's path for a filter. Built here so the store and its callers agree on one string. */
export function entriesPath(status?: string, cursor?: string): string {
  const query = new URLSearchParams();
  if (status) query.set('status', status);
  if (cursor) query.set('cursor', cursor);
  const written = query.toString();
  return written ? `${ENTRIES}?${written}` : ENTRIES;
}

class Jumble {
  #pages = $state<Record<string, ResourceState<EntryPage>>>({});
  /** Everything read so far for one filter, in arrival order, across the pages that have landed. */
  #held = $state<Record<string, readonly JumbleEntry[]>>({});
  #cursors = $state<Record<string, string | undefined>>({});

  stateOf(status?: string): ResourceState<EntryPage> {
    return this.#pages[entriesPath(status)] ?? { status: 'idle' };
  }

  of(status?: string): readonly JumbleEntry[] {
    return this.#held[entriesPath(status)] ?? [];
  }

  /** Whether there is another page. `undefined` means the end, which is not the same as unread. */
  moreAfter(status?: string): string | undefined {
    return this.#cursors[entriesPath(status)];
  }

  /** Starts the first page. **From `untrack`**, for the reason every other store records. */
  open(status?: string): () => void {
    const key = entriesPath(status);
    return engine.subscribe<EntryPage>({ path: key }, (next) => {
      this.#pages = { ...this.#pages, [key]: next };
      if (next.status === 'ready') {
        this.#held = { ...this.#held, [key]: next.data.data ?? [] };
        this.#cursors = { ...this.#cursors, [key]: next.data.next_cursor ?? undefined };
      }
    });
  }

  /**
   * Reads the next page and appends it.
   *
   * Cursor pagination, never page numbers: the API has none, so nothing here may imply them. The
   * appended page is held beside the first rather than replacing it, because "load more" is what
   * the reader asked for.
   */
  async more(status?: string): Promise<void> {
    const key = entriesPath(status);
    const cursor = this.#cursors[key];
    if (!cursor) return;
    const next = await engine.refresh<EntryPage>({ path: entriesPath(status, cursor) });
    if (next.status !== 'ready') return;
    this.#held = { ...this.#held, [key]: [...(this.#held[key] ?? []), ...(next.data.data ?? [])] };
    this.#cursors = { ...this.#cursors, [key]: next.data.next_cursor ?? undefined };
  }

  /** Quick capture from inside the product. */
  async capture(subject: string, body: string): Promise<void> {
    await engine.mutate('POST', ENTRIES, {
      channel: 'QUICK_CAPTURE',
      ...(subject ? { raw_subject: subject } : {}),
      ...(body ? { raw_body: body } : {}),
    }, { idempotencyKey: crypto.randomUUID(), invalidates: [ENTRIES] });
  }

  /** Turns one into work, and answers what it produced so the screen can link to it. */
  async convert(entryId: string, into: Conversion): Promise<JumbleEntry> {
    return engine.mutate<JumbleEntry>('POST', `${ENTRIES}/${entryId}:convert`, {
      collection_id: into.collectionId,
      ...(into.bucketId ? { bucket_id: into.bucketId } : {}),
      ...(into.title ? { title: into.title } : {}),
      ...(into.type ? { type: into.type } : {}),
    }, { idempotencyKey: crypto.randomUUID(), invalidates: TOUCHES });
  }

  /** Decides against one. It stays readable and ages out by retention rule. */
  async dismiss(entryId: string): Promise<void> {
    await engine.mutate('POST', `${ENTRIES}/${entryId}:dismiss`, {}, {
      idempotencyKey: crypto.randomUUID(),
      invalidates: [ENTRIES],
    });
  }

  /**
   * Mints a new intake address and answers it once.
   *
   * The caller shows it and drops it; this keeps no copy. Whatever posts to the old address stops
   * working at this moment, which is the sentence the screen owes before the button.
   */
  async rotateIntake(): Promise<IntakeToken> {
    return engine.mutate<IntakeToken>('POST', ROTATE, {}, { idempotencyKey: crypto.randomUUID() });
  }
}

export const jumble = new Jumble();
