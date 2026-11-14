<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Requests as their requester reads them (UC-SVC-05 check 3, P-12): the title, the state in the
  // desk's own words - the server's status name, never a key - and under it the number and what
  // comes next. Where requests of several departments stand together (PO-1), the department too.

  import { StateLozenge } from '@hubtask/design-system/components';
  import type { PortalRequest } from '@hubtask/sync-engine';

  import { actor } from '../data/account.svelte.ts';
  import { formatDateTime, formatDueShort } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { nextStep, requestState } from './portal.ts';

  interface Props {
    requests: readonly PortalRequest[];
    /** The department's name per collection, where several stand together. */
    departmentOf?: (collectionId: string) => string | undefined;
    onnavigate?: (path: string) => void;
    /** The state beside the title rather than beside both lines (scene `portalext`'s side column). */
    isStacked?: boolean;
  }
  const { requests, departmentOf, onnavigate, isStacked = false }: Props = $props();

  const words = $derived({ received: t('app.portal.state_received'), working: t('app.portal.state_working'), done: t('app.portal.state_done') });
  function stepOf(request: PortalRequest): string {
    const step = nextStep(request);
    const near = (at: string, allDay = false) => formatDueShort(at, messages.locale, actor.zone, { allDay });
    if (step.kind === 'done') return t('app.portal.next_done', { at: near(step.at, true) });
    if (step.kind === 'replied') return t('app.portal.next_replied', { at: near(step.at) });
    if (step.kind === 'promised') return t('app.portal.next_promised', { at: near(step.at) });
    if (step.kind === 'working') return t('app.portal.next_working', { at: formatDateTime(step.at, messages.locale) });
    return t('app.portal.next_received', { at: formatDateTime(step.at, messages.locale) });
  }
</script>

<ul class="rows" data-portal-requests data-stacked={isStacked ? '' : undefined}>
  {#each requests as request (request.id)}
    {@const state = requestState(request, words)}
    {@const department = departmentOf?.(request.collection_id)}
    <li>
      <a class="row" href={`/portal/requests/${request.id}`} data-portal-request={request.id}
        onclick={(event) => { if (onnavigate) { event.preventDefault(); onnavigate(`/portal/requests/${request.id}`); } }}>
        <span class="words">
          <span class="title">{request.title}</span>
          <span class="meta">
            {#if department}{department} · {/if}#{request.number} · {stepOf(request)}
            {#if request.is_own === false} · {t('app.portal.colleague')}{/if}
          </span>
        </span>
        <span class="state" data-portal-state={state.category}><StateLozenge label={state.label} vocabulary="category" state={state.category} /></span>
      </a>
    </li>
  {/each}
</ul>

<style>
  .rows { list-style: none; margin: 0; padding: 0; }
  .rows li + li { border-block-start: var(--bw-hairline) solid var(--border-subtle); }
  .row {
    display: flex; align-items: center; gap: var(--sp-150); padding: var(--sp-150) 0;
    color: inherit; text-decoration: none; min-block-size: var(--sp-600);
  }
  .row:hover { background: var(--bg-surface-hover); }
  .row:focus-visible { outline: var(--focus-ring); outline-offset: calc(-1 * var(--bw-thick)); }
  .words { display: flex; flex-direction: column; gap: var(--sp-025); flex: 1; min-inline-size: 0; }
  .title { font-weight: var(--fw-medium); overflow-wrap: anywhere; }
  .meta { color: var(--text-secondary); font-size: var(--fs-075); }
  .state { flex: none; }
  [data-stacked] .row { align-items: flex-start; }
</style>
