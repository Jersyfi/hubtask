// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The contact file of RFC 9116 for hubtask.eu, with the same two channels SECURITY.md names.
//
// Built rather than kept as a static file because the RFC requires an `Expires` field and asks
// for it to be less than a year ahead: a hand-written date would expire silently, a built one is
// renewed by every deploy of the site.

export const canonical = 'https://hubtask.eu/.well-known/security.txt';

const contacts = [
  'https://github.com/Jersyfi/hubtask/security/advisories/new',
  'mailto:security@hubtask.eu',
];

const policy = 'https://github.com/Jersyfi/hubtask/blob/main/SECURITY.md';

/** One day short of a year, so a file served on its last day is still within the RFC's advice. */
const validForMs = 364 * 24 * 60 * 60 * 1000;

/** The file's text as built at `builtAt`. */
export function securityTxt(builtAt: Date): string {
  const expires = new Date(builtAt.getTime() + validForMs);
  expires.setUTCHours(0, 0, 0, 0);
  return [
    ...contacts.map((contact) => `Contact: ${contact}`),
    `Expires: ${expires.toISOString().replace('.000Z', 'Z')}`,
    'Preferred-Languages: en',
    `Canonical: ${canonical}`,
    `Policy: ${policy}`,
    '',
  ].join('\n');
}
