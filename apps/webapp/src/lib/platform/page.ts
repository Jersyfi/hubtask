// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What every target does the way a page does it, until a shell needs its own: one place, so the
 * targets cannot drift apart on it. A shell that changes one of these writes its own in its
 * implementation instead of a condition here.
 */

/**
 * An object URL and a click, which is the only way a page saves bytes it already has. The URL is
 * revoked immediately afterwards: it is a handle to memory, and one left behind keeps the whole file
 * alive for the life of the document.
 */
export function saveByLink(bytes: Blob, fileName: string): void {
  const url = URL.createObjectURL(bytes);
  const link = document.createElement('a');
  link.href = url;
  link.download = fileName;
  document.body.append(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
}

/**
 * `navigator.languages` is the ordered list the reader configured; `language` is the first of them
 * and is the fallback where the list is missing. A webview reports the system's languages.
 */
export function pageLanguages(): readonly string[] {
  return navigator.languages?.length ? [...navigator.languages] : [navigator.language].filter(Boolean);
}
