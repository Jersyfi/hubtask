<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Setting up the authenticator app, a create page of its own (page-system §5.1, §9.5): the order
  // matters - start, scan and confirm, keep the codes - so the steps stand numbered under the head,
  // and the page can be left and come back to. `TotpEnrollment` does the work, as it does at sign-in.

  import { CreatePage } from '@hubtask/design-system/components';

  import TotpEnrollment from '../lib/frame/TotpEnrollment.svelte';
  import { page } from '../lib/frame/page.svelte.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { actor } from '../lib/data/account.svelte.ts';
  import { mfa } from '../lib/data/mfa.svelte.ts';
  import { passkeys } from '../lib/data/passkeys.svelte.ts';
  import { t } from '../lib/i18n/i18n.svelte.ts';
  import { profileTrail } from '../lib/profile/profilehead.ts';

  const { onnavigate }: { onnavigate?: (path: string) => void } = $props();

  const BACK = '/profile/security/second-factor';
  const title = $derived(t('app.mfa.setup_title'));
  $effect(() => page.entitle(title));

  /** The code was confirmed here: the codes are on screen and the factor is on. */
  let isConfirmed = $state(false);
  const step = $derived(isConfirmed ? 3 : mfa.enrollment ? 2 : 1);
  const STEPS = ['app.mfa.step_start', 'app.mfa.step_confirm', 'app.mfa.step_codes'] as const;

  async function armed(): Promise<void> {
    toast.show({ text: t('app.mfa.armed'), tone: 'success' });
    // The confirmation invalidated nothing, so both answers that just moved are read again: the
    // account's, and whether the last passkey may now go.
    await Promise.all([actor.reread(), passkeys.reread()]);
    onnavigate?.(BACK);
  }
</script>

<CreatePage
  head={{ title, subtitle: t('app.mfa.setup_lead'), breadcrumb: profileTrail('security', title) }}
  close={{ label: t('app.password.cancel'), onclose: () => onnavigate?.(BACK) }}
>
  {#snippet steps()}
    <ol class="steps" aria-label={t('app.mfa.steps')}>
      {#each STEPS as code, index (code)}
        <li aria-current={step === index + 1 ? 'step' : undefined} data-done={step > index + 1 ? '' : undefined}>
          <span class="number">{index + 1}</span>
          <span>{t(code)}</span>
        </li>
      {/each}
    </ol>
  {/snippet}

  <div class="region">
    {#if actor.hasAuthenticatorApp && !isConfirmed}
      <!-- Set up already: replacing it is the security page's, where the old one keeps working
           until the new one confirms. -->
      <p class="quiet">{t('app.mfa.on')}</p>
    {:else}
      <TotpEnrollment onconfirmed={() => (isConfirmed = true)} onarmed={() => void armed()} />
    {/if}
  </div>
</CreatePage>

<style>
  .steps { display: flex; flex-wrap: wrap; gap: var(--sp-100); margin: 0; padding: 0; list-style: none; }

  .steps li {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-100);
    padding: var(--sp-050) var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-full);
    color: var(--text-secondary);
    font-size: var(--fs-075);
  }

  .steps li[aria-current='step'] { border-color: var(--border-strong); color: var(--text-primary); font-weight: var(--fw-semibold); }

  .number {
    display: inline-grid;
    place-items: center;
    inline-size: var(--sp-250);
    block-size: var(--sp-250);
    border-radius: var(--r-full);
    background: var(--bg-surface-sunken);
    font-variant-numeric: tabular-nums;
  }

  .region {
    padding: var(--sp-250);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }

  .quiet { margin: 0; color: var(--text-secondary); }
</style>
