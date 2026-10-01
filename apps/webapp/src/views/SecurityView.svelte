<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The three things an account's own security is made of: the password, the second factor, and
  // the codes that get somebody back in when the factor is gone.
  //
  // **Changing the password asks for the step-up rather than for the old one.** Presenting the old
  // password *is* what a step-up is; a field for it beside one would be asking twice for a single
  // thing (3.3.7). What the change costs is said before it happens: every other session ends.
  //
  // **The recovery codes are shown the way every other one-time secret in this product is** -
  // `OneTimeSecret`, with reveal, copy and an acknowledgement, which is what the component exists
  // for. They used to be a plain list with no way to copy them, which is a set of ten codes
  // somebody has to retype by hand at the worst possible moment.
  //
  // **Whether one is armed decides what this screen offers.** `GET /accounts/me` answers
  // `has_second_factor`, and everything below the password hangs off it. Before it was answered,
  // this screen could not tell an account with no authenticator from one whose codes had all been
  // spent, and so it showed somebody with no authenticator a red "none left", a button to make new
  // codes the server would refuse to make, and a way to switch off a factor they did not have. An
  // action that cannot be taken is not offered here, in any of the three places - the whole panel
  // is absent, and what stands in its place says what would put it there.
  //
  // **Taking it off asks for the password again**, which is the one case where being signed in is
  // not enough: a stolen session removing the factor is exactly the attack the factor exists
  // against (`security.md` §5). And it says what it costs before it is opened, because "turn it
  // off" on its own names neither the thing nor the consequence.

  import { canCopy } from '@hubtask/design-system/components';
  import { Banner, Button, EmptyState, Input, OneTimeSecret, Stack } from '@hubtask/design-system/components';

  import SettingsHead from '../lib/frame/SettingsHead.svelte';
  import TotpEnrollment from '../lib/frame/TotpEnrollment.svelte';
  import PasswordField from '../lib/signin/PasswordField.svelte';

  import { actor } from '../lib/data/account.svelte.ts';
  import { mfa } from '../lib/data/mfa.svelte.ts';
  import { password as passwordApi } from '../lib/data/password.svelte.ts';
  import { signInRules } from '../lib/data/signinrules.svelte.ts';
  import { announcer } from '../lib/announce.svelte.ts';
  import { t } from '../lib/i18n/i18n.svelte.ts';

  const account = $derived(actor.account);

  // Whether a second factor is armed. The bundle ships inside the binary that answers this
  // (ADR-0028), so the field is never missing from a server this client is talking to; `=== true`
  // is nevertheless what is asked, because the permissive reading of an unanswered question is
  // what put three refusals on this screen.
  const armed = $derived(actor.hasSecondFactor === true);

  // How many recovery codes are left. Answered only where a factor is armed, so it needs no
  // defence of its own: the panel that reads it is inside that branch.
  const left = $derived(actor.recoveryCodesLeft);

  let disablePassword = $state('');
  let newPassword = $state('');
  let notice = $state<string | undefined>(undefined);

  // The rules this account's workspace applies, so the list under the field is this workspace's.
  $effect(() => signInRules.read());

  async function changePassword(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    const candidate = newPassword;
    newPassword = '';
    if (await passwordApi.change(candidate)) {
      notice = t('app.password.changed');
      announcer.say(notice);
    }
  }

  async function newCodes(): Promise<void> {
    if (await mfa.regenerate()) {
      announcer.say(t('auth.recovery_regenerated'));
      // The count moved. Nothing pushes it, so it is read again rather than guessed at.
      await actor.reread();
    }
  }

  async function armedFactor(): Promise<void> {
    notice = t('app.mfa.armed');
    announcer.say(notice);
    await actor.reread();
  }

  async function disableFactor(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    const password = disablePassword;
    disablePassword = '';
    if (await mfa.disable(password)) {
      notice = t('app.mfa.disabled');
      announcer.say(notice);
      await actor.reread();
    }
  }
</script>

{#if !account}
  <EmptyState kind="filtered" title={t('app.profile.signed_out')} />
{:else}
  <Stack gap="300">
    <SettingsHead row="security" />

    <div class="panel">
      <Stack gap="150">
        <h2>{t('app.password.change_title')}</h2>
        <p class="quiet">{t('app.password.change_body')}</p>
        {#if passwordApi.problem}
          <Banner tone="danger" title={passwordApi.problem.message}>
            {#if passwordApi.problem.reference}{passwordApi.problem.reference}{/if}
          </Banner>
        {/if}
        <form onsubmit={changePassword}>
          <Stack gap="150">
            <PasswordField
              bind:value={newPassword}
              label={t('app.password.new_label')}
              context={{ email: account.email ?? undefined, displayName: account.display_name }}
              proof={{ kind: 'bearer' }}
              hint={t('app.redeem.password_hint')}
            />
            <div>
              <Button
                type="submit"
                tone="primary"
                isBusy={passwordApi.isWorking}
                busyLabel={t('app.password.changing')}
              >
                {t('app.password.change_submit')}
              </Button>
            </div>
          </Stack>
        </form>
      </Stack>
    </div>

    <!-- The factor and the codes under one heading, because the codes are the factor's: they are
         made with it, they stop working when it goes, and a panel of its own suggested otherwise. -->
    <div class="panel">
      <Stack gap="150">
        <h2>{t('app.mfa.title')}</h2>
        <p class="quiet">{armed ? t('app.mfa.on') : t('app.mfa.off')}</p>
        <!-- Beside the state it is about. At the foot of the panel it sat under the "turn it off"
             section and read as a caption for it. -->
        {#if notice}<p class="quiet">{notice}</p>{/if}

        {#if armed}
          <div class="section">
            <Stack gap="150">
              <h3>{t('app.recovery.title')}</h3>
              {#if mfa.fresh}
                <OneTimeSecret
                  value={mfa.fresh.join('\n')}
                  label={t('app.recovery.title')}
                  hint={t('app.mfa.recovery_hint')}
                  revealLabel={t('app.recovery.reveal')}
                  hideLabel={t('app.recovery.hide')}
                  copyLabel={canCopy(navigator.clipboard) ? t('app.recovery.copy') : undefined}
                  copiedLabel={t('app.recovery.copied')}
                  acknowledgementLabel={t('app.recovery.acknowledge')}
                  notAcknowledgedReason={t('app.recovery.not_acknowledged')}
                  dismissLabel={t('app.recovery.dismiss')}
                  onDismiss={() => mfa.forget()}
                />
              {:else}
                <!-- Zero is the number to act on, so it is the one said in the danger tone - and
                     here it is always a number somebody can act on, because the button beside it
                     works. -->
                {#if left === 0}
                  <Banner tone="danger">{t('app.recovery.none_left')}</Banner>
                {:else if left !== undefined}
                  <p class="quiet">{t('app.recovery.left', { count: String(left), total: '10' })}</p>
                {/if}
                <p class="quiet">{t('app.recovery.regenerate_hint')}</p>
                <div>
                  <Button
                    tone="secondary"
                    isBusy={mfa.isWorking}
                    busyLabel={t('app.recovery.regenerating')}
                    onclick={() => void newCodes()}
                  >
                    {t('app.recovery.regenerate')}
                  </Button>
                </div>
              {/if}
            </Stack>
          </div>

          <div class="section">
            <details>
              <summary>{t('app.mfa.disable_summary')}</summary>
              <Stack gap="150">
                <p class="quiet">{t('app.mfa.disable_body')}</p>
                <p class="quiet">{t('app.mfa.disable_hint')}</p>
                <form onsubmit={disableFactor}>
                  <Stack gap="150">
                    <Input
                      label={t('app.step_up.password_label')}
                      bind:value={disablePassword}
                      type="password"
                      autocomplete="current-password"
                      spellcheck={false}
                      isRequired
                    />
                    <div>
                      <Button type="submit" tone="danger" isBusy={mfa.isWorking} busyLabel={t('app.mfa.disabling')}>
                        {t('app.mfa.disable')}
                      </Button>
                    </div>
                  </Stack>
                </form>
              </Stack>
            </details>
          </div>
        {:else}
          <TotpEnrollment onarmed={() => void armedFactor()} />
          <!-- Said where the codes would be, so that the question "where are my recovery codes"
               has an answer on the screen rather than a button that refuses. -->
          <p class="quiet">{t('app.recovery.none_yet')}</p>
        {/if}
      </Stack>
    </div>
  </Stack>
{/if}

<style>
  /* Each half stands on its own surface and keeps a measure: what is here is a form and prose,
     and neither wants the width (ADR-0065 decision 2). */
  .panel {
    max-inline-size: 60ch;
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }

  /* A rule rather than a second box: these belong to the panel above them, and a nested surface
     would say they are separate things. */
  .section {
    padding-block-start: var(--sp-150);
    border-block-start: var(--bw-hairline) solid var(--border-subtle);
  }

  form { margin: 0; max-inline-size: 52ch; }

  summary { cursor: pointer; color: var(--text-primary); }

  .quiet { margin: 0; color: var(--text-secondary); }

  h2 {
    margin: 0;
    font-family: var(--font-display);
    font-size: var(--fs-300);
    font-weight: var(--fw-semibold);
    line-height: var(--lh-tight);
  }

  h3 {
    margin: 0;
    font-size: var(--fs-200);
    font-weight: var(--fw-semibold);
    line-height: var(--lh-tight);
  }
</style>
