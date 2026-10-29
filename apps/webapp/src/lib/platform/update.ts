// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What the installed apps say about a newer release (UC-SYN-07 checks 5 and 6, ADR-0097): the
 * desktop's updater answers through the shell, the Android app reads the release's manifest
 * itself. Pure, so both are tested without a shell.
 */

/** The project's releases, where every app is published with its checksum. */
export const RELEASES = 'https://github.com/Jersyfi/hubtask/releases';

/** What a check found that the person is told about. */
export type AppUpdate =
  /** Downloaded and checked under the update key; installed once the person agrees. */
  | { readonly state: 'ready'; readonly version: string }
  /** A newer release to download: the Android app and the `.deb`, which install nothing themselves. */
  | { readonly state: 'available'; readonly version: string }
  /** The manifest or the package does not carry the project's signature; nothing was installed. */
  | { readonly state: 'refused'; readonly version: string; readonly refused: 'manifest' | 'package' };

/** The release's own page, where the download stands beside its checksum. */
export function releasePage(version: string): string {
  return `${RELEASES}/tag/v${productVersion(version)}`;
}

/** The product version a person reads: the stamped `1.3.0+57` without its build counter. */
export function productVersion(version: string): string {
  return version.split('+')[0] ?? version;
}

const SEMVER = /^(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z.-]+))?(?:\+[0-9A-Za-z.-]+)?$/;

/** Whether `candidate` is a later version than `current`, by semantic versioning's precedence. */
export function isNewer(candidate: string, current: string): boolean {
  const a = SEMVER.exec(candidate);
  const b = SEMVER.exec(current);
  if (!a || !b) return false;
  for (let part = 1; part <= 3; part++) {
    const difference = Number(a[part]) - Number(b[part]);
    if (difference !== 0) return difference > 0;
  }
  return comparePre(a[4], b[4]) > 0;
}

/** A release without a pre-release part comes after one with it; identifiers compare in turn. */
function comparePre(a: string | undefined, b: string | undefined): number {
  if (a === b) return 0;
  if (a === undefined) return 1;
  if (b === undefined) return -1;
  const left = a.split('.');
  const right = b.split('.');
  for (let at = 0; at < Math.max(left.length, right.length); at++) {
    const x = left[at];
    const y = right[at];
    if (x === undefined) return -1;
    if (y === undefined) return 1;
    if (x === y) continue;
    const numeric = /^\d+$/;
    if (numeric.test(x) && numeric.test(y)) return Number(x) - Number(y);
    if (numeric.test(x)) return -1;
    if (numeric.test(y)) return 1;
    return x < y ? -1 : 1;
  }
  return 0;
}

/**
 * The Android app's notice from the release's manifest (`latest.json`, versioning-release.md §8):
 * a newer version when the manifest names one with an APK, nothing otherwise - a manifest that
 * cannot be read says nothing rather than something wrong. The download opens the release's page,
 * never an address the manifest gives, so the manifest can at most name a version.
 */
export function noticeFrom(manifest: string, current: string): AppUpdate | undefined {
  let read: unknown;
  try {
    read = JSON.parse(manifest);
  } catch {
    return undefined;
  }
  if (typeof read !== 'object' || read === null) return undefined;
  const { version, platforms } = read as { version?: unknown; platforms?: Record<string, unknown> };
  if (typeof version !== 'string' || !SEMVER.test(version)) return undefined;
  const android = platforms?.['android'] as { url?: unknown } | undefined;
  if (typeof android?.url !== 'string') return undefined;
  return isNewer(version, current) ? { state: 'available', version: productVersion(version) } : undefined;
}

/** The desktop shell's answer (`update_check`, `src-tauri/src/update.rs`) as what the page says. */
export function fromShell(fields: Readonly<Record<string, string>>): AppUpdate | undefined {
  const version = productVersion(fields['version'] ?? '');
  switch (fields['state']) {
    case 'ready':
      return { state: 'ready', version };
    case 'available':
      return { state: 'available', version };
    case 'refused':
      return { state: 'refused', version, refused: fields['refused'] === 'package' ? 'package' : 'manifest' };
    default:
      return undefined;
  }
}
