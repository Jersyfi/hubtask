// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The step-up's two decisions, without the state that renders them (H-03).
 *
 * Here rather than in the store for this directory's usual reason: what is worth a test is the
 * shape of the recovery — try, ask once, retry once, never twice — and a rune cannot be run under
 * `node --test`. The store holds the pending prompt and hands `ask` to this.
 */

import { TransportError } from '@hubtask/sync-engine';

/** What the refusal said would be accepted. */
export type StepUpMethod = 'PASSWORD' | 'TOTP';

/**
 * Runs a call, and where the server refuses it for want of a proof, asks for one and runs it again.
 *
 * **Once.** A second `auth.step_up_required` after a fresh grant is the server saying the grant
 * was not what it wanted, and asking a third time would be a dialog somebody cannot escape. The
 * original refusal is what is thrown in that case, because it is the one that describes the
 * operation rather than the proof.
 *
 * A refusal the reader closes throws the refusal too: the operation did not happen, and saying so
 * with the server's own words is more use than a sentence this client invented about cancelling.
 */
export async function withStepUp<T>(
  call: (stepUpToken?: string) => Promise<T>,
  ask: (methods: readonly StepUpMethod[]) => Promise<string | undefined>,
): Promise<T> {
  try {
    return await call();
  } catch (cause) {
    if (!(cause instanceof TransportError) || !cause.needsStepUp) throw cause;

    const token = await ask(methodsOf(cause));
    if (token === undefined) throw cause;
    return call(token);
  }
}

/**
 * Which methods the refusal named, defaulting to the password.
 *
 * The server names them in the problem's parameters as a space-separated list — `PASSWORD TOTP`
 * for an account with a factor armed, `PASSWORD` for one without — and the contract says so at
 * `POST /auth/step-up`. Splitting on commas as well costs nothing and is what this once did
 * alone, which read the whole list as one unknown name and offered the password to exactly the
 * accounts a step-up protects (issue 544). A refusal that names none still has to produce a
 * usable prompt, and the password is the one every account has. A name this client does not
 * know is dropped rather than shown: a prompt with a field nobody can fill is worse than one
 * field fewer.
 */
export function methodsOf(cause: TransportError): readonly StepUpMethod[] {
  const named = cause.params?.methods;
  if (typeof named !== 'string' || named.trim() === '') return ['PASSWORD'];
  const methods = named
    .split(/[\s,]+/)
    .map((method) => method.trim().toUpperCase())
    .filter((method): method is StepUpMethod => method === 'PASSWORD' || method === 'TOTP');
  return methods.length > 0 ? methods : ['PASSWORD'];
}
