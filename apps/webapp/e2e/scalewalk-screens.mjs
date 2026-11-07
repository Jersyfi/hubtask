// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// What scenario S7 of test/load/PLAN.md records per screen of the walk: the script and style
// bytes the browser fetched for it, raw and gzip-compressed as the binary serves them, and the
// time until the screen settled.

import { gzipSync } from 'node:zlib';

/** A screen's path with its identifiers masked, so the three workspaces' screens line up. */
export function screenOf(path) {
  return String(path).replace(/[0-9a-f]{8}-[0-9a-f-]{27}/g, '?');
}

/**
 * Tallies the files one screen fetched: `files` is { kind: 'script' | 'stylesheet', body } per
 * response. Another kind is not counted.
 */
export function tally(files) {
  const out = { script_bytes: 0, script_gzip_bytes: 0, style_bytes: 0, style_gzip_bytes: 0, files: 0 };
  for (const { kind, body } of files) {
    const prefix = kind === 'script' ? 'script' : kind === 'stylesheet' ? 'style' : undefined;
    if (!prefix) continue;
    out.files += 1;
    out[`${prefix}_bytes`] += body.length;
    out[`${prefix}_gzip_bytes`] += gzipSync(body).length;
  }
  return out;
}
