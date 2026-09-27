<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The second half of "I forgot my password": the link from the mail, opened.
  //
  // **The token arrives in the fragment**, is read once, and leaves the address before anything is
  // sent - the same care `RedeemView` takes, in the one function both now share.
  //
  // **A reset does not walk past a second factor.** Where the account has one, the server answers
  // the second step rather than a session, and the sign-in screen finishes it. Control of a
  // mailbox is one proof; it does not replace the one the account already demanded.
  //
  // **One field, not two.** The eye is what replaces "repeat it", and the rules under the field
  // are the workspace's own, live - so nobody learns what was wrong by pressing the button.

  import { Banner, Button, Stack } from '@hubtask/design-system/components';

  import PasswordField from '../lib/signin/PasswordField.svelte';
  import SignInCard from '../lib/signin/SignInCard.svelte';
  import { takeFragmentToken } from '../lib/signin/fragmentToken.ts';
  import { password as passwordApi } from '../lib/data/password.svelte.ts';
  import { signInRules } from '../lib/data/signinrules.svelte.ts';
  import { t } from '../lib/i18n/i18n.svelte.ts';

  interface Props {
    /** Where to go once there is a session, as the other signed-out screens take it. */
    onnavigate?: (path: string) => void;
  }

  let { onnavigate }: Props = $props();

  /** `$state` rather than a derivation: the value has to survive the `replaceState` that hides it. */
  const token = $state(takeFragmentToken());
  let newPassword = $state('');

  $effect(() => signInRules.read());

  async function submit(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    const signedIn = await passwordApi.reset(token, newPassword);
    newPassword = '';
    if (signedIn) onnavigate?.('/');
  }
</script>

{#snippet notice()}
  {#if passwordApi.problem}
    <Banner tone="danger" title={passwordApi.problem.message}>
      {#if passwordApi.problem.reference}{passwordApi.problem.reference}{/if}
    </Banner>
  {/if}
{/snippet}

{#if token === ''}
  <SignInCard title={t('app.password.reset_title')} {notice}>
    <!-- Reached without a link, or with one already spent and the fragment lost on the way.
         Either way there is nothing to reset, and saying which would tell a probe whether the
         token was real. -->
    <Stack gap="200">
      <Banner tone="warning">{t('app.password.reset_no_token')}</Banner>
      <div>
        <Button tone="secondary" onclick={() => onnavigate?.('/')}>
          {t('app.password.back_to_sign_in')}
        </Button>
      </div>
    </Stack>
  </SignInCard>
{:else}
  <SignInCard title={t('app.password.reset_title')} {notice}>
    <form onsubmit={submit}>
      <Stack gap="200">
        <PasswordField
          bind:value={newPassword}
          label={t('app.password.new_label')}
          context={{ workspaceHost: signInRules.workspaceHost }}
          proof={{ kind: 'reset', token }}
          hint={t('app.redeem.password_hint')}
        />
        <Button
          type="submit"
          tone="primary"
          isFull
          isBusy={passwordApi.isWorking}
          busyLabel={t('app.password.reset_setting')}
        >
          {t('app.password.reset_submit')}
        </Button>
      </Stack>
    </form>
  </SignInCard>
{/if}

<style>
  form { margin: 0; }
</style>
