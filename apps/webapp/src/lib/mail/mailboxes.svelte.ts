// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The connected mailboxes of the mail page (ADR-0094): read once, changed by its dialogs, and the
 * provider's answer handed on. Held by the page rather than by its *Mailboxes* section, because the
 * head's *Connect a mailbox* and the answer on `/administration/mail/oauth` act while a phone shows
 * the list of sections and no section at all.
 */

import { announcer } from '../announce.svelte.ts';
import {
  checkMailbox,
  completeMailboxAuthorization,
  readMailboxes,
  removeMailbox,
  type Mailbox,
  type MailboxOAuthProvider,
  type MailboxPreset,
  type MailboxState,
} from '../data/mailintake.svelte.ts';
import { messages, t } from '../i18n/i18n.svelte.ts';
import { renderProblem } from '../problem.ts';

export const STATE_TONE: Record<MailboxState, 'success' | 'warning' | 'danger' | 'neutral'> = {
  reading: 'success',
  failed_once: 'warning',
  failing: 'danger',
  off: 'neutral',
};

/** How a mailbox is reached, in one line: the protocol, the server, the sign-in. */
export function wayOf(box: Mailbox): string {
  const server = box.host ?? box.session_url;
  return [
    box.protocol,
    ...(server ? [server] : []),
    ...(box.auth === 'OAUTH' ? [t('app.mail.via_oauth', { provider: t(`app.mail.oauth_provider.${box.oauth_provider ?? 'custom'}`) })] : []),
  ].join(' · ');
}

/** A mailbox's own record page (page-system §9.3). */
export const mailboxPath = (id: string): string => `/administration/mail/mailboxes/${encodeURIComponent(id)}`;

/** A sender rule as the page and its dialog hold it: a sender or `@domain`, and a collection. */
export interface SenderRule {
  readonly match: string;
  readonly collection: string;
}

export class MailboxList {
  mailboxes = $state<readonly Mailbox[]>([]);
  presets = $state<readonly MailboxPreset[]>([]);
  oauthProviders = $state<readonly MailboxOAuthProvider[]>([]);
  redirectUrl = $state('');
  isReading = $state(true);
  readFailure = $state<string | undefined>(undefined);
  isCompleting = $state(false);
  oauthFailure = $state<string | undefined>(undefined);
  /** The mailbox an act is running for. */
  working = $state<string | undefined>(undefined);
  rowFailure = $state<string | undefined>(undefined);
  /** The mailbox whose disconnecting is asked. */
  removing = $state<Mailbox | undefined>(undefined);
  /** Whether *Connect a mailbox* is open. */
  isAdding = $state(false);

  async read(): Promise<void> {
    try {
      const read = await readMailboxes();
      this.presets = read.presets;
      this.oauthProviders = read.oauth_providers ?? [];
      this.redirectUrl = read.oauth_redirect_url ?? '';
      this.mailboxes = read.mailboxes;
    } catch (error) {
      this.readFailure = renderProblem(error as never, messages).message;
    } finally {
      this.isReading = false;
    }
  }

  /** Hands the provider's answer on once; `done` drops the code and the state from the address. */
  async complete(answer: Readonly<Record<string, string>>, done: () => void): Promise<void> {
    this.oauthFailure = undefined;
    this.isCompleting = true;
    try {
      const connected = await completeMailboxAuthorization({
        state: answer['state'] ?? '',
        ...(answer['code'] ? { code: answer['code'] } : {}),
        ...(answer['error'] ? { error: answer['error'] } : {}),
      });
      this.mailboxes = [...this.mailboxes.filter((each) => each.id !== connected.id), connected];
      announcer.say(t('app.mail.mailbox_connected', { address: connected.address }));
    } catch (error) {
      this.oauthFailure = renderProblem(error as never, messages).message;
    } finally {
      this.isCompleting = false;
      done();
    }
  }

  add(box: Mailbox): void {
    this.mailboxes = [...this.mailboxes, box];
  }

  async check(box: Mailbox): Promise<void> {
    this.rowFailure = undefined;
    this.working = box.id;
    try {
      const checked = await checkMailbox(box.id);
      this.mailboxes = this.mailboxes.map((each) => (each.id === box.id ? checked : each));
      announcer.say(checked.failure ? t(checked.failure) : t('app.mail.mailbox_ok', { address: box.address }));
    } catch (error) {
      this.rowFailure = renderProblem(error as never, messages).message;
    } finally {
      this.working = undefined;
    }
  }

  /** Disconnecting is asked in a Dialog S (PS-6). */
  askRemoval(box: Mailbox): void {
    this.removing = box;
  }

  /** Whether disconnecting succeeded: a mailbox's own page leaves for the list then. */
  async remove(box: Mailbox): Promise<boolean> {
    this.rowFailure = undefined;
    this.working = box.id;
    try {
      await removeMailbox(box.id);
      this.mailboxes = this.mailboxes.filter((each) => each.id !== box.id);
      announcer.say(t('app.mail.mailbox_removed', { address: box.address }));
      this.removing = undefined;
      return true;
    } catch (error) {
      this.rowFailure = renderProblem(error as never, messages).message;
      return false;
    } finally {
      this.working = undefined;
    }
  }
}
