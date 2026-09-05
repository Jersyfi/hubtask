<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Creating a hub or a collection.
  //
  // One component for both, for the reason `ContainerView` is one view for both: they differ in
  // what they hold, not in what it takes to make one. A hub is created at the top level and a
  // collection under a named hub, and that is the whole difference in this form.
  //
  // A dialog rather than an inline field, and for the reason the rename beside it gives: a name
  // collision arrives as a 409 with `containers.name_taken`, the reader's next action is to type a
  // different name, and a sentence somewhere else on the screen is one they would have to carry
  // back to the field.
  //
  // Icon and colour are deliberately absent. `containers.create` takes both, and offering them
  // needs an icon picker and a colour picker - two components, and
  // `packages/design-system/CLAUDE.md` is explicit that no component arrives there as a side
  // effect of other work. They are a follow-up with a design system task in front of them.

  import { Button, Dialog, Input, Stack, Textarea } from '@hubtask/design-system/components';

  import { containers } from '../data/containers.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  import type { TransportError } from '@hubtask/sync-engine';

  interface Props {
    isOpen: boolean;
    /** `HUB` creates at the top level; `COLLECTION` creates under `parentId`. */
    type: 'HUB' | 'COLLECTION';
    /** The hub a collection goes into. Ignored for a hub, which sits in nothing. */
    parentId?: string;
    /** Ran after the write succeeded, with the container the server answered. */
    oncreated?: (id: string) => void;
  }

  let { isOpen = $bindable(), type, parentId, oncreated }: Props = $props();

  let name = $state('');
  let description = $state('');
  let isSaving = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let isNameFailure = $state(false);

  // Reopening starts a new answer rather than resuming the last refused one.
  $effect(() => {
    if (isOpen) return;
    name = '';
    description = '';
    failure = undefined;
    isNameFailure = false;
  });

  const isHub = $derived(type === 'HUB');

  async function create() {
    if (name.trim() === '' || isSaving) return;
    isSaving = true;
    failure = undefined;
    isNameFailure = false;
    try {
      // The `Idempotency-Key` is the caller's, as it is at every other write in this application:
      // pressing Create twice is one intent, not two hubs.
      const created = await containers.create(
        {
          type,
          parent_id: isHub ? null : parentId,
          name: name.trim(),
          description: description.trim() === '' ? null : description.trim(),
        },
        crypto.randomUUID(),
      );
      // It appears because the write invalidated the level and the engine re-read it. Nothing is
      // inserted into the sidebar or the list by hand.
      isOpen = false;
      oncreated?.(created.id);
    } catch (error) {
      const problem = error as TransportError;
      failure = renderProblem(problem, messages);
      isNameFailure =
        problem.detailCode === 'containers.name_taken' || failure.fields.has('/name');
    } finally {
      isSaving = false;
    }
  }
</script>

<Dialog
  bind:isOpen
  title={t(isHub ? 'app.workspace.create_hub' : 'app.workspace.create_collection')}
  dismissLabel={t('app.workspace.cancel')}
>
  <Stack gap="150">
    <Input
      label={t(isHub ? 'app.workspace.hub_name' : 'app.workspace.collection_name')}
      bind:value={name}
      error={isNameFailure ? failure?.message : undefined}
    />
    <Textarea label={t('app.workspace.description')} bind:value={description} />
    <!-- Everything that is not about the name is a sentence above the buttons rather than a field
         error, because there is no field for it to land on. -->
    {#if failure && !isNameFailure}
      <p class="failure">{failure.message}</p>
    {/if}
  </Stack>

  {#snippet actions()}
    <Button tone="secondary" onclick={() => (isOpen = false)}>{t('app.workspace.cancel')}</Button>
    <Button
      isBusy={isSaving}
      busyLabel={t('app.workspace.creating')}
      disabledReason={name.trim() === '' ? t('app.workspace.name_required') : undefined}
      onclick={create}
    >
      {t(isHub ? 'app.workspace.create_hub' : 'app.workspace.create_collection')}
    </Button>
  {/snippet}
</Dialog>

<style>
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); max-width: 64ch; }
</style>
