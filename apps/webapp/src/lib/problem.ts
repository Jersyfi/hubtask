// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A problem document, turned into something a person can read and act on.
 *
 * RFC 9457 with this API's four additions (ADR-0025, `api-guidelines.md` §3): `code` is the
 * category, `detail_code` is what actually happened, `params` fills the sentence, `field_errors[]`
 * says which input is wrong, and `request_id` is what a support request is answered by. This
 * module is where those become one sentence, a message per field, and a reference to quote.
 *
 * Three decisions live here rather than in a component.
 *
 * **The specific code wins, if there is a message for it.** `validation_failed` is true and
 * useless; `items.title_too_long` is the same failure said usefully. But a `detail_code` the
 * catalogue has never heard of is worse than the category, so the choice is made by asking the
 * catalogue rather than by preferring blindly.
 *
 * **A field error belongs under its field.** The server sends a `path`; a form that showed the
 * five sentences in one list at the top would make the reader match them to twelve inputs by hand.
 *
 * **An internal error carries its reference.** Without it a support thread cannot be traced, and
 * the reader is the only person who has the number.
 */

import type { TransportError } from '@hubtask/sync-engine';

import { formatDateTime, formatRelative } from './i18n/datetime.ts';
import type { MessageParams } from './i18n/format.ts';
import type { Messages } from './i18n/messages.ts';

export interface RenderedProblem {
  /** The one sentence to show. Never empty: every branch below ends in a message. */
  readonly message: string;
  /**
   * The reason without the reference: for a screen that folds the reference behind *Details for
   * support* (`FailureNote`, user-experience.md §5.8) rather than saying it in the sentence.
   */
  readonly sentence?: string;
  /** The catalogue code the sentence was chosen by, for the same fold. */
  readonly code?: string;
  /** Path from `field_errors[]` to the sentence for it, for a form to put beside its inputs. */
  readonly fields: ReadonlyMap<string, string>;
  /**
   * The request id, when there is one that is not already in the sentence. Shown where a person
   * can copy it - an internal error without its reference is a support thread nobody can trace.
   */
  readonly reference?: string;
  /** The request id whenever there is one, in the sentence or not: what a copy control copies. */
  readonly requestId?: string;
  /** Whether this is the server's fault. What decides whether a retry is offered as the answer. */
  readonly isServerFault: boolean;
}

/**
 * What a failure that never reached the server says. These are conditions of the client's own
 * making, so they have no server code - and they are codes in the one catalogue rather than
 * sentences written here (ADR-0011).
 */
const KIND_CODES = {
  timeout: 'app.request_timed_out',
  offline: 'app.offline',
  malformed: 'app.answer_unreadable',
} as const;

/** The codes to try, most specific first. */
function candidates(error: TransportError): string[] {
  const out: string[] = [];
  if (error.detailCode) out.push(error.detailCode);
  if (error.code) {
    // The contract's codes are bare - `validation_failed` - and the catalogue keys them under
    // `errors.`; a code that already carries its group is left alone.
    out.push(error.code.includes('.') ? error.code : `errors.${error.code}`);
  }
  if (error.kind !== 'problem') out.push(KIND_CODES[error.kind]);
  // The floor, and which floor depends on whether there is a reference to quote. `errors.internal`
  // is the server's sentence and it *contains* the request id - offering it without one puts a
  // literal `{request_id}` on the screen, which is what a gateway answering 502 with an empty body
  // does. That case gets a sentence of its own instead.
  out.push(error.requestId ? 'errors.internal' : 'app.something_went_wrong');
  return out;
}

/**
 * The parameters as a reader reads them. A parameter named `*_at` is a moment - a spent AI budget's
 * `renews_at`, a running access's `ends_at` - and a message cannot format one, so it is written in
 * the reader's locale here, before the sentence takes it. A `permission` is named as the role editor
 * names it, a rate limit's `level` by what it guards (user-experience.md §5.8); a key this build has
 * no name for stays as it arrived, which is still truer than nothing.
 */
function readable(params: Readonly<Record<string, string>> | undefined, messages: Messages): MessageParams {
  const out: Record<string, string> = {};
  for (const [name, value] of Object.entries(params ?? {})) {
    out[name] = name.endsWith('_at') ? formatDateTime(value, messages.locale)
      : name === 'permission' ? named(messages, `app.permissions.name.${value.toLowerCase()}`, value)
      : name === 'level' ? named(messages, `app.limit_level.${value}`, value)
      : value;
  }
  return out;
}

function named(messages: Messages, code: string, fallback: string): string {
  return messages.has(code) ? messages.t(code) : fallback;
}

/**
 * When to try again, from `Retry-After`: "in 2 minutes", or "in a moment" when the server said
 * nothing - a sentence that asks the reader to wait carries `{retry_at}` and must never show it.
 */
function retryAt(error: TransportError, messages: Messages, now: number): string {
  if (error.retryAfterMs === undefined) return messages.t('app.retry_soon');
  return formatRelative(new Date(now + Math.max(error.retryAfterMs, 60_000)).toISOString(), messages.locale, now);
}

/** The statuses whose answer is worth quoting: a refusal (audited as denied), a conflict or quota, a server fault. */
function quotable(status: number | undefined): boolean {
  return status === 403 || (status ?? 0) >= 409;
}

export function renderProblem(error: TransportError, messages: Messages, now: number = Date.now()): RenderedProblem {
  // The request id travels as a parameter as well as a field, because `errors.internal` renders it
  // inside its own sentence. Where it does, showing it again beneath would say it twice.
  const params: MessageParams = {
    retry_at: retryAt(error, messages, now),
    ...readable(error.params, messages),
    ...(error.requestId ? { request_id: error.requestId } : {}),
  };

  const code = candidates(error).find((candidate) => messages.has(candidate)) ?? 'errors.internal';
  let message = messages.t(code, params);
  // `errors.internal` says its reference inside its own sentence; the fold says it instead.
  const sentence = code === 'errors.internal' ? messages.t('app.something_went_wrong') : message;
  // A refusal or a failure the person may take to an administrator carries its reference in the
  // sentence, so every screen shows it whether or not it renders `reference`: the administrator
  // finds the request's entries by it (`GET /audit?request_id=`). A 404 or a plain 400 is not one.
  if (error.requestId && quotable(error.status) && !message.includes(error.requestId)) {
    message = messages.t('app.problem_with_reference', { message, request_id: error.requestId });
  }

  const fields = new Map<string, string>();
  for (const field of error.fieldErrors) {
    if (!field.path) continue;
    const fieldCode = field.code ?? 'usecase.input_invalid';
    fields.set(field.path, messages.t(fieldCode, field.params ?? {}));
  }

  const isServerFault = error.kind !== 'problem' || (error.status ?? 0) >= 500;
  const reference = error.requestId && !message.includes(error.requestId) ? error.requestId : undefined;

  return { message, sentence, code, fields, reference, requestId: error.requestId, isServerFault };
}

/** A refusal a job ended with: the code and parameters `GET /jobs/{id}` answers. */
export interface Refusal {
  readonly code: string;
  readonly params?: Readonly<Record<string, string>>;
}

/**
 * The sentence of a refusal a job met after it was accepted - a spent AI budget, AI switched off.
 * The same catalogue and the same rule for moments as a refusal on a request, so a budget spent
 * before the job ran reads exactly as one spent before the ask: what is used up, and when it renews.
 */
export function renderRefusal(refusal: Refusal, messages: Messages): string {
  const code = messages.has(refusal.code) ? refusal.code : 'app.something_went_wrong';
  return messages.t(code, readable(refusal.params, messages));
}
