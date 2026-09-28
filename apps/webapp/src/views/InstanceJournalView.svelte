<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The installation's own record (SI-17, H-06, audit.md §6).
  //
  // **Evidence of acts whose per-tenant trail cannot hold them.** After a hard delete the
  // workspace's own audit chain is gone by design, which is the reason this record exists — and
  // why its rows name a workspace by a bare identifier and a slug rather than by a reference.
  //
  // **Identifiers, slugs, counts and moments. Never content.** A journal carrying a title or a URL
  // would be a journal carrying somebody's data into a place nothing ever deletes from, so the
  // details are drawn as the counts and moments they are and this screen has no way to ask for
  // more.
  //
  // The action is a code and is rendered here (ADR-0011): the server holds no display text, and an
  // action this build has no word for is shown as the code rather than as a blank.

  import { untrack } from 'svelte';

  import { Banner, PageHeader, Spinner, Stack, Table } from '@hubtask/design-system/components';

  import InstanceGate from '../lib/instance/InstanceGate.svelte';
  import { instance } from '../lib/data/instance.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';

  interface Props {
    onnavigate: (path: string) => void;
  }

  const { onnavigate }: Props = $props();

  $effect(() => {
    const stop = untrack(() => instance.openJournal());
    return stop;
  });

  const reading = $derived(instance.journalState);
  const entries = $derived(instance.journal);
  const unreadable = $derived(
    reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined,
  );

  const columns = $derived([
    { id: 'occurred', label: t('app.instance.column_when') },
    { id: 'action', label: t('app.instance.column_action') },
    { id: 'workspace', label: t('app.instance.column_workspace') },
    { id: 'actor', label: t('app.instance.column_actor') },
  ]);

  /**
   * The word for an action, or the code where this build has none.
   *
   * The catalogue is asked rather than a table here: an installation running ahead of this client
   * records actions it has no word for, and a blank cell would hide an act rather than name it.
   */
  function wordFor(action: string): string {
    const code = `app.instance.action.${action.replaceAll('.', '_')}`;
    const rendered = messages.t(code, {});
    return rendered === code ? action : rendered;
  }

  /** The details, as the one line of counts and moments they are. */
  function summarise(details: Record<string, unknown> | undefined): string {
    if (!details) return '';
    return Object.entries(details)
      .map(([key, value]) => `${key}: ${String(value)}`)
      .join(' · ');
  }

  $effect(() => page.entitle(t('app.instance.journal')));
</script>

<Stack gap="300">
  <PageHeader title={t('app.instance.journal')} isTitleInBar={viewport.isCompact} />

  <InstanceGate onleave={() => onnavigate('/')}>
    <Stack gap="200">
      <p class="prose">{t('app.instance.journal_intro')}</p>

      {#if reading.status === 'loading' || reading.status === 'idle'}
        <p class="quiet">
          <Spinner label={t('app.instance.reading')} />
          <span>{t('app.instance.reading')}</span>
        </p>
      {:else if unreadable}
        <Banner tone="danger" title={unreadable.message}>
          {#if unreadable.reference}{t('app.error_reference', { request_id: unreadable.reference })}{/if}
        </Banner>
      {:else if entries.length === 0}
        <Banner tone="info">{t('app.instance.journal_empty')}</Banner>
      {:else}
        <Table label={t('app.instance.journal')} isLabelHidden columns={columns}>
          {#each entries as entry (entry.id)}
            <tr>
              <td class="quiet-cell">{entry.occurred_at}</td>
              <td>
                <span class="action">{wordFor(entry.action)}</span>
                {#if entry.details}
                  <span class="quiet-cell">{summarise(entry.details)}</span>
                {/if}
              </td>
              <td>
                {#if entry.tenant_slug}
                  <span class="action">{entry.tenant_slug}</span>
                {/if}
                {#if entry.tenant_id}
                  <span class="mono">{entry.tenant_id}</span>
                {/if}
              </td>
              <td class="quiet-cell">{entry.actor_label ?? ''}</td>
            </tr>
          {/each}
        </Table>
      {/if}
    </Stack>
  </InstanceGate>
</Stack>

<style>
  .prose { margin: 0; max-inline-size: 60ch; }
  .quiet { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }
  .quiet-cell { display: block; color: var(--text-secondary); font-size: var(--fs-100); }
  .action { display: block; font-weight: var(--fw-medium); }
  .mono { display: block; font-family: var(--font-mono); font-size: var(--fs-100); overflow-wrap: anywhere; }
</style>
