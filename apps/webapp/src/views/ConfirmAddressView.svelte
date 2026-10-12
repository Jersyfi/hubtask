<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The link that confirms a managed account's new address, opened (UC-ID-20 check 7).
  //
  // **The token arrives in the fragment**, is read once, and leaves the address before anything is
  // sent, the reset link's care. **Nothing happens on opening**: the reader presses *Confirm*, so a
  // mail program that opens links to look at them confirms nothing. The link proves the mailbox
  // and signs nobody in; afterwards the card says what changed and leads to the sign-in.

  import { Banner, Button, Stack } from '@hubtask/design-system/components';

  import SignInCard from '../lib/signin/SignInCard.svelte';
  import { takeFragmentToken } from '../lib/signin/fragmentToken.ts';
  import { password as passwordApi } from '../lib/data/password.svelte.ts';
  import { t } from '../lib/i18n/i18n.svelte.ts';

  interface Props {
    onnavigate?: (path: string) => void;
  }

  let { onnavigate }: Props = $props();

  /** `$state` rather than a derivation: the value has to survive the `replaceState` that hides it. */
  const token = $state(takeFragmentToken());
  let confirmed = $state(false);

  async function confirm(): Promise<void> {
    confirmed = await passwordApi.confirmAddress(token);
  }
</script>

{#snippet notice()}
  {#if passwordApi.problem}
    <Banner tone="danger" title={passwordApi.problem.message}>
      {#if passwordApi.problem.reference}{passwordApi.problem.reference}{/if}
    </Banner>
  {/if}
{/snippet}

<SignInCard title={t('app.confirm_address.title')} {notice}>
  <Stack gap="200">
    {#if confirmed}
      <Banner tone="success">{t('app.confirm_address.done')}</Banner>
      <div>
        <Button tone="primary" onclick={() => onnavigate?.('/')}>{t('app.confirm_address.onward')}</Button>
      </div>
    {:else if token === ''}
      <!-- Reached without a link, or with the fragment lost on the way: nothing to confirm, and
           saying more would tell a probe whether a token was real. -->
      <Banner tone="warning">{t('app.confirm_address.no_token')}</Banner>
      <div>
        <Button tone="secondary" onclick={() => onnavigate?.('/')}>{t('app.password.back_to_sign_in')}</Button>
      </div>
    {:else}
      <p class="lead">{t('app.confirm_address.intro')}</p>
      <Button
        tone="primary"
        isFull
        isBusy={passwordApi.isWorking}
        busyLabel={t('app.confirm_address.working')}
        onclick={() => void confirm()}
      >
        {t('app.confirm_address.submit')}
      </Button>
    {/if}
  </Stack>
</SignInCard>

<style>
  .lead { margin: 0; }
</style>
