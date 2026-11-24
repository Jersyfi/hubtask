<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Own domains* on the installation's *Workspaces* screen (UC-INS-15 check 10): every domain the
  // workspaces' owners set up, with its workspace and state, and *Block domain* with a reason the
  // owner reads word for word and the instance journal keeps. A blocked row has no menu: lifting a
  // block is not offered. Blocking a main domain says that the subdomain becomes main in the same
  // step (check 9). A DataTable (page-system PS-4) - under the workspaces with its heading, and on a
  // workspace's record page narrowed to its own domains, where the page draws the heading.

  import { untrack } from 'svelte';

  import { Badge, Banner, Button, DataTable, Dialog, EmptyState, Input, Stack, type DataColumn } from '@hubtask/design-system/components';
  import TableSection from '../page/TableSection.svelte';
  import type { AdminOwnDomain } from '@hubtask/sync-engine';
  import { TransportError } from '@hubtask/sync-engine';

  import { toneOf } from '../address/owndomain.ts';
  import { instance } from '../data/instance.svelte.ts';
  import { formatLongDay } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem, type RenderedProblem } from '../problem.ts';
  import FailureNote from '../frame/FailureNote.svelte';

  interface Props {
    /** One workspace's domains only, under the record page's own heading. */
    tenantId?: string;
  }

  const { tenantId }: Props = $props();

  $effect(() => untrack(() => instance.openOwnDomains()));

  const rows = $derived(tenantId === undefined ? instance.ownDomains : instance.ownDomains.filter((row) => row.tenant_id === tenantId));

  let blocking = $state<AdminOwnDomain | undefined>(undefined);
  let reason = $state('');
  let working = $state(false);
  let failure = $state<RenderedProblem | undefined>(undefined);

  const columns = $derived<DataColumn[]>([
    { id: 'domain', label: t('app.instance.own_domain_domain'), priority: 1 },
    ...(tenantId === undefined ? [{ id: 'workspace', label: t('app.instance.own_domain_workspace'), priority: 2 as const }] : []),
    { id: 'state', label: t('app.instance.own_domain_state'), priority: 2, isStatus: true },
    { id: 'since', label: t('app.instance.own_domain_since'), priority: 3 },
  ]);
  const keyOf = (row: AdminOwnDomain) => `${row.state}:${row.domain}`;

  function open(row: AdminOwnDomain): void {
    blocking = row;
    reason = '';
    failure = undefined;
  }

  async function block(): Promise<void> {
    if (!blocking) return;
    working = true;
    failure = undefined;
    try {
      await instance.blockOwnDomain(blocking.domain, reason);
      blocking = undefined;
    } catch (error) {
      if (!(error instanceof TransportError)) throw error;
      failure = renderProblem(error, messages);
    } finally {
      working = false;
    }
  }
</script>

{#snippet domains()}
  <DataTable
    label={t('app.instance.own_domains')}
    {columns}
    {rows}
    key={keyOf}
    rowLabel={(row: AdminOwnDomain) => row.domain}
    menu={(row: AdminOwnDomain) => (row.state === 'BLOCKED' ? undefined : { label: t('app.instance.own_domain_actions', { domain: row.domain }), items: [{ id: 'block', label: t('app.instance.own_domain_block'), isDestructive: true }] })}
    onmenu={(row: AdminOwnDomain) => open(row)}
    actionsLabel={t('app.instance.own_domain_what')}
    sheetDismissLabel={t('app.page.sheet_close')}
    sheetGrabLabel={t('app.page.sheet_grab')}
  >
    {#snippet cell(row: AdminOwnDomain, column: DataColumn)}
      {#if column.id === 'domain'}
        <span class="mono" data-own-domain={row.domain}>{row.domain}</span>
      {:else if column.id === 'workspace'}
        {row.tenant_name || t('app.instance.own_domain_no_workspace')}
      {:else if column.id === 'state'}
        <span class="row">
          <Badge tone={toneOf(row.state)}>{t(`app.address.own.state_${row.state.toLowerCase()}`)}</Badge>
          {#if row.is_main}<Badge tone="neutral">{t('app.instance.own_domain_main')}</Badge>{/if}
        </span>
        {#if row.state === 'BLOCKED' && row.blocked_reason}
          <span class="cellnote">{row.blocked_reason}</span>
        {/if}
      {:else}
        <span class="quiet">{formatLongDay(row.since, messages.locale)}</span>
      {/if}
    {/snippet}
    {#snippet empty()}<EmptyState kind="unused" title={t('app.instance.own_domains_none')} />{/snippet}
  </DataTable>
{/snippet}

{#if tenantId !== undefined}
  {@render domains()}
{:else if rows.length > 0}
  <TableSection title={t('app.instance.own_domains')} note={String(rows.length)} description={t('app.instance.own_domains_intro')}>
    {@render domains()}
  </TableSection>
{/if}

<Dialog
  size="s"
  title={t('app.instance.own_domain_block_title', { domain: blocking?.domain ?? '' })}
  isOpen={blocking !== undefined}
  dismissLabel={t('app.instance.cancel')}
  onClose={() => (blocking = undefined)}
>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => (blocking = undefined)}>{t('app.instance.cancel')}</Button>
    <Button tone="danger" isBusy={working} busyLabel={t('app.instance.working')} onclick={() => void block()}>
      {t('app.instance.own_domain_block')}
    </Button>
  {/snippet}
  <Stack gap="150">
    <Banner tone="warning">
      {blocking?.is_main && blocking.address_host
        ? t('app.instance.own_domain_block_cost_main', { workspace: blocking.tenant_name, host: blocking.address_host })
        : t('app.instance.own_domain_block_cost')}
    </Banner>
    {#if failure && !failure.fields.get('/reason')}
      <FailureNote problem={failure} />
    {/if}
    <Input
      label={t('app.instance.own_domain_reason')}
      hint={t('app.instance.own_domain_reason_hint')}
      isRequired
      maxlength={500}
      bind:value={reason}
      error={failure?.fields.get('/reason')}
    />
  </Stack>
</Dialog>

<style>
  .mono { font-family: var(--font-mono); overflow-wrap: anywhere; }
  .quiet { color: var(--text-subtle); font-size: var(--fs-075); }
  .row { display: flex; flex-wrap: wrap; gap: var(--sp-050); }
  .cellnote { display: block; margin-block-start: var(--sp-050); font-size: var(--fs-075); color: var(--text-subtle); }
</style>
