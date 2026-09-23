<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What a search is narrowed by (ADR-0063 decision 4, ADR-0064): one chip per question, each
  // opening its answers, each saying how many are chosen.
  //
  // **Only what the installation reports.** A chip is offered where `/meta/capabilities` names its
  // field in `query_fields`; one it does not name is not a control that would always refuse.
  //
  // **Two of them need a collection**, and that is the product's shape rather than an omission: a
  // label belongs to a collection and the people who may be assigned are the ones with a
  // membership along its path, so "every label in the workspace" is not a list this API has. Choose
  // where first and they appear.
  //
  // The arithmetic — what a chip holds, what the address carries, what it compiles to — is
  // `data/searchfilters.ts`, pure and tested beside itself.

  import { Badge, Button, Checkbox, Popover, Stack } from '@hubtask/design-system/components';

  import { containers } from '../data/containers.svelte.ts';
  import { labels } from '../data/labels.svelte.ts';
  import { accounts } from '../data/accounts.svelte.ts';
  import { people } from '../data/people.svelte.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { queryFields } from '../data/query.ts';
  import {
    CHIPS,
    CHIP_FIELDS,
    STATES,
    TYPES,
    WHENS,
    clearChip,
    countOf,
    toggle,
    type ChipName,
    type Chosen,
  } from '../data/searchfilters.ts';
  import { humanise } from '../i18n/messages.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    chosen: Chosen;
    onchange: (chosen: Chosen) => void;
  }

  const { chosen, onchange }: Props = $props();

  /** The fields this installation lets anything be filtered by. */
  const reported = $derived(new Set(queryFields(manifest.value).map((field) => field.field)));

  /** The one collection chosen, where exactly one is: what the label and the people chips need. */
  const collectionId = $derived(chosen.where?.length === 1 ? chosen.where[0] : undefined);

  $effect(() => {
    if (!collectionId) return;
    return labels.open(collectionId);
  });
  $effect(() => {
    if (!collectionId) return;
    return people.open({ collectionId });
  });

  /** The answers of one chip, as value and word. */
  function answers(chip: ChipName): { value: string; label: string }[] {
    switch (chip) {
      case 'type':
        // The manifest's own names, humanised the way the entry list already does one.
        return TYPES.map((value) => ({ value, label: humanise(value.toLowerCase()) }));
      case 'state':
        return STATES.map((value) => ({ value, label: t(`app.search.state_${value}`) }));
      case 'when':
        return WHENS.map((value) => ({ value, label: t(`app.search.when_${value}`) }));
      case 'where':
        return containers.hubs.flatMap((hub) =>
          containers.collectionsOf(hub.id).map((collection) => ({ value: collection.id, label: collection.name })),
        );
      case 'label':
        return collectionId
          ? labels.of(collectionId).map((label) => ({ value: label.id, label: label.name }))
          : [];
      case 'who':
        return collectionId
          ? people
              .candidates({ collectionId })
              .map((id) => ({ value: id, label: accounts.nameOf(id) ?? t('app.people.unnamed') }))
          : [];
      default:
        return [];
    }
  }

  /** Which chips are drawn: reported by the installation, and with something to answer. */
  const offered = $derived(
    CHIPS.filter((chip) => reported.has(CHIP_FIELDS[chip])).map((chip) => ({
      chip,
      options: answers(chip),
    })),
  );
</script>

<div class="chips" role="group" aria-label={t('app.search.narrow')}>
  {#each offered as { chip, options } (chip)}
    {#if options.length > 0}
      <Popover label={t(`app.search.chip_${chip}`)}>
        {#snippet trigger(props)}
          <button type="button" class="chip" data-chosen={countOf(chosen, chip) > 0 ? '' : undefined} {...props}>
            <span>{t(`app.search.chip_${chip}`)}</span>
            {#if countOf(chosen, chip) > 0}
              <Badge>{String(countOf(chosen, chip))}</Badge>
            {/if}
          </button>
        {/snippet}
        <Stack gap="100">
          {#each options as option (option.value)}
            <Checkbox
              label={option.label}
              checked={(chosen[chip] ?? []).includes(option.value)}
              onchange={() => onchange(toggle(chosen, chip, option.value))}
            />
          {/each}
          {#if countOf(chosen, chip) > 0}
            <div>
              <Button size="sm" tone="subtle" onclick={() => onchange(clearChip(chosen, chip))}>
                {t('app.search.chip_clear')}
              </Button>
            </div>
          {/if}
        </Stack>
      </Popover>
    {/if}
  {/each}
</div>

<style>
  .chips {
    display: flex;
    flex-wrap: wrap;
    gap: var(--sp-100);
  }

  .chip {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-050);
    min-block-size: var(--density-control-sm-min);
    padding-inline: var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-full);
    background: var(--bg-surface);
    color: var(--text-secondary);
    font: inherit;
    font-size: var(--fs-075);
    cursor: pointer;
  }

  .chip:hover { border-color: var(--border-default); color: var(--text-primary); }

  .chip[data-chosen] {
    border-color: var(--accent-primary);
    color: var(--text-primary);
    font-weight: var(--fw-medium);
  }

  .chip:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }
</style>
