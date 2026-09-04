<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Where an entry goes when it leaves the collection it is in.
  //
  // The one destination that cannot be a menu item on the row. Up, down, top, bottom, inside the
  // one above and out one level are all positions the reader can already see; another collection is
  // a choice out of everything the workspace holds, and a menu of that is a menu nobody can read.
  //
  // **It is also the only surface in this milestone that can lose something.** Invariant I-W6: a
  // label belongs to a collection (I-W3) and a bucket belongs to a board, so an entry carried to
  // another collection leaves both behind — reported rather than dropped in silence, which is what
  // the caller renders from the `MoveResult` this returns to it.
  //
  // Every hub's collections are read when this opens, and that is the one place it is right to.
  // `containers.svelte.ts` argues against reading them at boot because that would be a request per
  // hub for rows nobody asked to see; here somebody has asked, and a picker that offered only the
  // hubs they happened to have expanded would hide half the workspace.

  import { untrack } from 'svelte';

  import { Button, Dialog, Select } from '@hubtask/design-system/components';

  import { containers } from '../data/containers.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    isOpen?: boolean;
    /** The entry being moved, for the dialog's own name. */
    title: string;
    /** The collection it is in. Moving it there is not a move. */
    fromCollectionId: string;
    isBusy?: boolean;
    onmove: (collectionId: string) => void;
  }

  let { isOpen = $bindable(false), title, fromCollectionId, isBusy = false, onmove }: Props = $props();

  // `untrack` around the subscribing, for the reason every store in this application records: the
  // listener writes the level and writing it reads it, so an effect tracking that read cancels its
  // own subscription before the answer arrives. The dependency that matters — which hubs there are
  // — is read outside it on purpose.
  $effect(() => {
    if (!isOpen) return;
    const hubs = containers.hubs.map((hub) => hub.id);
    return untrack(() => {
      const stops = hubs.map((hubId) => containers.openLevel(hubId));
      return () => {
        for (const stop of stops) stop();
      };
    });
  });

  const destinations = $derived(
    containers.hubs.flatMap((hub) =>
      containers
        .collectionsOf(hub.id)
        // The collection it is already in is not a destination, and an archived one is read-only
        // (I-C3) — offering it would be offering a move the server refuses.
        .filter((collection) => collection.id !== fromCollectionId && !collection.effective_archived)
        .map((collection) => ({
          value: collection.id,
          label: t('app.move.destination_option', { hub: hub.name, collection: collection.name }),
        })),
    ),
  );

  let chosen = $state('');
</script>

<Dialog
  bind:isOpen
  title={t('app.rank.actions', { title })}
  dismissLabel={t('app.entries.cancel')}
>
  {#if destinations.length === 0}
    <p class="none">{t('app.move.no_destination')}</p>
  {:else}
    <Select
      label={t('app.move.destination')}
      placeholder={t('app.move.choose')}
      options={destinations}
      bind:value={chosen}
    />
    <p class="warning">{t('app.move.leaves_behind')}</p>
  {/if}

  {#snippet actions()}
    <Button tone="secondary" onclick={() => (isOpen = false)}>{t('app.entries.cancel')}</Button>
    <!-- Off with a reason rather than silently doing nothing: there is no `disabled` boolean in
         this system, and a button that answers a press with nothing is the silent ignoring the
         whole project has a rule against. -->
    <Button
      isBusy={isBusy}
      busyLabel={t('app.move.moving')}
      disabledReason={chosen === '' ? t('app.move.choose_first') : undefined}
      onclick={() => onmove(chosen)}
    >
      {t('app.move.confirm')}
    </Button>
  {/snippet}
</Dialog>

<style>
  .none,
  .warning { margin: 0; max-width: 64ch; font-size: var(--fs-075); color: var(--text-secondary); }

  /* Rule 3: the warning is not a colour on its own — it is a sentence saying what will be lost,
     and the colour only reinforces it. */
  .warning { color: var(--text-warning); }
</style>
