// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The workspace's own sign-in rule, and what the installation has already decided for it.
 *
 * **Three levels, one shape.** Every switch answers the same three things: what is in force here,
 * what the installation set as the default, and whether a lock is on it. A locked switch is
 * *shown* - with its value and with who locked it - rather than hidden, because a setting that
 * simply is not there is a setting somebody opens a support ticket about.
 *
 * **A workspace tightens; it never loosens.** The server is what enforces that, field by field,
 * and answers the refusal against the field. What this module does is not offer the impossible:
 * a locked switch is not editable here either, so the common case never becomes a round trip.
 *
 * The lock carries its origin so the sentence beside it can be the true one. Today there are two
 * origins - the installation and, once there are plans, the plan a workspace is on - and a screen
 * that had to guess between them would be a screen that tells somebody to ask the wrong person.
 */

import { TransportError } from '@hubtask/sync-engine';

import { engine } from './engine.ts';
import { messages } from '../i18n/i18n.svelte.ts';
import { renderProblem, type RenderedProblem } from '../problem.ts';

const TENANT = '/tenant';

/** Where a lock comes from, or `null` where there is none. */
export type LockOrigin = 'INSTANCE' | 'PLAN' | null;

/** One switch: what is in force, what the level above set, and whether it may be changed here. */
export interface Setting<T> {
  readonly value: T;
  /** What the installation (or the plan) set as the default. */
  readonly installation: T;
  readonly lock: LockOrigin;
}

/** The eighteen, in the three groups the settings screen draws. */
export interface SignInPolicy {
  readonly password: {
    readonly min_length: Setting<number>;
    readonly min_lowercase: Setting<number>;
    readonly min_uppercase: Setting<number>;
    readonly min_digits: Setting<number>;
    readonly min_symbols: Setting<number>;
    readonly min_classes: Setting<number>;
    readonly max_repeat: Setting<number | null>;
    readonly common_passwords: Setting<boolean>;
    readonly context_words: Setting<boolean>;
    readonly breach_check: Setting<boolean>;
    readonly max_age_days: Setting<number | null>;
    readonly history_count: Setting<number>;
    readonly min_age_hours: Setting<number | null>;
  };
  readonly mfa_required_for: Setting<'NONE' | 'ADMINS' | 'EVERYONE'>;
  readonly methods: Setting<readonly string[]>;
  readonly session: {
    readonly max_days: Setting<number>;
    readonly idle_minutes: Setting<number | null>;
  };
  /** The four links, each with its own lock: B2C locks them, B2B leaves them open. */
  readonly legal: {
    readonly imprint_url: Setting<string>;
    readonly privacy_url: Setting<string>;
    readonly terms_url: Setting<string>;
    readonly accessibility_url: Setting<string>;
  };
  /** When everybody was last asked for a new password, or `null`. Set by the action, never typed. */
  readonly rotation_from: string | null;
}

interface Workspace {
  readonly display_name: string;
  readonly slug: string;
  readonly version: number;
  readonly sign_in_policy?: SignInPolicy;
}

class SignInPolicyStore {
  #workspace = $state<Workspace | undefined>(undefined);
  #problem = $state<RenderedProblem | undefined>(undefined);
  #working = $state(false);
  #saved = $state(false);
  #opened = false;

  get policy(): SignInPolicy | undefined {
    return this.#workspace?.sign_in_policy;
  }

  get workspaceName(): string | undefined {
    return this.#workspace?.display_name;
  }

  get isWorking(): boolean {
    return this.#working;
  }

  get wasSaved(): boolean {
    return this.#saved;
  }

  get problem(): RenderedProblem | undefined {
    return this.#problem;
  }

  open(): () => void {
    if (this.#opened) return () => {};
    this.#opened = true;
    return engine.subscribe<Workspace>({ path: TENANT }, (next) => {
      if (next.status === 'ready') this.#workspace = next.data;
    });
  }

  /**
   * Writes the switches that changed.
   *
   * Merge-patch, so an omitted switch is left alone - which is what makes it safe for two
   * administrators to change two different things without one of them reverting the other.
   */
  async save(changes: Record<string, unknown>): Promise<boolean> {
    this.#working = true;
    this.#problem = undefined;
    this.#saved = false;
    try {
      this.#workspace = await engine.mutate<Workspace>('PATCH', TENANT, { sign_in_policy: changes });
      this.#saved = true;
      return true;
    } catch (cause) {
      this.#problem = cause instanceof TransportError
        ? renderProblem(cause, messages)
        : { message: messages.t('errors.internal', {}), fields: new Map(), isServerFault: true };
      return false;
    } finally {
      this.#working = false;
    }
  }

  /**
   * Asks everybody for a new password, from now.
   *
   * Its own call rather than a field, because it is an event rather than a setting: it sets a
   * moment, and every password older than it meets `PASSWORD_CHANGE` at the next sign-in while
   * every session opened before it ends. Nothing is written into anybody's row - which is what
   * makes it instant for ten accounts and for ten thousand.
   */
  async requireNewPasswords(): Promise<boolean> {
    return this.save({ rotation_from: 'now' });
  }
}

export const signInPolicy = new SignInPolicyStore();
