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
  // **Whether one is armed is not something this client is told.** No read answers it, and
  // inferring it from a sign-in that did not ask for a code would be inferring from an absence. So
  // the screen offers enrolment, and the server refuses one that is already armed — in its own
  // words, which is the honest answer rather than a guess.
  //
  // **Taking it off asks for the password again**, which is the one case where being signed in is
  // not enough: a stolen session removing the factor is exactly the attack the factor exists
  // against (`security.md` §5).

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
  /**
   * How many recovery codes are left.
   *
   * Read defensively: an installation that has not shipped the field yet answers nothing, and a
   * screen that turned that into "0 left" would send somebody to make new codes they do not need.
   */
  const left = $derived.by(() => {
    const answered = (account as unknown as { recovery_codes_remaining?: unknown } | undefined)?.recovery_codes_remaining;
    return typeof answered === 'number' ? answered : undefined;
  });

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
    if (await mfa.regenerate()) announcer.say(t('auth.recovery_regenerated'));
  }

  async function disableFactor(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    const password = disablePassword;
    disablePassword = '';
    if (await mfa.disable(password)) {
      notice = t('app.mfa.disabled');
      announcer.say(notice);
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

    <div class="panel">
      <Stack gap="150">
        <!-- The panel names the value itself, so the section does not name it twice. -->
        {#if !mfa.fresh}
          <h2>{t('app.recovery.title')}</h2>
          <!-- The count the contract has carried since H-02 and no screen had ever shown. Zero is
               the number to act on, so it is the one that is said in the danger tone. -->
          {#if left !== undefined}
            {#if left === 0}
              <Banner tone="danger">{t('app.recovery.none_left')}</Banner>
            {:else}
              <p class="quiet">{t('app.recovery.left', { count: String(left), total: '10' })}</p>
            {/if}
          {/if}
        {/if}
        {#if mfa.fresh}
          <OneTimeSecret
            value={mfa.fresh.join('\n')}
            label={t('app.recovery.title')}
            hint={t('app.mfa.recovery_hint')}
            revealLabel={t('app.recovery.reveal')}
            hideLabel={t('app.password.hide')}
            copyLabel={canCopy(navigator.clipboard) ? t('app.recovery.copy') : undefined}
            copiedLabel={t('app.recovery.copied')}
            acknowledgementLabel={t('app.recovery.acknowledge')}
            notAcknowledgedReason={t('app.recovery.not_acknowledged')}
            dismissLabel={t('app.recovery.dismiss')}
            onDismiss={() => mfa.forget()}
          />
        {:else}
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

    <div class="panel">
      <Stack gap="150">
        <TotpEnrollment
          onarmed={() => {
            notice = t('app.mfa.armed');
            announcer.say(notice);
          }}
        />
        {#if notice}<p class="quiet">{notice}</p>{/if}
      </Stack>
    </div>

    <div class="panel">
      <details>
        <summary>{t('app.mfa.disable')}</summary>
        <Stack gap="150">
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
</style>
