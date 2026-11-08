// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The backup screens' words: a schedule's rule as what it means, a moment, a server code as its
 * sentence. Shared by the settings page, a target's record page and their dialogs.
 */

import type { Archive, Run, Schedule, Target } from '../data/backup.svelte.ts';
import { readRule } from '../data/backup.ts';
import { formatDateTime } from '../i18n/datetime.ts';
import { messages, t } from '../i18n/i18n.svelte.ts';

export const when = (at: string | null | undefined): string | undefined => (at ? formatDateTime(at, messages.locale) : undefined);

/** A server code as a sentence, or the code where it has none. */
export const sentence = (code: string | null | undefined): string | undefined => (code ? (messages.has(code) ? t(code) : code) : undefined);

/** The rule as what it means. Never as when it next fires — that is `next_run_at`, the server's. */
export function ruleSentence(schedule: Schedule): string {
  const reading = readRule(schedule.rrule);
  if (reading.partial || !reading.frequency) return schedule.rrule;
  const at = reading.at
    ? t('app.backup.rule_at', { time: `${String(reading.at.hour).padStart(2, '0')}:${String(reading.at.minute).padStart(2, '0')}` })
    : '';
  const days = reading.weekdays.map((day) => t(`app.backup.weekday_${day.toLowerCase()}`)).join(', ');
  const every =
    reading.interval === 1
      ? t(`app.backup.rule_${reading.frequency.toLowerCase()}`)
      : t(`app.backup.rule_every_${reading.frequency.toLowerCase()}`, { interval: String(reading.interval) });
  return [every, days, at].filter((part) => part !== '').join(' ');
}

/** When a schedule next runs, or why it does not: off, or a rule that is spent rather than broken. */
export function nextRun(schedule: Schedule): string {
  if (schedule.next_run_at) return t('app.backup.next_run', { at: when(schedule.next_run_at) ?? '' });
  return schedule.enabled === false ? t('app.backup.no_next_run_off') : t('app.backup.no_next_run_spent');
}

/** What the last test of a target found, in words. */
export function testedWord(target: Target): string {
  if (!target.last_test_at) return t('app.backup.never_tested');
  return target.last_test_ok
    ? t('app.backup.tested_ok', { at: when(target.last_test_at) ?? '' })
    : t('app.backup.tested_failed', { at: when(target.last_test_at) ?? '' });
}

/**
 * An archive's state, the claim it supports: unfinished is not damaged (a run still going, or one
 * that died), and "opens" is only said of one this installation has checked.
 */
export function archiveState(archive: Archive, run: Run | undefined): { label: string; tone: 'success' | 'warning' | 'danger' | 'neutral' } {
  if (archive.complete === false) return { label: t('app.backup.archive_incomplete'), tone: 'warning' };
  if (run?.verify_ok === true) return { label: t('app.backup.archive_opens'), tone: 'success' };
  if (run?.verify_ok === false) return { label: t('app.backup.archive_broken'), tone: 'danger' };
  return { label: t('app.backup.archive_unchecked'), tone: 'neutral' };
}

/** The newest moment any of these archives was checked, from the runs this tab has read. */
export function lastVerified(archives: readonly Archive[], runOf: (id: string) => Run | undefined): string | undefined {
  const stamps = archives.map((archive) => runOf(archive.archive_id)?.verified_at).filter((stamp): stamp is string => typeof stamp === 'string');
  return stamps.sort().at(-1);
}
