// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// What a call can fail with, as values rather than as strings.
//
// The shape follows the server's: RFC 9457 problem documents carry a `code` plus `params`, never a
// finished sentence (ADR-0011), and the client renders them (F1-07). So the error a caller catches
// carries the code, and the sentence is somebody else's job - which is what stops an English string
// being baked into the one place every request passes through.

/** The reason a call did not produce an answer. */
export type FailureKind =
  /** The server answered, and the answer was a problem document. */
  | 'problem'
  /** The deadline passed, or the caller abandoned the call. */
  | 'timeout'
  /** Nothing answered: no network, DNS, a refused connection. */
  | 'offline'
  /** The server answered something this client cannot read. */
  | 'malformed';

/**
 * TransportError is what every Transport failure becomes, so a caller has one thing to catch.
 *
 * `code` is the server's message code where there was one; the renderer resolves it against
 * `locales/*.json`. A transport that invented a sentence here would put display text in the one
 * layer that must not have any.
 */
/**
 * One entry of `field_errors[]`: which field, and what is wrong with it as a code.
 *
 * `path` is the server's, and it is how the frame puts the message under the right input rather
 * than at the top of the form (ADR-0025, api-guidelines.md §3). A form that showed them all in one
 * list would make the reader match five sentences to twelve fields by hand.
 */
export interface FieldProblem {
  readonly path?: string;
  readonly code?: string;
  readonly params?: Readonly<Record<string, string>>;
}

export class TransportError extends Error {
  readonly kind: FailureKind;
  readonly status?: number;
  readonly code?: string;
  /**
   * The more specific code, where the server sent one. `code` is the contract's category -
   * `validation_failed` - and this is what actually happened; a client that has a message for it
   * says something useful, and one that does not falls back to the category (ADR-0025).
   */
  readonly detailCode?: string;
  readonly params?: Readonly<Record<string, string>>;
  /** Empty rather than absent when the server sent none, so a caller never has to guard twice. */
  readonly fieldErrors: readonly FieldProblem[];
  /** The server's correlation id, when it sent one. It is what a support request is answered by. */
  readonly requestId?: string;

  constructor(kind: FailureKind, init: {
    status?: number;
    code?: string;
    detailCode?: string;
    params?: Readonly<Record<string, string>>;
    fieldErrors?: readonly FieldProblem[];
    requestId?: string;
    cause?: unknown;
  } = {}) {
    // The message is for a developer reading a stack trace, never for a person reading a screen -
    // it is the code and the status, not a sentence.
    super(`${kind}${init.status ? ` ${init.status}` : ''}${init.code ? ` ${init.code}` : ''}`, {
      cause: init.cause,
    });
    this.name = 'TransportError';
    this.kind = kind;
    this.status = init.status;
    this.code = init.code;
    this.detailCode = init.detailCode;
    this.params = init.params;
    this.fieldErrors = init.fieldErrors ?? [];
    this.requestId = init.requestId;
  }

  /** Whether trying the same call again could plausibly succeed. */
  get isRetryable(): boolean {
    if (this.kind === 'timeout' || this.kind === 'offline') return true;
    // 429 and 5xx are the server saying "not now" rather than "not ever".
    return this.status === 429 || (this.status !== undefined && this.status >= 500);
  }
}
