<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The rules (`automation.md` §1) as a list page (page-system §7.1, §9.1): one row each - name,
  // what starts it, its health, its last run and its switch in place -, the strip *All · Mine* with
  // their counts; a row opens the editor that draws it, as *Write a rule* in the head does.
  //
  // **The form is gone.** What a rule does is written on a canvas at `/administration/rules/{id}`
  // and `/administration/rules/new`; this screen is the way in, and the two switches somebody
  // pressing them from the list expects.
  //
  // **The check runs when this screen opens** (ADR-0060): every rule's references resolved
  // against what exists now, the findings written on the rules and shown here - the banner with
  // the count, the health word and the first finding on each row. That is what makes "after an
  // update, the rules that need attention are shown" true without anything enumerating tenants:
  // the first person to open this screen asks, and the answer is stored for everybody after.
  //
  // **The health here is the findings' and the switch's, not the runs'.** A page of runs per row
  // would be a request per rule on every open; the rule's own screen reads its runs and says the
  // finer word.
  //
  // **Every list is read rather than compiled in**, and **nothing is pre-empted**: the server
  // refuses and this renders it, as before.
  //
  // *Write a rule* is the head's one primary act; the editor keeps its address - it is the create
  // page of a rule - and the primary navigates there the way the frame navigates.
  //
  // **A person's own rules (UC-AUT-01 check 10)** are the same list under *Your settings*
  // (`isOwn`): only theirs, written without the automation permission. The list and the check
  // filter quietly on the server, so a member reading here is refused nothing and nothing is
  // recorded; the workspace's runners and people are not read, since an own rule runs as its owner.

  import { untrack } from 'svelte';

  import {
    Badge, Banner, Button, DataTable, EmptyState, FilterBar, ListPage, Spinner, Switch, Tabs,
    type DataColumn, type DataRowMenu, type MenuItem, type PageAction,
  } from '@hubtask/design-system/components';
  import { actionsOfHead } from '../lib/palette/actions.ts';
  import { commands } from '../lib/palette/commands.svelte.ts';

  import { adminTrail } from '../lib/administration/trail.ts';
  import { findingWords, listHealth } from '../lib/automation/findings.ts';
  import { accounts } from '../lib/data/accounts.svelte.ts';
  import { actor } from '../lib/data/account.svelte.ts';
  import { isOwnRule } from '../lib/automation/model.ts';
  import { SETTINGS } from '../lib/navigation.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import { rules, type Rule } from '../lib/data/rules.svelte.ts';
  import { serviceAccounts } from '../lib/data/serviceaccounts.svelte.ts';
  import { includedHere, mayHere } from '../lib/data/capability.svelte.ts';
  import NotInPlan from '../lib/workspace/NotInPlan.svelte';
  import WorkspaceRefusal from '../lib/workspace/WorkspaceRefusal.svelte';
  import { page } from '../lib/frame/page.svelte.ts';
  import { announcer } from '../lib/announce.svelte.ts';
  import { formatRelative } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { eventWords } from '../lib/automation/words.ts';
  import { matchesSearch } from '../lib/page/list.ts';
  import { renderProblem } from '../lib/problem.ts';
  import SectionFindings from '../lib/settings/SectionFindings.svelte';
  import ViewSectionsAsFile from '../lib/settings/ViewSectionsAsFile.svelte';
  import { pagePart } from '../lib/settings/asFile.ts';
  import { viewAsFileAction } from '../lib/settings/viewasfile.svelte.ts';
  import FailureNote from '../lib/frame/FailureNote.svelte';

  interface Props {
    /** The reader's own rules, under Your settings, rather than the workspace's. */
    isOwn?: boolean;
    /** The administration's tab, from the address (`?tab=mine`), so a reload or a link keeps it. */
    tab?: string;
    onnavigate?: (path: string) => void;
  }

  const { isOwn = false, tab, onnavigate }: Props = $props();

  const TENANT = { scopeType: 'TENANT' } as const;

  const title = $derived(isOwn ? t('app.rules.mine_title') : t('app.rules.title'));
  $effect(() => page.entitle(title));

  const me = $derived(actor.account?.id);
  /** The administration's list shows every rule the reader may see, or only theirs (*Mine*). */
  const filter = $derived<'all' | 'mine'>(tab === 'mine' ? 'mine' : 'all');
  const mine = $derived(rules.all.filter((rule) => isOwnRule(rule, me)));
  const listed = $derived(isOwn || filter === 'mine' ? mine : rules.all);
  const base = $derived(isOwn ? '/profile/rules' : '/administration/rules');

  let search = $state('');
  const shown = $derived(listed.filter((rule) => matchesSearch(search, [rule.name])));

  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  /** The rule whose switch is being pressed, so only its own switch shows the work. */
  let working = $state<string | undefined>(undefined);

  $effect(() => untrack(() => rules.open()));
  $effect(() => {
    if (isOwn) return;
    return untrack(() => serviceAccounts.open());
  });
  $effect(() => {
    if (isOwn) return;
    return untrack(() => people.openScope(TENANT));
  });

  /** Once per open, after the list is there: the answer refreshes the list through the store. */
  let isChecking = $state(false);
  let checked = $state(false);
  $effect(() => {
    if (checked || rules.state.status !== 'ready') return;
    checked = true;
    isChecking = true;
    untrack(() =>
      rules
        .check()
        .catch(() => {
          // A refused check is not a refused list: the rules stay readable, and what the check
          // last found is still on them.
        })
        .finally(() => (isChecking = false)),
    );
  });

  const words = { t, has: (code: string) => messages.has(code) };
  /** When the rule last ran and how it ended, or that it never did. */
  function lastRunWords(rule: { last_run?: { at: string; status: string } | null }): string {
    const last = rule.last_run;
    if (!last) return t('app.rules.never_ran');
    const status = `app.runs.status_${last.status.toLowerCase()}`;
    return t('app.rules.last_run_words', { when: formatRelative(last.at, messages.locale), outcome: messages.has(status) ? t(status) : last.status });
  }

  const broken = $derived(listed.filter((rule) => (rule.findings ?? []).some((finding) => finding.level === 'BROKEN')).length);
  const attention = $derived(listed.filter((rule) => (rule.findings ?? []).length > 0).length - broken);

  const HEALTH_TONE = { works: 'success', sometimes: 'warning', off: 'neutral', attention: 'warning', broken: 'danger' } as const;

  function whereOf(pointer: string): string {
    if (pointer.startsWith('/trigger')) return t('app.flow.finding_trigger');
    if (pointer === '/run_as') return t('app.flow.finding_run_as');
    const condition = /^\/conditions\/(\d+)/.exec(pointer);
    if (condition) return t('app.flow.finding_condition', { n: Number(condition[1]) + 1 });
    const step = /^\/actions\/(.+?)(?:\/kind|\/params\/(?!then|else).*)?$/.exec(pointer);
    return t('app.flow.finding_step', { path: (step?.[1] ?? pointer).replace(/\/params\/(then|else)/g, '/$1') });
  }

  const reading = $derived(rules.state);
  const refusal = $derived(reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined);

  async function attempt(id: string, work: () => Promise<unknown>, said?: string): Promise<void> {
    failure = undefined;
    working = id;
    try {
      await work();
      if (said) announcer.say(said);
    } catch (cause) {
      failure = renderProblem(cause as never, messages);
    } finally {
      working = undefined;
    }
  }

  /** What starts a rule, in words rather than tokens. */
  function triggerWord(rule: Rule): string {
    const code = `app.rules.trigger_${rule.trigger.kind.toLowerCase()}`;
    const said = messages.has(code) ? t(code) : rule.trigger.kind;
    return rule.trigger.event_type ? `${said} · ${eventWords(words, rule.trigger.event_type).said}` : said;
  }

  const runnerName = (id: string) =>
    serviceAccounts.all.find((account) => account.id === id)?.display_name ?? accounts.nameOf(id) ?? t('app.people.unnamed');

  /** Whether the reader may change the workspace's automation (`AUTOMATION`), as the server asks. */
  const mayAutomate = $derived(mayHere('AUTOMATION').status === 'permitted');
  /**
   * Whether the workspace's plan includes the rules. Left out, they rest: no new rule and no
   * switching one on, while switching one off stays (multi-tenancy.md §4.2).
   */
  const rulesIncluded = $derived(includedHere('AUTOMATION_RULES') === true);
  const resting = $derived(includedHere('AUTOMATION_RULES') === false);

  /** The head's one action, handed to the head and to the palette alike (UC-WRK-27 check 2). */
  const headPrimary = $derived<PageAction | undefined>(
    (isOwn ? me !== undefined : mayAutomate) && rulesIncluded
      ? { label: t('app.rules.new_rule'), icon: 'plus', opener: 'new-rule', onclick: () => onnavigate?.(`${base}/new`) }
      : undefined,
  );
  /** Whether the reader may switch this rule: their own, or any with the automation permission. */
  const maySwitch = (rule: Rule): boolean => mayAutomate || isOwnRule(rule, me);
  /** Switching on is refused while the plan leaves the rules out; switching off never is. */
  const switchRefusal = (rule: Rule): string | undefined => (!rule.enabled && !rulesIncluded ? t('app.rules.switch_resting') : undefined);

  function toggle(rule: Rule): void {
    if (rule.enabled) void attempt(rule.id, () => rules.disable(rule.id), t('app.rules.disabled_announced'));
    else if (rulesIncluded) void attempt(rule.id, () => rules.enable(rule.id), t('app.rules.enabled_announced'));
  }

  const columns = $derived<DataColumn[]>([
    { id: 'name', label: t('app.rules.name'), priority: 1 },
    { id: 'trigger', label: t('app.rules.starts_on'), priority: 2 },
    { id: 'health', label: t('app.rules.health'), priority: 2 },
    { id: 'last', label: t('app.rules.last_run'), priority: 3 },
  ]);

  // The switch is the row's one verb from `medium` up; on a phone, where a row shows no verb, the
  // same act is in its menu.
  function rowMenu(rule: Rule): DataRowMenu {
    const items: MenuItem[] = [
      { id: 'open', label: t('app.page.open') },
      ...(maySwitch(rule) && (rule.enabled || rulesIncluded)
        ? [{ id: 'switch', label: rule.enabled ? t('app.rules.disable') : t('app.rules.enable'), hasSeparatorBefore: true }]
        : []),
    ];
    return { label: t('app.page.more_for', { name: rule.name }), items };
  }

  function onRowMenu(rule: Rule, id: string): void {
    if (id === 'open') onnavigate?.(`${base}/${rule.id}`);
    else if (id === 'switch') toggle(rule);
  }

  const settingsRow = SETTINGS.flatMap((group) => group.rows).find((row) => row.id === 'rules');
  let isFileOpen = $state(false);
  const fileAction = $derived(isOwn ? undefined : viewAsFileAction(() => (isFileOpen = true)));
  const head = $derived({
    title,
    breadcrumb: isOwn
      ? {
          trail: [
            { id: 'settings', label: t('app.nav.profile'), href: '/profile' },
            { id: 'screen', label: t(settingsRow?.titleCode ?? 'app.rules.mine_title') },
          ],
          label: t('app.you.trail'),
          expandLabel: t('app.you.expand_trail'),
        }
      : adminTrail({ id: 'screen', label: t('app.rules.title') }),
    primary: headPrimary,
    secondary: fileAction ? ([fileAction] as const) : ([] as const),
  });
  $effect(() => commands.offer('page', actionsOfHead('page', { primary: headPrimary })));
</script>

<ListPage {head}>
  {#snippet strip()}
    {#if !isOwn}
      <Tabs
        label={t('app.rules.filter')}
        selected={filter}
        onselect={(next) => onnavigate?.(next === 'mine' ? '/administration/rules?tab=mine' : '/administration/rules')}
        tabs={[
          { id: 'all', label: t('app.rules.filter_all'), count: reading.status === 'ready' ? rules.all.length : undefined },
          { id: 'mine', label: t('app.rules.filter_mine'), count: reading.status === 'ready' ? mine.length : undefined },
        ]}
      />
    {/if}
  {/snippet}
  {#snippet intro()}<p class="quiet">{isOwn ? t('app.rules.mine_intro') : t('app.rules.intro')}</p>{/snippet}
  {#snippet filters()}
    <FilterBar
      searchLabel={t('app.rules.search')}
      clearSearchLabel={t('app.page.clear_search')}
      bind:search
      countLabel={reading.status === 'ready' ? t('app.page.found', { shown: String(shown.length), count: String(listed.length) }) : undefined}
    />
  {/snippet}

  {#if !isOwn}<WorkspaceRefusal permission="AUTOMATION" />{/if}
  {#if resting}
    <NotInPlan title={t('app.plan.rules_title')} body={t('app.plan.rules_body', { count: rules.isComplete ? listed.length : 0 })} />
  {/if}
  {#if failure}<FailureNote problem={failure} />{/if}
  {#if broken + attention > 0}
    <Banner tone={broken > 0 ? 'danger' : 'warning'} title={broken + attention === 1 ? t('app.flow.check_banner_one') : t('app.flow.check_banner_title', { count: broken + attention })}>
      {t('app.flow.check_banner_body', { broken, attention })}
    </Banner>
  {/if}
  {#if isChecking}
    <p class="quiet small"><Spinner label={t('app.flow.check_running')} /> <span>{t('app.flow.check_running')}</span></p>
  {/if}
  {#if !isOwn}<SectionFindings sections={['rules']} />{/if}

  {#if reading.status === 'loading' || reading.status === 'idle'}
    <p class="quiet"><Spinner label={t('app.rules.reading')} /> <span>{t('app.rules.reading')}</span></p>
  {:else if refusal}
    <FailureNote problem={refusal} />
  {:else}
    <DataTable
      label={title}
      {columns}
      rows={shown}
      key={(rule: Rule) => rule.id}
      href={(rule: Rule) => `${base}/${rule.id}`}
      rowLabel={(rule: Rule) => rule.name}
      menu={rowMenu}
      onmenu={onRowMenu}
      actionsLabel={t('app.page.row_actions')}
      sheetDismissLabel={t('app.page.sheet_close')}
      sheetGrabLabel={t('app.page.sheet_grab')}
      shownLabel={listed.length === 0 ? undefined : t('app.page.showing', { count: String(shown.length) })}
    >
      {#snippet cell(rule: Rule, column: DataColumn)}
        {@const health = listHealth(rule)}
        {#if column.id === 'name'}
          {@const first = rule.findings?.[0]}
          <span class="name" data-rule={rule.id}>{rule.name}</span>
          {#if first}<span class="finding" class:broken={health === 'broken'}>{t('app.flow.finding_at', { where: whereOf(first.path), finding: findingWords(words, first) })}</span>{/if}
        {:else if column.id === 'trigger'}
          <span class="quiet">{triggerWord(rule)}</span>
          {#if !isOwn}<span class="quiet small block">{t('app.rules.runs_as')}: {runnerName(rule.run_as)}</span>{/if}
        {:else if column.id === 'health'}
          {#if resting}
            <Badge tone="neutral">{t('app.plan.resting')}</Badge>
          {:else}
            <Badge tone={HEALTH_TONE[health]}>{t(`app.flow.health_${health}`)}</Badge>
          {/if}
          {#if rule.failure_count > 0}<span class="quiet small block">{t('app.rules.failing', { count: String(rule.failure_count) })}</span>{/if}
        {:else}
          <span class="quiet">{lastRunWords(rule)}</span>
        {/if}
      {/snippet}
      {#snippet verb(rule: Rule)}
        {#if maySwitch(rule)}
          <Switch
            label={t('app.rules.switch_label', { name: rule.name })}
            isLabelHidden
            checked={rule.enabled}
            isBusy={working === rule.id}
            busyLabel={t('app.rules.working')}
            disabledReason={switchRefusal(rule)}
            onchange={() => toggle(rule)}
          />
        {:else}
          <span class="quiet small">{rule.enabled && !resting ? t('app.rules.on') : t('app.rules.off')}</span>
        {/if}
      {/snippet}
      {#snippet empty()}
        {#if listed.length === 0}
          <EmptyState kind="unused" title={isOwn || filter === 'mine' ? t('app.rules.mine_none') : t('app.rules.none')} />
        {:else}
          <EmptyState kind="filtered" title={t('app.rules.filtered_title')}>
            {#snippet action()}<Button onclick={() => (search = '')}>{t('app.people.filtered_clear')}</Button>{/snippet}
          </EmptyState>
        {/if}
      {/snippet}
    </DataTable>
  {/if}
</ListPage>

{#if !isOwn}
  <ViewSectionsAsFile {...pagePart('/administration/rules')} bind:isOpen={isFileOpen} isTriggerHidden />
{/if}

<style>
  .quiet { margin: 0; color: var(--text-secondary); }
  .small { font-size: var(--fs-075); }
  .block { display: block; }
  .name { display: block; color: var(--text-primary); font-weight: var(--fw-medium); }
  .finding { display: block; color: var(--text-secondary); font-size: var(--fs-075); font-weight: normal; }
  .finding.broken { color: var(--text-danger); }
</style>
