<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The labels on one entry, from its own page (F9-08): what the list's row does behind its
  // "Labels" control, as a panel a details row opens. The same set - a label belongs to the
  // collection (I-W3) - and the same write, `labels.setOnItem`; nothing about a label is decided
  // here. Offered only for a type whose profile has LABELS, with the reason where it has not
  // (domain-model.md §2: refused, never ignored).

  import { untrack } from 'svelte';

  import { CapabilityGate, Inline, LabelChip, LabelPicker, Stack } from '@hubtask/design-system/components';
  import type { WorkItem } from '@hubtask/sync-engine';

  import { announcer } from '../announce.svelte.ts';
  import { supports } from '../data/capability.svelte.ts';
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

  const capability = $derived(supports(item.type, 'LABELS'));
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
      announcer.say(t('app.labels.toggled_announced'));
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    }
  }
</script>

<CapabilityGate
  status={capability.status}
  reason={capability.status === 'refused' ? t(capability.code, capability.params) : undefined}
  pendingLabel={t('app.fields.deciding')}
>
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
</CapabilityGate>

<style>
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
</style>
