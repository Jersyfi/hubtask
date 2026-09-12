<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The moment an invited account becomes a person (H-01).
  //
  // **The token arrives in the fragment, and that is the server's decision rather than this
  // screen's.** `DeliverNotification.go` links `<base>/redeem#token=…`: a fragment is never sent
  // to any server, never reaches an access log, and never travels in a `Referer` to whatever the
  // invitation mail was rendered by. This screen owes it the same care in the other direction —
  // read once, and replaced in the history entry before the first request leaves, so that a
  // reader who presses Back, or bookmarks the page, is not carrying a live credential around.
  //
  // The password policy that binds is the server's (`security.md` §5, twelve characters). It is
  // checked here as well, and that is not a second policy: it is the same number said early, so
  // that somebody is told before a round trip rather than after. What the server refuses, the
  // server's own sentence says.

  import { Banner, Button, Input, Stack } from '@hubtask/design-system/components';

  import { t } from '../lib/i18n/i18n.svelte.ts';
  import { session } from '../lib/session.svelte.ts';

  interface Props {
    /**
     * Where to go once there is a session. The frame's router, as the OIDC callback takes it.
     *
     * The screen has to send the person on itself: once there is a session, `/redeem` is no
     * longer a screen - `App.svelte` renders it only while signed out - and a person left on the
     * address would be signed in and looking at "nothing here". An invited person has no path
     * to return to, so the start is where they go.
     */
    onnavigate?: (path: string) => void;
  }

  let { onnavigate }: Props = $props();

  /** `security.md` §5's floor, said here so that it is said before the round trip. */
  const MINIMUM_LENGTH = 12;

  let password = $state('');
  let repeated = $state('');
  let tooShort = $state(false);
  let mismatched = $state(false);

  const isBusy = $derived(session.status === 'verifying');

  /**
   * The token, taken from the fragment once and then removed from the address.
   *
   * `$state` rather than a `$derived` over `location`: the value has to survive the very
   * `replaceState` that removes it, and a derivation over the address would answer nothing the
   * moment the address stopped carrying it.
   */
  const token = $state(readToken());

  function readToken(): string {
    const fragment = new URLSearchParams(location.hash.replace(/^#/, ''));
    const held = fragment.get('token') ?? '';
    if (held !== '') {
      // Replaced rather than pushed: a Back that returned to the address with the credential in
      // it would put the credential back in the address bar.
      history.replaceState(null, '', location.pathname + location.search);
    }
    return held;
  }

  async function submit(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    tooShort = password.length < MINIMUM_LENGTH;
    mismatched = password !== repeated;
    if (tooShort || mismatched) return;

    const signedIn = await session.redeem(token, password);
    // Out of the component's state whatever happened.
    password = '';
    repeated = '';
    if (signedIn) onnavigate?.('/');
  }
</script>

<div class="screen">
  <Stack gap="300">
    <h1>{t('app.redeem.title')}</h1>

    {#if token === ''}
      <!-- Reached without a link, or with one that has already been used and the fragment lost on
           the way. Either way there is nothing to redeem, and saying which would tell a probe
           whether the token was real. -->
      <Banner tone="warning">{t('app.redeem.no_token')}</Banner>
    {:else}
      <p class="quiet">{t('app.redeem.intro')}</p>

      {#if session.problem}
        <Banner tone="danger" title={session.problem.message}>
          {#if session.problem.reference}{session.problem.reference}{/if}
        </Banner>
      {/if}

      <form onsubmit={submit}>
        <Stack gap="200">
          <Input
            label={t('app.redeem.password_label')}
            hint={t('app.redeem.password_hint')}
            error={tooShort ? t('app.redeem.password_short') : undefined}
            bind:value={password}
            type="password"
            autocomplete="new-password"
            spellcheck={false}
            isRequired
          />
          <Input
            label={t('app.redeem.repeat_label')}
            error={mismatched ? t('app.redeem.repeat_mismatch') : undefined}
            bind:value={repeated}
            type="password"
            autocomplete="new-password"
            spellcheck={false}
            isRequired
          />
          <div>
            <Button type="submit" tone="primary" {isBusy} busyLabel={t('app.redeem.working')}>
              {t('app.redeem.submit')}
            </Button>
          </div>
        </Stack>
      </form>
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

  .quiet { margin: 0; color: var(--text-secondary); }

  form { margin: 0; }
</style>
