<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The second half of "I forgot my password": the link from the mail, opened.
  //
  // **The token arrives in the fragment**, is read once, and leaves the address before anything is
  // sent - the same care `RedeemView` takes, in the one function both now share.
  //
  // **A reset does not walk past a second factor.** Where the account has one, the server answers
  // the second step rather than a session, and the sign-in card finishes it - the session store
  // carries the step, so this screen hands the reader to the card rather than drawing a second
  // copy of it. Control of a mailbox is one proof; it does not replace the one the account already
  // demanded. Staying on this form after the `202` was the defect SC-03 closed: the link was spent
  // and nothing on the screen said what came next.
  //
  // **One field, not two.** The eye is what replaces "repeat it", and the rules under the field
  // are the workspace's own, live - so nobody learns what was wrong by pressing the button.
  //
  // **Where the password is off, the link connects a provider instead** (SC-33, ADR-0078 §1). The
  // mail then links `#connect=…`, and this card offers the workspace's providers, each starting the
  // flow with the link bound to it on the server: the link and a fresh sign-in at the provider are
  // the account's proof. No password field - the workspace takes none - and the second factor, where
  // the account has one, is the sign-in card's next step as after every provider return.

  import { Banner, Button, Stack } from '@hubtask/design-system/components';

  import PasswordField from '../lib/signin/PasswordField.svelte';
  import ProviderMark from '../lib/signin/ProviderMark.svelte';
  import SignInCard from '../lib/signin/SignInCard.svelte';
  import { takeFragmentToken } from '../lib/signin/fragmentToken.ts';
  import { oidc } from '../lib/data/oidc.svelte.ts';
  import { password as passwordApi } from '../lib/data/password.svelte.ts';
  import { signInRules } from '../lib/data/signinrules.svelte.ts';
  import { t } from '../lib/i18n/i18n.svelte.ts';
  import { session } from '../lib/session.svelte.ts';

  interface Props {
    /** Where to go once there is a session, as the other signed-out screens take it. */
    onnavigate?: (path: string) => void;
  }

  let { onnavigate }: Props = $props();

  /** `$state` rather than a derivation: the value has to survive the `replaceState` that hides it. */
  const token = $state(takeFragmentToken());
  /** The connect link, where the mail carried one instead of a reset (read the same way, once). */
  const connect = $state(takeFragmentToken('connect'));
  let newPassword = $state('');

  /**
   * Hands the browser to one of the workspace's providers with the connect link bound to the flow.
   * A link that cannot be used is refused before the browser leaves, and said on this card.
   */
  async function useProvider(providerId: string): Promise<void> {
    const url = await oidc.begin(undefined, providerId, undefined, connect);
    // `assign`, as the invitation card does: Back returns here, with the link already out of the
    // address.
    if (url) location.assign(url);
  }

  $effect(() => signInRules.read());

  async function submit(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    const signedIn = await passwordApi.reset(token, newPassword);
    newPassword = '';
    if (signedIn) onnavigate?.('/');
    // The account demands a step the password could not give: the card at the sign-in address
    // draws it, and a successful step there lands on the overview.
    else if (session.secondFactorOwed) onnavigate?.('/sign-in');
  }
</script>

{#snippet notice()}
  {#if oidc.failure}
    <!-- The server's own code: a link that cannot be used, or a provider that is not there. -->
    <Banner tone="danger">{t(oidc.failure)}</Banner>
  {:else if passwordApi.problem}
    <Banner tone="danger" title={passwordApi.problem.message}>
      {#if passwordApi.problem.reference}{passwordApi.problem.reference}{/if}
    </Banner>
  {/if}
{/snippet}

{#if connect !== ''}
  <SignInCard title={t('app.password.connect_title')} lead={t('app.password.connect_intro')} {notice}>
    {#if signInRules.providers.length > 0}
      <!-- One button per provider the rules name, each carrying this link (ADR-0078 §1). -->
      <Stack gap="100">
        {#each signInRules.providers as provider (provider.id)}
          <Button
            tone="primary"
            isFull
            isBusy={oidc.isHandingOverTo(provider.id)}
            busyLabel={t('app.sign_in.provider_working')}
            onclick={() => void useProvider(provider.id)}
          >
            {#snippet lead()}
              <ProviderMark kind={provider.kind} name={provider.display_name} />
            {/snippet}
            {t('app.sign_in.provider_named', { name: provider.display_name })}
          </Button>
        {/each}
      </Stack>
    {:else if signInRules.wasRead}
      <!-- No provider the rules name: the sign-in card says what there is. -->
      <Button tone="secondary" onclick={() => onnavigate?.('/')}>
        {t('app.password.back_to_sign_in')}
      </Button>
    {/if}
  </SignInCard>
{:else if token === ''}
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
