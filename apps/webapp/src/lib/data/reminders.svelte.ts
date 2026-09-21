// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * An entry's reminders, and its series.
 *
 * **Both are unpaged, and for different reasons.** An entry carries at most
 * `max_reminders_per_item` reminders — "the bound that makes a page unnecessary rather than an
 * omission" — and it carries at most one series.
 *
 * **A series that is not there answers `404`, and that is the "none" state rather than an error.**
 * The contract says so, and this is where that is turned into a fact a screen can render: a failed
 * read whose status is 404 becomes "no series", and every other failure stays a failure.
 *
 * **An entry that repeats never is not asked.** `recurrence_rule_id` is on the row, null for an
 * entry with no series, so "none" is known before any request - and the request is not made
 * (issue 882): every `GET` that answered 404 was a red line in the browser's console, one per read, and
 * with the entry page's re-reads that was five to ten per page hiding a failure that mattered. The
 * 404 branch stays for the one case the row cannot settle: an occurrence restored from an archive
 * taken before the source column existed, which carries a rule id and has no rule of its own.
 *
 * **Deleting a series leaves every occurrence standing.** The entries it already made are ordinary
 * entries and somebody's work; the dialog says so, and nothing here pretends otherwise.
 */

import type {
  Recurrence,
  RecurrenceInput,
  Reminder,
  ReminderInput,
  ReminderUpdate,
  ResourceState,
  TransportError,
  WorkItem,
} from '@hubtask/sync-engine';

import { engine } from './engine.ts';
import { etagFor } from './etag.ts';
import { belongsToSeries } from './reminders.ts';
import { touchesOf } from './touches.ts';

export const remindersPath = (itemId: string) => `/items/${itemId}/reminders`;
export const recurrencePath = (itemId: string) => `/items/${itemId}/recurrence`;

/**
 * What a reminder write touches.
 *
 * The thread of reminders, and `/items` as well — because a reminder is not the entry's own row and
 * yet the *series* writes are: setting one materialises occurrences, which are entries. Naming both
 * from one constant would reload a list for a change it does not render, so the two have their own.
 */
const REMINDER_TOUCHES = (itemId: string) => [remindersPath(itemId)];

class Reminders {
  #lists = $state<Record<string, ResourceState<readonly Reminder[]>>>({});

  of(itemId: string): readonly Reminder[] {
    const state = this.#lists[remindersPath(itemId)];
    return state?.status === 'ready' ? state.data : [];
  }

  stateOf(itemId: string): ResourceState<readonly Reminder[]> | undefined {
    return this.#lists[remindersPath(itemId)];
  }

  /** Starts one entry's list. **From `untrack`**, for the reason every other store records. */
  open(itemId: string): () => void {
    const path = remindersPath(itemId);
    return engine.subscribe<readonly Reminder[]>({ path }, (next) => {
      this.#lists = { ...this.#lists, [path]: next };
    });
  }

  async add(itemId: string, body: ReminderInput, idempotencyKey: string): Promise<Reminder> {
    return engine.mutate<Reminder>('POST', remindersPath(itemId), body, {
      idempotencyKey,
      invalidates: REMINDER_TOUCHES(itemId),
    });
  }

  /**
   * Changes one. A merge patch: what is not sent is not touched, and the lists that are sent
   * replace the lists stored — channels and recipients are chosen whole.
   */
  async edit(
    itemId: string,
    reminder: Reminder,
    body: ReminderUpdate,
  ): Promise<Reminder> {
    return engine.mutate<Reminder>(
      'PATCH',
      `${remindersPath(itemId)}/${reminder.id}`,
      body,
      { ifMatch: etagFor(reminder.version), invalidates: REMINDER_TOUCHES(itemId) },
    );
  }

  /** Removes it. A hard delete of one row — what somebody deleted is gone, not a tombstone. */
  async remove(itemId: string, reminder: Reminder): Promise<void> {
    await engine.mutate<void>(
      'DELETE',
      `${remindersPath(itemId)}/${reminder.id}`,
      undefined,
      { ifMatch: etagFor(reminder.version), invalidates: REMINDER_TOUCHES(itemId) },
    );
  }
}

class Series {
  #rules = $state<Record<string, ResourceState<Recurrence>>>({});
  /** The entries whose row says they repeat never. Known without a request, and kept apart from
   *  `#rules` so that a rule a write just answered is not overwritten by the row it is ahead of. */
  #absent = $state<Record<string, true>>({});

  /**
   * Starts one entry's series - or settles it from the row, when the row says there is none.
   *
   * Takes the entry rather than its id because the answer is on the entry: a caller re-runs this
   * when `recurrence_rule_id` changes, and the read begins the moment the row says a series exists.
   * **From `untrack`**, for the reason every other store records.
   */
  open(item: Pick<WorkItem, 'id' | 'recurrence_rule_id'>): () => void {
    const path = recurrencePath(item.id);
    if (!belongsToSeries(item)) {
      this.#absent = { ...this.#absent, [path]: true };
      return () => {};
    }
    const { [path]: _known, ...others } = this.#absent;
    this.#absent = others;
    return engine.subscribe<Recurrence>({ path }, (next) => {
      this.#rules = { ...this.#rules, [path]: next };
    });
  }

  /** Keeps what a write answered, so the panel shows the series before the row has caught up. */
  #hold(path: string, rule: Recurrence | undefined): void {
    const { [path]: _known, ...others } = this.#absent;
    this.#absent = rule === undefined ? { ...this.#absent, [path]: true } : others;
    if (rule === undefined) {
      const { [path]: _gone, ...kept } = this.#rules;
      this.#rules = kept;
      return;
    }
    this.#rules = { ...this.#rules, [path]: { status: 'ready', data: rule, at: Date.now(), source: 'server' } };
  }

  /** The rule, when there is one. */
  of(itemId: string): Recurrence | undefined {
    const state = this.#rules[recurrencePath(itemId)];
    return state?.status === 'ready' ? state.data : undefined;
  }

  /**
   * Whether this entry is known to have no series.
   *
   * A `404` is the contract's "none", so it is answered here as a fact rather than left as a
   * failure for a screen to interpret — a panel that read the status itself would be a second
   * place that knows what 404 means on this one route.
   */
  hasNone(itemId: string): boolean {
    const path = recurrencePath(itemId);
    const state = this.#rules[path];
    if (state === undefined) return this.#absent[path] === true;
    return state.status === 'failed' && state.error.status === 404;
  }

  /**
   * A failure that is not the absence of a series — the error itself, ready to be rendered.
   *
   * The error rather than the state, so a caller does not have to narrow a union to reach it: the
   * one question worth asking here is "is something wrong", and 404 is not.
   */
  failureOf(itemId: string): TransportError | undefined {
    const state = this.#rules[recurrencePath(itemId)];
    return state?.status === 'failed' && state.error.status !== 404 ? state.error : undefined;
  }

  isReading(itemId: string): boolean {
    const path = recurrencePath(itemId);
    if (this.#absent[path] && this.#rules[path] === undefined) return false;
    const status = this.#rules[path]?.status;
    return status === undefined || status === 'idle' || status === 'loading';
  }

  /**
   * Sets the series, and changes it: one call for both, because a rule is one thing an entry
   * either carries or does not (D-04).
   *
   * `/items` is invalidated as well, because setting a series materialises occurrences — and those
   * are ordinary entries that a list is showing.
   */
  async set(itemId: string, body: RecurrenceInput, version: number | undefined): Promise<Recurrence> {
    const rule = await engine.mutate<Recurrence>('PUT', recurrencePath(itemId), body, {
      ...(version === undefined ? {} : { ifMatch: etagFor(version) }),
      invalidates: [recurrencePath(itemId), ...touchesOf(itemId)],
    });
    this.#hold(recurrencePath(itemId), rule);
    return rule;
  }

  /** Takes the series off. Every occurrence it already made stays where it is. */
  async remove(itemId: string, version: number): Promise<void> {
    await engine.mutate<void>('DELETE', recurrencePath(itemId), undefined, {
      ifMatch: etagFor(version),
      invalidates: [recurrencePath(itemId), ...touchesOf(itemId)],
    });
    this.#hold(recurrencePath(itemId), undefined);
  }

  /**
   * Skips the next occurrence the series has not produced yet.
   *
   * Under an idempotency key, because "skip the next one" said twice by a retry is not the same as
   * said twice by a person — and the contract is explicit that without a key, twice means two.
   */
  async skip(itemId: string, idempotencyKey: string): Promise<Recurrence> {
    return engine.mutate<Recurrence>(`POST`, `${recurrencePath(itemId)}:skip`, undefined, {
      idempotencyKey,
      invalidates: [recurrencePath(itemId), ...touchesOf(itemId)],
    });
  }
}

export const reminders = new Reminders();
export const series = new Series();
