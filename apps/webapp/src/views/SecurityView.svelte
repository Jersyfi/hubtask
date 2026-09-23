<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The second factor: setting one up, and taking it off.
  //
  // **Whether one is armed is not something this client is told.** No read answers it, and
  // inferring it from a sign-in that did not ask for a code would be inferring from an absence. So
  // the screen offers enrolment, and the server refuses one that is already armed — in its own
  // words, which is the honest answer rather than a guess.
  //
  // **Taking it off asks for the password again**, which is the one case where being signed in is
  // not enough: a stolen session removing the factor is exactly the attack the factor exists
  // against (`security.md` §5).

  import { Button, EmptyState, Input, Stack } from '@hubtask/design-system/components';

  import SettingsHead from '../lib/frame/SettingsHead.svelte';
  import TotpEnrollment from '../lib/frame/TotpEnrollment.svelte';

  import { actor } from '../lib/data/account.svelte.ts';
  import { mfa } from '../lib/data/mfa.svelte.ts';
  import { announcer } from '../lib/announce.svelte.ts';
  import { t } from '../lib/i18n/i18n.svelte.ts';

  const account = $derived(actor.account);

  let disablePassword = $state('');
  let notice = $state<string | undefined>(undefined);

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
</style>
