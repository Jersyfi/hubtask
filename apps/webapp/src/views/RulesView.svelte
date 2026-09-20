<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The rules (G-05, `automation.md` §1), as a list; each opens the editor that draws it (F8-04).
  //
  // **The form is gone.** What a rule does is written on a canvas at `/administration/rules/{id}`
  // and `/administration/rules/new`; this screen is the way in, and the two switches somebody
  // pressing them from the list expects.
  //
  // **The check runs when this screen opens** (ADR-0060, F8-07): every rule's references resolved
  // against what exists now, the findings written on the rules and shown here - the banner with
  // the count, the health word and the first finding on each card. That is what makes "after an
  // update, the rules that need attention are shown" true without anything enumerating tenants:
  // the first person to open this screen asks, and the answer is stored for everybody after.
  //
  // **The health here is the findings' and the switch's, not the runs'.** A page of runs per card
  // would be a request per rule on every open; the rule's own screen reads its runs and says the
  // finer word (F8-06).
  //
  // **Every list is read rather than compiled in**, and **nothing is pre-empted**: the server
  // refuses and this renders it, as before.

  import { untrack } from 'svelte';

  import { AutomationRuleCard, Banner, Button, Spinner, Stack } from '@hubtask/design-system/components';

  import { findingWords, listHealth } from '../lib/automation/findings.ts';
  import { accounts } from '../lib/data/accounts.svelte.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import { rules, type Rule } from '../lib/data/rules.svelte.ts';
  import { serviceAccounts } from '../lib/data/serviceaccounts.svelte.ts';
  import { announcer } from '../lib/announce.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { eventWords } from '../lib/automation/words.ts';
  import { renderProblem } from '../lib/problem.ts';

  const TENANT = { scopeType: 'TENANT' } as const;

  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let isWorking = $state(false);

  $effect(() => untrack(() => rules.open()));
  $effect(() => untrack(() => serviceAccounts.open()));
  $effect(() => untrack(() => people.openScope(TENANT)));

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
  const broken = $derived(rules.all.filter((rule) => (rule.findings ?? []).some((finding) => finding.level === 'BROKEN')).length);
  const attention = $derived(rules.all.filter((rule) => (rule.findings ?? []).length > 0).length - broken);

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

  async function attempt(work: () => Promise<unknown>, said?: string): Promise<void> {
    failure = undefined;
    isWorking = true;
    try {
      await work();
      if (said) announcer.say(said);
    } catch (cause) {
      failure = renderProblem(cause as never, messages);
    } finally {
      isWorking = false;
    }
  }

  /** What a card says about a rule, in words rather than tokens. */
  function triggerWord(rule: Rule): string {
    const code = `app.rules.trigger_${rule.trigger.kind.toLowerCase()}`;
    const said = messages.has(code) ? t(code) : rule.trigger.kind;
    return rule.trigger.event_type ? `${said} · ${eventWords(words, rule.trigger.event_type).said}` : said;
  }

  const runnerName = (id: string) =>
    serviceAccounts.all.find((account) => account.id === id)?.display_name ?? accounts.nameOf(id) ?? t('app.people.unnamed');
</script>

<div class="screen">
  <Stack gap="300">
    <h1>{t('app.rules.title')}</h1>
    <p class="quiet">{t('app.rules.intro')}</p>

    {#if failure}
      <Banner tone="danger" title={failure.message}>
        {#if failure.reference}{t('app.error_reference', { request_id: failure.reference })}{/if}
      </Banner>
    {/if}

    {#if broken + attention > 0}
      <Banner tone={broken > 0 ? 'danger' : 'warning'} title={broken + attention === 1 ? t('app.flow.check_banner_one') : t('app.flow.check_banner_title', { count: broken + attention })}>
        {t('app.flow.check_banner_body', { broken, attention })}
      </Banner>
    {/if}
    {#if isChecking}
      <p class="waiting small"><Spinner label={t('app.flow.check_running')} /> <span>{t('app.flow.check_running')}</span></p>
    {/if}

    {#if reading.status === 'loading' || reading.status === 'idle'}
      <p class="waiting"><Spinner label={t('app.rules.reading')} /> <span>{t('app.rules.reading')}</span></p>
    {:else if refusal}
      <Banner tone="danger" title={refusal.message}>
        {#if refusal.reference}{t('app.error_reference', { request_id: refusal.reference })}{/if}
      </Banner>
    {:else}
      <Stack gap="150">
        {#each rules.all as rule (rule.id)}
          {@const health = listHealth(rule)}
          {@const first = rule.findings?.[0]}
          <Stack gap="100">
            <AutomationRuleCard
              name={rule.name}
              href={`/administration/rules/${rule.id}`}
              trigger={{ label: t('app.rules.starts_on'), value: triggerWord(rule) }}
              actions={{ label: t('app.rules.actions'), value: String(rule.actions.length) }}
              runAs={{ label: t('app.rules.runs_as'), value: runnerName(rule.run_as) }}
              isEnabled={rule.enabled}
              stateLabel={rule.enabled ? t('app.rules.on') : t('app.rules.off')}
              failureLabel={rule.failure_count > 0 ? t('app.rules.failing', { count: String(rule.failure_count) }) : undefined}
              healthLabel={t(`app.flow.health_${health}`)}
              healthTone={HEALTH_TONE[health]}
              findingLabel={first ? t('app.flow.finding_at', { where: whereOf(first.path), finding: findingWords(words, first) }) : undefined}
              isBroken={health === 'broken'}
            />
            <div class="row">
              {#if rule.enabled}
                <Button size="sm" tone="secondary" isBusy={isWorking} busyLabel={t('app.rules.working')}
                  onclick={() => void attempt(() => rules.disable(rule.id), t('app.rules.disabled_announced'))}>
                  {t('app.rules.disable')}
                </Button>
              {:else}
                <Button size="sm" tone="primary" isBusy={isWorking} busyLabel={t('app.rules.working')}
                  onclick={() => void attempt(() => rules.enable(rule.id), t('app.rules.enabled_announced'))}>
                  {t('app.rules.enable')}
                </Button>
              {/if}
            </div>
          </Stack>
        {:else}
          <p class="quiet">{t('app.rules.none')}</p>
        {/each}
      </Stack>
    {/if}

    <div>
      <!-- An address, not a handler: the editor is a screen of its own that can be opened in a
           second tab; the frame's interception turns the click into a navigation. -->
      <a class="new" href="/administration/rules/new">{t('app.rules.new_rule')}</a>
    </div>
  </Stack>
</div>

<style>
  h1 { margin: 0; font-family: var(--font-display); font-size: var(--fs-400); font-weight: var(--fw-semibold); line-height: var(--lh-tight); }

  .quiet { margin: 0; color: var(--text-secondary); }

  .waiting { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }

  .small { font-size: var(--fs-075); }

  .row { display: flex; flex-wrap: wrap; align-items: end; gap: var(--sp-100); }

  .new {
    display: inline-flex;
    align-items: center;
    min-height: var(--density-control-md-min);
    padding: var(--density-control-md-block) var(--sp-200);
    border-radius: var(--r-sm);
    background: var(--accent-primary);
    color: var(--text-inverse);
    font-weight: var(--fw-medium);
    text-decoration: none;
  }

  .new:hover { background: var(--accent-primary-hover); }

  .new:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }
</style>
