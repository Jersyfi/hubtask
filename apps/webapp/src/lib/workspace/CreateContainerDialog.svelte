<!-- SPDX-License-Identifier: Apache-2.0
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
  // the design system grows in the order of design-system.md §4, never as a side effect of other
  // work, so this dialog offers neither.
  //
  // *Private* is offered for a hub where the workspace has more than one person
  // (`private_hubs_offered`, UC-ID-16 check 8): a workspace of one has nobody to keep a hub from.
  // Every person may make one, whatever their role, so the switch carries no gate of its own.
  // Somebody without the right to shape the workspace may create *only* a private hub of their own
  // (UC-ID-16 check 1): for them the dialog is that and nothing else - no switch, the hub is private.
  // Whoever opens it decides which, from the manifest (`hubCreation`).

  import { untrack } from 'svelte';

  import { Button, Dialog, Input, Stack, Switch, Textarea } from '@hubtask/design-system/components';

  import { containers } from '../data/containers.svelte.ts';
  import { workspace } from '../data/workspace.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  import type { TransportError } from '@hubtask/sync-engine';

  interface Props {
    isOpen: boolean;
    /** `HUB` creates at the top level; `COLLECTION` creates under `parentId`. */
    type: 'HUB' | 'COLLECTION';
    /** The hub a collection goes into. Ignored for a hub, which sits in nothing. */
    parentId?: string;
    /**
     * A hub that can only be private: the reader may not create a shared one. Ignored for a
     * collection.
     */
    isPrivateOnly?: boolean;
    /** Ran after the write succeeded, with the container the server answered. */
    oncreated?: (id: string) => void;
  }

  let { isOpen = $bindable(), type, parentId, isPrivateOnly = false, oncreated }: Props = $props();

  let name = $state('');
  let description = $state('');
  let isPrivate = $state(false);
  let isSaving = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let isNameFailure = $state(false);

  // Reopening starts a new answer rather than resuming the last refused one.
  $effect(() => {
    if (isOpen) return;
    name = '';
    description = '';
    isPrivate = false;
    failure = undefined;
    isNameFailure = false;
  });

  const isHub = $derived(type === 'HUB');

  $effect(() => {
    if (!isOpen || type !== 'HUB' || isPrivateOnly) return;
    return untrack(() => workspace.open());
  });
  const isPrivateHubOnly = $derived(isHub && isPrivateOnly);
  const isPrivacyOffered = $derived(isHub && !isPrivateOnly && workspace.workspace?.private_hubs_offered === true);
  const title = $derived(t(isPrivateHubOnly ? 'app.workspace.create_private_hub' : isHub ? 'app.workspace.create_hub' : 'app.workspace.create_collection'));

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
          ...(isPrivateHubOnly || (isPrivacyOffered && isPrivate) ? { private: true } : {}),
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
  {title}
  dismissLabel={t('app.workspace.cancel')}
>
  <Stack gap="150">
    <Input
      label={t(isHub ? 'app.workspace.hub_name' : 'app.workspace.collection_name')}
      bind:value={name}
      error={isNameFailure ? failure?.message : undefined}
    />
    <Textarea label={t('app.workspace.description')} bind:value={description} />
    {#if isPrivateHubOnly}
      <p class="private">{t('app.workspace.private_hint')}</p>
    {:else if isPrivacyOffered}
      <Switch label={t('app.workspace.private')} hint={t('app.workspace.private_hint')} bind:checked={isPrivate} />
    {/if}
    <!-- Everything that is not about the name is a sentence above the buttons rather than a field
         error, because there is no field for it to land on. -->
    {#if failure && !isNameFailure}
      <p class="failure" role="alert">{failure.message}</p>
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
      {title}
    </Button>
  {/snippet}
</Dialog>

<style>
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); max-width: 64ch; }
  .private { margin: 0; color: var(--text-subtle); font-size: var(--fs-075); max-width: 64ch; }
</style>
