<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The values of the fields this installation added, on one entry.
  //
  // **One key per call.** `offline-sync.md` §4.2 makes the merge rule per key, so two devices
  // setting two different keys converge to both — and that is only true of a client that writes
  // them separately. A form with a save button that PUT the whole `custom_fields` document would
  // erase every key it had not loaded, which is why there is no save button here: each control
  // writes its own key when it changes, with the `If-Match` the entry carried.
  //
  // **The key is the label, because the contract carries no other.** `CustomFieldDefinition` has a
  // key, a kind, options, `is_required` and `applies_to` — and no display name. So the identifier
  // is what a reader sees, drawn as one. Inventing a prettier form of it here (title case, spaces
  // for underscores) would be this client writing a name the workspace did not choose.
  //
  // **A refusal lands on the field it came from.** One request writes one key, so a
  // `validation_failed` on that request is about that key whatever path it names — a `SELECT`
  // outside its options and a `NUMBER` that is not one both arrive this way.
  //
  // **`USER` and `USER_SET` are the member picker, handed into the renderer's slot for it.** Who may
  // be named is the memberships along the entry's path, which is the same list the assignee picker
  // offers. **`REFERENCE` chooses among the collection's entries** this replica holds, or one found
  // across every collection by search (UC-PLN-01's picker); the server holds the choice to what the
  // writer may see (`fields.value_not_an_entry`). A value outside the replica is named by resolving
  // it for the reader, so one out of their sight says only that (UC-PLN-01 check 4).

  import { AssigneeControl, Button, Select, Stack, CustomFieldRenderer } from '@hubtask/design-system/components';
  import type { FieldValue } from '@hubtask/design-system/components';
  import type { CustomFieldDefinition, WorkItem } from '@hubtask/sync-engine';

  import { accounts } from '../data/accounts.svelte.ts';
  import { customFields } from '../data/customfields.svelte.ts';
  import { calculatedOf, definitionsFor, isCalculated, valueFor } from '../data/customfields.ts';
  import { queue } from '../data/queue.svelte.ts';
  import { people, type Path } from '../data/people.svelte.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { announcer } from '../announce.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { decimalSeparatorOf, formatDecimal, parseDecimal } from '../i18n/number.ts';
  import { formatDuration, parseDuration } from '../i18n/duration.ts';
  import { items } from '../data/items.svelte.ts';
  import { resolved, targetLabel } from '../data/links.svelte.ts';
  import { mayReadField, mayWriteField, roles } from '../data/roles.svelte.ts';
  import { groups } from '../data/groups.svelte.ts';
  import { media } from '../data/media.svelte.ts';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { onMount, untrack } from 'svelte';
  import EntryPicker from './EntryPicker.svelte';
  import AttachmentChoice from './AttachmentChoice.svelte';
  import { renderProblem } from '../problem.ts';

  interface Props {
    item: WorkItem;
    path: Path;
    /**
     * One definition's key, when the panel is a details row's editor: the entry page
     * shows one row per definition, and each opens this panel for its own field alone. Without
     * it the panel draws every definition in force, as the section always did.
     */
    only?: string;
  }

  const { item, path, only }: Props = $props();

  // A NUMBER field is written and read the way the reader's locale writes numbers:
  // the decimal mark is the manifest's for the resolved locale, else Intl's, and the text is
  // parsed before it reaches the contract.
  const separator = $derived(decimalSeparatorOf(messages.locale, manifest.supportedLocales));
  const numbers = $derived({
    format: (value: number) => formatDecimal(value, messages.locale),
    parse: (text: string) => parseDecimal(text, separator),
    invalidLabel: t('app.fields.number_invalid', { separator }),
  });
  const durations = $derived({
    format: formatDuration,
    parse: (text: string) => parseDuration(text, separator),
    invalidLabel: t('app.fields.duration_invalid'),
  });

  /** The entries a reference may point at: the collection's, the entry itself left out. */
  const referable = $derived(
    items.inCollection(item.collection_id).filter((each) => each.id !== item.id),
  );
  function referenceOptions(value: unknown) {
    const options = [
      { value: '', label: t('app.fields.pick_entry_none') },
      ...referable.map((each) => ({ value: each.id, label: each.title })),
    ];
    // A value pointing outside what this replica holds stays chosen, named as the reader may see it.
    if (typeof value === 'string' && value !== '' && !referable.some((each) => each.id === value)) {
      const at = elsewhere.indexOf(value);
      const target = at < 0 ? undefined : named.targets[at];
      const label = target === undefined
        ? t('app.fields.pick_entry_elsewhere')
        : (targetLabel(target) ?? t('app.links.unavailable'));
      options.push({ value, label });
    }
    return options;
  }

  /** The chosen references this replica does not hold, resolved for the reader. */
  const elsewhere = $derived(
    Object.values((item.custom_fields ?? {}) as Record<string, unknown>).filter(
      (value): value is string =>
        typeof value === 'string' && /^[0-9a-f-]{36}$/i.test(value) && !referable.some((each) => each.id === value),
    ),
  );
  const named = resolved(() => elsewhere.map((id) => ({ kind: 'ENTRY' as const, id })));

  // A GROUP picks among the workspace's groups, read only where a field of the kind is drawn; the
  // server holds the choice to them (`fields.value_not_a_group`). A FILE's picker reads the entry's
  // attachments itself (AttachmentChoice).
  function groupOptions(value: unknown) {
    const options = [
      { value: '', label: t('app.fields.pick_group_none') },
      ...groups.all.map((group) => ({ value: group.id, label: group.name })),
    ];
    if (typeof value === 'string' && value !== '' && !groups.all.some((group) => group.id === value)) {
      options.push({ value, label: t('app.links.unavailable') });
    }
    return options;
  }

  /** The field whose search across collections is open. */
  let searchingKey = $state<string | undefined>(undefined);


  const applicable = $derived(
    definitionsFor(customFields.of(item.collection_id), item.type as string).filter(
      (definition) => only === undefined || definition.key === only,
    ),
  );

  const values = $derived((item.custom_fields ?? {}) as Record<string, unknown>);

  const wantsGroups = $derived(applicable.some((definition) => definition.kind === 'GROUP'));
  onMount(() => {
    if (untrack(() => wantsGroups)) return groups.open();
    return undefined;
  });

  /**
   * Whether the reader may write a restricted field here (UC-ID-27 check 4): what they hold at the
   * entry's collection carries `FIELD_WRITE:<key>`. Asked only where a field is restricted, and
   * until answered no control is drawn for one - the server would refuse it (`fields.restricted`).
   */
  const isAnyRestricted = $derived(
    applicable.some((definition) => definition.restricted === true || definition.read_restricted === true),
  );
  const mine = $derived(isAnyRestricted ? roles.mineAt(item.collection_id) : undefined);
  /** The fields the reader sees: a field hidden from readers only with its permission (check 3). */
  const visible = $derived(applicable.filter((definition) => mayReadField(definition, mine)));
  function isWritable(definition: CustomFieldDefinition): boolean {
    return mayWriteField(definition, mine);
  }

  /** A value as words, for a field shown and not edited. */
  function shown(definition: CustomFieldDefinition, held: unknown): string {
    if (held === null || held === undefined || held === '' || (Array.isArray(held) && held.length === 0)) return t('app.fields.empty_value');
    const one = (value: unknown): string => {
      if (typeof value === 'number') return numbers.format(value);
      if (typeof value === 'boolean') return value ? t('app.permissions.yes') : t('app.permissions.no');
      if (typeof value === 'string' && (definition.kind === 'USER' || definition.kind === 'USER_SET')) return accounts.nameOf(value) ?? t('app.people.unnamed');
      if (typeof value === 'string' && definition.kind === 'GROUP') return groups.nameOf(value) ?? t('app.links.unavailable');
      if (typeof value === 'string' && definition.kind === 'FILE') {
        media.resolve([value]);
        return media.fileNameOf(value) ?? t('app.links.unavailable');
      }
      if (typeof value === 'string' && definition.kind === 'DATE_TIME') return formatDateTime(value, messages.locale);
      return typeof value === 'string' ? value : JSON.stringify(value);
    };
    return Array.isArray(held) ? held.map(one).join(', ') : one(held);
  }

  /** Which key is being written, so its control shows the refusal and not the one beside it. */
  let failedKey = $state<string | undefined>(undefined);
  let failure = $state<string | undefined>(undefined);
  let savingKey = $state<string | undefined>(undefined);
  /**
   * A change made while the field's last one is still on its way. The field is never switched off
   * for "Saving…" (design-system.md §4, case 7): a change arriving meanwhile waits here and is
   * written after the one in flight, against the version that one answered, so the two cannot meet
   * as a conflict. Only the latest waits - it is the value the reader ended on.
   */
  const waiting = new Map<string, { definition: CustomFieldDefinition; next: FieldValue }>();

  const candidateIds = $derived(people.candidates(path));
  const candidates = $derived(
    candidateIds.map((id) => ({ id, name: accounts.nameOf(id) ?? t('app.people.unnamed'), src: accounts.pictureOf(id) })),
  );

  function write(definition: CustomFieldDefinition, next: FieldValue) {
    if (savingKey === definition.key) {
      waiting.set(definition.key, { definition, next });
      return;
    }
    savingKey = definition.key;
    failedKey = undefined;
    failure = undefined;
    void (async () => {
      try {
        await customFields.setValue(
          item.id,
          definition.key,
          valueFor(definition.kind, next),
          item.version,
        );
        announcer.say(t('app.fields.value_saved_announced', { key: definition.key }));
      } catch (error) {
        failedKey = definition.key;
        failure = renderProblem(error as never, messages).message;
      } finally {
        savingKey = undefined;
        const after = waiting.get(definition.key);
        waiting.delete(definition.key);
        if (after) write(after.definition, after.next);
      }
    })();
  }

  /** What the renderer draws a definition as. The key is the label; see the note above. */
  function drawn(definition: CustomFieldDefinition) {
    return {
      key: definition.key,
      label: definition.key,
      kind: definition.kind as string,
      options: (definition.options ?? []) as readonly string[],
      isRequired: definition.is_required ?? false,
      hint: hintOf(definition),
    };
  }

  /** The standing help a kind needs: the currency an amount is in, how a duration is written. */
  function hintOf(definition: CustomFieldDefinition): string | undefined {
    switch (definition.kind as string) {
      case 'MONEY':
        return definition.currency ? t('app.fields.money_hint', { currency: definition.currency }) : undefined;
      case 'DURATION':
        return t('app.fields.duration_hint');
      case 'RICH_TEXT':
        return t('app.fields.rich_text_hint');
      case 'LABEL':
        return t('app.fields.label_hint');
      case 'IP_ADDRESS':
        return t('app.fields.address_hint');
      default:
        return undefined;
    }
  }
</script>

<!-- No gate here: the entry page draws this panel only where the type carries the capability
     (`carries`, ItemView), so a refusal could never reach it - a refused capability is an absent
     row, not a dead one (design-system.md §11.8). -->
  <Stack gap="150">
    {#if visible.length === 0}
      <p class="quiet">{t('app.fields.values_none')}</p>
    {:else}
      {#each visible as definition (definition.id)}
        {@const held = values[definition.key]}
        {#if isCalculated(definition.kind as string)}
          <!-- The server computes it from the entry's other fields: shown, never an input (UC-WRK-12).
               While the entry's writes wait to be sent, the browser computes it the same way. -->
          {@const shownValue = queue.isPending(item.id) ? calculatedOf(definition, customFields.of(item.collection_id), values) : held}
          <div class="calculated" data-calculated={definition.key}>
            <span class="label">{definition.key}</span>
            <output>{typeof shownValue === 'number' ? numbers.format(shownValue) : t('app.fields.calculated_empty')}</output>
            <p class="quiet">{t('app.fields.calculated_hint')}</p>
          </div>
        {:else if !isWritable(definition)}
          <!-- Restricted, and the reader holds no role naming its permission: shown, not edited. -->
          <div class="calculated" data-restricted={definition.key}>
            <span class="label">{definition.key}</span>
            <output>{shown(definition, held)}</output>
            <p class="quiet">{t('app.fields.restricted_read_only')}</p>
          </div>
        {:else}
        <div>
          <CustomFieldRenderer
            definition={drawn(definition)}
            value={(values[definition.key] ?? null) as FieldValue}
            unknownKindLabel={t('app.fields.unknown_kind')}
            emptyValueLabel={t('app.fields.empty_value')}
            {numbers}
            {durations}
            onChange={(next) => write(definition, next)}
          >
            {#snippet user({ value, multiple })}
              <AssigneeControl
                label={t('app.fields.pick_person')}
                {candidates}
                selected={Array.isArray(value)
                  ? [...(value as readonly string[])]
                  : typeof value === 'string' && value !== ''
                    ? [value]
                    : []}
                selection={multiple ? 'multiple' : 'single'}
                filterLabel={t('app.fields.pick_filter')}
                locale={messages.locale}
                emptyLabel={t('app.fields.pick_empty')}
                noMatchLabel={t('app.fields.pick_no_match')}
                chosenLabel={t('app.fields.pick_chosen')}
                unassignedLabel={t('app.fields.pick_none')}
                onSelect={(ids) => write(definition, multiple ? ids : (ids[0] ?? null))}
              />
            {/snippet}
            {#snippet group({ value })}
              <Select
                label={t('app.fields.pick_group')}
                size="sm"
                options={groupOptions(value)}
                value={typeof value === 'string' ? value : ''}
                onchange={(event) => write(definition, (event.currentTarget as HTMLSelectElement).value || null)}
              />
            {/snippet}
            {#snippet file({ value })}
              <AttachmentChoice itemId={item.id} {value} onchoose={(id) => write(definition, id)} />
            {/snippet}
            {#snippet reference({ value })}
              <Select
                label={t('app.fields.pick_entry')}
                size="sm"
                options={referenceOptions(value)}
                value={typeof value === 'string' ? value : ''}
                onchange={(event) => write(definition, (event.currentTarget as HTMLSelectElement).value || null)}
              />
              {#if searchingKey === definition.key}
                <EntryPicker
                  excluded={new Set([item.id])}
                  onchoose={(found) => {
                    if (!found) return;
                    searchingKey = undefined;
                    write(definition, found.id);
                  }}
                />
              {:else}
                <Button size="sm" tone="subtle" icon="search" data-reference-search={definition.key} onclick={() => (searchingKey = definition.key)}>
                  {t('app.fields.pick_entry_search')}
                </Button>
              {/if}
            {/snippet}
          </CustomFieldRenderer>
          {#if failedKey === definition.key && failure}
            <p class="failure" role="alert">{failure}</p>
          {/if}
        </div>
        {/if}
      {/each}
    {/if}
  </Stack>

<style>
  .quiet { margin: 0; color: var(--text-secondary); }

  .calculated { display: grid; gap: var(--sp-050); }
  .calculated .label { font-size: var(--fs-075); color: var(--text-secondary); }
  .calculated output { font-variant-numeric: tabular-nums; }

  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
</style>
