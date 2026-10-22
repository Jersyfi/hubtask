<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Changing a hub's or a collection's colour and symbol (UC-WRK-33), with the fields the create
  // dialog offers.
  //
  // A dialog with a save rather than a write per press: the two choices are seen together on each
  // tile, and a person trying colours against a symbol should not leave a trail of versions behind
  // them. The version is the one the dialog opened with, so a change that lost a race is refused
  // rather than winning by being second (ADR-0025).

  import { untrack } from 'svelte';

  import { Button, Dialog, Stack } from '@hubtask/design-system/components';

  import { announcer } from '../announce.svelte.ts';
  import { containers } from '../data/containers.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import LookFields from './LookFields.svelte';
  import { chosenColour, chosenSymbol, lookBody, type PlaceSymbol } from './look.ts';

  import type { LabelToken } from '@hubtask/design-system';
  import type { Container, TransportError } from '@hubtask/sync-engine';

  interface Props {
    isOpen: boolean;
    container: Container;
  }

  let { isOpen = $bindable(), container }: Props = $props();

  let symbol = $state<PlaceSymbol | undefined>(undefined);
  let colour = $state<LabelToken | undefined>(undefined);
  let version = $state(0);
  let isSaving = $state(false);
  let failure = $state<string | undefined>(undefined);

  // Opening starts from what the place has now; what was tried and not saved is gone.
  $effect(() => {
    if (!isOpen) return;
    untrack(() => {
      symbol = chosenSymbol(container);
      colour = chosenColour(container);
      version = container.version;
      failure = undefined;
    });
  });

  async function save() {
    if (isSaving) return;
    isSaving = true;
    failure = undefined;
    try {
      await containers.update(container.id, lookBody(symbol, colour), version);
      announcer.say(t('app.look.saved_announced'));
      isOpen = false;
    } catch (error) {
      failure = renderProblem(error as TransportError, messages).message;
    } finally {
      isSaving = false;
    }
  }
</script>

<Dialog bind:isOpen title={t('app.look.title')} dismissLabel={t('app.workspace.cancel')}>
  <Stack gap="200">
    <LookFields type={container.type} bind:symbol bind:colour />
    {#if failure}<p class="failure" role="alert">{failure}</p>{/if}
  </Stack>

  {#snippet actions()}
    <Button tone="secondary" onclick={() => (isOpen = false)}>{t('app.workspace.cancel')}</Button>
    <Button isBusy={isSaving} busyLabel={t('app.workspace.saving')} onclick={save}>{t('app.workspace.save')}</Button>
  {/snippet}
</Dialog>

<style>
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); max-width: 64ch; }
</style>
