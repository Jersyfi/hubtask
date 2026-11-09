<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One of the reader's own access tokens, a record page (page-system §7.2, §9.5): what it may do in
  // words, by area; when it runs out and was last used; revoking in the danger zone, asked first in a
  // Dialog S. The keys stay under *Details* (PS-15). Read from the reader's own listing - the same
  // read the list page holds - so nobody else's token can be addressed here.

  import { untrack } from 'svelte';

  import { DataTable, Disclosure, EmptyState, RecordPage, Spinner, type DataColumn, type RecordList } from '@hubtask/design-system/components';

  import FailureNote from '../lib/frame/FailureNote.svelte';
  import FinalActDialog from '../lib/frame/FinalActDialog.svelte';
  import { page } from '../lib/frame/page.svelte.ts';
  import { tokens } from '../lib/data/tokens.svelte.ts';
  import { announcer } from '../lib/announce.svelte.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { profileTitle } from '../lib/profile/profilehead.ts';
  import { areaOf } from '../lib/tokens/scopeareas.ts';
  import { scopeWord, standingOf } from '../lib/tokens/tokenwords.ts';

  const { id, onnavigate }: { id: string; onnavigate?: (path: string) => void } = $props();

  const LIST = '/profile/tokens';

  $effect(() => untrack(() => tokens.open()));
  const reading = $derived(tokens.stateOf());
  const token = $derived(tokens.of().find((each) => each.id === id));
  const name = $derived(token?.name ?? t('app.tokens.title'));
  $effect(() => page.entitle(name));

  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let isRevoking = $state(false);

  const when = (at: string | null | undefined) => (at ? formatDateTime(at, messages.locale) : undefined);
  const standing = $derived(token ? standingOf(token) : undefined);
  const TONE = { live: 'success', expired: 'warning', revoked: 'danger' } as const;

  const lists = $derived<RecordList[]>(token ? [{ id: 'scopes', title: t('app.tokens.may_do_title'), count: String(token.scopes.length) }] : []);
  const columns = $derived<DataColumn[]>([
    { id: 'scope', label: t('app.tokens.scope_column'), priority: 1 },
    { id: 'area', label: t('app.tokens.area_column'), priority: 2 },
  ]);
</script>

{#snippet notHere()}
  {#if reading.status === 'failed'}
    <FailureNote problem={renderProblem(reading.error, messages)} />
  {:else if reading.status === 'ready'}
    <EmptyState kind="settled" title={t('app.tokens.not_here')} />
  {:else}
    <p class="quiet waiting"><Spinner label={t('app.tokens.reading')} /> <span>{t('app.tokens.reading')}</span></p>
  {/if}
{/snippet}

<RecordPage
  record={{
    kind: t('app.tokens.kind'),
    title: name,
    back: { label: profileTitle('tokens'), href: LIST, onnavigate: () => onnavigate?.(LIST) },
    status: standing ? { label: t(`app.tokens.${standing}`), tone: TONE[standing] } : undefined,
    facts: token
      ? [
          t('app.tokens.expires_on', { when: when(token.expires_at) ?? '' }),
          when(token.last_used_at) ? t('app.tokens.used_on', { when: when(token.last_used_at) ?? '' }) : t('app.tokens.not_used_yet'),
        ].join(' · ')
      : undefined,
  }}
  {lists}
  facts={token
    ? {
        title: t('app.tokens.facts_title'),
        rows: [
          { label: t('app.tokens.created'), value: when(token.created_at) ?? '' },
          { label: t('app.tokens.expires'), value: when(token.expires_at) ?? '' },
          { label: t('app.tokens.last_used'), value: when(token.last_used_at) ?? t('app.tokens.never_used') },
          ...(token.revoked_at ? [{ label: t('app.tokens.revoked'), value: when(token.revoked_at) ?? '' }] : []),
        ],
      }
    : undefined}
  danger={token && !token.revoked_at
    ? {
        title: t('app.page.danger_zone'),
        acts: [{ id: 'revoke', title: t('app.tokens.revoke_title_zone'), cost: t('app.tokens.revoke_cost'), actionLabel: t('app.tokens.revoke_menu') }],
        onact: () => (isRevoking = true),
      }
    : undefined}
  refused={token ? undefined : notHere}
>
  {#snippet list(item: RecordList)}
    {#if item.id === 'scopes' && token}
      {#if failure}<FailureNote problem={failure} />{/if}
      <DataTable label={item.title} {columns} rows={token.scopes} key={(scope: string) => scope}>
        {#snippet cell(scope: string, column: DataColumn)}
          {#if column.id === 'scope'}
            {scopeWord(scope)}
          {:else}
            <span class="quiet">{t(`app.tokens.area_${areaOf(scope)}`)}</span>
          {/if}
        {/snippet}
      </DataTable>
      <!-- The keys, for whoever checks a script against the reference (PS-15). -->
      <div class="keys">
        <Disclosure label={t('app.tokens.details_title')} tone="quiet"><code>{token.scopes.join(' ')}</code></Disclosure>
      </div>
    {/if}
  {/snippet}
</RecordPage>

{#if token}
  <FinalActDialog
    isOpen={isRevoking}
    title={t('app.tokens.revoke_confirm_title', { name: token.name })}
    body={t('app.tokens.revoke_confirm_body', { name: token.name })}
    confirmLabel={t('app.tokens.revoke')}
    busyLabel={t('app.tokens.working')}
    onConfirm={async () => {
      try {
        await tokens.revoke(token.id);
        announcer.say(t('app.tokens.revoked_announced'));
      } catch (cause) {
        failure = renderProblem(cause as never, messages);
      }
    }}
    onClose={() => (isRevoking = false)}
  />
{/if}

<style>
  .quiet { margin: 0; color: var(--text-secondary); }
  .waiting { display: flex; align-items: center; gap: var(--sp-100); }

  .keys { margin-block-start: var(--sp-150); }
  .keys code { display: block; font-family: var(--font-mono); font-size: var(--fs-075); overflow-wrap: anywhere; }
</style>
