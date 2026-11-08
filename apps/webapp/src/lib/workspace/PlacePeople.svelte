<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Who holds a role at a hub or a collection, as faces in its head; pressing them opens the
  // people, the same dialog as *Settings → People*. One drawing for every head of the place - a
  // collection's on each of its views -, so the faces never come and go with the view.
  import { AvatarGroup, IconButton } from '@hubtask/design-system/components';

  import { accounts } from '../data/accounts.svelte.ts';
  import { containers } from '../data/containers.svelte.ts';
  import { people } from '../data/people.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';
  import MembersDialog from '../people/MembersDialog.svelte';

  interface Props {
    placeId: string;
  }
  const { placeId }: Props = $props();

  const place = $derived(containers.find(placeId));
  const path = $derived(
    place?.type === 'HUB' ? { hubId: place.id } : { hubId: place?.parent_id ?? undefined, collectionId: place?.id },
  );
  const scope = $derived(
    place?.type === 'HUB' ? { scopeType: 'HUB' as const, scopeId: place.id } : { scopeType: 'COLLECTION' as const, scopeId: place?.id ?? '' },
  );
  const memberIds = $derived(place ? [...new Set(people.along(path).map((membership) => membership.account_id))] : []);
  $effect(() => {
    if (place) people.open(path);
  });
  $effect(() => {
    accounts.resolve(memberIds);
  });

  let isOpen = $state(false);
</script>

{#if place}
  {#if memberIds.length === 0}
    <!-- Nobody to show yet: the way to the people stays, as their mark (ux-inventory.md §E9). -->
    <IconButton icon="users" label={t('app.people.title')} size="sm" data-opener="members" onclick={() => (isOpen = true)} />
  {:else}
    <!-- The faces are decoration on the button: its name is the word. -->
    <button type="button" class="members" aria-label={t('app.people.title')} data-opener="members" onclick={() => (isOpen = true)}>
      <span aria-hidden="true">
        <AvatarGroup
          people={memberIds.map((accountId) => ({
            name: accounts.nameOf(accountId) ?? t('app.people.unnamed'),
            src: accounts.pictureOf(accountId),
          }))}
          size="sm"
          max={4}
          overflowLabel={t('app.people.more_members', { count: String(Math.max(memberIds.length - 4, 0)) })}
        />
      </span>
    </button>
  {/if}
  <MembersDialog bind:isOpen title={t('app.people.title')} {scope} {path} />
{/if}

<style>
  .members {
    display: inline-flex;
    align-items: center;
    padding: var(--sp-025);
    border: 0;
    border-radius: var(--r-full);
    background: transparent;
    cursor: pointer;
  }

  .members:hover { background: var(--bg-surface-hover); }

  .members:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }
</style>
