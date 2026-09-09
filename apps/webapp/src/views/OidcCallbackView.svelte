<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Where the provider sends the browser back (H-04).
  //
  // **This address is the server's, not this screen's.** `cmd/server/main.go` derives
  // `<base>/auth/callback` as the redirect URI and registers it with the provider; nothing about
  // where the code comes back is taken from a request. So the path is fixed, and this is the
  // screen that has been missing under it since the flow shipped.
  //
  // **The code and the state leave the address before anything is sent.** An authorization code is
  // a credential for one exchange, and one left in the address bar is one in the history entry, in
  // a bookmark, and in the `Referer` of whatever the reader clicks next. `replaceState` rather
  // than `pushState`, for the same reason `RedeemView` uses it: a Back that returned to the
  // address with the code in it would put the code back.
  //
  // **Nothing here is offered twice.** A single-use `state` means a reload is not a retry, and the
  // way out of every failure is the same one sentence and a way back to the sign-in screen.

  import { Banner, Spinner, Stack } from '@hubtask/design-system/components';

  import { oidc } from '../lib/data/oidc.svelte.ts';
  import { readArrival } from '../lib/data/oidc.ts';
  import { t } from '../lib/i18n/i18n.svelte.ts';

  interface Props {
    /** Where to go once there is a session. The frame's router, as every other view takes it. */
    onnavigate?: (path: string) => void;
  }

  let { onnavigate }: Props = $props();

  /**
   * The address, read once and then cleaned.
   *
   * `$state` rather than a derivation over `location`: the value has to survive the very
   * `replaceState` that removes it.
   */
  const arrival = $state(takeArrival());

  function takeArrival() {
    const read = readArrival(location.search);
    if (location.search !== '') history.replaceState(null, '', location.pathname);
    return read;
  }

  $effect(() => {
    if (arrival.kind === 'handoff') {
      void oidc.complete(arrival.handoff).then((ok) => ok && onnavigate?.('/'));
    } else {
      // The provider refused, or there is no flow in this address at all. Both end here, and
      // saying which would tell somebody standing at the callback whether a handle they do not
      // hold was ever real.
      oidc.refuse();
    }
    return () => oidc.forget();
  });
</script>

<div class="screen">
  <Stack gap="300">
    <h1>{t('app.callback.title')}</h1>

    {#if oidc.failure}
      <!-- The server's own code: a spent or unknown `state`, a provider that could not be
           reached, a workspace whose provider is switched off. -->
      <Banner tone="danger">{t(oidc.failure)}</Banner>
      <p><a href="/">{t('app.callback.back')}</a></p>
    {:else}
      <p class="quiet">
        <Spinner /> {t('app.callback.working')}
      </p>
    {/if}
  </Stack>
</div>

<style>
  /* Rule 4: a column that grows with its text and stops before it becomes a line nobody can read. */
  .screen { max-width: 60ch; }

  h1 {
    margin: 0;
    font-family: var(--font-display);
    font-size: var(--fs-400);
    font-weight: var(--fw-semibold);
    line-height: var(--lh-tight);
  }

  .quiet {
    margin: 0;
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    color: var(--text-secondary);
  }
</style>
