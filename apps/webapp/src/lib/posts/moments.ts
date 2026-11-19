// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A post's moments as its writer types them: a day and a time on this device's clock, sent as an
 * instant. A field left empty or unreadable is no moment at all, never "now".
 */

const pad = (n: number) => String(n).padStart(2, '0');

/** The instant a day (`2026-11-16`) and a time (`08:00`) name on this device's clock. */
export function atLocal(day: string, time: string): string | undefined {
  if (!/^\d{4}-\d{2}-\d{2}$/u.test(day) || !/^\d{2}:\d{2}$/u.test(time)) return undefined;
  const at = new Date(`${day}T${time}:00`);
  return Number.isNaN(at.getTime()) ? undefined : at.toISOString();
}

/** The day an instant falls on, on this device's clock, as a date field holds it. */
export function dayOf(iso: string): string {
  const at = new Date(iso);
  if (Number.isNaN(at.getTime())) return '';
  return `${at.getFullYear()}-${pad(at.getMonth() + 1)}-${pad(at.getDate())}`;
}

/** The time of day of an instant on this device's clock, as a time field holds it. */
export function timeOf(iso: string): string {
  const at = new Date(iso);
  if (Number.isNaN(at.getTime())) return '';
  return `${pad(at.getHours())}:${pad(at.getMinutes())}`;
}
