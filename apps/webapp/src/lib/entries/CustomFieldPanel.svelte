<!-- SPDX-License-Identifier: BUSL-1.1
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
  // **`USER` is F3-07's picker, handed into the slot F3-05 left for it.** Who may be named is the
  // memberships along the entry's path, which is the same list the assignee picker offers.

  import { AssigneeControl, CapabilityGate, Stack, CustomFieldRenderer } from '@hubtask/design-system/components';
  import type { FieldValue } from '@hubtask/design-system/components';
  import type { CustomFieldDefinition, WorkItem } from '@hubtask/sync-engine';

  import { accounts } from '../data/accounts.svelte.ts';
  import { supports } from '../data/capability.svelte.ts';
  import { customFields } from '../data/customfields.svelte.ts';
  import { definitionsFor, valueFor } from '../data/customfields.ts';
  import { people, type Path } from '../data/people.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  const { item, path }: { item: WorkItem; path: Path } = $props();

  const capability = $derived(supports(item.type, 'CUSTOM_FIELDS'));

  const applicable = $derived(
    definitionsFor(customFields.of(item.collection_id), item.type as string),
  );

  const values = $derived((item.custom_fields ?? {}) as Record<string, unknown>);

  /** Which key is being written, so its control shows the refusal and not the one beside it. */
  let failedKey = $state<string | undefined>(undefined);
  let failure = $state<string | undefined>(undefined);
  let savingKey = $state<string | undefined>(undefined);

  const candidateIds = $derived(people.candidates(path));
  const candidates = $derived(
    candidateIds.map((id) => ({ id, name: accounts.nameOf(id) ?? t('app.people.unnamed') })),
  );

  function write(definition: CustomFieldDefinition, next: FieldValue) {
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
      } catch (error) {
        failedKey = definition.key;
        failure = renderProblem(error as never, messages).message;
      } finally {
        savingKey = undefined;
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
    };
  }
</script>

<CapabilityGate
  status={capability.status}
  reason={capability.status === 'refused' ? t(capability.code, capability.params) : undefined}
  pendingLabel={t('app.fields.deciding')}
>
  <Stack gap="150">
    {#if applicable.length === 0}
      <p class="quiet">{t('app.fields.values_none')}</p>
    {:else}
      {#each applicable as definition (definition.id)}
        <div>
          <CustomFieldRenderer
            definition={drawn(definition)}
            value={(values[definition.key] ?? null) as FieldValue}
            unknownKindLabel={t('app.fields.unknown_kind')}
            emptyValueLabel={t('app.fields.empty_value')}
            disabledReason={savingKey === definition.key ? t('app.workspace.saving') : undefined}
            onChange={(next) => write(definition, next)}
          >
            {#snippet user({ value })}
              <AssigneeControl
                label={t('app.fields.pick_person')}
                {candidates}
                selected={typeof value === 'string' && value !== '' ? [value] : []}
                selection="single"
                filterLabel={t('app.fields.pick_filter')}
                emptyLabel={t('app.fields.pick_empty')}
                noMatchLabel={t('app.fields.pick_no_match')}
                chosenLabel={t('app.fields.pick_chosen')}
                unassignedLabel={t('app.fields.pick_none')}
                onSelect={(ids) => write(definition, ids[0] ?? null)}
              />
            {/snippet}
          </CustomFieldRenderer>
          {#if failedKey === definition.key && failure}
            <p class="failure">{failure}</p>
          {/if}
        </div>
      {/each}
    {/if}
  </Stack>
</CapabilityGate>

<style>
  .quiet { margin: 0; color: var(--text-secondary); }

  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
</style>
