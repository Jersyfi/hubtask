<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One registered app as its record page (page-system §7.2): its redirect addresses as its list,
  // what it is and when it was registered as facts, and removing it in the head's ⋯ and the danger
  // zone, behind a Dialog S that says what it costs.
  //
  // No *Edit*: the contract changes no registered app - one with other addresses is registered
  // anew. Its secret was shown once, when it was registered, and exists nowhere to show again.

  import { untrack } from 'svelte';

  import { DataTable, EmptyState, RecordPage, type DataColumn, type RecordList } from '@hubtask/design-system/components';

  import { apps } from '../lib/data/apps.svelte.ts';
  import { mayHere } from '../lib/data/capability.svelte.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import RemoveAppDialog from '../lib/machines/RemoveAppDialog.svelte';

  const { id, onnavigate }: { id: string; onnavigate: (path: string) => void } = $props();

  const LIST = '/administration/machines?tab=apps';

  $effect(() => untrack(() => apps.open()));

  const reading = $derived(apps.state);
  const app = $derived(apps.all.find((each) => each.id === id));
  const name = $derived(app?.name ?? t('app.apps.kind'));
  $effect(() => page.entitle(name));

  /** Whether the reader may remove apps (`MANAGE_MEMBERS`), as the server asks. */
  const mayManage = $derived(mayHere('MANAGE_MEMBERS').status === 'permitted');
  let isRemoving = $state(false);

  const lists = $derived<RecordList[]>(app ? [{ id: 'uris', title: t('app.apps.uris'), count: String(app.redirect_uris.length) }] : []);
  const columns = $derived<DataColumn[]>([{ id: 'uri', label: t('app.apps.uri_column'), priority: 1 }]);
</script>

<RecordPage
  record={{
    kind: t('app.apps.kind'),
    title: name,
    back: { label: t('app.apps.title'), href: LIST, onnavigate: () => onnavigate(LIST) },
    status: app ? { label: app.confidential ? t('app.apps.confidential') : t('app.apps.public'), tone: app.confidential ? 'info' : 'neutral' } : undefined,
    menu:
      mayManage && app
        ? { label: t('app.page.more_for', { name }), items: [{ id: 'remove', label: t('app.apps.remove_menu'), isDestructive: true }], onselect: () => (isRemoving = true) }
        : undefined,
  }}
  {lists}
  facts={app
    ? {
        title: t('app.apps.facts_title'),
        rows: [
          { label: t('app.apps.kind_column'), value: app.confidential ? t('app.apps.confidential') : t('app.apps.public') },
          { label: t('app.apps.registered_at'), value: formatDateTime(app.created_at, messages.locale) },
        ],
      }
    : undefined}
  danger={mayManage && app
    ? {
        title: t('app.page.danger_zone'),
        acts: [{ id: 'remove', title: t('app.apps.remove_zone_title'), cost: t('app.apps.remove_cost'), actionLabel: t('app.apps.remove_menu') }],
        onact: () => (isRemoving = true),
      }
    : undefined}
  refused={reading.status === 'ready' && !app ? notHere : undefined}
>
  {#snippet list(item: RecordList)}
    <DataTable label={item.title} {columns} rows={app?.redirect_uris ?? []} key={(uri: string) => uri}>
      {#snippet cell(uri: string)}<span class="uri">{uri}</span>{/snippet}
    </DataTable>
  {/snippet}
</RecordPage>

<RemoveAppDialog app={isRemoving ? app : undefined} onClose={() => (isRemoving = false)} onRemoved={() => onnavigate(LIST)} />

{#snippet notHere()}
  <EmptyState kind="settled" title={t('app.apps.not_here')} />
{/snippet}

<style>
  .uri { font-family: var(--font-mono); font-size: var(--fs-075); overflow-wrap: anywhere; }
</style>
