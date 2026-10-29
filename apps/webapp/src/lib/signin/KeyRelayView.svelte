<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The workspace's page an installed app opens in the system browser for a passkey step-up
  // (ADR-0096 decision 5): the passkey belongs to this host, so the browser asks for it here and
  // hands the answer to the app's loopback address. The options came in the fragment, which leaves
  // the address before anything else happens; nothing is sent from this page to the server.

  import { untrack } from 'svelte';

  import { Banner, Button, Stack } from '@hubtask/design-system/components';

  import { ceremonyEndOf, keyAnswerOf, keyRequestOptionsOf, type AssertedCredential } from '../data/passkeys.ts';
  import { t } from '../i18n/i18n.svelte.ts';
  import SignInCard from './SignInCard.svelte';
  import { answerUrl, declinedUrl, readRelay } from './keyRelay.ts';

  const relay = readRelay(location.hash);
  if (location.hash !== '') history.replaceState(history.state, '', location.pathname);

  let isAsking = $state(false);
  let failed = $state(false);

  async function ask(): Promise<void> {
    if (relay === undefined || isAsking) return;
    isAsking = true;
    failed = false;
    try {
      const credential = await navigator.credentials.get(keyRequestOptionsOf(relay.options));
      if (credential !== null) {
        location.assign(answerUrl(relay, keyAnswerOf(credential as unknown as AssertedCredential)));
        return;
      }
    } catch (cause) {
      // Closed by the person: nothing said, the button there for another try (P-11).
      if (ceremonyEndOf(cause) !== 'cancelled') failed = true;
    }
    isAsking = false;
  }

  function decline(): void {
    if (relay !== undefined) location.assign(declinedUrl(relay));
  }

  // Asked at once, as the app's dialog promised; a browser that wants a press first refuses
  // quietly, and the button is that press.
  $effect(() => untrack(() => void ask()));
</script>

<SignInCard title={t('app.step_up.title')} lead={relay ? t('app.step_up.relay_lead') : undefined}>
  {#if relay === undefined}
    <Banner tone="warning">{t('app.step_up.relay_nothing')}</Banner>
  {:else}
    {#if failed}<Banner tone="danger">{t('app.sign_in.key_failed')}</Banner>{/if}
    <Stack gap="100">
      <Button tone="primary" icon="key" isFull isBusy={isAsking} busyLabel={t('app.sign_in.key_waiting')} onclick={() => void ask()}>
        {t('app.step_up.with_key')}
      </Button>
      <Button tone="subtle" isFull onclick={decline}>{t('app.step_up.relay_decline')}</Button>
    </Stack>
  {/if}
</SignInCard>
