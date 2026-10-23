<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The collection's labels: making them, renaming them, taking them away.
  //
  // `labels` has `create`, `update` and `remove`, and without this the set a `LabelPicker`
  // offers could only be made outside the application: the picker's own empty state - "No labels
  // in this collection yet." - would be a dead end rather than a beginning.
  //
  // **Here rather than in the picker.** Putting a create control inside `LabelPicker` would mean
  // changing a design system component, and the design system grows in the order of
  // design-system.md §4, never as a side effect of application work. It also belongs here on its own
  // merits: a label is a property of the collection, and this is the collection's screen. The
  // picker stays what it is - a chooser among what exists.
  //
  // **Ten colours, and no more.** `design-system.md` §4 says it in five words: "ten colorToken
  // values, nothing else". Each is a measured pair of background and foreground, legible in both
  // themes, which is what a colour wheel could not promise - so the choice is a list of the ten
  // rather than a picker, and `labelTokens` is where the ten are.

  import { untrack } from 'svelte';

  import { labelTokens } from '@hubtask/design-system';
  import { Button, IconButton, Inline, Input, LabelChip, Stack } from '@hubtask/design-system/components';

  import { labels } from '../data/labels.svelte.ts';
  import { offerUndo } from './undo.svelte.ts';
  import { announcer } from '../announce.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import { focusFirst } from '../forms/missing.ts';

  import type { Label } from '@hubtask/sync-engine';
  import PanelOrDialog from '../workspace/PanelOrDialog.svelte';

  interface Props {
    /** Drawn as a tab of the *Structure* panel rather than as a dialog of its own. */
    isEmbedded?: boolean;
    isOpen: boolean;
    collectionId: string;
  }

  let { isEmbedded = false, isOpen = $bindable(), collectionId }: Props = $props();

  let name = $state('');
  let colour = $state<string>(labelTokens[0]);
  let editing = $state<Label | undefined>(undefined);
  let isSaving = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);

  const existing = $derived(labels.of(collectionId));
  /**
   * What was taken out of the vocabulary, with the way back (UC-WRK-28 check 3): a deletion's
   * *Undo* stands only as long as its message, so the labels themselves are the place it stays.
   */
  const deleted = $derived(labels.deletedOf(collectionId));

  $effect(() => {
    if (!isOpen) return;
    const wanted = collectionId;
    return untrack(() => labels.openDeleted(wanted));
  });

  async function restore(label: Label) {
    failure = undefined;
    try {
      await labels.restore(collectionId, label.id, crypto.randomUUID());
      announcer.say(t('app.labels.restored_announced', { name: label.name }));
    } catch (error) {
      failure = renderProblem(error as never, messages);
    }
  }

  // Opening starts from nothing. Closing leaves the fields alone: they are re-read on the next
  // open, and clearing them here would blank the dialog while it is still fading out.
  $effect(() => {
    if (!isOpen) return;
    reset();
  });

  function reset() {
    name = '';
    colour = labelTokens[0];
    editing = undefined;
    failure = undefined;
  }

  function startEditing(label: Label) {
    editing = label;
    name = label.name;
    colour = label.color_token ?? labelTokens[0];
    failure = undefined;
  }

  /** The save was pressed without a name: the field says so (design-system.md §4, case 3). */
  let isNameMissing = $state(false);
  let form = $state<HTMLElement | null>(null);

  async function save() {
    if (isSaving) return;
    if (name.trim() === '') {
      isNameMissing = true;
      queueMicrotask(() => focusFirst(form));
      return;
    }
    isSaving = true;
    failure = undefined;
    try {
      if (editing) {
        // The version the reader had when they started typing (ADR-0025).
        await labels.update(
          collectionId,
          editing.id,
          { name: name.trim(), color_token: colour },
          editing.version,
        );
      } else {
        await labels.create(
          collectionId,
          { name: name.trim(), color_token: colour },
          crypto.randomUUID(),
        );
      }
      announcer.say(t('app.labels.saved_announced'));
      reset();
    } catch (error) {
      failure = renderProblem(error as never, messages);
    } finally {
      isSaving = false;
    }
  }

  async function remove(label: Label) {
    failure = undefined;
    try {
      await labels.remove(collectionId, label.id, label.version);
      // At once, with the way back (UC-WRK-28 check 3): the entries that carried it carry it again.
      offerUndo({ kind: 'label-deleted', collectionId, labelId: label.id, name: label.name });
      if (editing?.id === label.id) reset();
    } catch (error) {
      failure = renderProblem(error as never, messages);
    }
  }
</script>

<PanelOrDialog {isEmbedded} bind:isOpen title={t('app.labels.manage')} dismissLabel={t('app.workspace.cancel')}>
  <Stack gap="200">
    {#if existing.length === 0}
      <p class="notice">{t('app.labels.none_yet')}</p>
    {:else}
      <Stack gap="050">
        {#each existing as label (label.id)}
          <Inline gap="100">
            <LabelChip name={label.name} colorToken={label.color_token} />
            <IconButton
              icon="pencil"
              label={t('app.labels.rename', { name: label.name })}
              size="sm"
              onclick={() => startEditing(label)}
            />
            <IconButton
              icon="trash"
              label={t('app.labels.delete', { name: label.name })}
              size="sm"
              onclick={() => remove(label)}
            />
          </Inline>
        {/each}
      </Stack>
    {/if}

    {#if deleted.length > 0}
      <section class="deleted" aria-labelledby="labels-deleted">
        <h3 id="labels-deleted" class="heading">{t('app.labels.deleted_heading')}</h3>
        <Stack gap="050">
          {#each deleted as label (label.id)}
            <Inline gap="100">
              <LabelChip name={label.name} colorToken={label.color_token} />
              <Button size="sm" tone="secondary" onclick={() => restore(label)}>
                {t('app.labels.restore', { name: label.name })}
              </Button>
            </Inline>
          {/each}
        </Stack>
      </section>
    {/if}

    <div bind:this={form}>
    <Stack gap="150">
      <Input
        label={t('app.labels.name')}
        bind:value={name}
        isRequired
        oninput={() => (isNameMissing = false)}
        error={isNameMissing ? t('app.labels.name_required') : undefined}
      />

      <!-- A radio group by hand rather than ten `Radio`s: what a reader is choosing between is the
           chips themselves, and a list of ten labelled radios would name the colours in words the
           catalogue would then have to carry in every language. The chip is the label. -->
      <fieldset class="colours">
        <legend>{t('app.workspace.colour')}</legend>
        {#each labelTokens as token (token)}
          <label class="swatch">
            <input type="radio" name="label-colour" value={token} bind:group={colour} />
            <LabelChip name={name.trim() === '' ? t('app.labels.name') : name.trim()} colorToken={token} />
          </label>
        {/each}
      </fieldset>

      {#if failure}<p class="failure" role="alert">{failure.message}</p>{/if}

      <Inline gap="100">
        <Button
          isBusy={isSaving}
          busyLabel={t('app.workspace.saving')}
          onclick={save}
        >
          {t(editing ? 'app.workspace.save' : 'app.labels.create')}
        </Button>
        {#if editing}
          <Button tone="secondary" onclick={reset}>{t('app.workspace.cancel')}</Button>
        {/if}
      </Inline>
    </Stack>
    </div>
  </Stack>
</PanelOrDialog>

<style>
  .notice { margin: 0; color: var(--text-secondary); max-width: 64ch; }
  .deleted { display: flex; flex-direction: column; gap: var(--sp-100); }
  .heading { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); font-weight: var(--fw-medium); }
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); max-width: 64ch; }

  .colours {
    display: flex;
    flex-wrap: wrap;
    gap: var(--sp-050);
    margin: 0;
    padding: 0;
    border: 0;
  }
  .colours legend {
    padding: 0;
    margin-block-end: var(--sp-050);
    color: var(--text-secondary);
    font-size: var(--fs-075);
  }
  .swatch { display: inline-flex; align-items: center; gap: var(--sp-050); cursor: pointer; }
  /* The radio stays in the tab order and keeps the arrow-key behaviour a group has; what marks the
     chosen one is the ring, which is the same ring every other control here focuses with. */
  .swatch input { position: absolute; opacity: 0; pointer-events: none; }
  .swatch:has(input:checked) { outline: var(--bw-ring) solid var(--accent-primary); border-radius: var(--r-sm); }
  .swatch:has(input:focus-visible) { outline: var(--bw-ring) solid var(--focus-ring); border-radius: var(--r-sm); }
</style>
