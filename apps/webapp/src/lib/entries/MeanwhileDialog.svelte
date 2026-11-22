<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The warning before a text somebody else changed meanwhile is overwritten (UC-SYN-02 check 10,
  // offline-sync.md §5): an alert dialog that says who changed it and when, shows the saved
  // version and the reader's, and asks. Two answers: keep the saved version - the reader's text is
  // dropped, the safe answer the focus starts on - or overwrite it with the reader's. Closing it
  // answers nothing: the text stays as the reader had it, unsaved, and the next save asks again.
  // While the overwrite runs it cannot be closed; a refusal says so here and keeps the question open.

  import { Button, Dialog } from '@hubtask/design-system/components';

  import FailureNote from '../frame/FailureNote.svelte';
  import RichText from './RichText.svelte';
  import { t } from '../i18n/i18n.svelte.ts';
  import type { RenderedProblem } from '../problem.ts';

  interface Props {
    isOpen?: boolean;
    field: 'title' | 'notes';
    /** The version the entry holds now. */
    theirs: string;
    /** The reader's own, not saved. */
    mine: string;
    /** Who changed it last and when, as resolved text; `isReader` where it was the reader elsewhere. */
    by: { readonly name?: string; readonly isReader?: boolean; readonly at?: string };
    failure?: RenderedProblem;
    isOverwriting?: boolean;
    onkeep: () => void;
    onoverwrite: () => void;
    /** Where a mention or a card in a description leads. */
    onnavigate?: (path: string) => void;
  }

  let { isOpen = $bindable(false), field, theirs, mine, by, failure, isOverwriting = false, onkeep, onoverwrite, onnavigate }: Props = $props();

  const said = $derived(
    by.isReader
      ? t(field === 'title' ? 'app.sync.meanwhile.by_you_title' : 'app.sync.meanwhile.by_you_notes', { when: by.at ? 'known' : 'unknown', at: by.at ?? '' })
      : t(field === 'title' ? 'app.sync.meanwhile.by_title' : 'app.sync.meanwhile.by_notes', { name: by.name ?? '', when: by.at ? 'known' : 'unknown', at: by.at ?? '' }),
  );
</script>

<Dialog
  bind:isOpen
  size="s"
  isAlert
  title={field === 'title' ? t('app.sync.meanwhile.dialog_title') : t('app.sync.meanwhile.dialog_notes')}
  dismissLabel={t('app.sheet_close')}
  onDismiss={() => !isOverwriting}
  primary={{ label: t('app.sync.meanwhile.overwrite'), tone: 'danger', onclick: onoverwrite, isBusy: isOverwriting, busyLabel: t('app.sync.meanwhile.overwriting') }}
>
  {#snippet actions()}
    <Button tone="secondary" onclick={onkeep} disabledReason={isOverwriting ? t('app.sync.meanwhile.overwriting') : undefined}>{t('app.sync.meanwhile.keep')}</Button>
  {/snippet}
  <p class="said" data-meanwhile={field}>
    {said} {t('app.sync.meanwhile.consequence')}{#if field === 'notes'} {t('app.sync.meanwhile.not_kept')}{/if}
  </p>
  <p class="said">{t('app.sync.meanwhile.close_hint')}</p>
  {#if failure}
    <FailureNote problem={failure}>{t('app.sync.meanwhile.not_written')}</FailureNote>
  {/if}
  <div class="versions">
    <section class="version" aria-label={t('app.sync.meanwhile.theirs')}>
      <h3 class="label">{t('app.sync.meanwhile.theirs')}</h3>
      {@render text(theirs, t('app.sync.meanwhile.theirs'))}
    </section>
    <section class="version" data-mine aria-label={t('app.sync.meanwhile.mine')}>
      <h3 class="label">{t('app.sync.meanwhile.mine')}</h3>
      {@render text(mine, t('app.sync.meanwhile.mine'))}
    </section>
  </div>
</Dialog>

<!-- A description as the editor shows it - its marks, mentions and cards - never its Markdown source; a title as it is. -->
{#snippet text(value: string, label: string)}
  {#if field === 'notes'}<RichText {value} {label} {onnavigate} />{:else}<p class="text">{value}</p>{/if}
{/snippet}

<style>
  .said { margin: 0; }

  /* One version under the other: a confirmation's width holds one column of prose well and two
     badly, and the saved one comes first, as it is the one in place. */
  .versions { display: grid; gap: var(--sp-150); }

  .version {
    display: grid;
    gap: var(--sp-100);
    padding: var(--sp-150);
    border: var(--bw-hairline) solid var(--border-default);
    border-radius: var(--r-md);
    min-inline-size: 0;
  }

  /* The reader's, not saved: the border carries it as well as the heading does. */
  .version[data-mine] { border-color: var(--border-strong); }

  .label {
    margin: 0;
    font-size: var(--fs-075);
    font-weight: var(--fw-medium);
    color: var(--text-secondary);
  }

  /* A title is the person's own text, drawn as text and never as markup. */
  .text {
    margin: 0;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    font-family: var(--font-ui);
    font-size: var(--fs-100);
    color: var(--text-primary);
  }
</style>
