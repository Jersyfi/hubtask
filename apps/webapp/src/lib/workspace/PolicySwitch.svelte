<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One switch of the sign-in rule that is not a way in - *Private hubs allowed* (ADR-0073 §6,
  // UC-ID-16 check 7) and *Own logo and name* (ADR-0112, levelled as the first): one switch, with
  // where its value came from and whether a level above locked it. Each is drawn where a person looks
  // for it, flat on the canvas: the in-place rung of page-system §5.1.
  //
  // **It acts at once, behind the step-up**, as every switch of the rule does: one row is not a
  // form. It shows its work in place and says it landed as a passing message. The hint under the
  // switch says what off does to what is already there, because that is the question a person has
  // before switching.
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

  interface Props {
    /** The switch of the rule, as `sign_in_policy` names it. */
    key: 'private_hubs' | 'own_branding';
    label: string;
    hint: string;
    /** Why it cannot be switched on here where a level above switched it off without a lock. */
    offAbove: string;
    /** The passing message once it landed. */
    saved: string;
  }

  const { key, label, hint, offAbove, saved }: Props = $props();

  $effect(() => untrack(() => signInPolicy.open()));

  const setting = $derived(signInPolicy.policy?.[key]);
  let refusal = $state<string | undefined>(undefined);
  let isSaving = $state(false);

  function lockedBecause(lock: LockOrigin): string | undefined {
    if (lock === 'INSTANCE') return t('app.signin_settings.locked_installation');
    if (lock === 'PLAN') return t('app.signin_settings.locked_plan');
    return undefined;
  }

  const disabledReason = $derived.by(() => {
    if (!setting) return undefined;
    return lockedBecause(setting.lock) ?? (setting.installation ? undefined : offAbove);
  });

  async function change(event: Event) {
    // Kept before the await: the event's target is gone once the save has answered.
    const control = event.currentTarget as HTMLInputElement;
    const next = control.checked;
    refusal = undefined;
    isSaving = true;
    const problem = await signInPolicy.saveAtOnce({ [key]: next });
    isSaving = false;
    if (problem) {
      refusal = problem.fields.get(`/sign_in_policy/${key}`) ?? problem.message;
      // The read is what is in force; the control goes back to it.
      control.checked = !next;
      return;
    }
    toast.show({ text: saved, tone: 'success' });
  }
</script>

{#if setting}
  <div class="allowed" data-policy-switch={key}>
    <Stack gap="100">
      <Switch
        {label}
        {hint}
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
