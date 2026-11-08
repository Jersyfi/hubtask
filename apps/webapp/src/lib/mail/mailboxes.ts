// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/** A connected mailbox in words: how it is reached and the tone of its state. */

import type { Mailbox, MailboxState } from '../data/mailintake.svelte.ts';
import { t } from '../i18n/i18n.svelte.ts';

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

/** A sender rule as the page and its dialog hold it: a sender or `@domain`, and a collection. */
export interface SenderRule {
  readonly match: string;
  readonly collection: string;
}
