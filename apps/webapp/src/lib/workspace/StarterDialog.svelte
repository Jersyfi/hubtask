<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The workspace's first start (UC-WRK-30 check 5): asked once, of whoever may shape a new
  // workspace (`starter_offered` on `GET /tenant`), what it is used for - for myself, our
  // household, my clients, our team -, and answered with a hub and collections on matching presets.
  //
  // **Closed first, then created.** `POST /tenant/starter:close` goes before the first hub, so an
  // interruption leaves a partial set rather than a second one; a skip - or dismissing the
  // dialog - closes it the same way. Nobody is asked twice.
  //
  // **Everything made is ordinary.** The hubs and collections are created through the same
  // `POST /containers` as any other, so each can be renamed, changed or removed; the dialog says so.

  import { untrack } from 'svelte';

  import { Button, Dialog, Radio, Stack } from '@hubtask/design-system/components';
  import type { TransportError } from '@hubtask/sync-engine';

  import { announcer } from '../announce.svelte.ts';
  import { containers } from '../data/containers.svelte.ts';
  import { workspace } from '../data/workspace.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import { PURPOSES, STARTER_SETS, type Purpose } from './starterSet.ts';

  interface Props {
    /** Ran after the set was made, with the hub to open. */
    onstarted?: (hubId: string) => void;
  }

  let { onstarted }: Props = $props();

  $effect(() => untrack(() => workspace.open()));

  let isOpen = $state(false);
  let isAnswered = $state(false);
  // Plain, not state: the dialog's onClose runs in an effect's teardown, where a state read answers
  // the value from before the change - a skip already sent would look unsent.
  let isClosing = false;
  let purpose = $state<Purpose>('MYSELF');
  let isBusy = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);

  // Opened once, when the workspace says the question is offered.
  $effect(() => {
    if (workspace.workspace?.starter_offered === true && !isAnswered) isOpen = true;
  });

  const options = $derived(
    PURPOSES.map((each) => ({ value: each, label: t(`app.starter.purpose.${each}`), hint: t(`app.starter.purpose_about.${each}`) })),
  );

  async function skip() {
    if (isClosing) return;
    isClosing = true;
    isAnswered = true;
    isOpen = false;
    try {
      await workspace.closeStarter();
    } catch {
      // Asked again on the next visit: nothing was made.
      isClosing = false;
      isAnswered = false;
    }
  }

  async function start() {
    if (isBusy) return;
    isBusy = true;
    failure = undefined;
    const set = STARTER_SETS[purpose];
    try {
      isClosing = true;
      await workspace.closeStarter();
      isAnswered = true;
      const hub = await containers.create({ type: 'HUB', name: t(set.hub.name), preset: set.hub.preset }, crypto.randomUUID());
      for (const collection of set.collections) {
        await containers.create(
          { type: 'COLLECTION', parent_id: hub.id, name: t(collection.name), ...(collection.preset ? { preset: collection.preset } : {}) },
          crypto.randomUUID(),
        );
      }
      announcer.say(t('app.starter.done'));
      isOpen = false;
      onstarted?.(hub.id);
    } catch (error) {
      failure = renderProblem(error as TransportError, messages);
    } finally {
      isBusy = false;
    }
  }
</script>

<!-- Dismissed without an answer is a skip: the question is not asked again. -->
<Dialog bind:isOpen title={t('app.starter.title')} dismissLabel={t('app.starter.skip')} onClose={() => void skip()}>
  <div data-starter>
    <Stack gap="150">
      <p class="hint">{t('app.starter.about')}</p>
      <Radio label={t('app.starter.question')} options={options} bind:value={purpose} />
      <p class="hint">{t('app.starter.changeable')}</p>
      {#if failure}<p class="failure" role="alert">{failure.message}</p>{/if}
    </Stack>
  </div>

  {#snippet actions()}
    <Button tone="secondary" onclick={() => void skip()}>{t('app.starter.skip')}</Button>
    <Button isBusy={isBusy} busyLabel={t('app.starter.starting')} onclick={() => void start()}>{t('app.starter.start')}</Button>
  {/snippet}
</Dialog>

<style>
  .hint { margin: 0; color: var(--text-subtle); font-size: var(--fs-075); max-width: 64ch; }
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); max-width: 64ch; }
</style>
