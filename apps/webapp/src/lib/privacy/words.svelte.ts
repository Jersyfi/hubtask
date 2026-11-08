// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/** A privacy case's words, shared by the list, a case's page and their dialogs (`data-protection.md` §4). */

import { accounts } from '../data/accounts.svelte.ts';
import type { Request } from '../data/privacy.svelte.ts';
import { workspace } from '../data/workspace.svelte.ts';
import { formatDateTime, formatDue } from '../i18n/datetime.ts';
import { messages, t } from '../i18n/i18n.svelte.ts';
import { deviceZone, todayIn } from '../i18n/zone.ts';

export const kindWord = (kind: string): string => t(`app.privacy.kind_${kind.toLowerCase()}`);
export const statusWord = (status: string): string => t(`app.privacy.status_${status.toLowerCase()}`);
export const statusTone = (status: string): 'success' | 'neutral' | 'info' =>
  status === 'COMPLETED' ? 'success' : status === 'REJECTED' ? 'neutral' : 'info';

/** Who the case is about: the resolved name, the address it named, or nobody nameable. */
export const subjectOf = (request: Request): string =>
  accounts.nameOf(request.subject_account_id) ?? request.subject_email ?? t('app.privacy.subject_unknown');

export const when = (at: string | null | undefined): string | undefined => (at ? formatDateTime(at, messages.locale) : undefined);

/** A calendar day as the reader's locale writes one, never shifted by a zone: it has none. */
export const day = (value: string): string => formatDue(`${value}T12:00:00Z`, messages.locale, 'UTC', { allDay: true });

/** The workspace's zone: a deadline named as a day ends at the end of that day there. Read `workspace.open()` first. */
export const zoneOf = (): string => workspace.workspace?.default_time_zone ?? deviceZone();

/**
 * Today in the workspace's zone, as YYYY-MM-DD: the latest day somebody can have been told. The
 * server bounds the day there, so the device's own calendar would be refused around midnight.
 */
export const today = (): string => todayIn(zoneOf());
