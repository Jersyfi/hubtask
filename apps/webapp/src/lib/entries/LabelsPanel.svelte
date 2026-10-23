<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The labels on one entry, from its own page: what the list's row does behind its
  // "Labels" control, as a panel a details row opens. The same set - a label belongs to the
  // collection (I-W3) - and the same write, `labels.setOnItem`; nothing about a label is decided
  // here. Offered only for a type whose profile has LABELS, with the reason where it has not
  // (domain-model.md §2: refused, never ignored).

  import { untrack } from 'svelte';

  import { Inline, LabelChip, LabelPicker, Stack } from '@hubtask/design-system/components';
  import type { WorkItem } from '@hubtask/sync-engine';

  import { announcer } from '../announce.svelte.ts';
  import { offerUndo } from './undo.svelte.ts';
  import { labels } from '../data/labels.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    item: WorkItem;
    /** Why the labels may not change: archived, or a role that may not. */
    disabledReason?: string;
  }

  const { item, disabledReason }: Props = $props();

  $effect(() => {
    const wanted = item.collection_id;
    return untrack(() => labels.open(wanted));
  });

  const available = $derived(
    labels.of(item.collection_id).map((entry) => ({ id: entry.id, name: entry.name, colorToken: entry.color_token, description: entry.description })),
  );
  const carried = $derived((item.label_ids ?? []).map((id) => available.find((each) => each.id === id)).filter((each) => each !== undefined));

  let failure = $state<string | undefined>(undefined);

  async function toggle(labelId: string) {
    failure = undefined;
    try {
      const isOn = (item.label_ids ?? []).includes(labelId);
      await labels.setOnItem(item.id, labelId, !isOn, crypto.randomUUID());
      // Taking one off is at once, with the way back (UC-WRK-28 check 3).
      if (isOn) offerUndo({ kind: 'label-removed', itemId: item.id, labelId, title: item.title });
      else announcer.say(t('app.labels.toggled_announced'));
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    }
  }
</script>

<!-- No gate here: the entry page draws this panel only where the type carries the capability
     (`carries`, ItemView), so a refusal could never reach it - a refused capability is an absent
     row, not a dead one (design-system.md §11.8). -->
  <Stack gap="150">
    {#if carried.length > 0}
      <Inline gap="050">
        {#each carried as entry (entry.id)}
          <LabelChip
            name={entry.name}
            colorToken={entry.colorToken}
            description={entry.description}
            removeLabel={disabledReason ? undefined : t('app.labels.remove_from', { name: entry.name, title: item.title })}
            onRemove={() => toggle(entry.id)}
          />
        {/each}
      </Inline>
    {/if}
    {#if disabledReason}
      <p class="quiet">{disabledReason}</p>
    {:else}
      <LabelPicker
        label={t('app.labels.on_entry', { title: item.title })}
        labels={available}
        selected={item.label_ids ?? []}
        filterLabel={t('app.labels.filter')}
        locale={messages.locale}
        emptyLabel={t('app.labels.none_yet')}
        noMatchLabel={t('app.labels.no_match')}
        onToggle={toggle}
      />
    {/if}
    {#if failure}<p class="failure" role="alert">{failure}</p>{/if}
  </Stack>

<style>
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
</style>
