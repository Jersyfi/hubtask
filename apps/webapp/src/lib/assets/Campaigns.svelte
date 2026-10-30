<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A hub's certification campaigns (UC-AST-10 check 5): each with its score - asked, confirmed,
  // disputed, pending, overdue and the share answered -, one opened with every CI it asks about,
  // who owns it and who answered what, and a campaign started or closed.
  //
  // **Who may answer is the server's rule, drawn** (P-05): the person asked answers their own CIs
  // with READ on the hub, a writer any of them; a closed campaign offers no answer. Starting and
  // closing need WRITE_ITEMS and are not drawn without it.

  import { untrack } from 'svelte';
  import { Badge, Banner, Button, CheckboxGroup, Dialog, Input, Spinner, Stack } from '@hubtask/design-system/components';
  import type { CiCampaign, CiCampaignDetail, CiCampaignList, CiClass } from '@hubtask/sync-engine';

  import { actor } from '../data/account.svelte.ts';
  import { accounts } from '../data/accounts.svelte.ts';
  import {
    attestationsInOrder, campaignDraft, campaignMissing, campaignPath, campaignStart, campaignsPath, classWord, cmdb, instantiable, mayAttest,
    percent, verdictTone, type CampaignDraft, type Words,
  } from '../data/cmdb.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    hubId: string;
    classes: readonly CiClass[];
    words: Words;
    mayWrite: boolean;
  }

  const { hubId, classes, words, mayWrite }: Props = $props();

  const me = $derived(actor.account?.id);
  const when = (at: string) => formatDateTime(at, messages.locale);

  let campaigns = $state<readonly CiCampaign[] | undefined>(undefined);
  let refusal = $state<string | undefined>(undefined);
  $effect(() => {
    const at = hubId;
    return untrack(() =>
      engine.subscribe<CiCampaignList>({ path: campaignsPath(at) }, (state) => {
        if (state.status === 'ready') campaigns = state.data.data;
        if (state.status === 'failed') refusal = renderProblem(state.error, messages).message;
      }),
    );
  });

  let openId = $state<string | undefined>(undefined);
  let detail = $state<CiCampaignDetail | undefined>(undefined);
  $effect(() => {
    const id = openId;
    detail = undefined;
    if (!id) return;
    return untrack(() =>
      engine.subscribe<CiCampaignDetail>({ path: campaignPath(id) }, (state) => {
        if (state.status === 'ready') detail = state.data;
        if (state.status === 'failed') toast.show({ text: renderProblem(state.error, messages).message, tone: 'danger' });
      }),
    );
  });
  const questions = $derived(detail ? attestationsInOrder(detail, me) : []);

  async function act<T>(run: () => Promise<T>, done: string): Promise<T | undefined> {
    try {
      const out = await run();
      toast.show({ text: done, tone: 'success' });
      return out;
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
      return undefined;
    }
  }

  // Starting one.
  let isStartOpen = $state(false);
  let draft = $state<CampaignDraft>(campaignDraft());
  let isStarting = $state(false);
  const today = () => new Date().toISOString().slice(0, 10);
  const missing = $derived(campaignMissing(draft, today()));
  const classChecks = $derived(instantiable(classes).map((each) => ({ value: each.key, label: classWord(each.key, classes, words) })));

  function openStart() {
    draft = campaignDraft();
    isStartOpen = true;
  }

  async function start() {
    isStarting = true;
    const started = await act(() => cmdb.startCampaign(hubId, campaignStart(draft)), t('app.campaigns.started'));
    isStarting = false;
    if (!started) return;
    isStartOpen = false;
    openId = started.id;
  }
</script>

{#if refusal}
  <Banner tone="danger">{refusal}</Banner>
{:else if !campaigns}
  <Spinner label={t('app.assets.loading')} />
{:else}
  <Stack gap="200">
    <p class="meta">{t('app.campaigns.hint')}</p>
    {#if mayWrite}<div><Button icon="plus" data-start-campaign onclick={openStart}>{t('app.campaigns.start')}</Button></div>{/if}
    {#if campaigns.length === 0}
      <p class="meta">{t('app.campaigns.none')}</p>
    {:else}
      <ul class="rows">
        {#each campaigns as campaign (campaign.id)}
          <li class="row" data-campaign={campaign.id}>
            <button type="button" class="pick" aria-expanded={openId === campaign.id} onclick={() => (openId = openId === campaign.id ? undefined : campaign.id)}>
              <strong>{campaign.name}</strong>
              <span class="meta">
                {t('app.campaigns.due', { day: campaign.due_on })} ·
                {campaign.classes.length > 0 ? campaign.classes.map((each) => classWord(each, classes, words)).join(', ') : t('app.cmdb_admin.every_class')}
              </span>
            </button>
            <span class="score">
              <Badge size="sm" tone={campaign.closed_at ? 'neutral' : 'info'}>{campaign.closed_at ? t('app.campaigns.closed') : t('app.campaigns.open')}</Badge>
              <span class="meta">
                {t('app.campaigns.score', {
                  answered: campaign.score.confirmed + campaign.score.disputed, total: campaign.score.total, share: percent(campaign.score.rate),
                })}
              </span>
            </span>
          </li>
        {/each}
      </ul>
    {/if}

    {#if openId}
      <section class="part" aria-labelledby="campaign-open" data-campaign-detail>
        {#if !detail}
          <Spinner label={t('app.assets.loading')} />
        {:else}
          <div class="part-head">
            <h2 id="campaign-open" class="heading">{detail.name}</h2>
            {#if mayWrite && !detail.closed_at}
              <Button size="sm" tone="secondary" data-close-campaign onclick={() => detail && void act(() => cmdb.closeCampaign(detail!), t('app.campaigns.closed_done'))}>
                {t('app.campaigns.close')}
              </Button>
            {/if}
          </div>
          <dl class="figures">
            <div><dt>{t('app.campaigns.total')}</dt><dd>{detail.score.total}</dd></div>
            <div><dt>{t('app.campaigns.confirmed')}</dt><dd>{detail.score.confirmed}</dd></div>
            <div><dt>{t('app.campaigns.disputed')}</dt><dd>{detail.score.disputed}</dd></div>
            <div><dt>{t('app.campaigns.pending')}</dt><dd>{detail.score.pending}</dd></div>
            <div><dt>{t('app.campaigns.overdue')}</dt><dd>{detail.score.overdue}</dd></div>
            <div><dt>{t('app.campaigns.rate')}</dt><dd>{t('app.assets.percent', { value: percent(detail.score.rate) })}</dd></div>
          </dl>
          <ul class="rows">
            {#each questions as question (question.ci_id)}
              <li class="row" data-attestation={question.ci_id}>
                <span class="what">
                  <a href={`/cis/${question.ci_id}`}>{question.name}</a>
                  <span class="meta">
                    {classWord(question.class, classes, words)} ·
                    {question.owner_id ? t('app.campaigns.asked', { name: accounts.nameOf(question.owner_id) ?? '—' }) : t('app.campaigns.no_owner')}
                    {#if question.attested_at} · {t('app.campaigns.answered_by', { name: (question.attested_by && accounts.nameOf(question.attested_by)) || '—', at: when(question.attested_at) })}{/if}
                  </span>
                </span>
                <span class="answer">
                  <Badge size="sm" tone={verdictTone(question.verdict)}>{t(`app.campaigns.verdict.${question.verdict}`)}</Badge>
                  {#if detail && mayAttest(detail, question, me, mayWrite)}
                    <Button size="sm" tone="secondary" data-confirm-ci={question.ci_id} onclick={() => detail && void act(() => cmdb.attest(detail!, question.ci_id, 'CONFIRMED'), t('app.campaigns.confirmed_done'))}>
                      {t('app.campaigns.confirm')}
                    </Button>
                    <Button size="sm" tone="subtle" data-dispute-ci={question.ci_id} onclick={() => detail && void act(() => cmdb.attest(detail!, question.ci_id, 'DISPUTED'), t('app.campaigns.disputed_done'))}>
                      {t('app.campaigns.dispute')}
                    </Button>
                  {/if}
                </span>
              </li>
            {/each}
          </ul>
        {/if}
      </section>
    {/if}
  </Stack>
{/if}

{#if isStartOpen}
  <Dialog bind:isOpen={isStartOpen} title={t('app.campaigns.start')} dismissLabel={t('app.assets.cancel')}>
    <Stack gap="150">
      <Input label={t('app.campaigns.name')} bind:value={draft.name} maxlength={200} isRequired />
      <CheckboxGroup label={t('app.campaigns.classes')} hint={t('app.campaigns.classes_hint')} options={classChecks} selected={draft.classes} onChange={(next) => (draft.classes = [...next])} />
      <Input label={t('app.campaigns.due_by')} hint={t('app.campaigns.due_by_hint')} type="date" bind:value={draft.dueBy} />
      <Input label={t('app.campaigns.due_on')} type="date" bind:value={draft.dueOn} isRequired />
      <Input label={t('app.campaigns.months')} hint={t('app.campaigns.months_hint')} inputmode="numeric" bind:value={draft.months} />
    </Stack>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isStartOpen = false)}>{t('app.assets.cancel')}</Button>
      <Button tone="primary" isBusy={isStarting} disabledReason={missing[0] ? t(missing[0]) : undefined} onclick={() => void start()}>{t('app.campaigns.start')}</Button>
    {/snippet}
  </Dialog>
{/if}

<style>
  .rows { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-050); }
  .row {
    display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: var(--sp-100);
    padding: var(--sp-100) var(--sp-150); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md);
    background: var(--bg-surface);
  }
  .pick { display: grid; gap: var(--sp-025); padding: 0; border: none; background: none; font: inherit; color: var(--text-primary); text-align: start; cursor: pointer; }
  .pick:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }
  .what { display: grid; gap: var(--sp-025); min-inline-size: 0; }
  .what a { color: var(--text-primary); font-weight: var(--fw-medium); }
  .score, .answer { display: inline-flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }
  .part { display: grid; gap: var(--sp-100); }
  .part-head { display: flex; flex-wrap: wrap; align-items: end; justify-content: space-between; gap: var(--sp-100); }
  .heading { margin: 0; font-size: var(--fs-200); font-weight: var(--fw-semibold); }
  .figures { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, var(--sp-1000)), 1fr)); gap: var(--sp-100); margin: 0; }
  .figures div {
    display: grid; gap: var(--sp-025); padding: var(--sp-100) var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md); background: var(--bg-surface);
  }
  .figures dt { color: var(--text-secondary); font-size: var(--fs-075); }
  .figures dd { margin: 0; font-size: var(--fs-300); font-weight: var(--fw-semibold); }
  .meta { color: var(--text-subtle); font-size: var(--fs-075); margin: 0; }
</style>
