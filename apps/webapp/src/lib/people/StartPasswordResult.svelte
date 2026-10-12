<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A start password's one showing (ADR-0074 §2, §4): at the creation of a managed account and at a
  // new start password alike. It is shown in `OneTimeSecret`, to copy, and on a slip to print.
  // Nothing keeps it: the caller holds the answer only until the reader dismisses it, and the slip
  // exists in the document only while the browser prints it - a credential is in the DOM no longer
  // than its one showing.

  import { tick } from 'svelte';

  import { Button, OneTimeSecret } from '@hubtask/design-system/components';

  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    /** The sign-in name the slip prints beside the password. */
    signInName: string;
    startPassword: string;
    /** What the value is called on the screen: a first start password, or a new one. */
    label: string;
    onDismiss: () => void;
  }

  const { signInName, startPassword, label, onDismiss }: Props = $props();

  /** True only while the browser prints the slip. */
  let printing = $state(false);

  async function print(): Promise<void> {
    printing = true;
    await tick();
    window.print();
    printing = false;
  }
</script>

<OneTimeSecret
  value={startPassword}
  {label}
  hint={t('app.people.managed_start_password_hint')}
  revealLabel={t('app.people.managed_reveal')}
  hideLabel={t('app.people.managed_hide')}
  copyLabel={t('app.people.managed_copy')}
  copiedLabel={t('app.people.managed_copied')}
  acknowledgementLabel={t('app.people.managed_kept')}
  notAcknowledgedReason={t('app.people.managed_keep_first')}
  dismissLabel={t('app.tokens.done')}
  {onDismiss}
/>
<div>
  <Button tone="secondary" onclick={() => void print()}>{t('app.people.managed_print')}</Button>
</div>
{#if printing}
  <!-- What a person takes to the child: where, with what name, with which password, and that they
       choose their own at once. Drawn for the printer only. -->
  <div class="slip" data-slip>
    <p class="slip-title">{t('app.people.managed_slip_title')}</p>
    <dl>
      <dt>{t('app.people.managed_slip_address')}</dt>
      <dd>{location.host}</dd>
      <dt>{t('app.people.managed_slip_name')}</dt>
      <dd>{signInName}</dd>
      <dt>{t('app.people.managed_slip_password')}</dt>
      <dd class="code">{startPassword}</dd>
    </dl>
    <p>{t('app.people.managed_slip_next')}</p>
  </div>
{/if}

<style>
  /* The slip is for paper only: on screen it is never drawn. */
  .slip { display: none; }

  @media print {
    /* While the slip is printed, it is the page: everything else stays off the paper. The rule
       reaches outside this component on purpose - in print only, and only while a slip exists, so
       printing this screen at any other moment prints the screen. */
    :global(body:has([data-slip]) *) { visibility: hidden; }

    .slip,
    .slip * { visibility: visible; }

    .slip {
      display: block;
      position: absolute;
      inset-block-start: 0;
      inset-inline-start: 0;
      padding: var(--sp-300);
      font-size: var(--fs-300);
      color: var(--text-primary);
    }

    .slip-title { font-family: var(--font-display); font-weight: var(--fw-semibold); }

    .code { font-family: var(--font-mono); }
  }
</style>
