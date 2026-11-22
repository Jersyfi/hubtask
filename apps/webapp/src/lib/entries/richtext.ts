// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What a rich text draws when the writer leaves it. A value that arrived while the focus was in it
 * was held back so as not to draw over the typing; it is taken now only when the writer changed
 * nothing - otherwise the text holds their own change, which the commit on leaving saves.
 */
export function catchUpOnBlur(value: string, current: string, atFocus: string): string | undefined {
  return current === atFocus && value !== current ? value : undefined;
}
