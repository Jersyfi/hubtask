// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What a sign-in screen is allowed to know before anybody has signed in.
 *
 * `GET /auth/sign-in-rules` is public the way sign-in is, and the workspace is resolved from the
 * host exactly as it is there - never from a body. It answers four things and deliberately no
 * more: which methods this workspace signs in with, which providers, what a password has to meet,
 * and the links its operator is obliged to show.
 *
 * **Why a public route at all.** F4 refused one that would have existed only to hide a button, and
 * that reasoning stands. This one exists because a *configurable* hint would otherwise be wrong:
 * a screen that says "at least twelve characters" in a workspace that demands fifteen is a screen
 * that lies, and the password is refused after the person has typed it. What it does not answer is
 * everything a guesser could use - the expiry, the history, the timeouts, the lists.
 *
 * **It is offered only where the installation serves it.** `/meta/capabilities` answers which
 * optional parts of an installation are configured, and `features.sign_in_rules` is this one. Until
 * a server declares it, nothing here is read and nothing is asked - the sign-in screen is exactly
 * the screen it was, with no probe of a route that does not exist. That is the same rule every
 * other optional part of the product follows: what an installation permits is read, never compiled
 * in.
 *
 * **The check is not a keystroke.** `POST /auth/password:check` costs Argon2 comparisons, so it is
 * asked only once the local rules hold, only after the typing stops, and only once per value. The
 * same proof the *setting* demands is presented: without it the route would be an oracle that told
 * anybody whether a word is on a blocklist or in somebody else's history.
 */

import { TransportError } from '@hubtask/sync-engine';

import { manifest } from './capabilities.svelte.ts';
import { engine } from './engine.ts';
import {
  RULES_BEFORE_READING,
  localRulesHold,
  type PasswordContext,
  type PasswordRules,
  type SignInRules,
} from './signinrules.ts';

const RULES = '/auth/sign-in-rules';
const CHECK = '/auth/password:check';

/** How long the typing has to stop before the server is asked. */
const QUIET_MS = 400;
/** What a check answers: the ids of the rules this password is refused by. */
interface CheckAnswer {
  readonly violations?: readonly { readonly rule: string }[];
}

/** The proof a password change carries, whichever of the four doors it came through. */
export type PasswordProof =
  | { readonly kind: 'bearer'; readonly stepUpToken?: string }
  | { readonly kind: 'pending'; readonly token: string }
  | { readonly kind: 'invitation'; readonly token: string }
  | { readonly kind: 'reset'; readonly token: string };

function proofBody(proof: PasswordProof): Record<string, string> {
  switch (proof.kind) {
    case 'bearer':
      return proof.stepUpToken ? { step_up_token: proof.stepUpToken } : {};
    case 'pending':
      return { pending_token: proof.token };
    case 'invitation':
      return { invitation_token: proof.token };
    case 'reset':
      return { reset_token: proof.token };
  }
}

class SignInRulesStore {
  #rules = $state<SignInRules | undefined>(undefined);

  /** What was checked last, by value: the same candidate is never sent twice. */
  #answers = new Map<string, readonly string[]>();
  #checking = $state<string | undefined>(undefined);
  #answered = $state<string | undefined>(undefined);
  #violations = $state<readonly string[]>([]);
  #unreachable = $state(false);
  #timer: ReturnType<typeof setTimeout> | undefined;
  #generation = 0;

  /** The whole answer, or nothing while it is in flight. */
  get rules(): SignInRules | undefined {
    return this.#rules;
  }

  /** The password half, with the least a screen can draw until the real answer lands. */
  get password(): PasswordRules {
    return this.#rules?.password ?? RULES_BEFORE_READING;
  }

  get providers(): SignInRules['providers'] {
    return this.#rules?.providers ?? [];
  }

  get legal(): SignInRules['legal'] {
    return this.#rules?.legal ?? {};
  }

  /**
   * Whether this installation serves the sign-in surface at all.
   *
   * Everything this module adds is behind it: the rules under a password field, the forgotten-
   * password link, the list of providers. A screen that offered a link to a route answering `404`
   * would be worse than the screen without it.
   */
  get isServed(): boolean {
    return manifest.value?.features?.['sign_in_rules'] === true;
  }

  /** Whether the answer is in hand, which is what the screens draw from. */
  get wasRead(): boolean {
    return this.#rules !== undefined;
  }

  /** Whether this workspace still signs in with a password at all. */
  get hasPassword(): boolean {
    return this.#rules === undefined || this.#rules.methods.includes('PASSWORD');
  }

  get workspaceHost(): string | undefined {
    return this.#rules?.workspace_host;
  }

  get isChecking(): boolean {
    return this.#checking !== undefined;
  }

  get isUnreachable(): boolean {
    return this.#unreachable;
  }

  /** Whether an answer for this exact password is in hand. */
  isAnswered(password: string): boolean {
    return this.#answered === password;
  }

  /** The rules this password was refused by, as ids - empty until an answer names some. */
  violationsFor(password: string): readonly string[] {
    return this.#answered === password ? this.#violations : [];
  }

  /**
   * Reads the rules once.
   *
   * A failure is silence rather than a message: the screen has a sensible list to draw and a form
   * that works, and "the rules could not be read" is a sentence nobody can act on. The server
   * refuses a password that breaks a rule either way.
   *
   * Hands back the unsubscribe, as every read here does, so a screen that is left stops listening.
   * Every caller subscribes: the engine holds one entry per path and hands the answer it already
   * has to a second listener, and a guard here would leave the *second* screen with no read at all
   * once the first had unsubscribed.
   */
  read(): () => void {
    if (!this.isServed) return () => {};
    return engine.subscribe<SignInRules>({ path: RULES }, (next) => {
      if (next.status === 'ready') this.#rules = next.data;
    });
  }

  /**
   * Asks the server about this password, once the local rules hold and the typing has stopped.
   *
   * Every earlier answer is kept by value, so walking back over a password already checked shows
   * its answer at once. A generation counter drops the answer to a value that is no longer in the
   * field - the request cannot be recalled, but its answer can be ignored.
   */
  check(password: string, context: PasswordContext, proof: PasswordProof): void {
    clearTimeout(this.#timer);
    if (!this.isServed) return;
    const candidate = password;
    if (!localRulesHold(this.password, candidate, context)) {
      this.#checking = undefined;
      this.#answered = undefined;
      this.#violations = [];
      return;
    }
    const remembered = this.#answers.get(candidate);
    if (remembered !== undefined) {
      this.#checking = undefined;
      this.#answered = candidate;
      this.#violations = remembered;
      return;
    }
    this.#checking = candidate;
    this.#answered = undefined;
    const generation = ++this.#generation;
    this.#timer = setTimeout(() => void this.#ask(candidate, proof, generation), QUIET_MS);
  }

  /** Forgets what was checked. Called when a screen is left, so nothing outlives its field. */
  forget(): void {
    clearTimeout(this.#timer);
    this.#generation += 1;
    this.#answers.clear();
    this.#checking = undefined;
    this.#answered = undefined;
    this.#violations = [];
    this.#unreachable = false;
  }

  async #ask(password: string, proof: PasswordProof, generation: number): Promise<void> {
    try {
      const answer = await engine.mutate<CheckAnswer>('POST', CHECK, {
        password,
        ...proofBody(proof),
      });
      if (generation !== this.#generation) return;
      const violations = (answer.violations ?? []).map((entry) => entry.rule);
      this.#answers.set(password, violations);
      this.#violations = violations;
      this.#answered = password;
      this.#unreachable = false;
    } catch (cause) {
      if (generation !== this.#generation) return;
      // A refusal of the *proof* is not an answer about the password, and neither is an outage.
      // Both leave the server's lines unchecked, and the send decides.
      this.#unreachable = !(cause instanceof TransportError) || cause.status === undefined || cause.status >= 500;
      this.#answered = undefined;
      this.#violations = [];
    } finally {
      if (generation === this.#generation) this.#checking = undefined;
    }
  }
}

export const signInRules = new SignInRulesStore();
