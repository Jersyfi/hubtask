// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/** The credentials held at boot, until their screen takes them. */
const heldAtBoot = new Map<string, string>();

/** The screens a mail links with a credential in the fragment (routes.ts). */
const credentialScreens = new Set(['/redeem', '/reset', '/confirm-address']);

/**
 * A credential that arrives in the fragment, read once and removed from the address.
 *
 * The invitation mail and the reset mail both link `<base>/<screen>#token=…`, and the fragment is
 * the server's decision rather than a screen's: it is never sent to any server, never reaches an
 * access log, and never travels in a `Referer` to whatever rendered the mail. The care owed in the
 * other direction is this: read it once, and replace the history entry before the first request
 * leaves, so that Back, a bookmark and the address bar are not carrying a live credential.
 *
 * Three screens do exactly this, which is why it is one function rather than three copies. The
 * boot takes the credential off first (holdFragmentCredentials), so the screen reads it from there.
 */
export function takeFragmentToken(name = 'token'): string {
  const kept = heldAtBoot.get(name);
  if (kept !== undefined) {
    heldAtBoot.delete(name);
    return kept;
  }
  const fragment = new URLSearchParams(location.hash.replace(/^#/, ''));
  const held = fragment.get(name) ?? '';
  if (held !== '') {
    // Replaced rather than pushed: a Back that returned to the address with the credential in it
    // would put the credential back in the address bar.
    history.replaceState(null, '', location.pathname + location.search);
  }
  return held;
}


/**
 * holdFragmentCredentials takes the fragment's credentials off the address at boot, before the
 * application's first request (the manifest's) leaves; the screen that mounts later takes them
 * from here. Only on the screens a mail links with one: any other fragment is not ours.
 */
export function holdFragmentCredentials(): void {
  if (!credentialScreens.has(location.pathname)) return;
  const fragment = new URLSearchParams(location.hash.replace(/^#/, ''));
  let found = false;
  for (const name of ['token', 'connect']) {
    const value = fragment.get(name);
    if (value) {
      heldAtBoot.set(name, value);
      found = true;
    }
  }
  if (found) history.replaceState(null, '', location.pathname + location.search);
}
