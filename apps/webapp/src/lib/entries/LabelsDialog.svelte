<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The collection's labels: making them, renaming them, taking them away.
  //
  // `labels` has had `create`, `update` and `remove` since F2-10 and no caller, so the set a
  // `LabelPicker` offers could only ever be made outside the application. The picker's own empty
  // state - "No labels in this collection yet." - was therefore a dead end rather than a beginning,
  // and the first use of every collection needed `hubctl`.
  //
  // **Here rather than in the picker.** Putting a create control inside `LabelPicker` would mean
  // changing a design system component, and `packages/design-system/CLAUDE.md` rules out a
  // component changing as a side effect of application work. It also belongs here on its own
  // merits: a label is a property of the collection, and this is the collection's screen. The
  // picker stays what it is - a chooser among what exists.
  //
  // **Ten colours, and no more.** `design-system.md` §4 says it in five words: "ten colorToken
  // values, nothing else". Each is a measured pair of background and foreground, legible in both
  // themes, which is what a colour wheel could not promise - so the choice is a list of the ten
  // rather than a picker, and `labelTokens` is where the ten are.

  import { labelTokens } from '@hubtask/design-system';
  import { Button, Dialog, IconButton, Inline, Input, LabelChip, Stack } from '@hubtask/design-system/components';

  import { labels } from '../data/labels.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  import type { Label } from '@hubtask/sync-engine';

  interface Props {
    isOpen: boolean;
    collectionId: string;
  }

  let { isOpen = $bindable(), collectionId }: Props = $props();

  let name = $state('');
  let colour = $state<string>(labelTokens[0]);
  let editing = $state<Label | undefined>(undefined);
  let isSaving = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);

  const existing = $derived(labels.of(collectionId));

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

  async function save() {
    if (name.trim() === '' || isSaving) return;
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
      if (editing?.id === label.id) reset();
    } catch (error) {
      failure = renderProblem(error as never, messages);
    }
  }
</script>

<Dialog bind:isOpen title={t('app.labels.manage')} dismissLabel={t('app.workspace.cancel')}>
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
              icon="trash-2"
              label={t('app.labels.delete', { name: label.name })}
              size="sm"
              onclick={() => remove(label)}
            />
          </Inline>
        {/each}
      </Stack>
    {/if}

    <Stack gap="150">
      <Input label={t('app.labels.name')} bind:value={name} isRequired />

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

      {#if failure}<p class="failure">{failure.message}</p>{/if}

      <Inline gap="100">
        <Button
          isBusy={isSaving}
          busyLabel={t('app.workspace.saving')}
          disabledReason={name.trim() === '' ? t('app.labels.name_required') : undefined}
          onclick={save}
        >
          {t(editing ? 'app.workspace.save' : 'app.labels.create')}
        </Button>
        {#if editing}
          <Button tone="secondary" onclick={reset}>{t('app.workspace.cancel')}</Button>
        {/if}
      </Inline>
    </Stack>
  </Stack>
</Dialog>

<style>
  .notice { margin: 0; color: var(--text-secondary); max-width: 64ch; }
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
