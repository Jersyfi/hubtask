<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // "Mark the moments": the account's, not the device's (ADR-0043) - somebody who switched the
  // moments off switched them off everywhere - so it stands on *You* beside the name and the
  // language rather than among the theme and motion of *On this device*.
  //
  // Immediate, because it stands outside a form (ux-inventory.md §E10, "Saving"): written at once
  // and put back when a write is refused, so the switch never shows a choice the server did not
  // take.

  import { Stack, Switch } from '@hubtask/design-system/components';

  import { actor } from '../data/account.svelte.ts';
  import { preferences } from '../data/preferences.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  const account = $derived(actor.account);

  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let isSaving = $state(false);
  let isOn = $state(true);
  $effect(() => {
    isOn = account?.celebrations !== false;
  });

  async function set(next: boolean) {
    if (!account) return;
    isSaving = true;
    failure = undefined;
    try {
      // `true` is written as the default's own value rather than cleared: what the person chose
      // is a choice, and "on" is not the same fact as "never decided".
      await preferences.setAccount(account.id, { celebrations: next });
      // Confirmed as a passing message, which the frame also says once (design-system.md §4, case 7).
      toast.show({ text: t(next ? 'app.profile.celebrations_on_announced' : 'app.profile.celebrations_off_announced'), tone: 'success' });
    } catch (error) {
      failure = renderProblem(error as never, messages);
      isOn = account?.celebrations !== false;
    } finally {
      isSaving = false;
    }
  }
</script>

{#if account}
  <div class="panel">
    <Stack gap="150">
      <Switch
        label={t('app.profile.celebrations')}
        data-setting="celebrations"
        hint={t('app.profile.celebrations_hint')}
        bind:checked={isOn}
        isBusy={isSaving}
        busyLabel={t('app.workspace.saving')}
        onchange={() => void set(isOn)}
      />
      {#if failure}<p class="failure" role="alert">{failure.message}</p>{/if}
    </Stack>
  </div>
{/if}

<style>
  .panel {
    max-inline-size: 52ch;
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
    box-shadow: var(--shadow-raised);
  }

  .failure { margin: 0; color: var(--text-danger); }
</style>
