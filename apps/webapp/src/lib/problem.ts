// SPDX-License-Identifier: BUSL-1.1
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

import type { MessageParams } from './i18n/format.ts';
import type { Messages } from './i18n/messages.ts';

export interface RenderedProblem {
  /** The one sentence to show. Never empty: every branch below ends in a message. */
  readonly message: string;
  /** Path from `field_errors[]` to the sentence for it, for a form to put beside its inputs. */
  readonly fields: ReadonlyMap<string, string>;
  /**
   * The request id, when there is one that is not already in the sentence. Shown where a person
   * can copy it - an internal error without its reference is a support thread nobody can trace.
   */
  readonly reference?: string;
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

export function renderProblem(error: TransportError, messages: Messages): RenderedProblem {
  // The request id travels as a parameter as well as a field, because `errors.internal` renders it
  // inside its own sentence. Where it does, showing it again beneath would say it twice.
  const params: MessageParams = { ...error.params, ...(error.requestId ? { request_id: error.requestId } : {}) };

  const code = candidates(error).find((candidate) => messages.has(candidate)) ?? 'errors.internal';
  const message = messages.t(code, params);

  const fields = new Map<string, string>();
  for (const field of error.fieldErrors) {
    if (!field.path) continue;
    const fieldCode = field.code ?? 'usecase.input_invalid';
    fields.set(field.path, messages.t(fieldCode, field.params ?? {}));
  }

  const isServerFault = error.kind !== 'problem' || (error.status ?? 0) >= 500;
  const reference = error.requestId && !message.includes(error.requestId) ? error.requestId : undefined;

  return { message, fields, reference, isServerFault };
}
