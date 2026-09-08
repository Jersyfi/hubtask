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
} from '@hubtask/sync-engine';

import { engine } from './engine.ts';
import { etagFor } from './etag.ts';

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

  open(itemId: string): () => void {
    const path = recurrencePath(itemId);
    return engine.subscribe<Recurrence>({ path }, (next) => {
      this.#rules = { ...this.#rules, [path]: next };
    });
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
    const state = this.#rules[recurrencePath(itemId)];
    return state?.status === 'failed' && state.error.status === 404;
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
    const status = this.#rules[recurrencePath(itemId)]?.status;
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
    return engine.mutate<Recurrence>('PUT', recurrencePath(itemId), body, {
      ...(version === undefined ? {} : { ifMatch: etagFor(version) }),
      invalidates: [recurrencePath(itemId), '/items'],
    });
  }

  /** Takes the series off. Every occurrence it already made stays where it is. */
  async remove(itemId: string, version: number): Promise<void> {
    await engine.mutate<void>('DELETE', recurrencePath(itemId), undefined, {
      ifMatch: etagFor(version),
      invalidates: [recurrencePath(itemId), '/items'],
    });
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
      invalidates: [recurrencePath(itemId), '/items'],
    });
  }
}

export const reminders = new Reminders();
export const series = new Series();
