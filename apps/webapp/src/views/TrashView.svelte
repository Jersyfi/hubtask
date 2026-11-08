<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What was deleted, what it takes with it, and how long there is to change your mind.
  //
  // **The one trash** (user-experience.md §5.9): hubs, collections, entries, pages and whiteboards
  // in one list with one window and one hold, narrowed by kind, each restored through its own kind.
  //
  // **One row per deletion.** A hub with two hundred entries under it went in as one act and comes
  // back as one act (I-C2), so every row here is the *root* of a deletion and restoring it brings
  // the batch. There is deliberately nothing on this screen that restores one entry out of a batch:
  // the invariant the batch exists for is "restoring is atomic", and an offer that broke it would
  // be a control the model has no answer for.
  //
  // **Emptying is irreversible**, and it is treated as such. It says
  // how many deletions and what goes with them, it is confirmed rather than offered as a plain
  // button, it carries the destructive tone, and it is never the default action of anything — the
  // dialog opens on Cancel's side and the confirm is named for what it does rather than "OK".
  //
  // **The window is stated where a person decides**, because "it will be gone anyway in a week"
  // changes the decision. It counts down to the date the server gives each entry - the retention
  // pass's own, the trash period and the offline window together - to every reader of the trash
  // (UC-LIF-02 check 6). Only an older server without that date leaves the client to count from the
  // policy, and where it may not read the policy it says so rather than asserting a default.

  import { untrack } from 'svelte';

  import {
    Badge,
    Button,
    DataTable,
    Dialog,
    EmptyState,
    FilterBar,
    ListPage,
    Radio,
    Skeleton,
    type DataColumn,
    type DataRowMenu,
    type Filter,
  } from '@hubtask/design-system/components';
  import FailureState from '../lib/frame/FailureState.svelte';
  import type { TrashEntry } from '@hubtask/sync-engine';

  import { announcer } from '../lib/announce.svelte.ts';
  import { holdsHere } from '../lib/data/capability.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { items } from '../lib/data/items.svelte.ts';
  import {
    daysUntil,
    isContainer,
    isPurgeableAlone,
    isRestorable,
    remainingDays,
    TRASH_KINDS,
    type TrashKind,
  } from '../lib/data/lifecycle.ts';
  import { pages } from '../lib/data/pages.svelte.ts';
  import { whiteboards } from '../lib/data/whiteboards.svelte.ts';
  import { retention } from '../lib/data/retention.svelte.ts';
  import { trash } from '../lib/data/trash.svelte.ts';
  import { actor as signedIn } from '../lib/data/account.svelte.ts';
  import { accounts } from '../lib/data/accounts.svelte.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { matchesSearch } from '../lib/page/list.ts';

  // `untrack` for the reason every store here records: the load writes the store and writing it
  // reads it, so an effect that tracks that read re-runs on its own first answer.
  interface Props {
    /** The kind the address narrows the list to (`/trash?kind=PAGE`); none, or one it does not know, is every kind. */
    kind?: string;
    /** Writes the chosen kind into the address, so a hub's *Trash* link and a reload land on the same list. */
    onnavigate?: (path: string) => void;
  }
  const { kind: asked, onnavigate }: Props = $props();

  const knownKind = (value: string | undefined): TrashKind | undefined =>
    value !== undefined && (TRASH_KINDS as readonly string[]).includes(value) ? (value as TrashKind) : undefined;
  /** The kind chosen on this screen without an address to write it into; `null` until one is. */
  let chosenHere = $state<TrashKind | undefined | null>(null);
  /** The kind the list is narrowed to; none is every kind. */
  const kind = $derived(chosenHere !== null ? chosenHere : knownKind(asked));

  $effect(() => {
    const shown = kind;
    void untrack(() => trash.load(shown));
  });

  // The policies only where the manifest says the server will answer them: `STRUCTURE`, or the
  // auditor's `READ_CONFIGURATION`, on the workspace - the condition the read itself asks. Asked of
  // anybody else it is refused, and every refusal is a DENIED entry in the trail. Tracked, so a
  // manifest that arrives after the screen still starts the read.
  const mayReadPolicies = $derived(holdsHere('STRUCTURE') === true || holdsHere('READ_CONFIGURATION') === true);
  $effect(() => {
    if (mayReadPolicies) void untrack(() => retention.start());
  });

  // Emptying sweeps hubs and collections along with entries, so the server asks the owner's
  // DELETE_CONTAINER on the workspace for it (rule 2): somebody who may not delete a hub must not
  // delete one by emptying the trash it is in. Offered only to a reader who holds it (P-05).
  const mayEmpty = $derived(holdsHere('DELETE_CONTAINER') === true);

  const now = Date.now();

  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let busyId = $state<string | undefined>(undefined);
  /** Which row the reader is being asked about, and whether they are being asked about all of it. */
  let purging = $state<TrashEntry | undefined>(undefined);
  let isEmptying = $state(false);
  let isWorking = $state(false);
  /** What the last pass of an empty did, kept until the reader has read it. */
  let summary = $state<{ matched: number; removed: number; blocked: Record<string, number> } | undefined>(
    undefined,
  );

  const listFailure = $derived(
    trash.status === 'failed' && trash.error ? renderProblem(trash.error, messages) : undefined,
  );

  /**
   * The window, or nothing. `undefined` days is not "0 days" and must not read like it.
   *
   * The sentence names the number rather than inflecting around it, because the source catalogue
   * stays inside the **simple-argument** subset: `infrastructure/i18n/Catalogue_test.go` holds the
   * whole file to it, so that the day a message needs a plural somebody teaches the Go renderer
   * rather than watching it print braces at a reader.
   */
  function window(row: TrashEntry): string {
    // A legal hold or a restriction keeps it past any date: the pass does not take it while it stands, and
    // when it ends is nobody's to promise here (data-retention.md §4).
    if (row.held) return t(row.held_by === 'restriction' ? 'app.trash.window_restricted' : 'app.trash.window_held');
    const left = row.purge_after ? daysUntil(row.purge_after, now) : remainingDays(row.deleted_at, retention.trashDays, now);
    if (left === undefined) return t('app.trash.window_unknown');
    return left === 0 ? t('app.trash.window_last') : t('app.trash.window', { days: left });
  }

  /**
   * Who deleted it, as a sentence — or nothing, which is a real answer here.
   *
   * Three states, and they are different things rather than degrees of the same one. A row deleted
   * before the columns existed carries no actor at all and says nothing extra: inventing "somebody"
   * for it would be a claim the row cannot support. An actor with an account is named once the
   * account read has answered, and reads as its kind until then. An automation or the system has no
   * account by nature, and its kind is the whole truth about it.
   */
  function whoDeleted(row: TrashEntry): string | undefined {
    const who = row.deleted_by;
    if (!who) return undefined;

    if (who.id && who.id === signedIn.account?.id) return t('app.trash.deleted_by_you');

    const name = accounts.nameOf(who.id);
    if (name) return t('app.trash.deleted_by', { name });

    const kind = `app.activity.actor_${who.type}`;
    return t('app.trash.deleted_by', {
      name: t(messages.has(kind) ? kind : 'app.activity.actor_someone'),
    });
  }

  // The names this page needs, asked for once the page has arrived. The same shape `ItemView` uses
  // for the history, and the same cache — a trash and a history showing the same person ask once.
  $effect(() => {
    accounts.resolve(
      trash.rows
        .filter((row) => row.deleted_by?.type === 'USER' || row.deleted_by?.type === 'SERVICE_ACCOUNT')
        .map((row) => row.deleted_by?.id),
    );
  });

  async function restore(row: TrashEntry) {
    failure = undefined;
    busyId = row.id;
    try {
      // Which endpoint is the row's own answer: the trash mixes the kinds by design.
      if (isContainer(row.kind)) await containers.restore(row.id, crypto.randomUUID());
      else if (row.kind === 'PAGE') await pages.restore(row.hub_id ?? '', row.id);
      else if (row.kind === 'WHITEBOARD') await whiteboards.restore({ id: row.id, hub_id: row.hub_id ?? '' });
      else await items.restore(row.id, crypto.randomUUID());
      announcer.say(t('app.trash.restored', { title: row.title }));
      await trash.load(kind);
    } catch (error) {
      failure = renderProblem(error as never, messages);
    } finally {
      busyId = undefined;
    }
  }

  /**
   * Destroys one deletion for good.
   *
   * Only an **entry** can be purged on its own — the contract has `:purge` for an item and none
   * for a container, so a container in the trash goes when the trash is emptied or when the
   * retention job reaches it. The control says why rather than being absent.
   */
  async function purge(row: TrashEntry) {
    failure = undefined;
    isWorking = true;
    try {
      await items.purge(row.id, crypto.randomUUID());
      announcer.say(t('app.trash.purged', { title: row.title }));
      purging = undefined;
      await trash.load(kind);
    } catch (error) {
      // A legal hold refuses this, and `lifecycle.legal_hold` is its own sentence naming the scope
      // — which is `problem.ts` choosing the detail code over the category, not a branch here.
      failure = renderProblem(error as never, messages);
    } finally {
      isWorking = false;
    }
  }

  async function empty() {
    failure = undefined;
    isWorking = true;
    try {
      const pass = await trash.empty(crypto.randomUUID(), kind);
      summary = {
        matched: pass.matched,
        removed: pass.removed,
        blocked: (pass.blocked ?? {}) as Record<string, number>,
      };
      announcer.say(t('app.trash.summary', { removed: pass.removed, matched: pass.matched }));
      isEmptying = false;
    } catch (error) {
      failure = renderProblem(error as never, messages);
    } finally {
      isWorking = false;
    }
  }
  // The bar carries the page's title on a phone (ADR-0061 decision 1's table); the head then
  // reads its heading rather than drawing it, so the screen keeps one heading.
  $effect(() => page.entitle(t('app.trash.title')));

  /** The kind chip writes the address, so a hub's *Trash* link and a reload land on the same list. */
  function choose(value: string): void {
    const chosen = knownKind(value);
    if (onnavigate) onnavigate(chosen ? `/trash?kind=${chosen}` : '/trash');
    else chosenHere = chosen;
  }

  // The kind narrows on the server; the search narrows the rows loaded.
  let search = $state('');
  const shown = $derived(trash.rows.filter((row) => matchesSearch(search, [row.title, row.subtype])));
  const chips = $derived<Filter[]>([
    { id: 'kind', label: t('app.trash.kind_chip', { choice: t(kind ? `app.trash.kind_${kind.toLowerCase()}` : 'app.trash.kind_all') }), isActive: kind !== undefined },
  ]);
  function clearFilters(): void {
    search = '';
    if (kind !== undefined) choose('');
  }

  const columns = $derived<DataColumn[]>([
    { id: 'title', label: t('app.trash.column_title'), priority: 1 },
    { id: 'deleted', label: t('app.trash.column_deleted'), priority: 2 },
    { id: 'kept', label: t('app.trash.column_kept'), priority: 3 },
  ]);

  /** Delete for good is destructive, so it lives in the row's ⋯ only (PS-6). */
  function rowMenu(row: TrashEntry): DataRowMenu {
    return {
      label: t('app.page.more_for', { name: row.title }),
      items: [
        {
          id: 'purge',
          label: t('app.trash.purge'),
          isDestructive: true,
          disabledReason: isPurgeableAlone(row.kind) ? undefined : t(isContainer(row.kind) ? 'app.trash.purge_container' : 'app.trash.purge_document'),
        },
      ],
    };
  }
</script>

<ListPage
  head={{
    title: t('app.trash.title'),
    menu: mayEmpty && trash.rows.length > 0
      ? { label: t('app.trash.more'), items: [{ id: 'empty', label: t('app.trash.empty_all'), isDestructive: true }], onselect: () => (isEmptying = true) }
      : undefined,
  }}
>
  {#snippet intro()}<p class="detail">{t('app.trash.empty_detail')} {t('app.trash.batch')}</p>{/snippet}
  {#snippet filters()}
    <FilterBar
      searchLabel={t('app.trash.search')}
      clearSearchLabel={t('app.page.clear_search')}
      bind:search
      filters={chips}
      moreLabel={t('app.page.more_filters')}
      filtersLabel={t('app.page.filters', { count: String(kind ? 1 : 0) })}
      sheetTitle={t('app.page.filters', { count: '0' })}
      sheetDismissLabel={t('app.page.sheet_close')}
      sheetGrabLabel={t('app.page.sheet_grab')}
      showLabel={t('app.page.show_results', { count: String(shown.length) })}
      clearLabel={t('app.page.clear')}
      onclear={clearFilters}
      countLabel={trash.status === 'ready' ? t('app.page.found', { shown: String(shown.length), count: String(trash.rows.length) }) : undefined}
    >
      {#snippet picker(_filter: Filter)}
        <Radio
          label={t('app.trash.kind_filter')}
          bind:value={() => kind ?? '', (next) => choose(next)}
          options={[
            { value: '', label: t('app.trash.kind_all') },
            ...TRASH_KINDS.map((each) => ({ value: each, label: t(`app.trash.kind_${each.toLowerCase()}`) })),
          ]}
        />
      {/snippet}
    </FilterBar>
  {/snippet}

  {#if failure}<p class="failure" role="alert">{failure.message}</p>{/if}

  {#if summary}
    <div class="summary" role="status">
      <p>{t('app.trash.summary', { removed: summary.removed, matched: summary.matched })}</p>
      <!-- What was **kept**, and why. A legal hold does not fail the call; it is counted in the
           answer, and an answer that reported only a number would leave the reader wondering
           which of their deletions is still there. -->
      {#each Object.entries(summary.blocked) as [reason, count] (reason)}
        {#if count > 0}
          <p>{t('app.trash.kept', { count, reason: t(`app.trash.blocked.${reason}`) })}</p>
        {/if}
      {/each}
      {#if summary.matched > summary.removed && Object.keys(summary.blocked).length === 0}
        <p>{t('app.trash.again')}</p>
      {/if}
      <div>
        <Button size="sm" tone="secondary" onclick={() => (summary = undefined)}>
          {t('app.dismiss')}
        </Button>
      </div>
    </div>
  {/if}

  {#if listFailure}
    <FailureState problem={listFailure} onRetry={() => trash.load(kind)} />
  {:else if trash.status === 'loading' && trash.rows.length === 0}
    <div aria-busy="true"><Skeleton lines={4} /></div>
  {:else}
    <DataTable
      label={t('app.trash.title')}
      {columns}
      rows={shown}
      key={(row: TrashEntry) => row.id}
      rowLabel={(row: TrashEntry) => row.title}
      menu={rowMenu}
      onmenu={(row: TrashEntry) => (purging = row)}
      actionsLabel={t('app.page.row_actions')}
      sheetDismissLabel={t('app.page.sheet_close')}
      sheetGrabLabel={t('app.page.sheet_grab')}
      shownLabel={trash.rows.length === 0 ? undefined : t('app.page.showing', { count: String(shown.length) })}
    >
      {#snippet cell(row: TrashEntry, column: DataColumn)}
        {#if column.id === 'title'}
          <span class="title"><Badge size="sm">{row.subtype}</Badge> {row.title}</span>
        {:else if column.id === 'deleted'}
          <span class="detail">
            {t('app.trash.deleted_at', { when: formatDateTime(row.deleted_at, messages.locale) })}
            {#if whoDeleted(row)}· {whoDeleted(row)}{/if}
          </span>
        {:else}
          <span class="detail">{window(row)}</span>
        {/if}
      {/snippet}
      {#snippet verb(row: TrashEntry)}
        <Button
          size="sm"
          tone="subtle"
          isBusy={busyId === row.id}
          busyLabel={t('app.trash.restore')}
          onclick={() => restore(row)}
          disabledReason={isRestorable(row.kind) ? undefined : t('app.trash.restore_unknown')}
        >
          {t('app.trash.restore')}
        </Button>
      {/snippet}
      {#snippet empty()}
        {#if trash.rows.length === 0}
          <!-- `settled` rather than `unused`: an empty trash is the good outcome, and §4.3 offers
               nothing to do about it. The same empty as the archive's, in one form (G4). -->
          <EmptyState kind="settled" title={t(kind ? 'app.trash.empty_kind' : 'app.trash.empty')} description={t('app.trash.empty_detail')} icon="trash" />
        {:else}
          <EmptyState kind="filtered" title={t('app.trash.filtered_title')}>
            {#snippet action()}<Button onclick={clearFilters}>{t('app.people.filtered_clear')}</Button>{/snippet}
          </EmptyState>
        {/if}
      {/snippet}
    </DataTable>

    {#if trash.isPartial}<p class="detail">{t('app.trash.partial')}</p>{/if}
  {/if}
</ListPage>

{#if purging}
  <Dialog
    size="s"
    isOpen={true}
    title={t('app.trash.purge_title', { title: purging.title })}
    dismissLabel={t('app.workspace.cancel')}
    onClose={() => (purging = undefined)}
  >
    <p class="detail">{t('app.trash.purge_body')}</p>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (purging = undefined)}>
        {t('app.workspace.cancel')}
      </Button>
      <Button
        tone="danger"
        isBusy={isWorking}
        busyLabel={t('app.trash.purge_confirm')}
        onclick={() => purging && purge(purging)}
      >
        {t('app.trash.purge_confirm')}
      </Button>
    {/snippet}
  </Dialog>
{/if}

{#if isEmptying && mayEmpty}
  <Dialog
    size="s"
    bind:isOpen={isEmptying}
    title={t('app.trash.empty_title')}
    dismissLabel={t('app.workspace.cancel')}
  >
    <!-- It says how many and what goes with them, because nothing here deletes without the person
         having read what it deletes. -->
    <p class="detail">{t('app.trash.empty_body', { count: trash.rows.length })}</p>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isEmptying = false)}>
        {t('app.workspace.cancel')}
      </Button>
      <Button
        tone="danger"
        isBusy={isWorking}
        busyLabel={t('app.trash.empty_confirm')}
        onclick={empty}
      >
        {t('app.trash.empty_confirm')}
      </Button>
    {/snippet}
  </Dialog>
{/if}

<style>

  .title { overflow-wrap: anywhere; }

  .detail { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); max-width: 64ch; }

  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); max-width: 64ch; }

  .summary {
    display: flex;
    flex-direction: column;
    gap: var(--sp-100);
    max-width: 64ch;
    padding: var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
    background: var(--bg-surface);
    font-size: var(--fs-075);
  }

  .summary p { margin: 0; }
</style>
