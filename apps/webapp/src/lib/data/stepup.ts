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

/** What the refusal said would be accepted: the four ways of ADR-0075, each where the account holds it. */
export type StepUpMethod = 'PASSWORD' | 'TOTP' | 'RECOVERY' | 'PROVIDER';

const KNOWN: readonly StepUpMethod[] = ['PASSWORD', 'TOTP', 'RECOVERY', 'PROVIDER'];

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
  ask: (methods: readonly StepUpMethod[], provider?: string) => Promise<string | undefined>,
  held: () => string | undefined = () => undefined,
): Promise<T> {
  try {
    return await call();
  } catch (first) {
    if (!(first instanceof TransportError) || !first.needsStepUp) throw first;

    // A grant held from the provider's round trip (ADR-0075 §2) answers the first call that asks
    // for a proof - only one that asks, so a call that needed none never spends it. Refused, it
    // was not what this call wanted, and the person is asked as if there had been none.
    let cause: TransportError = first;
    const carried = held();
    if (carried !== undefined) {
      try {
        return await call(carried);
      } catch (again) {
        if (!(again instanceof TransportError) || !again.needsStepUp) throw again;
        cause = again;
      }
    }

    const token = await ask(methodsOf(cause), providerOf(cause));
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
 * accounts a step-up protects (issue 544). A refusal without the parameter is an older server,
 * which named the password for everybody; one that names an empty list is an account that signs in
 * only through a provider and holds no factor, and the prompt says so instead of drawing a field. A name this client does not
 * know is dropped rather than shown: a prompt with a field nobody can fill is worse than one
 * field fewer.
 */
export function methodsOf(cause: TransportError): readonly StepUpMethod[] {
  const named = cause.params?.methods;
  if (typeof named !== 'string') return ['PASSWORD'];
  // Named, and empty: the account holds neither a password nor a factor (UC-ID-05 check 5).
  if (named.trim() === '') return [];
  const methods = named
    .split(/[\s,]+/)
    .map((method) => method.trim().toUpperCase())
    .filter((method): method is StepUpMethod => (KNOWN as readonly string[]).includes(method));
  return methods.length > 0 ? methods : ['PASSWORD'];
}

/** The provider a `PROVIDER` proof goes to, as the refusal names it — what "Confirm with …" says. */
export function providerOf(cause: TransportError): string | undefined {
  const named = cause.params?.provider;
  return typeof named === 'string' && named.trim() !== '' ? named : undefined;
}

/**
 * What survives the trip to the provider and back (ADR-0075 §2).
 *
 * The browser leaves this document for the provider and comes back to a fresh one at the callback,
 * so two things are written down in the tab's own storage: where the person was, and — after the
 * return — the grant the server answered, until the one action it is for consumes it. The tab's,
 * because the session this proves is the tab's too. Taken rather than read: a second callback is not
 * a second step-up, and a grant is one action's.
 */
type Store = Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>;

const RETURN = 'hubtask.step_up.return';
const GRANT = 'hubtask.step_up.grant';

export interface StepUpReturn {
  readonly returnTo: string;
  readonly provider: string;
}

export interface HeldGrant {
  readonly token: string;
  readonly expiresAt: string;
  readonly provider: string;
}

/**
 * How long the note that a step-up left for the provider is believed: the server's flow lives ten
 * minutes, and a note older than that is a trip abandoned with Back, which must not turn the tab's
 * next provider sign-in into a step-up.
 */
const RETURN_LIFETIME_MS = 10 * 60_000;

/** Whether a step-up is on its way back from the provider - the callback's question. */
export function isReturning(store: Store, now: number): boolean {
  const raw = store.getItem(RETURN);
  if (raw === null) return false;
  try {
    const left = (JSON.parse(raw) as { at?: number }).at;
    if (typeof left === 'number' && now - left < RETURN_LIFETIME_MS) return true;
  } catch {
    // Unreadable is not a trip.
  }
  store.removeItem(RETURN);
  return false;
}

/** Notes where to come back to before the browser leaves for the provider, and when it left. */
export function rememberReturn(store: Store, returnTo: string, provider: string, now: number): void {
  store.setItem(RETURN, JSON.stringify({ returnTo, provider, at: now }));
}

/** Forgets a trip to the provider: the tab signed out, or the callback found it was a sign-in after all. */
export function forgetReturn(store: Store): void {
  store.removeItem(RETURN);
}

/**
 * The way back, once. Only a path of this application is followed: a value that named another origin
 * would turn a step-up into a redirect somebody else chose.
 */
export function takeReturn(store: Store): StepUpReturn | undefined {
  const raw = store.getItem(RETURN);
  store.removeItem(RETURN);
  if (raw === null) return undefined;
  try {
    const read = JSON.parse(raw) as Partial<StepUpReturn>;
    const path = typeof read.returnTo === 'string' && read.returnTo.startsWith('/') && !read.returnTo.startsWith('//')
      ? read.returnTo
      : '/';
    return { returnTo: path, provider: typeof read.provider === 'string' ? read.provider : '' };
  } catch {
    return undefined;
  }
}

/** Holds the grant the provider's round trip earned, for the one action it is for. */
export function holdGrant(store: Store, grant: HeldGrant): void {
  store.setItem(GRANT, JSON.stringify(grant));
}

/** The held grant, if there is one still inside its window — without consuming it. */
export function heldGrant(store: Store, now: number): HeldGrant | undefined {
  const raw = store.getItem(GRANT);
  if (raw === null) return undefined;
  try {
    const read = JSON.parse(raw) as HeldGrant;
    if (typeof read.token === 'string' && Date.parse(read.expiresAt) > now) return read;
  } catch {
    // Unreadable is gone.
  }
  store.removeItem(GRANT);
  return undefined;
}

/** The held grant's token, consumed: the action it is presented to is the only one it covers. */
export function takeGrant(store: Store, now: number): string | undefined {
  const held = heldGrant(store, now);
  store.removeItem(GRANT);
  return held?.token;
}
