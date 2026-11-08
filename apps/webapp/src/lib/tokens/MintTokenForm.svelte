<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The form that makes a token in a service account's *New token* dialog (`security.md` §5); one's
  // own *New token* is a page of its own. Both fill a `TokenDraft`, which holds the three rules a
  // token's making must not soften. The credential it makes is handed to the caller (`onminted`),
  // which shows it once.

  import { untrack } from 'svelte';

  import { Button, CheckboxGroup, Input, Radio, Stack } from '@hubtask/design-system/components';

  import FailureNote from '../frame/FailureNote.svelte';
  import type { MintedToken } from '../data/tokens.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';
  import { focusFirst, isComplete, without } from '../forms/missing.ts';
  import { NO_INSTALLATION, TokenDraft } from './draft.svelte.ts';
  import { scopeWord } from './tokenwords.ts';

  interface Props {
    /** A service account's token, where the caller may mint for one. Absent means one's own. */
    accountId?: string;
    onminted: (minted: MintedToken) => void;
    /** In a dialog the dialog's primary submits it (`form.requestSubmit()`), so it draws no button. */
    isInDialog?: boolean;
    form?: HTMLFormElement | null;
    isWorking?: boolean;
    /** Whether anything is typed or ticked: what a dialog's discard guard asks. */
    hasChanges?: boolean;
  }

  let { accountId, onminted, isInDialog = false, form = $bindable(null), isWorking = $bindable(false), hasChanges = $bindable(false) }: Props = $props();

  const draft = new TokenDraft(untrack(() => accountId));

  $effect(() => {
    hasChanges = draft.hasChanges;
  });
  $effect(() => {
    isWorking = draft.isWorking;
  });

  async function mint(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    const minted = await draft.mint();
    if (minted) onminted(minted);
    else if (!isComplete(draft.missing)) queueMicrotask(() => focusFirst(form));
  }
</script>

<!-- `novalidate`: what is missing is said by the form itself, in its own words, when it is
     pressed - not by the browser's bubble (design-system.md §4, case 3). -->
<form onsubmit={mint} bind:this={form} novalidate>
  <Stack gap="200">
    {#if draft.failure}<FailureNote problem={draft.failure} />{/if}
    <Input
      label={t('app.tokens.name')}
      hint={t('app.tokens.name_hint')}
      bind:value={draft.name}
      isRequired
      oninput={() => (draft.missing = without(draft.missing, 'name'))}
      error={draft.missing.name ? t(draft.missing.name) : undefined}
    />

    <!-- `max` as well as `required`: the server refuses more than a year, and being told
         before the round trip is the same number said earlier rather than a second policy. -->
    <Input
      label={t('app.tokens.expires')}
      hint={t('app.tokens.expires_hint')}
      bind:value={draft.expiresAt}
      oninput={() => (draft.missing = without(draft.missing, 'expires'))}
      error={draft.missing.expires ? t(draft.missing.expires) : undefined}
      type="date"
      min={draft.today}
      max={draft.latestAllowed}
      isRequired
    />

    {#if draft.scopes.length === 0}
      <!-- The manifest has not arrived. Offering nothing is honest; offering a list written
           here would be wrong on somebody's installation. -->
      <Stack gap="050">
        <span class="label">{t('app.tokens.scopes')}</span>
        <p class="quiet small">{t('app.tokens.scopes_unknown')}</p>
      </Stack>
    {:else}
      <!-- Nothing said in advance when none is ticked: *Create token* says it if it is pressed so. -->
      <CheckboxGroup
        label={t('app.tokens.scopes')}
        hint={t('app.tokens.scopes_hint')}
        options={draft.scopes.map((scope) => ({ value: scope, label: scopeWord(scope), hint: scopeWord(scope) === scope ? undefined : scope }))}
        selected={draft.chosen}
        error={draft.missing.scopes ? t(draft.missing.scopes) : undefined}
        onChange={(next) => {
          draft.chosen = [...next];
          draft.missing = without(draft.missing, 'scopes');
        }}
      />
    {/if}

    {#if draft.installationOptions.length > 0}
      <!-- Absent, not disabled, for somebody the register does not name (P-05). -->
      <Radio
        label={t('app.tokens.installation')}
        options={[
          { value: NO_INSTALLATION, label: t('app.tokens.installation_none'), hint: t('app.tokens.installation_none_hint') },
          ...draft.installationOptions,
        ]}
        bind:value={draft.installation}
      />
    {/if}

    {#if !isInDialog}
      <div>
        <Button type="submit" tone="primary" isBusy={draft.isWorking} busyLabel={t('app.tokens.minting')}>
          {t('app.tokens.mint')}
        </Button>
      </div>
    {/if}
  </Stack>
</form>

<style>
  .label { color: var(--text-primary); font-size: var(--fs-075); font-weight: var(--fw-medium); }

  .quiet { margin: 0; color: var(--text-secondary); }

  .small { font-size: var(--fs-075); }

  form { margin: 0; }
</style>
