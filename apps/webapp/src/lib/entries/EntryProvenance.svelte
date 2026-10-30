<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Which of an entry's values came from AI (UC-AGT-05 check 5): one quiet line naming the fields
  // an accepted suggestion wrote, and on opening it each acceptance with its model, prompt and the
  // person who accepted it. The mark holds no value, so neither does this. Nothing is drawn where
  // no value came from AI, nor where the marks cannot be read - the entry is not less for it.

  import { untrack } from 'svelte';
  import { Icon } from '@hubtask/design-system/components';
  import type { ValueProvenanceList } from '@hubtask/sync-engine';

  import { accounts } from '../data/accounts.svelte.ts';
  import { aiFields, provenancePath } from '../data/agents.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';

  interface Props {
    itemId: string;
  }

  const { itemId }: Props = $props();

  // Keyed on the entry by its caller, so the path is fixed for its life.
  const read = resource<ValueProvenanceList>(untrack(() => provenancePath(itemId)));
  const marks = $derived(read.state.status === 'ready' ? read.state.data.data : []);
  const fields = $derived(aiFields(marks));
  const fieldWord = (name: string) => (messages.has(`app.provenance.field.${name}`) ? t(`app.provenance.field.${name}`) : name);

  $effect(() => {
    const ids = marks.map((each) => each.accepted_by).filter((id): id is string => Boolean(id));
    if (ids.length > 0) untrack(() => accounts.resolve(ids));
  });
</script>

{#if fields.length > 0}
  <details class="provenance" data-provenance>
    <summary><Icon name="sparkles" size="sm" />{t('app.provenance.line', { fields: fields.map(fieldWord).join(', ') })}</summary>
    <ul>
      {#each marks as mark (mark.id)}
        <li>
          {t('app.provenance.mark', {
            fields: mark.fields.map(fieldWord).join(', '),
            model: mark.model || t('app.provenance.model_unknown'),
            prompt: mark.prompt_version ? `${mark.prompt_id}@${mark.prompt_version}` : mark.prompt_id,
            name: mark.accepted_by ? (accounts.nameOf(mark.accepted_by) ?? t('app.people.unnamed')) : t('app.people.unnamed'),
            at: formatDateTime(mark.accepted_at, messages.locale),
          })}
        </li>
      {/each}
    </ul>
  </details>
{/if}

<style>
  .provenance { color: var(--text-subtle); font-size: var(--fs-075); }
  summary { display: inline-flex; align-items: center; gap: var(--sp-050); cursor: pointer; }
  ul { margin: var(--sp-050) 0 0; padding-inline-start: var(--sp-200); display: grid; gap: var(--sp-025); }
</style>
