<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Private hubs allowed* (ADR-0073 §6, UC-ID-16 check 7): one switch, with where its value came
  // from and whether a level above locked it.
  //
  // **Here rather than on *Sign-in*.** It is stored and resolved with the sign-in rule's switches -
  // the installation's and the plan's default and lock are that rule's - but it is not a way in, and
  // the one place a person looks for whether private hubs may be made is the page that lists them.
  // It stands in that list page's intro, flat on the canvas: the in-place rung of page-system §5.1.
  //
  // **It acts at once, behind the step-up**, as every switch of the rule does: one row is not a
  // form. It shows its work in place and says it landed as a passing message. Off stops new private
  // hubs only; the sentence under the switch says so, because the question a person has before
  // switching is whether the hubs already there open up. They do not.
  //
  // **What a level above decided is not offered.** A lock keeps the switch's value and switches the
  // control off with the reason; the chip beside it says whose value it is and whom to ask
  // (UC-WRK-32 check 5); an "off" above cannot be switched on here.

  import { untrack } from 'svelte';

  import { Stack, Switch } from '@hubtask/design-system/components';

  import { signInPolicy, type LockOrigin } from '../data/signinpolicy.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';
  import ValueSource from './ValueSource.svelte';

  $effect(() => untrack(() => signInPolicy.open()));

  const setting = $derived(signInPolicy.policy?.private_hubs);
  let refusal = $state<string | undefined>(undefined);
  let isSaving = $state(false);

  function lockedBecause(lock: LockOrigin): string | undefined {
    if (lock === 'INSTANCE') return t('app.signin_settings.locked_installation');
    if (lock === 'PLAN') return t('app.signin_settings.locked_plan');
    return undefined;
  }

  const disabledReason = $derived.by(() => {
    if (!setting) return undefined;
    return lockedBecause(setting.lock)
      ?? (setting.installation ? undefined : t('app.private_hubs.allowed_off_above'));
  });

  async function change(event: Event) {
    // Kept before the await: the event's target is gone once the save has answered.
    const control = event.currentTarget as HTMLInputElement;
    const next = control.checked;
    refusal = undefined;
    isSaving = true;
    const problem = await signInPolicy.saveAtOnce({ private_hubs: next });
    isSaving = false;
    if (problem) {
      refusal = problem.fields.get('/sign_in_policy/private_hubs') ?? problem.message;
      // The read is what is in force; the control goes back to it.
      control.checked = !next;
      return;
    }
    toast.show({ text: t('app.private_hubs.allowed_saved'), tone: 'success' });
  }
</script>

{#if setting}
  <div class="allowed" data-private-hubs-allowed>
    <Stack gap="100">
      <Switch
        label={t('app.private_hubs.allowed')}
        hint={t('app.private_hubs.allowed_hint')}
        checked={setting.value}
        onchange={change}
        isBusy={isSaving}
        busyLabel={t('app.workspace.saving')}
        {disabledReason}
      />
      {#if refusal}<p class="refusal" role="alert">{refusal}</p>{/if}
      <p class="origin">
        <ValueSource
          source={setting.source}
          lock={setting.lock}
          value={setting.value ? t('app.signin_settings.on') : t('app.signin_settings.off')}
        />
      </p>
    </Stack>
  </div>
{/if}

<style>
  .origin { margin: 0; }

  .refusal { margin: 0; color: var(--text-danger); }
</style>
