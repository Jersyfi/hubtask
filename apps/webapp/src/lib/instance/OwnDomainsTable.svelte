<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Own domains* on the installation's *Workspaces* screen (UC-INS-15 check 10): every domain the
  // workspaces' owners set up, with its workspace and state, and *Block domain* with a reason the
  // owner reads word for word and the instance journal keeps. A blocked row has no menu: lifting a
  // block is not offered. Blocking a main domain says that the subdomain becomes main in the same
  // step (check 9).

  import { untrack } from 'svelte';

  import { Badge, Banner, Button, Dialog, IconButton, Input, Menu, Stack, Table } from '@hubtask/design-system/components';
  import type { AdminOwnDomain } from '@hubtask/sync-engine';
  import { TransportError } from '@hubtask/sync-engine';

  import { toneOf } from '../address/owndomain.ts';
  import { instance } from '../data/instance.svelte.ts';
  import { formatLongDay } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem, type RenderedProblem } from '../problem.ts';

  $effect(() => untrack(() => instance.openOwnDomains()));

  const rows = $derived(instance.ownDomains);

  let blocking = $state<AdminOwnDomain | undefined>(undefined);
  let reason = $state('');
  let working = $state(false);
  let failure = $state<RenderedProblem | undefined>(undefined);

  const columns = $derived([
    { id: 'domain', label: t('app.instance.own_domain_domain') },
    { id: 'workspace', label: t('app.instance.own_domain_workspace') },
    { id: 'state', label: t('app.instance.own_domain_state') },
    { id: 'since', label: t('app.instance.own_domain_since') },
    { id: 'actions', label: t('app.instance.own_domain_what'), isLabelHidden: true },
  ]);

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

{#if rows.length > 0}
  <Stack gap="150">
    <h2 class="heading">{t('app.instance.own_domains')}</h2>
    <p class="prose">{t('app.instance.own_domains_intro')}</p>
    <Table label={t('app.instance.own_domains')} isLabelHidden {columns}>
      {#each rows as row (`${row.state}:${row.domain}`)}
        <tr data-own-domain={row.domain}>
          <td class="mono">{row.domain}</td>
          <td>{row.tenant_name || t('app.instance.own_domain_no_workspace')}</td>
          <td>
            <span class="row">
              <Badge tone={toneOf(row.state)}>{t(`app.address.own.state_${row.state.toLowerCase()}`)}</Badge>
              {#if row.is_main}<Badge tone="neutral">{t('app.instance.own_domain_main')}</Badge>{/if}
            </span>
            {#if row.state === 'BLOCKED' && row.blocked_reason}
              <span class="cellnote">{row.blocked_reason}</span>
            {/if}
          </td>
          <td class="quiet">{formatLongDay(row.since, messages.locale)}</td>
          <td>
            {#if row.state !== 'BLOCKED'}
              <Menu
                label={t('app.instance.own_domain_actions', { domain: row.domain })}
                items={[{ id: 'block', label: t('app.instance.own_domain_block'), isDestructive: true }]}
                placement={{ side: 'block-end', align: 'end' }}
                onselect={() => open(row)}
              >
                {#snippet trigger(props)}
                  <IconButton icon="ellipsis" label={t('app.instance.own_domain_actions', { domain: row.domain })} size="sm" {...props} />
                {/snippet}
              </Menu>
            {/if}
          </td>
        </tr>
      {/each}
    </Table>
  </Stack>
{/if}

<Dialog
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
      <Banner tone="danger" title={failure.message} />
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
  .heading {
    margin: 0;
    font-family: var(--font-display);
    font-size: var(--fs-300);
    font-weight: var(--fw-semibold);
  }
  .prose { margin: 0; max-inline-size: 60ch; }
  .mono { font-family: var(--font-mono); overflow-wrap: anywhere; }
  .quiet { color: var(--text-subtle); font-size: var(--fs-075); }
  .row { display: flex; flex-wrap: wrap; gap: var(--sp-050); }
  .cellnote { display: block; margin-block-start: var(--sp-050); font-size: var(--fs-075); color: var(--text-subtle); }
</style>
