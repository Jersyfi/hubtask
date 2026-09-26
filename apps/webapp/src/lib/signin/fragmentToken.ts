// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A credential that arrives in the fragment, read once and removed from the address.
 *
 * The invitation mail and the reset mail both link `<base>/<screen>#token=…`, and the fragment is
 * the server's decision rather than a screen's: it is never sent to any server, never reaches an
 * access log, and never travels in a `Referer` to whatever rendered the mail. The care owed in the
 * other direction is this: read it once, and replace the history entry before the first request
 * leaves, so that Back, a bookmark and the address bar are not carrying a live credential.
 *
 * Two screens do exactly this, which is why it is one function rather than two copies.
 */
export function takeFragmentToken(name = 'token'): string {
  const fragment = new URLSearchParams(location.hash.replace(/^#/, ''));
  const held = fragment.get(name) ?? '';
  if (held !== '') {
    // Replaced rather than pushed: a Back that returned to the address with the credential in it
    // would put the credential back in the address bar.
    history.replaceState(null, '', location.pathname + location.search);
  }
  return held;
}
