<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The setting *What counts* (UC-WRK-29 check 1): which figure the collection's head and its row
  // show. Opened from the collection's menu for a reader who may change its shape; the Structure
  // panel takes this same control when it exists.
  //
  // It offers what `figureOffer.ts` allows - nothing the server refuses (P-05) - says where the
  // value comes from, shows a lock as a lock, and explains a progress the levels show as open
  // entries rather than leaving the reader to wonder where the percentage went.

  import { untrack } from 'svelte';

  import { Button, Dialog, Select, Stack } from '@hubtask/design-system/components';
  import type { Container } from '@hubtask/sync-engine';

  import { announcer } from '../announce.svelte.ts';
  import { containers } from '../data/containers.svelte.ts';
  import { customFields } from '../data/customfields.svelte.ts';
  import { countsChange, figureOffer, isComplete, type FigureKind } from '../data/figureOffer.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    isOpen: boolean;
    container: Container;
  }

  let { isOpen = $bindable(), container }: Props = $props();

  let figure = $state<FigureKind>('NONE');
  let field = $state('');
  let isSaving = $state(false);
  /** A sum chosen without its field, marked when the reader saves (missing input is marked on submit). */
  let isFieldMissing = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);

  // The number fields a sum may name: the collection's own and the workspace's.
  // From `untrack`: subscribing delivers at once and writes the store this effect would read.
  $effect(() => {
    if (!isOpen) return;
    const id = container.id;
    return untrack(() => {
      const stops = [customFields.open(id), customFields.open()];
      return () => {
        for (const stop of stops) stop();
      };
    });
  });

  const offer = $derived(
    container.structure
      ? figureOffer(container.structure, [...customFields.of(container.id), ...customFields.of()])
      : undefined,
  );

  // Opening fills the choice from what is in force, once: the fields arriving afterwards change the
  // offer, and must not take back what the reader has chosen meanwhile.
  let isFilled = false;
  $effect(() => {
    if (!isOpen) {
      isFilled = false;
      return;
    }
    if (!offer || isFilled) return;
    isFilled = true;
    figure = offer.clippedFrom ?? offer.current;
    field = offer.currentField ?? '';
    isFieldMissing = false;
    failure = undefined;
  });

  const options = $derived(
    (offer?.figures ?? []).map((kind) => ({ value: kind, label: t(`app.figure.option.${kind.toLowerCase()}`) })),
  );
  const isLocked = $derived(offer?.lockedBy !== undefined);
  const sourceText = $derived(offer ? t(`app.figure.source.${offer.source.toLowerCase()}`) : '');

  async function save() {
    if (isSaving || isLocked) return;
    isFieldMissing = !isComplete(figure, field);
    if (isFieldMissing) return;
    isSaving = true;
    failure = undefined;
    try {
      await containers.setStructure(container.id, { settings: { counts: countsChange(figure, field || undefined) } }, container.version);
      announcer.say(t('app.figure.saved_announced'));
      isOpen = false;
    } catch (error) {
      failure = renderProblem(error as never, messages);
    } finally {
      isSaving = false;
    }
  }
</script>

<Dialog bind:isOpen title={t('app.figure.title')} dismissLabel={t('app.workspace.cancel')}>
  {#if offer}
    <Stack gap="200">
      <p class="hint">{t('app.figure.intro')}</p>
      {#if isLocked}
        <!-- A lock is shown, not offered: the value in force and who decided it. -->
        <p class="locked" data-figure-locked>
          {t('app.figure.locked', { figure: t(`app.figure.option.${offer.current.toLowerCase()}`), origin: offer.lockedBy ?? '' })}
        </p>
      {:else}
        <Select label={t('app.figure.choice')} hint={sourceText} bind:value={figure} {options} />
        {#if figure === 'FLOW'}
          <p class="hint" data-figure-flow>{t('app.figure.option.flow_later')}</p>
        {/if}
        {#if figure === 'FIELD_SUM'}
          <Select
            label={t('app.figure.field')}
            bind:value={field}
            placeholder={t('app.figure.field_choose')}
            error={isFieldMissing && !field ? t('app.figure.field_choose') : undefined}
            options={offer.fields.map((option) => ({ value: option.key, label: option.name }))}
          />
        {/if}
      {/if}
      {#if offer.clippedFrom === 'PROGRESS'}
        <p class="hint" data-figure-clipped>{t('app.figure.clipped', { source: t(`app.figure.source.${offer.source.toLowerCase()}`) })}</p>
      {/if}
      {#if failure}<p class="failure" role="alert">{failure.message}</p>{/if}
    </Stack>
  {/if}

  {#snippet actions()}
    <Button tone="secondary" onclick={() => (isOpen = false)}>{t('app.workspace.cancel')}</Button>
    {#if !isLocked}
      <Button
        isBusy={isSaving}
        busyLabel={t('app.workspace.saving')}
        onclick={save}
      >
        {t('app.workspace.save')}
      </Button>
    {/if}
  {/snippet}
</Dialog>

<style>
  .hint, .locked { margin: 0; color: var(--text-secondary); }
  .failure { margin: 0; color: var(--status-danger-text); }
</style>
