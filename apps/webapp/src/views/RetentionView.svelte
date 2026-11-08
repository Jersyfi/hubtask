<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The rules that delete things, the preview that has to come first, and the holds above them.
  //
  // **One section per address** (page-system §7.3, §9.3, PS-9): *Rules* and *Holds*, each at
  // `/administration/retention/<section>`, each a table. Every act is its own write - a dialog's -,
  // so neither has a save bar (PS-10). A rule opens as a Dialog M with what it would do and its
  // switch; a new rule, a hold and *What is in force where* are Dialogs M; withdrawing a rule and
  // lifting a hold Dialogs S.
  //
  // **A rule is written switched off, previewed, and only then armed** - the dialog that writes it
  // hands the rule to its own dialog with the preview, where arming is a second, deliberate press.
  // **The mode a broad rule lands in is said where it lands** (`data-retention.md` §7).
  //
  // **A hold is beside the rules rather than on a page of its own**: it overrides the workspace's
  // own periods and belongs where somebody reads what those periods are.

  import { untrack } from 'svelte';

  import { Badge, Banner, Button, DataTable, EmptyState, SettingsLayout, Spinner, Stack, type DataColumn, type SectionLink } from '@hubtask/design-system/components';
  import type { WorkItem } from '@hubtask/sync-engine';

  import { adminTrail } from '../lib/administration/trail.ts';
  import { accounts } from '../lib/data/accounts.svelte.ts';
  import { mayHere } from '../lib/data/capability.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { engine } from '../lib/data/engine.ts';
  import { itemPath } from '../lib/data/item.svelte.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import { policies, type Hold, type Policy } from '../lib/data/policies.svelte.ts';
  import FailureNote from '../lib/frame/FailureNote.svelte';
  import { page } from '../lib/frame/page.svelte.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import TableSection from '../lib/page/TableSection.svelte';
  import { renderProblem } from '../lib/problem.ts';
  import EffectiveDialog from '../lib/retention/EffectiveDialog.svelte';
  import LiftHoldDialog from '../lib/retention/LiftHoldDialog.svelte';
  import PlaceHoldDialog from '../lib/retention/PlaceHoldDialog.svelte';
  import RuleDialog from '../lib/retention/RuleDialog.svelte';
  import WithdrawRuleDialog from '../lib/retention/WithdrawRuleDialog.svelte';
  import WriteRuleDialog from '../lib/retention/WriteRuleDialog.svelte';
  import { actionWord, kindWord } from '../lib/retention/words.svelte.ts';
  import WorkspaceRefusal from '../lib/workspace/WorkspaceRefusal.svelte';

  interface Props {
    /** The section the address names (`/administration/retention/:section`); none is the list. */
    section?: string;
    /** Replaces the address: a move between sections. */
    onreplace?: (path: string) => void;
  }

  const { section, onreplace }: Props = $props();

  const BASE = '/administration/retention';

  let isWriting = $state(false);
  let isPlacing = $state(false);
  let isEffectiveOpen = $state(false);
  let showReleased = $state(false);
  let openedRule = $state<string | undefined>(undefined);
  let withdrawing = $state<Policy | undefined>(undefined);
  let lifting = $state<Hold | undefined>(undefined);
  /** Set when a rule came back in a mode nobody asked for. Cleared by the next rule written. */
  let landedIn = $state<{ policyId: string; asked: string } | undefined>(undefined);

  $effect(() => untrack(() => policies.open()));
  $effect(() => {
    const released = showReleased;
    return untrack(() => policies.openHolds(released));
  });
  $effect(() => untrack(() => containers.start()));

  // Placing and lifting a hold, and writing, arming and withdrawing a rule, are the owner's
  // (DELETE_CONTAINER, UC-LIF-06 check 1); everybody who reaches this screen reads them. Asked of
  // the manifest's `workspace_permissions`. A prediction; the server decides (P-05).
  $effect(() => untrack(() => people.open({})));
  const mayHold = $derived(mayHere('DELETE_CONTAINER').status === 'permitted');

  // What a hold can name, picked rather than typed (P-12): the people of the workspace, its hubs
  // with their collections, and an entry found by its title.
  const holdPeople = $derived(people.candidates({}));
  $effect(() => {
    const hubs = containers.hubs;
    untrack(() => hubs.forEach((hub) => containers.openLevel(hub.id)));
  });
  const holdContainers = $derived(
    containers.hubs.flatMap((hub) => [
      { value: hub.id, label: hub.name },
      ...containers.collectionsOf(hub.id).map((collection) => ({ value: collection.id, label: `${hub.name} / ${collection.name}` })),
    ]),
  );
  // Guarded: an answer that is not a list (an older server, a stub) names nobody.
  const placed = $derived<readonly Hold[]>(Array.isArray(policies.placed) ? policies.placed : []);
  $effect(() => {
    const named = placed.filter((hold) => hold.scope.kind === 'ACCOUNT').map((hold) => hold.scope.id);
    untrack(() => accounts.resolve(named));
  });

  /**
   * The titles of the entries a hold names, each read by its own address. A hold names few, and an
   * entry the reader cannot open stays unnamed rather than shown as its identifier.
   */
  let entryTitles = $state<Record<string, string>>({});
  $effect(() => {
    const ids = [...new Set(placed.filter((hold) => hold.scope.kind === 'ITEM' && hold.scope.id).map((hold) => hold.scope.id as string))];
    return untrack(() => {
      const stops = ids.map((id) =>
        engine.subscribe<WorkItem>({ path: itemPath(id) }, (next) => {
          if (next.status === 'ready') entryTitles = { ...entryTitles, [id]: next.data.title };
        }),
      );
      return () => stops.forEach((stop) => stop());
    });
  });

  /** What a hold covers, in words: the workspace, a person's name, a hub's, an entry's title. */
  const holdCovers = (scope: { kind: string; id?: string | null }) => {
    switch (scope.kind) {
      case 'TENANT':
        return t('app.retention.scope_workspace');
      case 'ACCOUNT':
        return t('app.retention.hold_covers_person', { name: accounts.nameOf(scope.id) ?? scope.id ?? '' });
      case 'CONTAINER':
        return holdContainers.find((option) => option.value === scope.id)?.label ?? scope.id ?? '';
      default: {
        const title = scope.id ? entryTitles[scope.id] : undefined;
        return title ? t('app.retention.hold_covers_entry', { title }) : t('app.retention.hold_covers_entry_unnamed');
      }
    }
  };

  const reading = $derived(policies.stateOf());
  const listed = $derived(policies.of());
  const refusal = $derived(reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined);
  const failing = $derived(listed.filter((policy) => policy.last_failure));
  const opened = $derived(openedRule ? listed.find((policy) => policy.id === openedRule) : undefined);

  const when = (at: string | null | undefined) => (at ? formatDateTime(at, messages.locale) : undefined);
  const containerName = (id: string | null | undefined) => (id ? containers.hubs.find((container) => container.id === id)?.name : undefined) ?? id ?? '';
  const scopeWord = (policy: Policy) =>
    policy.scope?.kind === 'TENANT' || !policy.scope?.id ? t('app.retention.scope_workspace') : containerName(policy.scope.id);

  /** The rule as written opens with its preview, which runs at once: arming waits for it. */
  function written(policy: Policy, asked: string): void {
    // §7's switch, as the caller sees it: what came back is not what was asked for.
    landedIn = policy.action === asked ? undefined : { policyId: policy.id, asked };
    openedRule = policy.id;
    void policies.preview(policy.id).catch(() => undefined);
  }

  const SECTIONS = [
    { id: 'rules', code: 'app.retention.rules_title' },
    { id: 'holds', code: 'app.retention.holds_title' },
  ] as const;
  const isKnown = (id: string | undefined): boolean => id !== undefined && SECTIONS.some((each) => each.id === id);
  const shown = $derived(isKnown(section) ? (section as string) : 'rules');
  const view = $derived<'list' | 'detail'>(isKnown(section) ? 'detail' : 'list');

  const ruleColumns: readonly DataColumn[] = $derived([
    { id: 'kind', label: t('app.retention.kind'), priority: 1 },
    { id: 'action', label: t('app.retention.action'), priority: 2 },
    { id: 'days', label: t('app.retention.days'), priority: 3 },
    { id: 'scope', label: t('app.retention.column_where'), priority: 3 },
    { id: 'state', label: t('app.retention.column_state'), isStatus: true },
  ]);
  const holdColumns: readonly DataColumn[] = $derived([
    { id: 'covers', label: t('app.retention.hold_scope'), priority: 1 },
    { id: 'reason', label: t('app.retention.hold_reason_field'), priority: 2 },
    { id: 'placed', label: t('app.retention.column_placed'), priority: 3 },
    { id: 'state', label: t('app.retention.column_state'), isStatus: true },
  ]);

  function ruleMenu(policy: Policy) {
    return {
      label: t('app.page.more_for', { name: kindWord(policy.data_kind) }),
      items: [
        { id: 'open', label: t('app.retention.preview') },
        ...(mayHold ? [{ id: 'withdraw', label: t('app.retention.withdraw'), isDestructive: true, hasSeparatorBefore: true }] : []),
      ],
    };
  }

  function onRuleMenu(policy: Policy, id: string): void {
    if (id === 'withdraw') withdrawing = policy;
    else {
      openedRule = policy.id;
      void policies.preview(policy.id).catch(() => undefined);
    }
  }

  function statusOf(id: string): Pick<SectionLink, 'status' | 'statusTone'> {
    if (id === 'rules' && reading.status === 'ready') {
      if (failing.length > 0) return { status: t('app.retention.failing_badge'), statusTone: 'danger' };
      const on = listed.filter((policy) => policy.enabled !== false).length;
      return on > 0 ? { status: t('app.retention.status_on', { count: String(on) }), statusTone: 'set' } : { status: t('app.retention.off') };
    }
    if (id === 'holds' && policies.holds.status === 'ready') {
      const inForce = placed.filter((hold) => !hold.released_at).length;
      return inForce > 0 ? { status: t('app.retention.status_held', { count: String(inForce) }), statusTone: 'warning' } : { status: t('app.retention.status_none') };
    }
    return {};
  }

  const title = $derived(t('app.retention.title'));
  const links = $derived<SectionLink[]>(SECTIONS.map((each) => ({ id: each.id, label: t(each.code), href: `${BASE}/${each.id}`, ...statusOf(each.id) })));

  $effect(() => page.entitle(title));
</script>

<SettingsLayout
  head={{
    title,
    subtitle: t('app.retention.intro'),
    breadcrumb: adminTrail({ id: 'retention', label: title }),
    menu: { label: t('app.page.more_for', { name: title }), items: [{ id: 'effective', label: t('app.retention.effective_menu') }], onselect: () => (isEffectiveOpen = true) },
  }}
  nav={{ label: t('app.settings_page.sections', { name: title }), sections: links, current: shown, onselect: (next) => onreplace?.(`${BASE}/${next}`) }}
  {view}
  back={{ label: title, onback: () => onreplace?.(BASE) }}
>
  <div class="content">
    <WorkspaceRefusal permission="DELETE_CONTAINER" />
    {#if shown === 'rules'}
      {#if landedIn}
        <!-- Said where it happened rather than discovered at the first pass that removed nothing. -->
        <Banner tone="warning" title={t('app.retention.landed_title')}>
          {t('app.retention.landed_note', { asked: actionWord(landedIn.asked) })}
        </Banner>
      {/if}
      {#if failing.length > 0}
        <!-- UC-LIF-04 check 10: a rule whose condition failed acts on nothing until a pass evaluates
             it again, and the other rules go on. The code's own sentence says what failed. -->
        <Banner tone="danger" title={t('app.retention.failing_title')}>
          <Stack gap="050">
            {#each failing as policy (policy.id)}
              <span data-failing={policy.id}>
                {kindWord(policy.data_kind)}:
                {t('app.retention.failing', { at: when(policy.last_failure?.at) ?? policy.last_failure?.at ?? '' })}
                {#if policy.last_failure && messages.has(policy.last_failure.code)}{t(policy.last_failure.code)}{/if}
              </span>
            {/each}
          </Stack>
        </Banner>
      {/if}
      <TableSection title={t('app.retention.rules_title')} note={reading.status === 'ready' ? String(listed.length) : undefined} description={t('app.retention.rules_intro')}>
        {#snippet acts()}
          {#if mayHold}<Button icon="plus" data-list-add onclick={() => (isWriting = true)}>{t('app.retention.write_title')}</Button>{/if}
        {/snippet}
        {#if reading.status === 'loading' || reading.status === 'idle'}
          <p class="waiting"><Spinner label={t('app.retention.reading')} /> <span>{t('app.retention.reading')}</span></p>
        {:else if refusal}
          <FailureNote problem={refusal} />
        {:else}
          <DataTable
            label={t('app.retention.rules_title')}
            columns={ruleColumns}
            rows={listed}
            key={(policy: Policy) => policy.id}
            onopen={(policy: Policy) => (openedRule = policy.id)}
            rowLabel={(policy: Policy) => kindWord(policy.data_kind)}
            menu={ruleMenu}
            onmenu={onRuleMenu}
            actionsLabel={t('app.page.row_actions')}
            sheetDismissLabel={t('app.page.sheet_close')}
            sheetGrabLabel={t('app.page.sheet_grab')}
            shownLabel={listed.length === 0 ? undefined : t('app.page.showing', { count: String(listed.length) })}
          >
            {#snippet cell(policy: Policy, column: DataColumn)}
              {#if column.id === 'kind'}
                <span class="name">{kindWord(policy.data_kind)}</span>
              {:else if column.id === 'action'}
                <Badge size="sm" tone={policy.action === 'NOTIFY_ONLY' ? 'info' : 'warning'}>{actionWord(policy.action)}</Badge>
              {:else if column.id === 'days'}
                <span class="quiet">{t('app.retention.days_value', { days: String(policy.retain_days) })}</span>
              {:else if column.id === 'scope'}
                <span class="quiet">{scopeWord(policy)}</span>
              {:else if policy.last_failure}
                <Badge size="sm" tone="danger">{t('app.retention.failing_badge')}</Badge>
              {:else}
                <Badge size="sm" tone={policy.enabled === false ? 'neutral' : 'success'}>{policy.enabled === false ? t('app.retention.off') : t('app.retention.on')}</Badge>
              {/if}
            {/snippet}
            {#snippet empty()}<EmptyState kind="unused" title={t('app.retention.no_rules')} />{/snippet}
          </DataTable>
        {/if}
      </TableSection>
    {:else}
      <TableSection title={t('app.retention.holds_title')} note={String(placed.filter((hold) => !hold.released_at).length)} description={t('app.retention.holds_intro')}>
        {#snippet acts()}
          <Button tone="subtle" size="sm" onclick={() => (showReleased = !showReleased)}>
            {showReleased ? t('app.retention.hide_released') : t('app.retention.show_released')}
          </Button>
          {#if mayHold}<Button icon="plus" data-list-add onclick={() => (isPlacing = true)}>{t('app.retention.place_title')}</Button>{/if}
        {/snippet}
        <DataTable
          label={t('app.retention.holds_title')}
          columns={holdColumns}
          rows={placed}
          key={(hold: Hold) => hold.id}
          rowLabel={(hold: Hold) => holdCovers(hold.scope)}
          menu={(hold: Hold) => (mayHold && !hold.released_at
            ? { label: t('app.page.more_for', { name: holdCovers(hold.scope) }), items: [{ id: 'lift', label: t('app.retention.release'), isDestructive: true }] }
            : undefined)}
          onmenu={(hold: Hold) => (lifting = hold)}
          actionsLabel={t('app.page.row_actions')}
          sheetDismissLabel={t('app.page.sheet_close')}
          sheetGrabLabel={t('app.page.sheet_grab')}
          shownLabel={placed.length === 0 ? undefined : t('app.page.showing', { count: String(placed.length) })}
        >
          {#snippet cell(hold: Hold, column: DataColumn)}
            {#if column.id === 'covers'}
              <span class="name">{holdCovers(hold.scope)}</span>
            {:else if column.id === 'reason'}
              <span class="quiet">{hold.reason}</span>
              {#if hold.released_at}
                <span class="quiet small block">{t('app.retention.hold_lifted', { at: when(hold.released_at) ?? '', reason: hold.released_reason ?? '' })}</span>
              {/if}
            {:else if column.id === 'placed'}
              <span class="quiet">{when(hold.placed_at)}</span>
            {:else}
              <Badge size="sm" tone={hold.released_at ? 'neutral' : 'danger'}>
                {hold.released_at ? t('app.retention.hold_released') : t('app.retention.hold_in_force')}
              </Badge>
            {/if}
          {/snippet}
          {#snippet verb(hold: Hold)}
            {#if mayHold && !hold.released_at}
              <Button size="sm" tone="subtle" onclick={() => (lifting = hold)}>{t('app.retention.release')}</Button>
            {/if}
          {/snippet}
          {#snippet empty()}<EmptyState kind="settled" title={t('app.retention.no_holds')} />{/snippet}
        </DataTable>
      </TableSection>
    {/if}
  </div>
</SettingsLayout>

<WriteRuleDialog bind:isOpen={isWriting} onWritten={written} />
<RuleDialog policy={opened} scope={opened ? scopeWord(opened) : ''} mayArm={mayHold} onClose={() => (openedRule = undefined)} />
<WithdrawRuleDialog policy={withdrawing} onClose={() => (withdrawing = undefined)} />
<PlaceHoldDialog bind:isOpen={isPlacing} people={holdPeople} places={holdContainers} />
<LiftHoldDialog hold={lifting} covers={lifting ? holdCovers(lifting.scope) : ''} onClose={() => (lifting = undefined)} />
<EffectiveDialog bind:isOpen={isEffectiveOpen} hubs={containers.hubs.map((hub) => ({ value: hub.id, label: hub.name }))} {scopeWord} />

<style>
  .content { display: flex; flex-direction: column; gap: var(--sp-250); min-width: 0; }
  .quiet { margin: 0; color: var(--text-secondary); }
  .small { font-size: var(--fs-075); }
  .block { display: block; }
  .name { color: var(--text-primary); font-weight: var(--fw-medium); }
  .waiting { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }
</style>
